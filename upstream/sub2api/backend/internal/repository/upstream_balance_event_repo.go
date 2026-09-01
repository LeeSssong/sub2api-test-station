package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const upstreamBalanceDefaultActiveLimit = 500

var upstreamBalanceErrorCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var upstreamBalanceEventColumns = []string{
	"id",
	"rule_id",
	"status",
	"scope_type",
	"scope_key",
	"notification_state",
	"metric_value",
	"last_observed_at",
	"last_delivered_at",
	"delivery_generation",
	"delivery_attempt_count",
	"next_attempt_at",
	"delivery_lease_token",
	"delivery_lease_until",
	"last_delivery_error_code",
	"created_at",
}

const upstreamBalanceEventSelectColumns = `
  id,
  rule_id,
  status,
  scope_type,
  scope_key,
  notification_state,
  metric_value,
  last_observed_at,
  last_delivered_at,
  delivery_generation,
  delivery_attempt_count,
  next_attempt_at,
  COALESCE(delivery_lease_token, ''),
  delivery_lease_until,
  COALESCE(last_delivery_error_code, ''),
	  created_at`

type upstreamBalanceEventRepository struct {
	db       *sql.DB
	newToken func() string
}

func NewUpstreamBalanceEventRepository(db *sql.DB) service.UpstreamBalanceEventRepository {
	return newUpstreamBalanceEventRepository(db, randomUpstreamBalanceLeaseToken)
}

func newUpstreamBalanceEventRepository(db *sql.DB, newToken func() string) *upstreamBalanceEventRepository {
	if newToken == nil {
		newToken = randomUpstreamBalanceLeaseToken
	}
	return &upstreamBalanceEventRepository{db: db, newToken: newToken}
}

func (r *upstreamBalanceEventRepository) GetRuleID(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("nil upstream balance event repository")
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT id
FROM ops_alert_rules
WHERE name = $1
LIMIT 1`, service.UpstreamBalanceAlertRuleName).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *upstreamBalanceEventRepository) Claim(ctx context.Context, input service.UpstreamBalanceClaimInput) (service.UpstreamBalanceDeliveryLease, bool, error) {
	if err := validateUpstreamBalanceClaimInput(input); err != nil {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	if r == nil || r.db == nil {
		return service.UpstreamBalanceDeliveryLease{}, false, errors.New("nil upstream balance event repository")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var locked bool
	if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", upstreamBalanceScopeLockID(input.RuleID, input.ScopeKey)).Scan(&locked); err != nil {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	if !locked {
		return service.UpstreamBalanceDeliveryLease{}, false, nil
	}

	event, err := selectActiveUpstreamBalanceEvent(ctx, tx, input.RuleID, input.ScopeKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	insertEvent := errors.Is(err, sql.ErrNoRows)
	if insertEvent {
		latestObservedAt, historyErr := selectLatestUpstreamBalanceObservation(ctx, tx, input.RuleID, input.ScopeKey)
		if historyErr != nil && !errors.Is(historyErr, sql.ErrNoRows) {
			return service.UpstreamBalanceDeliveryLease{}, false, historyErr
		}
		if historyErr == nil && !input.ObservedAt.After(latestObservedAt) {
			if err := tx.Commit(); err != nil {
				return service.UpstreamBalanceDeliveryLease{}, false, err
			}
			return service.UpstreamBalanceDeliveryLease{}, false, nil
		}
	}
	if !insertEvent && !shouldClaimUpstreamBalanceEvent(event, input) {
		if err := tx.Commit(); err != nil {
			return service.UpstreamBalanceDeliveryLease{}, false, err
		}
		return service.UpstreamBalanceDeliveryLease{}, false, nil
	}
	if !insertEvent && upstreamBalanceSilenced(ctx, tx, input.RuleID, input.ScopeKey, input.Now) {
		if err := tx.Commit(); err != nil {
			return service.UpstreamBalanceDeliveryLease{}, false, err
		}
		return service.UpstreamBalanceDeliveryLease{}, false, nil
	}

	leaseUntil := input.Now.Add(input.LeaseDuration)
	token := r.newToken()
	if strings.TrimSpace(token) == "" || len(token) > 64 {
		return service.UpstreamBalanceDeliveryLease{}, false, errors.New("invalid upstream balance lease token")
	}

	if insertEvent {
		event, err = insertUpstreamBalanceEvent(ctx, tx, input, token, leaseUntil)
	} else {
		event, err = updateUpstreamBalanceEventClaim(ctx, tx, event, input, token, leaseUntil)
		if errors.Is(err, sql.ErrNoRows) {
			if commitErr := tx.Commit(); commitErr != nil {
				return service.UpstreamBalanceDeliveryLease{}, false, commitErr
			}
			return service.UpstreamBalanceDeliveryLease{}, false, nil
		}
	}
	if err != nil {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	if strings.TrimSpace(input.ActionTokenHash) != "" {
		if err := persistUpstreamBalanceActionTokenHash(ctx, tx, event.ID, input.ActionTokenHash); err != nil {
			return service.UpstreamBalanceDeliveryLease{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return service.UpstreamBalanceDeliveryLease{}, false, err
	}
	lease := upstreamBalanceDeliveryLease(*event)
	lease.ActionToken = input.ActionToken
	return lease, true, nil
}

func (r *upstreamBalanceEventRepository) GetCurrent(ctx context.Context, eventID int64) (*service.UpstreamBalanceEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil upstream balance event repository")
	}
	if eventID <= 0 {
		return nil, errors.New("invalid upstream balance event id")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT`+upstreamBalanceEventSelectColumns+`
FROM ops_alert_events
WHERE id = $1`, eventID)
	event, err := scanUpstreamBalanceEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return event, err
}

