package openclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHealthWithCapacityJSON(t *testing.T) {
	report := CapacityReport{GPUFreeMB: 8192, JobsQueued: 2, JobsRunning: 1}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	cap, err := c.HealthWithCapacity(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cap)
	require.Equal(t, 8192, cap.GPUFreeMB)
	require.Equal(t, 2, cap.JobsQueued)
	require.Equal(t, 1, cap.JobsRunning)
}

func TestHealthWithCapacityPlainText(t *testing.T) {
	// Plain-text "ok" response — no capacity data, but not an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	cap, err := c.HealthWithCapacity(context.Background())
	require.NoError(t, err)
	require.Nil(t, cap) // no JSON capacity reported
}

func TestHealthWithCapacityNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.HealthWithCapacity(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "503")
}

func TestHealthWithCapacityRequestError(t *testing.T) {
	// Point at a non-existent server.
	c := &Client{
		httpClient:     &http.Client{Timeout: 100 * time.Millisecond},
		baseURL:        "http://127.0.0.1:1",
		healthPath:     "/health",
		directivesPath: "/directives",
	}
	_, err := c.HealthWithCapacity(context.Background())
	require.Error(t, err)
}

// mockCapacityUpdater captures UpdateCapacity calls for assertions.
type mockCapacityUpdater struct {
	calls []*CapacityReport
}

func (m *mockCapacityUpdater) UpdateCapacity(_ context.Context, _ string, cap *CapacityReport) error {
	m.calls = append(m.calls, cap)
	return nil
}

func TestWithCapacityUpdaterSetsField(t *testing.T) {
	lister := newMockLister(nil)
	notifier := &mockNotifier{}
	cfg := LivenessCheckerConfig{
		Interval:   time.Hour,
		Timeout:    time.Second,
		HealthPath: "/health",
	}
	lc := NewLivenessChecker(lister, notifier, cfg)
	updater := &mockCapacityUpdater{}
	lc2 := lc.WithCapacityUpdater(updater)
	require.Equal(t, lc, lc2, "WithCapacityUpdater returns the same checker")
	require.Equal(t, updater, lc.capacityUpdater)
}

func TestLivenessCheckerCapacityUpdated(t *testing.T) {
	// Health endpoint returns JSON capacity data.
	report := CapacityReport{GPUFreeMB: 4096, JobsQueued: 1, JobsRunning: 0}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}))
	defer srv.Close()

	host, port := parseTestServerURL(t, srv.URL)
	lister := newMockLister([]InstanceRecord{
		{ID: "id1", Name: "bot1", Host: host, Port: port, ChannelID: "C1", WasAlive: true},
	})
	notifier := &mockNotifier{}
	cfg := LivenessCheckerConfig{
		Interval:       time.Hour,
		Timeout:        2 * time.Second,
		HealthPath:     "/health",
		DirectivesPath: "/directives",
	}
	lc := NewLivenessChecker(lister, notifier, cfg)
	updater := &mockCapacityUpdater{}
	lc.WithCapacityUpdater(updater)
	lc.CheckOnce(context.Background())

	require.Len(t, updater.calls, 1)
	require.Equal(t, 4096, updater.calls[0].GPUFreeMB)
}

func TestLivenessCheckerNoCapacityOnDead(t *testing.T) {
	// Bot is dead — capacity updater should not be called.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	host, port := parseTestServerURL(t, srv.URL)
	lister := newMockLister([]InstanceRecord{
		{ID: "id1", Name: "bot1", Host: host, Port: port, ChannelID: "C1", WasAlive: false},
	})
	notifier := &mockNotifier{}
	cfg := LivenessCheckerConfig{
		Interval:       time.Hour,
		Timeout:        500 * time.Millisecond,
		HealthPath:     "/health",
		DirectivesPath: "/directives",
	}
	lc := NewLivenessChecker(lister, notifier, cfg)
	updater := &mockCapacityUpdater{}
	lc.WithCapacityUpdater(updater)
	lc.CheckOnce(context.Background())

	require.Empty(t, updater.calls)
}
