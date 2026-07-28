package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"github.com/jackc/pgx/v5"
)

type GroupSignal struct {
	GroupName        string
	SourceKind       string
	SourceKey        string
	Payload          json.RawMessage
	SourceObservedAt time.Time
	ExpiresAt        time.Time
}

type DecisionRecord struct {
	DecisionKey   string
	Family        string
	PolicyVersion int
	SourceKind    string
	Decision      string
	Reason        string
	Details       json.RawMessage
	ObservedAt    time.Time
}

type Baseline struct {
	Key          string
	CurrentValue string
	EvidenceHash string
	UpdatedAt    time.Time
}

func (s *Store) UpsertGroupSignal(ctx context.Context, signal GroupSignal) error {
	signal.GroupName = strings.TrimSpace(signal.GroupName)
	signal.SourceKind = strings.TrimSpace(signal.SourceKind)
	signal.SourceKey = strings.TrimSpace(signal.SourceKey)
	if !validBoundedText(signal.GroupName, 256) ||
		!validBoundedText(signal.SourceKind, 128) ||
		!validBoundedText(signal.SourceKey, 256) ||
		signal.SourceObservedAt.IsZero() ||
		signal.ExpiresAt.IsZero() ||
		!signal.ExpiresAt.After(signal.SourceObservedAt) ||
		len(signal.Payload) == 0 ||
		len(signal.Payload) > 64<<10 ||
		!json.Valid(signal.Payload) {
		return fmt.Errorf("group impact signal is invalid")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.group_impact_signals
			(group_name, source_kind, source_key, payload, source_observed_at, expires_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		ON CONFLICT (group_name, source_kind, source_key) DO UPDATE
		SET payload=EXCLUDED.payload,
		    source_observed_at=EXCLUDED.source_observed_at,
		    expires_at=EXCLUDED.expires_at,
		    updated_at=NOW()`,
		signal.GroupName, signal.SourceKind, signal.SourceKey, string(signal.Payload),
		signal.SourceObservedAt.UTC(), signal.ExpiresAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert group impact signal: %w", err)
	}
	return nil
}

func (s *Store) ListFreshGroupSignals(
	ctx context.Context,
	groupName string,
	now time.Time,
) ([]GroupSignal, error) {
	groupName = strings.TrimSpace(groupName)
	if !validBoundedText(groupName, 256) || now.IsZero() {
		return nil, fmt.Errorf("fresh group signal query is invalid")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT group_name, source_kind, source_key, payload,
		       source_observed_at, expires_at
		FROM relay_ops.group_impact_signals
		WHERE group_name=$1 AND expires_at>$2
		ORDER BY source_kind, source_key`, groupName, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("list fresh group impact signals: %w", err)
	}
	defer rows.Close()
	signals := make([]GroupSignal, 0)
	for rows.Next() {
		var signal GroupSignal
		if err := rows.Scan(
			&signal.GroupName, &signal.SourceKind, &signal.SourceKey, &signal.Payload,
			&signal.SourceObservedAt, &signal.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan fresh group impact signal: %w", err)
		}
		signals = append(signals, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fresh group impact signals: %w", err)
	}
	return signals, nil
}

func (s *Store) ReserveOneShot(
	ctx context.Context,
	reservation notify.OneShotReservation,
) (int64, bool, error) {
	if err := validateOneShotReservation(reservation); err != nil {
		return 0, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin one-shot reservation: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO relay_ops.notification_messages
			(notification_key, family, policy_version, source_kind, dedup_key,
			 message_hash, message_payload, delivery_status, attempt_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, 'reserved', 1)
		ON CONFLICT (notification_key) DO UPDATE
		SET family=EXCLUDED.family,
		    policy_version=EXCLUDED.policy_version,
		    source_kind=EXCLUDED.source_kind,
		    dedup_key=EXCLUDED.dedup_key,
		    message_hash=EXCLUDED.message_hash,
		    message_payload=EXCLUDED.message_payload,
		    delivery_status='reserved',
		    response_code=NULL,
		    delivered_at=NULL,
		    message_id=NULL,
		    urgent_status=NULL,
		    urgent_response_code=NULL,
		    attempt_count=notification_messages.attempt_count+1,
		    next_attempt_at=NULL,
		    updated_at=NOW()
		WHERE notification_messages.delivery_status='failed'
		  AND notification_messages.attempt_count<5
		RETURNING id`,
		reservation.NotificationKey, reservation.Family, reservation.PolicyVersion,
		reservation.SourceKind, reservation.DedupKey, reservation.MessageHash,
		string(reservation.Payload)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if scanErr := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM relay_ops.notification_messages
			 WHERE notification_key=$1 OR dedup_key=$2
			)`, reservation.NotificationKey, reservation.DedupKey).Scan(&exists); scanErr != nil {
			return 0, false, fmt.Errorf("check one-shot reservation: %w", scanErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, false, fmt.Errorf("commit duplicate one-shot reservation: %w", err)
		}
		if exists {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("one-shot reservation was not created")
	}
	if err != nil {
		return 0, false, fmt.Errorf("reserve one-shot notification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit one-shot reservation: %w", err)
	}
	return id, true, nil
}

func (s *Store) FinishOneShot(
	ctx context.Context,
	deliveryID int64,
	outcome notify.DeliveryOutcome,
) error {
	if deliveryID <= 0 || (outcome.Status != "delivered" && outcome.Status != "failed") {
		return fmt.Errorf("one-shot delivery status is invalid")
	}
	var payload any
	if len(outcome.Payload) > 0 {
		if !json.Valid(outcome.Payload) {
			return fmt.Errorf("one-shot delivery payload is invalid")
		}
		payload = string(outcome.Payload)
	}
	var responseCode any
	if outcome.ResponseCode > 0 {
		responseCode = outcome.ResponseCode
	}
	var urgentResponseCode any
	if outcome.UrgentResponseCode > 0 {
		urgentResponseCode = outcome.UrgentResponseCode
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin one-shot finish: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		UPDATE relay_ops.notification_messages
		SET delivery_status=$2,
		    response_code=$3,
		    delivered_at=CASE WHEN $2='delivered' THEN NOW() ELSE NULL END,
		    message_payload=COALESCE($4::jsonb, message_payload),
		    message_id=NULLIF($5, ''),
		    urgent_status=NULLIF($6, ''),
		    urgent_response_code=$7,
		    next_attempt_at=CASE
		      WHEN $2='failed' AND attempt_count<5 THEN
		        NOW() + CASE attempt_count
		          WHEN 1 THEN INTERVAL '1 minute'
		          WHEN 2 THEN INTERVAL '2 minutes'
		          WHEN 3 THEN INTERVAL '5 minutes'
		          ELSE INTERVAL '10 minutes'
		        END
		      ELSE NULL
		    END,
		    updated_at=NOW()
		WHERE id=$1 AND delivery_status='reserved'`,
		deliveryID, outcome.Status, responseCode, payload, outcome.MessageID,
		outcome.UrgentStatus, urgentResponseCode)
	if err != nil {
		return fmt.Errorf("finish one-shot notification: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("one-shot notification not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit one-shot finish: %w", err)
	}
	return nil
}

func (s *Store) RecordNotificationDecision(ctx context.Context, record DecisionRecord) error {
	record.DecisionKey = strings.TrimSpace(record.DecisionKey)
	record.Family = strings.TrimSpace(record.Family)
	record.SourceKind = strings.TrimSpace(record.SourceKind)
	record.Decision = strings.TrimSpace(record.Decision)
	record.Reason = strings.TrimSpace(record.Reason)
	if !validBoundedText(record.DecisionKey, 512) ||
		!approvedNotificationFamily(record.Family) ||
		record.PolicyVersion <= 0 ||
		!validBoundedText(record.SourceKind, 128) ||
		!validBoundedText(record.Decision, 128) ||
		!validBoundedText(record.Reason, 256) ||
		record.ObservedAt.IsZero() ||
		len(record.Details) == 0 ||
		len(record.Details) > 64<<10 ||
		!json.Valid(record.Details) {
		return fmt.Errorf("notification decision is invalid")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.notification_decisions
			(decision_key, family, policy_version, source_kind, decision, reason,
			 details, observed_at, first_seen_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $8, $8)
		ON CONFLICT (decision_key) DO UPDATE
		SET family=EXCLUDED.family,
		    policy_version=EXCLUDED.policy_version,
		    source_kind=EXCLUDED.source_kind,
		    decision=EXCLUDED.decision,
		    reason=EXCLUDED.reason,
		    details=EXCLUDED.details,
		    observed_at=EXCLUDED.observed_at,
		    last_seen_at=EXCLUDED.observed_at,
		    observation_count=notification_decisions.observation_count+1,
		    updated_at=NOW()`,
		record.DecisionKey, record.Family, record.PolicyVersion, record.SourceKind,
		record.Decision, record.Reason, string(record.Details), record.ObservedAt.UTC())
	if err != nil {
		return fmt.Errorf("record notification decision: %w", err)
	}
	return nil
}

func (s *Store) GetOperationalBaseline(
	ctx context.Context,
	key string,
) (Baseline, bool, error) {
	key = strings.TrimSpace(key)
	if !validBoundedText(key, 512) {
		return Baseline{}, false, fmt.Errorf("operational baseline key is invalid")
	}
	var baseline Baseline
	err := s.pool.QueryRow(ctx, `
		SELECT baseline_key, current_value, evidence_hash, updated_at
		FROM relay_ops.operational_baselines
		WHERE baseline_key=$1`, key).Scan(
		&baseline.Key, &baseline.CurrentValue, &baseline.EvidenceHash, &baseline.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Baseline{}, false, nil
	}
	if err != nil {
		return Baseline{}, false, fmt.Errorf("get operational baseline: %w", err)
	}
	return baseline, true, nil
}

func (s *Store) PutOperationalBaseline(ctx context.Context, baseline Baseline) error {
	baseline.Key = strings.TrimSpace(baseline.Key)
	baseline.CurrentValue = strings.TrimSpace(baseline.CurrentValue)
	baseline.EvidenceHash = strings.TrimSpace(baseline.EvidenceHash)
	if !validBoundedText(baseline.Key, 512) ||
		!validBoundedText(baseline.CurrentValue, 4096) ||
		!validBoundedText(baseline.EvidenceHash, 256) ||
		baseline.UpdatedAt.IsZero() {
		return fmt.Errorf("operational baseline is invalid")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.operational_baselines
			(baseline_key, current_value, evidence_hash, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (baseline_key) DO UPDATE
		SET current_value=EXCLUDED.current_value,
		    evidence_hash=EXCLUDED.evidence_hash,
		    updated_at=EXCLUDED.updated_at`,
		baseline.Key, baseline.CurrentValue, baseline.EvidenceHash, baseline.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("put operational baseline: %w", err)
	}
	return nil
}

func (s *Store) SupersedeLegacyNotificationIncidents(
	ctx context.Context,
	now time.Time,
) (int64, error) {
	if now.IsZero() {
		return 0, fmt.Errorf("legacy supersession time is required")
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET state='superseded',
		    last_seen_at=$1,
		    next_escalation_at=NULL,
		    escalation_claim_token=NULL,
		    escalation_claimed_at=NULL
		WHERE state NOT IN ('recovered', 'superseded')
		  AND family='legacy'
		  AND (
		    incident_key LIKE 'daily-report:%'
		    OR incident_key LIKE 'native-monitor:%'
		    OR incident_key LIKE 'site:account:%:paused'
		    OR incident_key LIKE 'site:account:%:balance_exhausted'
		    OR incident_key LIKE 'site:group:%:availability'
		    OR incident_key LIKE 'site:group:%:error_rate'
		    OR incident_key LIKE 'site:group:%:ttft_p95'
		    OR incident_key LIKE 'upstream:%:pricing'
		    OR incident_key LIKE 'candidate:%'
		    OR incident_key LIKE 'quality:%'
		    OR incident_key LIKE 'synthetic:%'
		    OR incident_key LIKE 'usage_session:%'
		  )`, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("supersede legacy notification incidents: %w", err)
	}
	return command.RowsAffected(), nil
}

func validateOneShotReservation(reservation notify.OneShotReservation) error {
	if !validBoundedText(strings.TrimSpace(reservation.NotificationKey), 512) ||
		!approvedNotificationFamily(strings.TrimSpace(reservation.Family)) ||
		reservation.PolicyVersion <= 0 ||
		!validBoundedText(strings.TrimSpace(reservation.SourceKind), 128) ||
		!validBoundedText(strings.TrimSpace(reservation.DedupKey), 256) ||
		!validBoundedText(strings.TrimSpace(reservation.MessageHash), 256) ||
		len(reservation.Payload) == 0 ||
		len(reservation.Payload) > 30<<10 ||
		!json.Valid(reservation.Payload) {
		return fmt.Errorf("one-shot notification identity is incomplete")
	}
	return nil
}

func approvedNotificationFamily(value string) bool {
	for _, family := range notificationpolicy.ApprovedFamilies() {
		if value == string(family) {
			return true
		}
	}
	return false
}

func validBoundedText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && !strings.ContainsRune(value, '\x00')
}
