package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const newAPIRateRegistrationExtraKey = "newapi_rate_registration"

func (r *accountRepository) ClaimNewAPIRateRefresh(ctx context.Context, accountID int64, refreshDate, claimToken string, claimUntil time.Time) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	if accountID <= 0 || claimToken == "" || refreshDate == "" || claimUntil.IsZero() {
		return false, service.ErrAccountNilInput
	}
	claim, err := json.Marshal(map[string]any{
		"claim_token":      claimToken,
		"claim_date":       refreshDate,
		"claim_expires_at": claimUntil.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET extra = jsonb_set(
			COALESCE(extra, '{}'::jsonb),
			'{newapi_rate_registration}',
			COALESCE(extra -> 'newapi_rate_registration', '{}'::jsonb) || $1::jsonb,
			true
		), updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE id = $2
		  AND deleted_at IS NULL
		  AND type = $3
		  AND (
			COALESCE(extra -> 'account_monitor_balance' ->> 'source', '') = $4
			OR (COALESCE(extra -> 'account_monitor_balance' ->> 'source', '') = ''
				AND COALESCE(extra -> 'upstream_billing_probe' ->> 'status', '') = $5)
		  )
		  AND COALESCE(extra -> 'upstream_billing_probe' ->> 'status', '') <> $6
		  AND COALESCE(extra -> 'newapi_rate_registration' ->> 'last_refresh_date', '') <> $7
		  AND (
			(extra -> 'newapi_rate_registration' ->> 'claim_token') IS NULL
			OR (extra -> 'newapi_rate_registration' ->> 'claim_expires_at') IS NULL
			OR (extra -> 'newapi_rate_registration' ->> 'claim_expires_at' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' AND
				(extra -> 'newapi_rate_registration' ->> 'claim_expires_at')::timestamptz <= clock_timestamp())
		  )
	`, string(claim), accountID, service.AccountTypeAPIKey, service.AccountMonitorBalanceSourceNewAPI, service.UpstreamBillingProbeStatusUnsupported, service.UpstreamBillingProbeStatusOK, refreshDate)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *accountRepository) CompleteNewAPIRateRefresh(ctx context.Context, input service.NewAPIRateRefreshCompletion) error {
	if r == nil || r.client == nil {
		return errors.New("account repository client is not configured")
	}
	if input.AccountID <= 0 || input.ClaimToken == "" || input.RefreshDate == "" || input.ObservedAt.IsZero() || input.UsageLogID <= 0 || math.IsNaN(input.GroupRatio) || math.IsInf(input.GroupRatio, 0) || input.GroupRatio < 0 || input.GroupRatio > 100 {
		return service.ErrAccountNilInput
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Client().QueryContext(txCtx, `SELECT id FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, input.AccountID)
	if err != nil {
		return err
	}
	if !rows.Next() {
		_ = rows.Close()
		return service.ErrAccountNotFound
	}
	if err := rows.Close(); err != nil {
		return err
	}

	snapshot, err := json.Marshal(map[string]any{
		"status":              "registered",
		"source":              "newapi_log_other_group_ratio",
		"group_ratio":         input.GroupRatio,
		"last_observed_at":    input.ObservedAt.UTC().Format(time.RFC3339Nano),
		"last_refresh_date":   input.RefreshDate,
		"source_usage_log_id": input.UsageLogID,
	})
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(txCtx, `
		UPDATE accounts
		SET rate_multiplier = ROUND($1::numeric, 4),
		    extra = jsonb_set(
				COALESCE(extra, '{}'::jsonb),
				'{newapi_rate_registration}',
				(CASE WHEN COALESCE(extra -> 'newapi_rate_registration' ->> 'registered_at', '') = ''
					THEN jsonb_build_object('registered_at', $3::timestamptz::text)
					ELSE '{}'::jsonb END) ||
					(COALESCE(extra -> 'newapi_rate_registration', '{}'::jsonb) - 'claim_token' - 'claim_date' - 'claim_expires_at') ||
					$2::jsonb,
				true
			),
		    updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE id = $4
		  AND deleted_at IS NULL
		  AND type = $5
		  AND (
			COALESCE(extra -> 'account_monitor_balance' ->> 'source', '') = $6
			OR (COALESCE(extra -> 'account_monitor_balance' ->> 'source', '') = ''
				AND COALESCE(extra -> 'upstream_billing_probe' ->> 'status', '') = $7)
		  )
		  AND COALESCE(extra -> 'upstream_billing_probe' ->> 'status', '') <> $8
		  AND extra -> 'newapi_rate_registration' ->> 'claim_token' = $9
		  AND extra -> 'newapi_rate_registration' ->> 'claim_date' = $10
		`, input.GroupRatio, string(snapshot), input.ObservedAt.UTC().Format(time.RFC3339Nano), input.AccountID, service.AccountTypeAPIKey, service.AccountMonitorBalanceSourceNewAPI, service.UpstreamBillingProbeStatusUnsupported, service.UpstreamBillingProbeStatusOK, input.ClaimToken, input.RefreshDate)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUpstreamBillingProbeIdentityChanged
	}
	if err := enqueueSchedulerOutbox(txCtx, tx, service.SchedulerOutboxEventAccountChanged, &input.AccountID, nil, nil); err != nil {
		return fmt.Errorf("enqueue scheduler outbox: %w", err)
	}
	return tx.Commit()
}

func (r *accountRepository) ReleaseNewAPIRateRefresh(ctx context.Context, accountID int64, claimToken string) error {
	if r == nil || r.sql == nil {
		return errors.New("account repository SQL executor is not configured")
	}
	if accountID <= 0 || claimToken == "" {
		return service.ErrAccountNilInput
	}
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{newapi_rate_registration}',
			COALESCE(extra -> 'newapi_rate_registration', '{}'::jsonb) - 'claim_token' - 'claim_date' - 'claim_expires_at', true),
		    updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE id = $1 AND deleted_at IS NULL
		  AND extra -> 'newapi_rate_registration' ->> 'claim_token' = $2
	`, accountID, claimToken)
	return err
}
