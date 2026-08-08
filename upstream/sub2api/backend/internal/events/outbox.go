package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/integration"
)

const defaultLease = 2 * time.Minute

type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Outbox writes integration events in the caller's transaction and claims
// pending rows with PostgreSQL row locks. It never writes core business tables.
type Outbox struct {
	db    *sql.DB
	lease time.Duration
}

func NewOutbox(db *sql.DB) *Outbox {
	return &Outbox{db: db, lease: defaultLease}
}

func NewOutboxWithLease(db *sql.DB, lease time.Duration) (*Outbox, error) {
	if db == nil {
		return nil, errors.New("outbox database is required")
	}
	if lease <= 0 {
		return nil, errors.New("outbox lease must be positive")
	}
	return &Outbox{db: db, lease: lease}, nil
}

func (o *Outbox) Append(ctx context.Context, tx DBTX, event integration.Event) error {
	if tx == nil {
		return errors.New("outbox transaction is required")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate integration event: %w", err)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO externalization_outbox
			(event_id, event_type, occurred_at, source_version, contract_version, payload)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		ON CONFLICT (event_id) DO NOTHING
	`, event.EventID, event.Type, event.OccurredAt.UTC(), event.SourceVersion, event.ContractVersion, json.RawMessage(event.Payload))
	if err != nil {
		return fmt.Errorf("append integration event: %w", err)
	}
	return nil
}

func (o *Outbox) ClaimBatch(ctx context.Context, consumer string, limit int) ([]integration.Event, error) {
	if o == nil || o.db == nil {
		return nil, errors.New("outbox database is required")
	}
	consumer = strings.TrimSpace(consumer)
	if consumer == "" {
		return nil, errors.New("consumer is required")
	}
	if limit <= 0 {
		return []integration.Event{}, nil
	}
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id
			FROM externalization_outbox
			WHERE status IN ('pending', 'retry')
			  AND available_at <= NOW()
			  AND (claimed_at IS NULL OR claimed_at < NOW() - $3::interval)
			ORDER BY occurred_at ASC, event_id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE externalization_outbox AS e
		SET status = 'processing', claimed_by = $1, claimed_at = NOW(), attempts = e.attempts + 1
		FROM candidates
		WHERE e.event_id = candidates.event_id
		RETURNING e.event_id, e.event_type, e.occurred_at, e.source_version,
		          e.contract_version, e.payload
	`, consumer, limit, leaseInterval(o.lease))
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()
	claimed := make([]integration.Event, 0, limit)
	for rows.Next() {
		var event integration.Event
		if err := rows.Scan(&event.EventID, &event.Type, &event.OccurredAt, &event.SourceVersion, &event.ContractVersion, &event.Payload); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("scan claimed event: %w", err)
		}
		claimed = append(claimed, event)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("read claimed events: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return claimed, nil
}

func leaseInterval(d time.Duration) string {
	// PostgreSQL interval input is text; use microseconds to avoid locale-
	// dependent formatting and preserve sub-second test leases.
	return fmt.Sprintf("%d microseconds", d.Microseconds())
}

func (o *Outbox) MarkPublished(ctx context.Context, eventID, consumer string) error {
	if o == nil || o.db == nil {
		return errors.New("outbox database is required")
	}
	result, err := o.db.ExecContext(ctx, `
		UPDATE externalization_outbox
		SET status = 'published', published_at = NOW(), claimed_at = NULL, claimed_by = NULL
		WHERE event_id = $1 AND claimed_by = $2
	`, eventID, consumer)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("outbox event %q was not claimed by %q", eventID, consumer)
	}
	return nil
}

func (o *Outbox) MarkFailed(ctx context.Context, eventID, consumer string, class ErrorClass, cause error) error {
	if o == nil || o.db == nil {
		return errors.New("outbox database is required")
	}
	if class == "" {
		class = ClassifyError(cause)
		if class == "" {
			class = ErrorClassUnknown
		}
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := o.db.ExecContext(ctx, `
		UPDATE externalization_outbox
		SET status = CASE WHEN $3 IN ('permanent', 'contract') THEN 'dead' ELSE 'retry' END,
		    available_at = NOW() + CASE WHEN $3 IN ('permanent', 'contract') THEN INTERVAL '0' ELSE LEAST((2 ^ LEAST(attempts, 8)) * INTERVAL '1 second', INTERVAL '15 minutes') END,
		    last_error_class = $3, last_error = NULLIF($4, ''), claimed_at = NULL, claimed_by = NULL
		WHERE event_id = $1 AND claimed_by = $2
	`, eventID, consumer, class, message)
	if err != nil {
		return fmt.Errorf("mark outbox event failed: %w", err)
	}
	return nil
}
