package refund

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SQLStore is the durable outbox-backed worker store. Claim leases one
// pending/unknown adjustment at a time and commits before the provider call.
type SQLStore struct {
	DB    *sql.DB
	Lease time.Duration
}

func (s *SQLStore) Claim(ctx context.Context) (*Job, error) {
	if s == nil || s.DB == nil {
		return nil, ErrWorkerUnavailable
	}
	lease := s.Lease
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id, attempt int64
	var requestKey, providerID, payloadRaw string
	err = tx.QueryRowContext(ctx, `SELECT id,attempt_count,COALESCE(provider_request_key,''),COALESCE(refund_provider_instance_id,''),COALESCE(provider_response_snapshot,'{}'::jsonb)::text FROM user_quota_adjustments WHERE status IN ('pending','unknown','reconciling') AND (next_retry_at IS NULL OR next_retry_at <= NOW()) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &attempt, &requestKey, &providerID, &payloadRaw)
	if err == sql.ErrNoRows {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_quota_adjustments SET status='pending',provider_state='requested',attempt_count=attempt_count+1,last_attempt_at=NOW(),next_retry_at=NOW()+$1::interval,updated_at=NOW() WHERE id=$2`, fmt.Sprintf("%d seconds", int(lease.Seconds())), id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	payload := map[string]string{}
	_ = json.Unmarshal([]byte(payloadRaw), &payload)
	return &Job{AdjustmentID: id, Attempt: int(attempt) + 1, RequestKey: requestKey, ProviderID: providerID, Payload: payload}, nil
}

func (s *SQLStore) Complete(ctx context.Context, id int64, result ProviderResult) error {
	if s == nil || s.DB == nil {
		return ErrWorkerUnavailable
	}
	raw, _ := json.Marshal(result.Snapshot)
	_, err := s.DB.ExecContext(ctx, `UPDATE user_quota_adjustments SET provider_state='succeeded',provider_refund_id=NULLIF($1,''),provider_response_snapshot=$2,status='completed',next_retry_at=NULL,updated_at=NOW() WHERE id=$3`, result.RefundID, raw, id)
	return err
}

func (s *SQLStore) Retry(ctx context.Context, id int64, at time.Time, providerErr error) error {
	if s == nil || s.DB == nil {
		return ErrWorkerUnavailable
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE user_quota_adjustments SET provider_state='unknown',provider_error_message=$1,status='unknown',next_retry_at=$2,updated_at=NOW() WHERE id=$3`, providerErr.Error(), at, id)
	return err
}

func (s *SQLStore) DeadLetter(ctx context.Context, id int64, providerErr error) error {
	if s == nil || s.DB == nil {
		return ErrWorkerUnavailable
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE user_quota_adjustments SET provider_state='failed',provider_error_message=$1,status='failed',next_retry_at=NULL,updated_at=NOW() WHERE id=$2`, providerErr.Error(), id)
	return err
}

func (s *SQLStore) MarkUnknown(ctx context.Context, id int64, result ProviderResult) error {
	if s == nil || s.DB == nil {
		return ErrWorkerUnavailable
	}
	raw, _ := json.Marshal(result.Snapshot)
	_, err := s.DB.ExecContext(ctx, `UPDATE user_quota_adjustments SET provider_state='unknown',provider_refund_id=NULLIF($1,''),provider_response_snapshot=$2,status='unknown',updated_at=NOW() WHERE id=$3`, result.RefundID, raw, id)
	return err
}
