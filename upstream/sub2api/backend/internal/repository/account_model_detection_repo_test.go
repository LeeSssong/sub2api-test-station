package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountModelDetectionRepositoryRejectsInvalidRunBeforeDatabase(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	repo := &accountModelDetectionRepository{db: mockDB}
	if _, _, err := repo.Enqueue(context.Background(), service.AccountModelDetectionRun{}); err == nil {
		t.Fatal("expected invalid run error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryReusesCompletedScheduledSlot(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	now := time.Date(2026, 8, 17, 2, 0, 0, 0, time.UTC)
	slot := "2026-08-17T10:00"
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "slot_key", "trigger_kind", "model_id", "claimed_model", "status", "queued_at", "started_at", "finished_at", "created_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", int64(7), slot, "scheduled", "gpt-5.6-sol", "gpt-5.6-sol", service.AccountModelDetectionStatusNormal, now, now, now, now))
	repo := &accountModelDetectionRepository{db: mockDB}
	run, reused, err := repo.Enqueue(context.Background(), service.AccountModelDetectionRun{ID: "22222222-2222-2222-2222-222222222222", AccountID: 7, SlotKey: &slot, TriggerKind: "scheduled", ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: service.AccountModelDetectionStatusQueued, QueuedAt: now, CreatedAt: now})
	if err != nil || !reused || run.ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("run=%#v reused=%v err=%v", run, reused, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryRereadsSlotAfterConflict(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	now := time.Date(2026, 8, 17, 2, 0, 0, 0, time.UTC)
	slot := "2026-08-17T10:00"
	columns := []string{"id", "account_id", "slot_key", "trigger_kind", "model_id", "claimed_model", "status", "queued_at", "started_at", "finished_at", "created_at"}
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO account_model_detection_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).
		WillReturnRows(sqlmock.NewRows(columns).AddRow("11111111-1111-1111-1111-111111111111", int64(7), slot, "scheduled", "gpt-5.6-sol", "gpt-5.6-sol", service.AccountModelDetectionStatusRunning, now, now, nil, now))
	repo := &accountModelDetectionRepository{db: mockDB}
	run, reused, err := repo.Enqueue(context.Background(), service.AccountModelDetectionRun{ID: "22222222-2222-2222-2222-222222222222", AccountID: 7, SlotKey: &slot, TriggerKind: "scheduled", ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: service.AccountModelDetectionStatusQueued, QueuedAt: now, CreatedAt: now})
	if err != nil || !reused || run.ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("run=%#v reused=%v err=%v", run, reused, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryDoesNotInsertAfterSlotLookupFailure(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	slot := "2026-08-17T10:00"
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).WillReturnError(errors.New("database unavailable"))
	repo := &accountModelDetectionRepository{db: mockDB}
	_, _, err = repo.Enqueue(context.Background(), service.AccountModelDetectionRun{ID: "22222222-2222-2222-2222-222222222222", AccountID: 7, SlotKey: &slot, TriggerKind: "scheduled", ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol"})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("error = %v, want original slot lookup failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryReturnsErrorWhenConflictCannotBeResolved(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	slot := "2026-08-17T10:00"
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO account_model_detection_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, account_id, slot_key.*slot_key = \\$2").WithArgs(int64(7), slot).WillReturnError(errors.New("conflict reread failed"))
	repo := &accountModelDetectionRepository{db: mockDB}
	_, reused, err := repo.Enqueue(context.Background(), service.AccountModelDetectionRun{ID: "22222222-2222-2222-2222-222222222222", AccountID: 7, SlotKey: &slot, TriggerKind: "scheduled", ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol"})
	if err == nil || reused {
		t.Fatalf("reused=%v err=%v, want conflict reread error", reused, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryCompletesWithJSONCastsAndSidecarErrorCode(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	mock.ExpectExec("juice_summary = \\$4::jsonb, fingerprint_candidate = \\$5, fingerprint_similarity = \\$6::jsonb").
		WithArgs("11111111-1111-1111-1111-111111111111", service.AccountModelDetectionStatusInsufficient, "partial", `{"score":0.5}`, "gpt-5.6-sol", `{"gpt-5.6-sol":0.8}`, "detector-1", "evidence_low", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := &accountModelDetectionRepository{db: mockDB}
	err = repo.Complete(context.Background(), "11111111-1111-1111-1111-111111111111", service.AccountModelDetectionResponse{
		Status: service.AccountModelDetectionStatusInsufficient, JuiceStatus: "partial", JuiceSummary: map[string]any{"score": 0.5}, FingerprintCandidate: "gpt-5.6-sol", FingerprintSimilarity: map[string]any{"gpt-5.6-sol": 0.8}, DetectorVersion: "detector-1", ErrorCode: "evidence_low",
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositoryListsQueuedRunsInFIFOOrder(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	mock.ExpectQuery("SELECT id::text FROM account_model_detection_runs WHERE status = 'queued' ORDER BY queued_at ASC LIMIT \\$1").WithArgs(4).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("run-1").AddRow("run-2"))
	repo := &accountModelDetectionRepository{db: mockDB}
	runIDs, err := repo.ListQueued(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(runIDs) != 2 || runIDs[0] != "run-1" || runIDs[1] != "run-2" {
		t.Fatalf("run ids = %#v", runIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountModelDetectionRepositorySavesSettings(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer mockDB.Close()
	mock.ExpectExec("INSERT INTO account_model_detection_settings").WithArgs(int64(7), "gpt-5.6-sol", "gpt-5.6-sol", int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	repo := &accountModelDetectionRepository{db: mockDB}
	err = repo.SaveSettings(context.Background(), service.AccountModelDetectionSettings{AccountID: 7, ConnectionProbeModel: "gpt-5.6-sol", ModelDetectionModel: "gpt-5.6-sol", UpdatedBy: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInferenceForLegacyQueuedRunRestoresTierMetadata(t *testing.T) {
	run := service.AccountModelDetectionRun{Status: service.AccountModelDetectionStatusRunning, TriggerKind: "scheduled", SlotKey: ptrString("2026-08-26T00:00")}
	if got := inferredQueuedDetectionProfile(run); got != service.AccountModelDetectionProfileMedium {
		t.Fatalf("profile=%q, want medium", got)
	}
	if got := inferredQueuedDetectionMode(run); got != service.AccountModelDetectionModeMonitor {
		t.Fatalf("mode=%q, want monitor", got)
	}
	if got := inferredDetectionPlannedRequests(service.AccountModelDetectionProfileMedium); got != 49 {
		t.Fatalf("planned=%d, want 49", got)
	}
}

func ptrString(value string) *string { return &value }
