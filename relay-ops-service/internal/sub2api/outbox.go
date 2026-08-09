package sub2api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/events"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CoreOutbox is the only cross-database boundary. Its SQL allowlist contains
// externalization_outbox claim/ack columns only; it cannot query core business
// tables or write projection data.
type CoreOutbox struct {
	pool *pgxpool.Pool
}

type OutboxClaim struct {
	Event events.Event
	Token string
}

func NewCoreOutbox(ctx context.Context, databaseURL string) (*CoreOutbox, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("core database URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse core database URL: %w", err)
	}
	config.MaxConns = 4
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open core outbox connection: %w", err)
	}
	return &CoreOutbox{pool: pool}, nil
}

func (o *CoreOutbox) Close() {
	if o != nil && o.pool != nil {
		o.pool.Close()
	}
}

func (o *CoreOutbox) ClaimBatch(ctx context.Context, consumer string, limit int) ([]OutboxClaim, error) {
	if o == nil || o.pool == nil {
		return nil, errors.New("core outbox is not initialized")
	}
	consumer = strings.TrimSpace(consumer)
	if consumer == "" || limit <= 0 {
		return nil, errors.New("consumer and positive limit are required")
	}
	token, err := claimToken()
	if err != nil {
		return nil, err
	}
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin core outbox claim: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT event_id FROM externalization_outbox
			WHERE (status IN ('pending', 'retry') AND available_at <= NOW())
			   OR (status='processing' AND claimed_at < NOW() - INTERVAL '60 seconds')
			ORDER BY occurred_at ASC, event_id ASC FOR UPDATE SKIP LOCKED LIMIT $3
		)
		UPDATE externalization_outbox AS e
		SET status='processing', claimed_by=$1, claim_token=$2, claimed_at=NOW(), attempts=e.attempts+1
		FROM candidates WHERE e.event_id=candidates.event_id
		RETURNING e.event_id, e.event_type, e.occurred_at, e.source_version,
		          e.contract_version, e.payload, e.claim_token`, consumer, token, limit)
	if err != nil {
		return nil, fmt.Errorf("claim core outbox events: %w", err)
	}
	defer rows.Close()
	result := make([]OutboxClaim, 0, limit)
	for rows.Next() {
		var claim OutboxClaim
		if err := rows.Scan(&claim.Event.EventID, &claim.Event.Type, &claim.Event.OccurredAt, &claim.Event.SourceVersion, &claim.Event.ContractVersion, &claim.Event.Payload, &claim.Token); err != nil {
			return nil, fmt.Errorf("scan core outbox event: %w", err)
		}
		result = append(result, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read core outbox events: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit core outbox claim: %w", err)
	}
	return result, nil
}

func (o *CoreOutbox) MarkPublished(ctx context.Context, claim OutboxClaim, consumer string) error {
	if o == nil || o.pool == nil {
		return errors.New("core outbox is not initialized")
	}
	result, err := o.pool.Exec(ctx, `UPDATE externalization_outbox SET status='published', published_at=NOW(), claimed_at=NULL, claimed_by=NULL, claim_token=NULL WHERE event_id=$1 AND claimed_by=$2 AND claim_token=$3`, claim.Event.EventID, consumer, claim.Token)
	if err != nil {
		return fmt.Errorf("mark core outbox published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("core outbox event %q is not owned by %q", claim.Event.EventID, consumer)
	}
	return nil
}

func (o *CoreOutbox) MarkFailed(ctx context.Context, claim OutboxClaim, consumer string, cause error) error {
	if o == nil || o.pool == nil {
		return errors.New("core outbox is not initialized")
	}
	message := "unknown relay consumer failure"
	if cause != nil {
		message = cause.Error()
	}
	result, err := o.pool.Exec(ctx, `UPDATE externalization_outbox SET status='retry', available_at=NOW()+LEAST((2 ^ LEAST(attempts, 8))*INTERVAL '1 second', INTERVAL '15 minutes'), last_error_class='relay', last_error=NULLIF($4,''), claimed_at=NULL, claimed_by=NULL, claim_token=NULL WHERE event_id=$1 AND claimed_by=$2 AND claim_token=$3`, claim.Event.EventID, consumer, claim.Token, message)
	if err != nil {
		return fmt.Errorf("mark core outbox retry: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("core outbox event %q is not owned by %q", claim.Event.EventID, consumer)
	}
	return nil
}

func (o *CoreOutbox) PumpOnce(ctx context.Context, consumerName string, consumer *events.Consumer) error {
	if consumer == nil {
		return errors.New("persistent consumer is required")
	}
	claimed, err := o.ClaimBatch(ctx, consumerName, 100)
	if err != nil {
		return err
	}
	for _, claim := range claimed {
		if err := consumer.Handle(ctx, claim.Event); err != nil {
			if markErr := o.MarkFailed(ctx, claim, consumerName, err); markErr != nil {
				return errors.Join(err, markErr)
			}
			continue
		}
		if err := o.MarkPublished(ctx, claim, consumerName); err != nil {
			return err
		}
	}
	return nil
}

func claimToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate core outbox claim token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func (o *CoreOutbox) Run(ctx context.Context, consumerName string, consumer *events.Consumer) error {
	if o == nil {
		return errors.New("core outbox is not initialized")
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := o.PumpOnce(ctx, consumerName, consumer); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
