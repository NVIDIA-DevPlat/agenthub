package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NVIDIA-DevPlat/agenthub/src/internal/dolt"
)

type registerRequest struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	ChannelID      string `json:"channel_id"`
	OwnerSlackUser string `json:"owner_slack_user"`
}

type registerResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.validateRegistrationToken(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Host == "" || req.Port == 0 {
		http.Error(w, `{"error":"name, host, and port are required"}`, http.StatusBadRequest)
		return
	}

	// Verify the bot is reachable before registering, unless the caller
	// explicitly opts out (e.g. when agent and server are on separate networks).
	skipProbe := r.URL.Query().Get("skip_probe") == "1"
	if s.healthProber != nil && !skipProbe {
		if err := s.healthProber.Probe(r.Context(), req.Host, req.Port); err != nil {
			http.Error(w, `{"error":"bot health check failed: `+err.Error()+`"}`, http.StatusServiceUnavailable)
			return
		}
	}

	id, err := newUUID()
	if err != nil {
		http.Error(w, `{"error":"failed to generate ID"}`, http.StatusInternalServerError)
		return
	}

	inst := dolt.Instance{
		ID:             id,
		Name:           req.Name,
		Host:           req.Host,
		Port:           req.Port,
		ChannelID:      req.ChannelID,
		OwnerSlackUser: req.OwnerSlackUser,
	}

	if s.registrar == nil {
		http.Error(w, `{"error":"registration not configured"}`, http.StatusServiceUnavailable)
		return
	}
	if err := s.registrar.CreateInstance(r.Context(), inst); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(registerResponse{ID: id, Name: req.Name})
}

// newUUID generates a random UUID v4.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