func (r *upstreamBalanceEventRepository) ConfirmDelivery(ctx context.Context, result service.UpstreamBalanceDeliveryResult) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("nil upstream balance event repository")
	}
	if result.EventID <= 0 || result.Generation <= 0 || strings.TrimSpace(result.LeaseToken) == "" || result.At.IsZero() {
		return false, errors.New("invalid upstream balance delivery result")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE ops_alert_events
SET last_delivered_at = $2,
    delivery_attempt_count = 0,
    next_attempt_at = NULL,
    delivery_lease_token = NULL,
    delivery_lease_until = NULL,
    last_delivery_error_code = NULL
WHERE id = $1
  AND delivery_generation = $3
  AND delivery_lease_token = $4
  AND status = $5`, result.EventID, result.At, result.Generation, result.LeaseToken, service.OpsAlertStatusFiring)
	return upstreamBalanceCASResult(res, err)
}

func (r *upstreamBalanceEventRepository) RecordFailure(ctx context.Context, failure service.UpstreamBalanceDeliveryFailure) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("nil upstream balance event repository")
	}
	if failure.EventID <= 0 || failure.Generation <= 0 || strings.TrimSpace(failure.LeaseToken) == "" || failure.NextAttemptAt.IsZero() {
		return false, errors.New("invalid upstream balance delivery failure")
	}
	if !upstreamBalanceErrorCodePattern.MatchString(failure.ErrorCode) {
		return false, errors.New("invalid upstream balance delivery error code")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE ops_alert_events
SET delivery_attempt_count = delivery_attempt_count + 1,
    next_attempt_at = $4,
    last_delivery_error_code = $5,
    delivery_lease_token = NULL,
    delivery_lease_until = NULL
WHERE id = $1
  AND delivery_generation = $2
  AND delivery_lease_token = $3
  AND status = $6`, failure.EventID, failure.Generation, failure.LeaseToken, failure.NextAttemptAt, failure.ErrorCode, service.OpsAlertStatusFiring)
	return upstreamBalanceCASResult(res, err)
}

