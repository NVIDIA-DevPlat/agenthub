package dolt

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

var capacityCols = []string{"bot_id", "gpu_free_mb", "jobs_queued", "jobs_running", "updated_at"}

func TestNewDB(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	doltDB := NewDB(db)
	require.NotNil(t, doltDB)
	_ = db.Close()
}

func TestUpdateCapacitySuccess(t *testing.T) {
	doltDB, mock := newMockDB(t)
	mock.ExpectExec("INSERT INTO bot_capacity").
		WillReturnResult(sqlmock.NewResult(1, 1))
	cap := Capacity{BotID: "bot1", GPUFreeMB: 8192, JobsQueued: 2, JobsRunning: 1}
	require.NoError(t, doltDB.UpdateCapacity(context.Background(), "bot1", cap))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCapacityError(t *testing.T) {
	doltDB, mock := newMockDB(t)
	mock.ExpectExec("INSERT INTO bot_capacity").
		WillReturnError(fmt.Errorf("upsert failed"))
	cap := Capacity{BotID: "bot1"}
	err := doltDB.UpdateCapacity(context.Background(), "bot1", cap)
	require.Error(t, err)
	require.Contains(t, err.Error(), "updating capacity")
}

func TestGetCapacityFound(t *testing.T) {
	doltDB, mock := newMockDB(t)
	now := time.Now()
	rows := sqlmock.NewRows(capacityCols).
		AddRow("bot1", 4096, 3, 2, now)
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	cap, err := doltDB.GetCapacity(context.Background(), "bot1")
	require.NoError(t, err)
	require.NotNil(t, cap)
	require.Equal(t, "bot1", cap.BotID)
	require.Equal(t, 4096, cap.GPUFreeMB)
	require.Equal(t, 3, cap.JobsQueued)
	require.Equal(t, 2, cap.JobsRunning)
}

func TestGetCapacityNotFound(t *testing.T) {
	doltDB, mock := newMockDB(t)
	rows := sqlmock.NewRows(capacityCols) // empty — ErrNoRows on Scan
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	cap, err := doltDB.GetCapacity(context.Background(), "unknown")
	require.NoError(t, err)
	require.Nil(t, cap)
}

func TestGetCapacityScanError(t *testing.T) {
	doltDB, mock := newMockDB(t)
	// Inject a non-integer gpu_free_mb to trigger a scan error.
	rows := sqlmock.NewRows(capacityCols).
		AddRow("bot1", "not-an-int", 0, 0, time.Now())
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	_, err := doltDB.GetCapacity(context.Background(), "bot1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "scanning capacity")
}

func TestGetAllCapacitiesSuccess(t *testing.T) {
	doltDB, mock := newMockDB(t)
	now := time.Now()
	rows := sqlmock.NewRows(capacityCols).
		AddRow("bot1", 8192, 1, 0, now).
		AddRow("bot2", 4096, 0, 2, now)
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	caps, err := doltDB.GetAllCapacities(context.Background())
	require.NoError(t, err)
	require.Len(t, caps, 2)
	require.NotNil(t, caps["bot1"])
	require.Equal(t, 8192, caps["bot1"].GPUFreeMB)
	require.NotNil(t, caps["bot2"])
	require.Equal(t, 2, caps["bot2"].JobsRunning)
}

func TestGetAllCapacitiesEmpty(t *testing.T) {
	doltDB, mock := newMockDB(t)
	rows := sqlmock.NewRows(capacityCols)
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	caps, err := doltDB.GetAllCapacities(context.Background())
	require.NoError(t, err)
	require.Empty(t, caps)
}

func TestGetAllCapacitiesQueryError(t *testing.T) {
	doltDB, mock := newMockDB(t)
	mock.ExpectQuery("SELECT bot_id").WillReturnError(fmt.Errorf("connection lost"))
	_, err := doltDB.GetAllCapacities(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "querying capacities")
}

func TestGetAllCapacitiesScanError(t *testing.T) {
	doltDB, mock := newMockDB(t)
	rows := sqlmock.NewRows(capacityCols).
		AddRow("bot1", "bad-int", 0, 0, time.Now())
	mock.ExpectQuery("SELECT bot_id").WillReturnRows(rows)
	_, err := doltDB.GetAllCapacities(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "scanning capacity row")
}
