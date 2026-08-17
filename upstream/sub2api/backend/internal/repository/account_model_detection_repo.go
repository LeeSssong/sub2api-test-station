package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountModelDetectionRepository struct{ db *sql.DB }

func NewAccountModelDetectionRepository(db *sql.DB) service.AccountModelDetectionRepository {
	return &accountModelDetectionRepository{db: db}
}

func (r *accountModelDetectionRepository) LoadSettings(ctx context.Context, accountID int64) (service.AccountModelDetectionSettings, error) {
	var settings service.AccountModelDetectionSettings
	err := r.db.QueryRowContext(ctx, `SELECT account_id, connection_probe_model, model_detection_model, updated_by, updated_at FROM account_model_detection_settings WHERE account_id = $1`, accountID).
		Scan(&settings.AccountID, &settings.ConnectionProbeModel, &settings.ModelDetectionModel, &settings.UpdatedBy, &settings.UpdatedAt)
	return settings, err
}

func (r *accountModelDetectionRepository) SaveSettings(ctx context.Context, settings service.AccountModelDetectionSettings) error {
	if settings.AccountID <= 0 {
		return errors.New("account_id is required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_model_detection_settings (account_id, connection_probe_model, model_detection_model, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (account_id) DO UPDATE SET connection_probe_model = EXCLUDED.connection_probe_model,
		model_detection_model = EXCLUDED.model_detection_model, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at
	`, settings.AccountID, settings.ConnectionProbeModel, settings.ModelDetectionModel, settings.UpdatedBy)
	return err
}

func (r *accountModelDetectionRepository) Enqueue(ctx context.Context, run service.AccountModelDetectionRun) (service.AccountModelDetectionRun, bool, error) {
	if run.ID == "" || run.AccountID <= 0 || run.ModelID == "" || run.ClaimedModel == "" || run.TriggerKind == "" {
		return service.AccountModelDetectionRun{}, false, errors.New("invalid model detection run")
	}
	if run.Status == "" {
		run.Status = service.AccountModelDetectionStatusQueued
	}
	if run.QueuedAt.IsZero() {
		run.QueuedAt = time.Now().UTC()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = run.QueuedAt
	}
	var existing service.AccountModelDetectionRun
	query := `SELECT id, account_id, slot_key, trigger_kind, model_id, claimed_model, status, queued_at, started_at, finished_at, created_at FROM account_model_detection_runs WHERE account_id = $1`
	args := []any{run.AccountID}
	if run.SlotKey != nil {
		query += ` AND slot_key = $2`
		args = append(args, *run.SlotKey)
	} else {
		query += ` AND status IN ('queued', 'running')`
	}
	query += ` ORDER BY created_at DESC LIMIT 1`
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&existing.ID, &existing.AccountID, &existing.SlotKey, &existing.TriggerKind, &existing.ModelID, &existing.ClaimedModel, &existing.Status, &existing.QueuedAt, &existing.StartedAt, &existing.FinishedAt, &existing.CreatedAt)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return service.AccountModelDetectionRun{}, false, err
	}
	insertResult, err := r.db.ExecContext(ctx, `INSERT INTO account_model_detection_runs (id, account_id, slot_key, trigger_kind, model_id, claimed_model, status, queued_at, created_at) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`, run.ID, run.AccountID, run.SlotKey, run.TriggerKind, run.ModelID, run.ClaimedModel, run.Status, run.QueuedAt.UTC(), run.CreatedAt.UTC())
	if err != nil {
		return service.AccountModelDetectionRun{}, false, err
	}
	if affected, affectedErr := insertResult.RowsAffected(); affectedErr == nil && affected == 0 {
		existingErr := r.db.QueryRowContext(ctx, query, args...).Scan(&existing.ID, &existing.AccountID, &existing.SlotKey, &existing.TriggerKind, &existing.ModelID, &existing.ClaimedModel, &existing.Status, &existing.QueuedAt, &existing.StartedAt, &existing.FinishedAt, &existing.CreatedAt)
		if existingErr == nil {
			return existing, true, nil
		}
		return service.AccountModelDetectionRun{}, false, existingErr
	} else if affectedErr != nil {
		return service.AccountModelDetectionRun{}, false, affectedErr
	}
	return run, false, nil
}

func (r *accountModelDetectionRepository) Claim(ctx context.Context, runID string) (*service.AccountModelDetectionRun, error) {
	if runID == "" {
		return nil, errors.New("run_id is required")
	}
	row := r.db.QueryRowContext(ctx, `UPDATE account_model_detection_runs SET status = 'running', started_at = NOW() WHERE id = $1::uuid AND status = 'queued' RETURNING id, account_id, slot_key, trigger_kind, model_id, claimed_model, status, queued_at, started_at, finished_at, created_at`, runID)
	run, err := scanModelDetectionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *accountModelDetectionRepository) ListQueued(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id::text FROM account_model_detection_runs WHERE status = 'queued' ORDER BY queued_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		result = append(result, runID)
	}
	return result, rows.Err()
}

func (r *accountModelDetectionRepository) Complete(ctx context.Context, runID string, response service.AccountModelDetectionResponse, errorCode, errorMessage string) error {
	if runID == "" {
		return errors.New("run_id is required")
	}
	juice, _ := json.Marshal(response.JuiceSummary)
	fingerprint, _ := json.Marshal(response.FingerprintSimilarity)
	status := response.Status
	if status == "" {
		status = service.AccountModelDetectionStatusFailed
	}
	if errorCode != "" {
		status = service.AccountModelDetectionStatusFailed
	}
	if errorCode == "" {
		errorCode = response.ErrorCode
	}
	_, err := r.db.ExecContext(ctx, `UPDATE account_model_detection_runs SET status = $2, juice_status = $3, juice_summary = $4::jsonb, fingerprint_candidate = $5, fingerprint_similarity = $6::jsonb, detector_version = $7, error_code = NULLIF($8, ''), error_message = NULLIF($9, ''), finished_at = NOW() WHERE id = $1::uuid AND status = 'running'`, runID, status, response.JuiceStatus, nullableJSON(juice), response.FingerprintCandidate, nullableJSON(fingerprint), response.DetectorVersion, errorCode, errorMessage)
	return err
}

func (r *accountModelDetectionRepository) ListRecent(ctx context.Context, accountID int64, limit int) ([]service.AccountModelDetectionRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, account_id, slot_key, trigger_kind, model_id, claimed_model, status, juice_status, juice_summary, fingerprint_candidate, fingerprint_similarity, detector_version, error_code, error_message, queued_at, started_at, finished_at, created_at FROM account_model_detection_runs WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]service.AccountModelDetectionRun, 0)
	for rows.Next() {
		run, err := scanFullModelDetectionRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanModelDetectionRun(row scanner) (service.AccountModelDetectionRun, error) {
	var run service.AccountModelDetectionRun
	err := row.Scan(&run.ID, &run.AccountID, &run.SlotKey, &run.TriggerKind, &run.ModelID, &run.ClaimedModel, &run.Status, &run.QueuedAt, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	return run, err
}

func scanFullModelDetectionRun(row scanner) (service.AccountModelDetectionRun, error) {
	var run service.AccountModelDetectionRun
	var juice, fingerprint []byte
	var juiceStatus, candidate, version, errorCode, errorMessage sql.NullString
	err := row.Scan(&run.ID, &run.AccountID, &run.SlotKey, &run.TriggerKind, &run.ModelID, &run.ClaimedModel, &run.Status, &juiceStatus, &juice, &candidate, &fingerprint, &version, &errorCode, &errorMessage, &run.QueuedAt, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if err != nil {
		return run, err
	}
	run.JuiceStatus, run.FingerprintCandidate, run.DetectorVersion = juiceStatus.String, candidate.String, version.String
	run.ErrorCode, run.ErrorMessage = errorCode.String, errorMessage.String
	if len(juice) > 0 {
		_ = json.Unmarshal(juice, &run.JuiceSummary)
	}
	if len(fingerprint) > 0 {
		_ = json.Unmarshal(fingerprint, &run.FingerprintSimilarity)
	}
	return run, nil
}

func nullableJSON(value []byte) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return string(value)
}

var _ service.AccountModelDetectionRepository = (*accountModelDetectionRepository)(nil)