func (r *upstreamBalanceEventRepository) Resolve(ctx context.Context, ruleID int64, scopeKey string, observedAt time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("nil upstream balance event repository")
	}
	if ruleID <= 0 || strings.TrimSpace(scopeKey) == "" || observedAt.IsZero() {
		return false, errors.New("invalid upstream balance resolve input")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var locked bool
	if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", upstreamBalanceScopeLockID(ruleID, scopeKey)).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}
	res, err := tx.ExecContext(ctx, `
UPDATE ops_alert_events
SET status = $4,
    resolved_at = NOW(),
    last_observed_at = $3,
    delivery_generation = delivery_generation + 1,
    delivery_lease_token = NULL,
    delivery_lease_until = NULL,
    next_attempt_at = NULL,
    last_delivery_error_code = NULL
WHERE rule_id = $1
  AND scope_type = $5
  AND scope_key = $2
  AND status = $6
  AND (last_observed_at IS NULL OR last_observed_at < $3)`, ruleID, scopeKey, observedAt, service.OpsAlertStatusResolved, service.UpstreamBalanceScopeTypeBaseURL, service.OpsAlertStatusFiring)
	changed, err := upstreamBalanceCASResult(res, err)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (r *upstreamBalanceEventRepository) ListActive(ctx context.Context, limit int) ([]service.UpstreamBalanceEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil upstream balance event repository")
	}
	if limit <= 0 || limit > upstreamBalanceDefaultActiveLimit {
		limit = upstreamBalanceDefaultActiveLimit
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT`+upstreamBalanceEventSelectColumns+`
FROM ops_alert_events
WHERE status = $1
  AND scope_type = $2
ORDER BY id ASC
LIMIT $3`, service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.UpstreamBalanceEvent, 0)
	for rows.Next() {
		event, err := scanUpstreamBalanceEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *event)
	}
	return out, rows.Err()
}

func (r *upstreamBalanceEventRepository) WithScopeLock(ctx context.Context, ruleID int64, scopeKey string, fn func(context.Context) error) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("nil upstream balance event repository")
	}
	if ruleID <= 0 || strings.TrimSpace(scopeKey) == "" || fn == nil {
		return false, errors.New("invalid upstream balance scope lock input")
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close() }()
	lockID := upstreamBalanceScopeLockID(ruleID, scopeKey)
	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID)
	}()
	return true, fn(ctx)
}

func (r *upstreamBalanceEventRepository) IsSilenced(ctx context.Context, ruleID int64, scopeKey string, now time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("nil upstream balance event repository")
	}
	var until sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT silenced_until FROM ops_alert_events WHERE rule_id=$1 AND scope_type=$2 AND scope_key=$3 AND status=$4`, ruleID, service.UpstreamBalanceScopeTypeBaseURL, scopeKey, service.OpsAlertStatusFiring).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return until.Valid && until.Time.After(now), err
}

func (r *upstreamBalanceEventRepository) Silence(ctx context.Context, input service.UpstreamBalanceNotificationSilenceInput) (bool, error) {
	if r == nil || r.db == nil || input.RuleID <= 0 || strings.TrimSpace(input.ScopeKey) == "" || len(input.ActionTokenHash) != 64 || input.Until.Before(input.Now) {
		return false, errors.New("invalid upstream balance silence input")
	}
	res, err := r.db.ExecContext(ctx, `UPDATE ops_alert_events SET silenced_until=$1, delivery_lease_token=NULL, delivery_lease_until=NULL, next_attempt_at=$2 WHERE rule_id=$3 AND scope_type=$4 AND scope_key=$5 AND status=$6 AND action_token_hash=$7`, input.Until, input.Until, input.RuleID, service.UpstreamBalanceScopeTypeBaseURL, input.ScopeKey, service.OpsAlertStatusFiring, input.ActionTokenHash)
	return upstreamBalanceCASResult(res, err)
}

func (r *upstreamBalanceEventRepository) SilenceByActionToken(ctx context.Context, actionTokenHash string, until, now time.Time) (bool, error) {
	if r == nil || r.db == nil || len(actionTokenHash) != 64 || until.Before(now) {
		return false, errors.New("invalid upstream balance silence token input")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE ops_alert_events
SET silenced_until = $1, delivery_lease_token = NULL, delivery_lease_until = NULL, next_attempt_at = $1
WHERE status = $2 AND scope_type = $3 AND action_token_hash = $4`,
		until, service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL, actionTokenHash)
	return upstreamBalanceCASResult(res, err)
}

func selectActiveUpstreamBalanceEvent(ctx context.Context, tx *sql.Tx, ruleID int64, scopeKey string) (*service.UpstreamBalanceEvent, error) {
	row := tx.QueryRowContext(ctx, `
SELECT`+upstreamBalanceEventSelectColumns+`
FROM ops_alert_events
WHERE rule_id = $1
  AND scope_type = $2
  AND scope_key = $3
  AND status = $4
FOR UPDATE`, ruleID, service.UpstreamBalanceScopeTypeBaseURL, scopeKey, service.OpsAlertStatusFiring)
	return scanUpstreamBalanceEvent(row)
}

func selectLatestUpstreamBalanceObservation(ctx context.Context, tx *sql.Tx, ruleID int64, scopeKey string) (time.Time, error) {
	var observedAt time.Time
	err := tx.QueryRowContext(ctx, `
SELECT last_observed_at
FROM ops_alert_events
WHERE rule_id = $1
  AND scope_type = $2
  AND scope_key = $3
  AND status <> $4
  AND last_observed_at IS NOT NULL
ORDER BY last_observed_at DESC, id DESC
LIMIT 1`, ruleID, service.UpstreamBalanceScopeTypeBaseURL, scopeKey, service.OpsAlertStatusFiring).Scan(&observedAt)
	return observedAt, err
}

func insertUpstreamBalanceEvent(ctx context.Context, tx *sql.Tx, input service.UpstreamBalanceClaimInput, token string, leaseUntil time.Time) (*service.UpstreamBalanceEvent, error) {
	row := tx.QueryRowContext(ctx, `
INSERT INTO ops_alert_events (
  rule_id, severity, status, scope_type, scope_key, notification_state,
  metric_value, last_observed_at, fired_at, delivery_generation, delivery_attempt_count,
  delivery_lease_token, delivery_lease_until, created_at
) VALUES (
  $1, CASE WHEN $4 = 'zero' THEN 'P1' ELSE 'P2' END, 'firing', $2, $3, $4,
  $5, $6, $7, 1, 0, $8, $9, $7
)
RETURNING`+upstreamBalanceEventSelectColumns,
		input.RuleID, service.UpstreamBalanceScopeTypeBaseURL, input.ScopeKey,
		input.NotificationState, input.ValueUSD, input.ObservedAt, input.Now, token, leaseUntil)
	return scanUpstreamBalanceEvent(row)
}

func updateUpstreamBalanceEventClaim(ctx context.Context, tx *sql.Tx, event *service.UpstreamBalanceEvent, input service.UpstreamBalanceClaimInput, token string, leaseUntil time.Time) (*service.UpstreamBalanceEvent, error) {
	stateChanged := event.NotificationState != input.NotificationState
	row := tx.QueryRowContext(ctx, `
UPDATE ops_alert_events
SET severity = CASE WHEN $2 = 'zero' THEN 'P1' ELSE 'P2' END,
    notification_state = $2,
    metric_value = $3,
    last_observed_at = $4,
    delivery_generation = delivery_generation + 1,
    delivery_attempt_count = CASE WHEN $5 THEN 0 ELSE delivery_attempt_count END,
    next_attempt_at = CASE WHEN $5 THEN NULL ELSE next_attempt_at END,
    last_delivery_error_code = CASE WHEN $5 THEN NULL ELSE last_delivery_error_code END,
    delivery_lease_token = $6,
    delivery_lease_until = $7
WHERE id = $1
  AND status = 'firing'
  AND (
    last_observed_at IS NULL
    OR last_observed_at < $4
    OR (
      last_observed_at = $4
      AND notification_state = $2
      AND metric_value = $3
    )
  )
RETURNING`+upstreamBalanceEventSelectColumns,
		event.ID, input.NotificationState, input.ValueUSD, input.ObservedAt, stateChanged, token, leaseUntil)
	return scanUpstreamBalanceEvent(row)
}

func shouldClaimUpstreamBalanceEvent(event *service.UpstreamBalanceEvent, input service.UpstreamBalanceClaimInput) bool {
	if event == nil || event.NotificationState != input.NotificationState {
		return true
	}
	if event.DeliveryLeaseToken != "" && event.DeliveryLeaseUntil != nil {
		return !event.DeliveryLeaseUntil.After(input.Now)
	}
	if event.NextAttemptAt != nil {
		return !event.NextAttemptAt.After(input.Now)
	}
	if event.LastDeliveredAt == nil {
		return true
	}
	return !event.LastDeliveredAt.Add(input.RepeatInterval).After(input.Now)
}

type upstreamBalanceEventScanner interface {
	Scan(dest ...any) error
}

func scanUpstreamBalanceEvent(row upstreamBalanceEventScanner) (*service.UpstreamBalanceEvent, error) {
	var event service.UpstreamBalanceEvent
	var lastDeliveredAt sql.NullTime
	var nextAttemptAt sql.NullTime
	var leaseUntil sql.NullTime
	if err := row.Scan(
		&event.ID,
		&event.RuleID,
		&event.Status,
		&event.ScopeType,
		&event.ScopeKey,
		&event.NotificationState,
		&event.ValueUSD,
		&event.LastObservedAt,
		&lastDeliveredAt,
		&event.DeliveryGeneration,
		&event.DeliveryAttemptCount,
		&nextAttemptAt,
		&event.DeliveryLeaseToken,
		&leaseUntil,
		&event.LastDeliveryErrorCode,
		&event.CreatedAt,
	); err != nil {
		return nil, err
	}
	if lastDeliveredAt.Valid {
		event.LastDeliveredAt = &lastDeliveredAt.Time
	}
	if nextAttemptAt.Valid {
		event.NextAttemptAt = &nextAttemptAt.Time
	}
	if leaseUntil.Valid {
		event.DeliveryLeaseUntil = &leaseUntil.Time
	}
	return &event, nil
}

func upstreamBalanceDeliveryLease(event service.UpstreamBalanceEvent) service.UpstreamBalanceDeliveryLease {
	lease := service.UpstreamBalanceDeliveryLease{
		EventID: event.ID, RuleID: event.RuleID, ScopeKey: event.ScopeKey,
		NotificationState: event.NotificationState, ObservedAt: event.LastObservedAt,
		ValueUSD:   event.ValueUSD,
		Generation: event.DeliveryGeneration, Token: event.DeliveryLeaseToken,
	}
	if event.DeliveryLeaseUntil != nil {
		lease.LeaseUntil = *event.DeliveryLeaseUntil
	}
	return lease
}

func upstreamBalanceSilenced(ctx context.Context, tx *sql.Tx, ruleID int64, scopeKey string, now time.Time) bool {
	var until sql.NullTime
	err := tx.QueryRowContext(ctx, `
SELECT silenced_until
FROM ops_alert_events
WHERE rule_id = $1 AND scope_type = $2 AND scope_key = $3 AND status = $4`,
		ruleID, service.UpstreamBalanceScopeTypeBaseURL, scopeKey, service.OpsAlertStatusFiring).Scan(&until)
	return err == nil && until.Valid && until.Time.After(now)
}

func persistUpstreamBalanceActionTokenHash(ctx context.Context, tx *sql.Tx, eventID int64, hash string) error {
	if len(hash) != 64 {
		return errors.New("invalid upstream balance action token hash")
	}
	_, err := tx.ExecContext(ctx, `UPDATE ops_alert_events SET action_token_hash = $2 WHERE id = $1`, eventID, hash)
	return err
}

func validateUpstreamBalanceClaimInput(input service.UpstreamBalanceClaimInput) error {
	if input.RuleID <= 0 || strings.TrimSpace(input.ScopeKey) == "" || len(input.ScopeKey) > 2048 {
		return errors.New("invalid upstream balance claim scope")
	}
	if input.NotificationState != service.UpstreamBalanceNotificationStateLow && input.NotificationState != service.UpstreamBalanceNotificationStateZero {
		return errors.New("invalid upstream balance notification state")
	}
	if input.ValueUSD < 0 || input.ValueUSD >= 5 || math.IsNaN(input.ValueUSD) || math.IsInf(input.ValueUSD, 0) ||
		input.ObservedAt.IsZero() || input.Now.IsZero() || input.RepeatInterval <= 0 || input.LeaseDuration <= 0 {
		return errors.New("invalid upstream balance claim timing")
	}
	return nil
}

func upstreamBalanceCASResult(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func upstreamBalanceScopeLockID(ruleID int64, scopeKey string) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "upstream-balance:%d:%s", ruleID, scopeKey)
	return int64(h.Sum64())
}

func randomUpstreamBalanceLeaseToken() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(token[:])
}
