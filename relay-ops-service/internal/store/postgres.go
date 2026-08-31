package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/events"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/projection"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var initialMigration string

//go:embed migrations/008_accounting_ledger.sql
var accountingLedgerMigration string

//go:embed migrations/009_upstream_cost_reconciliation.sql
var upstreamCostReconciliationMigration string

//go:embed migrations/010_billing_account_mapping.sql
var billingAccountMappingMigration string

//go:embed migrations/011_reconciliation_group_scope.sql
var reconciliationGroupScopeMigration string

//go:embed migrations/012_cost_guard.sql
var costGuardMigration string

//go:embed migrations/013_externalization_read_models.sql
var externalizationReadModelsMigration string

//go:embed migrations/014_externalization_commands.sql
var externalizationCommandsMigration string

var ErrConflict = errors.New("record conflicts with existing identity")

var externalizationCommandNames = map[string]struct{}{
	"refresh_account": {},
	"account_update":  {},
}

func init() {
	initialMigration += "\n" + accountingLedgerMigration
	initialMigration += "\n" + upstreamCostReconciliationMigration
	initialMigration += "\n" + reconciliationGroupScopeMigration
	initialMigration += "\n" + costGuardMigration
	initialMigration += "\n" + externalizationReadModelsMigration
	initialMigration += "\n" + externalizationCommandsMigration
}

type Store struct {
	pool *pgxpool.Pool
}

func (s *Store) ClaimExternalizationCommand(ctx context.Context, actorID, accountID int64, idempotencyKey, command string, contractVersion int) (bool, string, error) {
	if s == nil || s.pool == nil {
		return false, "", errors.New("store is not initialized")
	}
	if actorID <= 0 || accountID <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return false, "", errors.New("invalid externalization command")
	}
	if _, allowed := externalizationCommandNames[command]; !allowed {
		return false, "", errors.New("invalid externalization command")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s", actorID, accountID, command, idempotencyKey)))
	payload, _ := json.Marshal(map[string]any{"command": command, "account_id": accountID})
	commandID := hex.EncodeToString(sum[:])
	result, err := s.pool.Exec(ctx, `INSERT INTO relay_ops.externalization_commands (command_id, actor_id, account_id, idempotency_key, payload, status, result, contract_version, command_name) VALUES ($1,$2,$3,$4,$5::jsonb,'processing','pending',$6,$7) ON CONFLICT (idempotency_key) DO NOTHING`, commandID, actorID, accountID, idempotencyKey, payload, contractVersion, command)
	if err != nil {
		return false, "", fmt.Errorf("claim externalization command: %w", err)
	}
	if result.RowsAffected() == 1 {
		return true, "pending", nil
	}
	var existingActor, existingAccount int64
	var existingCommand, status, storedResult string
	err = s.pool.QueryRow(ctx, `SELECT actor_id, account_id, command_name, status, result FROM relay_ops.externalization_commands WHERE idempotency_key=$1`, idempotencyKey).Scan(&existingActor, &existingAccount, &existingCommand, &status, &storedResult)
	if err != nil {
		return false, "", fmt.Errorf("load externalization command: %w", err)
	}
	if existingActor != actorID || existingAccount != accountID || existingCommand != command {
		return false, "", ErrConflict
	}
	return false, storedResult, nil
}

func (s *Store) ClaimAccountUpdateCommand(ctx context.Context, command billing.AccountUpdateCommand, payloadHash string, contractVersion int) (bool, string, error) {
	if s == nil || s.pool == nil {
		return false, "", errors.New("store is not initialized")
	}
	if err := command.Validate(); err != nil {
		return false, "", err
	}
	if payloadHash == "" || contractVersion != 1 {
		return false, "", errors.New("invalid account update command identity")
	}
	payload, err := json.Marshal(map[string]any{"command": "account_update", "fields": command.Fields, "payload_hash": payloadHash})
	if err != nil {
		return false, "", fmt.Errorf("encode account update payload: %w", err)
	}
	result, err := s.pool.Exec(ctx, `INSERT INTO relay_ops.externalization_commands (command_id, actor_id, account_id, idempotency_key, payload, status, result, contract_version, command_name) VALUES ($1,$2,$3,$4,$5::jsonb,'processing','pending',$6,'account_update') ON CONFLICT (idempotency_key) DO NOTHING`, command.CommandID, command.ActorID, command.AccountID, command.IdempotencyKey, payload, contractVersion)
	if err != nil {
		return false, "", fmt.Errorf("claim account update command: %w", err)
	}
	if result.RowsAffected() == 1 {
		return true, "pending", nil
	}
	var existingID string
	var actorID, accountID int64
	var existingPayloadHash string
	var commandName string
	var version int
	var storedResult string
	err = s.pool.QueryRow(ctx, `SELECT command_id, actor_id, account_id, payload->>'payload_hash', command_name, contract_version, result FROM relay_ops.externalization_commands WHERE idempotency_key=$1`, command.IdempotencyKey).Scan(&existingID, &actorID, &accountID, &existingPayloadHash, &commandName, &version, &storedResult)
	if err != nil {
		return false, "", fmt.Errorf("load account update command: %w", err)
	}
	if existingID != command.CommandID || actorID != command.ActorID || accountID != command.AccountID || commandName != "account_update" || version != contractVersion || existingPayloadHash != payloadHash {
		return false, "", ErrConflict
	}
	return false, storedResult, nil
}

func (s *Store) CompleteAccountUpdateCommand(ctx context.Context, command billing.AccountUpdateCommand, result string, contractVersion int) error {
	if result != "accepted" && result != "failed" {
		return errors.New("invalid account update command result")
	}
	completed, err := s.pool.Exec(ctx, `UPDATE relay_ops.externalization_commands SET status='completed', result=$2, completed_at=NOW() WHERE command_id=$1 AND actor_id=$3 AND account_id=$4 AND idempotency_key=$5 AND command_name='account_update' AND contract_version=$6`, command.CommandID, result, command.ActorID, command.AccountID, command.IdempotencyKey, contractVersion)
	if err != nil {
		return fmt.Errorf("complete account update command: %w", err)
	}
	if completed.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) CompleteExternalizationCommand(ctx context.Context, actorID, accountID int64, idempotencyKey, result string, contractVersion int) error {
	if result != "accepted" && result != "failed" {
		return errors.New("invalid externalization command result")
	}
	command, err := s.pool.Exec(ctx, `UPDATE relay_ops.externalization_commands SET status='completed', result=$4, contract_version=$5, completed_at=NOW() WHERE actor_id=$1 AND account_id=$2 AND idempotency_key=$3`, actorID, accountID, idempotencyKey, result, contractVersion)
	if err != nil {
		return fmt.Errorf("complete externalization command: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) RecordExternalizationCommand(ctx context.Context, actorID, accountID int64, idempotencyKey, result string, contractVersion int) error {
	if s == nil || s.pool == nil {
		return errors.New("store is not initialized")
	}
	if actorID <= 0 || accountID <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return errors.New("invalid externalization command")
	}
	if strings.TrimSpace(result) == "" {
		result = "accepted"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", actorID, accountID, idempotencyKey)))
	commandID := hex.EncodeToString(sum[:])
	payload, _ := json.Marshal(map[string]any{"account_id": accountID})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.externalization_commands
			(command_id, actor_id, account_id, idempotency_key, payload, status, result, contract_version, completed_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$6,$7,NOW())
		ON CONFLICT (idempotency_key) DO UPDATE SET result=EXCLUDED.result, status=EXCLUDED.status,
			contract_version=EXCLUDED.contract_version, completed_at=EXCLUDED.completed_at`,
		commandID, actorID, accountID, idempotencyKey, payload, result, contractVersion)
	if err != nil {
		return fmt.Errorf("record externalization command: %w", err)
	}
	return nil
}

// AppendBalanceSnapshot records an immutable provider balance fact in the
// control-plane schema. A byte-for-byte replay is accepted; a changed fact at
// the same observation time is rejected rather than overwriting evidence.
func (s *Store) AppendBalanceSnapshot(ctx context.Context, snapshot billing.BalanceSnapshot) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("store is not initialized")
	}
	if err := snapshot.Validate(); err != nil {
		return false, err
	}
	result, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.balance_snapshots (account_id, observed_at, amount, currency, fresh_until, source)
		VALUES ($1, $2, $3::numeric, $4, $5, $6)
		ON CONFLICT (account_id, observed_at) DO NOTHING`,
		snapshot.AccountID, snapshot.ObservedAt.UTC(), snapshot.Amount, snapshot.Currency,
		snapshot.FreshUntil.UTC(), snapshot.Source)
	if err != nil {
		return false, fmt.Errorf("append balance snapshot: %w", err)
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var same bool
	err = s.pool.QueryRow(ctx, `
		SELECT amount = $3::numeric AND currency = $4 AND fresh_until = $5 AND source = $6
		FROM relay_ops.balance_snapshots WHERE account_id = $1 AND observed_at = $2`,
		snapshot.AccountID, snapshot.ObservedAt.UTC(), snapshot.Amount, snapshot.Currency,
		snapshot.FreshUntil.UTC(), snapshot.Source).Scan(&same)
	if err != nil {
		return false, fmt.Errorf("load replayed balance snapshot: %w", err)
	}
	if !same {
		return false, ErrConflict
	}
	return false, nil
}

// LatestFreshBalanceSnapshot returns the newest balance fact that has not
// expired at now. Expired facts remain retained for audit and reconciliation.
func (s *Store) LatestFreshBalanceSnapshot(ctx context.Context, accountID int64, now time.Time) (billing.BalanceSnapshot, bool, error) {
	if s == nil || s.pool == nil {
		return billing.BalanceSnapshot{}, false, errors.New("store is not initialized")
	}
	if accountID <= 0 {
		return billing.BalanceSnapshot{}, false, errors.New("account ID must be positive")
	}
	var snapshot billing.BalanceSnapshot
	err := s.pool.QueryRow(ctx, `
		SELECT account_id, amount::text, currency, observed_at, fresh_until, source
		FROM relay_ops.balance_snapshots
		WHERE account_id = $1 AND fresh_until > $2
		ORDER BY observed_at DESC LIMIT 1`, accountID, now.UTC()).Scan(
		&snapshot.AccountID, &snapshot.Amount, &snapshot.Currency, &snapshot.ObservedAt, &snapshot.FreshUntil, &snapshot.Source)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.BalanceSnapshot{}, false, nil
	}
	if err != nil {
		return billing.BalanceSnapshot{}, false, fmt.Errorf("load fresh balance snapshot: %w", err)
	}
	snapshot.ObservedAt = snapshot.ObservedAt.UTC()
	snapshot.FreshUntil = snapshot.FreshUntil.UTC()
	return snapshot, true, nil
}

var _ events.Journal = (*Store)(nil)

type Upstream struct {
	ID          domain.UpstreamID
	Name        string
	Role        string
	BaseURL     string
	AdapterType string
	Enabled     bool
}

type PricingSnapshot struct {
	UpstreamID     domain.UpstreamID
	SourceURL      string
	SourceType     string
	FetchedAt      time.Time
	ContentHash    string
	NormalizedJSON []byte
	DiffSummary    []byte
	EvidenceLevel  string
}

func Open(ctx context.Context, databaseURLFile string) (*Store, error) {
	data, err := os.ReadFile(databaseURLFile)
	if err != nil {
		return nil, fmt.Errorf("read database URL: %w", err)
	}
	databaseURL := strings.TrimSpace(string(data))
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 5
	config.MinConns = 0
	config.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open relay ops database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping relay ops database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, initialMigration); err != nil {
		return fmt.Errorf("migrate relay ops schema: %w", err)
	}
	if err := s.preflightBillingAccountMapping(ctx); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, billingAccountMappingMigration); err != nil {
		return fmt.Errorf("migrate relay ops billing account mapping schema: %w", err)
	}
	return nil
}

const externalizationEventLease = 2 * time.Minute

const externalizationProjectionLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('relay-ops:externalization-projections', 0))`

func (s *Store) ClaimEvent(ctx context.Context, event events.Event) (events.Claim, error) {
	claimToken, err := newExternalizationClaimToken()
	if err != nil {
		return events.Claim{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return events.Claim{}, fmt.Errorf("begin externalization event claim: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, externalizationProjectionLockSQL); err != nil {
		return events.Claim{}, fmt.Errorf("lock externalization event claim: %w", err)
	}
	leaseUntil := time.Now().UTC().Add(externalizationEventLease)
	command, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.externalization_events
			(event_id, source_version, event_type, contract_version, occurred_at, payload,
			 status, attempts, claim_token, claim_generation, lease_until)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, 'processing', 1, $7, 1, $8)
		ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, event.SourceVersion, event.Type, event.ContractVersion, event.OccurredAt.UTC(), event.Payload, claimToken, leaseUntil)
	if err != nil {
		return events.Claim{}, fmt.Errorf("insert externalization event claim: %w", err)
	}
	if command.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `
			UPDATE relay_ops.externalization_watermarks SET completeness=$2 WHERE source=$1`,
			event.SourceVersion, events.CompletenessPartial); err != nil {
			return events.Claim{}, fmt.Errorf("mark externalization watermark partial: %w", err)
		}
		if err := setProjectionRowsCompleteness(ctx, tx, events.CompletenessPartial); err != nil {
			return events.Claim{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return events.Claim{}, fmt.Errorf("commit externalization event claim: %w", err)
		}
		return events.Claim{Acquired: true, Token: claimToken, Generation: 1}, nil
	}

	var envelopeMatches bool
	var status string
	var existingLease *time.Time
	var generation int64
	if err := tx.QueryRow(ctx, `
		SELECT source_version=$2 AND event_type=$3 AND contract_version=$4
			AND occurred_at=$5 AND payload=$6::jsonb,
			status, lease_until, claim_generation
		FROM relay_ops.externalization_events
		WHERE event_id=$1
		FOR UPDATE`, event.EventID, event.SourceVersion, event.Type, event.ContractVersion, event.OccurredAt.UTC(), event.Payload,
	).Scan(&envelopeMatches, &status, &existingLease, &generation); err != nil {
		return events.Claim{}, fmt.Errorf("read existing externalization event: %w", err)
	}
	if !envelopeMatches {
		return events.Claim{}, fmt.Errorf("externalization event %s conflicts with persisted envelope", event.EventID)
	}
	if status != "processing" || existingLease == nil || existingLease.After(time.Now().UTC()) {
		if err := tx.Commit(ctx); err != nil {
			return events.Claim{}, fmt.Errorf("commit duplicate externalization event: %w", err)
		}
		return events.Claim{}, nil
	}
	generation++
	command, err = tx.Exec(ctx, `
		UPDATE relay_ops.externalization_events
		SET attempts=attempts+1, claim_token=$2, claim_generation=$3, lease_until=$4, updated_at=NOW()
		WHERE event_id=$1 AND status='processing' AND lease_until <= NOW()`, event.EventID, claimToken, generation, leaseUntil)
	if err != nil {
		return events.Claim{}, fmt.Errorf("resume externalization event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.externalization_watermarks SET completeness=$2 WHERE source=$1`,
		event.SourceVersion, events.CompletenessPartial); err != nil {
		return events.Claim{}, fmt.Errorf("mark resumed externalization watermark partial: %w", err)
	}
	if err := setProjectionRowsCompleteness(ctx, tx, events.CompletenessPartial); err != nil {
		return events.Claim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return events.Claim{}, fmt.Errorf("commit resumed externalization event: %w", err)
	}
	return events.Claim{Acquired: command.RowsAffected() == 1, Token: claimToken, Generation: generation}, nil
}

func newExternalizationClaimToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate externalization claim token: %w", err)
	}
	return hex.EncodeToString(value), nil
}

type projectionTransactionContextKey struct{}

func (s *Store) ApplyEvent(ctx context.Context, event events.Event, claim events.Claim, processedAt time.Time, apply func(context.Context) error) (events.Watermark, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return events.Watermark{}, fmt.Errorf("begin externalization event completion: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, externalizationProjectionLockSQL); err != nil {
		return events.Watermark{}, fmt.Errorf("lock externalization projections: %w", err)
	}
	var status, token string
	var generation int64
	var leaseUntil *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status, COALESCE(claim_token, ''), claim_generation, lease_until
		FROM relay_ops.externalization_events WHERE event_id=$1 FOR UPDATE`, event.EventID,
	).Scan(&status, &token, &generation, &leaseUntil); err != nil {
		return events.Watermark{}, fmt.Errorf("lock externalization event: %w", err)
	}
	if status != "processing" || token != claim.Token || generation != claim.Generation || leaseUntil == nil || leaseUntil.Before(time.Now().UTC()) {
		return events.Watermark{}, fmt.Errorf("%w: event %s generation %d", events.ErrStaleClaim, event.EventID, claim.Generation)
	}
	if apply == nil {
		return events.Watermark{}, errors.New("externalization projection callback is required")
	}
	applyCtx := context.WithValue(ctx, projectionTransactionContextKey{}, tx)
	if err := apply(applyCtx); err != nil {
		return events.Watermark{}, err
	}
	command, err := tx.Exec(ctx, `
		UPDATE relay_ops.externalization_events
		SET status='processed', processed_at=$4, lease_until=NULL, last_error=NULL, updated_at=NOW()
		WHERE event_id=$1 AND status='processing' AND claim_token=$2 AND claim_generation=$3`,
		event.EventID, claim.Token, claim.Generation, processedAt.UTC())
	if err != nil {
		return events.Watermark{}, fmt.Errorf("complete externalization event: %w", err)
	}
	if command.RowsAffected() != 1 {
		return events.Watermark{}, fmt.Errorf("externalization event %s is not claimed", event.EventID)
	}
	var sourceHasGap, projectionHasGap bool
	if err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM relay_ops.externalization_events
				WHERE source_version=$1 AND status <> 'processed'
			),
			EXISTS (
				SELECT 1 FROM relay_ops.externalization_events
				WHERE status <> 'processed'
			)`, event.SourceVersion).Scan(&sourceHasGap, &projectionHasGap); err != nil {
		return events.Watermark{}, fmt.Errorf("read externalization completeness: %w", err)
	}
	completeness := events.CompletenessComplete
	if sourceHasGap {
		completeness = events.CompletenessPartial
	}
	projectionCompleteness := events.CompletenessComplete
	if projectionHasGap {
		projectionCompleteness = events.CompletenessPartial
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.externalization_watermarks
			(source, last_event_id, occurred_at, processed_at, completeness, calculation_version)
		VALUES ($1, $2, $3, $4, $5, 'event-consumer-v1')
		ON CONFLICT (source) DO UPDATE
		SET last_event_id = CASE WHEN (externalization_watermarks.occurred_at, externalization_watermarks.last_event_id)
				< (EXCLUDED.occurred_at, EXCLUDED.last_event_id) THEN EXCLUDED.last_event_id ELSE externalization_watermarks.last_event_id END,
			occurred_at = GREATEST(externalization_watermarks.occurred_at, EXCLUDED.occurred_at),
			processed_at = GREATEST(externalization_watermarks.processed_at, EXCLUDED.processed_at),
			completeness = EXCLUDED.completeness`,
		event.SourceVersion, event.EventID, event.OccurredAt.UTC(), processedAt.UTC(), completeness); err != nil {
		return events.Watermark{}, fmt.Errorf("save externalization watermark: %w", err)
	}
	if err := setProjectionRowsCompleteness(ctx, tx, projectionCompleteness); err != nil {
		return events.Watermark{}, err
	}
	var watermark events.Watermark
	if err := tx.QueryRow(ctx, `
		SELECT source, last_event_id, occurred_at, processed_at, completeness
		FROM relay_ops.externalization_watermarks WHERE source=$1`, event.SourceVersion,
	).Scan(&watermark.Source, &watermark.LastEventID, &watermark.OccurredAt, &watermark.ProcessedAt, &watermark.Completeness); err != nil {
		return events.Watermark{}, fmt.Errorf("read completed externalization watermark: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return events.Watermark{}, fmt.Errorf("commit externalization event completion: %w", err)
	}
	return watermark, nil
}

func (s *Store) FailEvent(ctx context.Context, event events.Event, claim events.Claim, failedAt time.Time, cause error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin externalization dead letter: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, externalizationProjectionLockSQL); err != nil {
		return fmt.Errorf("lock externalization dead letter: %w", err)
	}
	message := "unknown projection failure"
	if cause != nil {
		message = cause.Error()
	}
	command, err := tx.Exec(ctx, `
		UPDATE relay_ops.externalization_events
		SET status='dead', processed_at=$4, lease_until=NULL, last_error=$5, updated_at=NOW()
		WHERE event_id=$1 AND status='processing' AND claim_token=$2 AND claim_generation=$3`,
		event.EventID, claim.Token, claim.Generation, failedAt.UTC(), message)
	if err != nil {
		return fmt.Errorf("mark externalization event dead: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: event %s generation %d", events.ErrStaleClaim, event.EventID, claim.Generation)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.externalization_dead_letters
			(event_id, source_version, event_type, contract_version, occurred_at, payload, error, failed_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		ON CONFLICT (event_id) DO UPDATE SET error=EXCLUDED.error, failed_at=EXCLUDED.failed_at`,
		event.EventID, event.SourceVersion, event.Type, event.ContractVersion, event.OccurredAt.UTC(), event.Payload, message, failedAt.UTC()); err != nil {
		return fmt.Errorf("save externalization dead letter: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.externalization_watermarks SET completeness=$2 WHERE source=$1`,
		event.SourceVersion, events.CompletenessPartial); err != nil {
		return fmt.Errorf("mark failed externalization watermark partial: %w", err)
	}
	if err := setProjectionRowsCompleteness(ctx, tx, events.CompletenessPartial); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit externalization dead letter: %w", err)
	}
	return nil
}

func setProjectionRowsCompleteness(ctx context.Context, executor projectionDB, completeness string) error {
	for _, table := range []string{
		"account_read_models",
		"profitability_read_models",
		"accounting_read_models",
		"reconciliation_read_models",
	} {
		if _, err := executor.Exec(ctx, `UPDATE relay_ops.`+table+` SET completeness=$1`, completeness); err != nil {
			return fmt.Errorf("update %s completeness: %w", table, err)
		}
	}
	return nil
}

func (s *Store) LoadWatermark(ctx context.Context, source string) (events.Watermark, bool, error) {
	var watermark events.Watermark
	err := s.pool.QueryRow(ctx, `
		SELECT source, last_event_id, occurred_at, processed_at, completeness
		FROM relay_ops.externalization_watermarks WHERE source=$1`, source,
	).Scan(&watermark.Source, &watermark.LastEventID, &watermark.OccurredAt, &watermark.ProcessedAt, &watermark.Completeness)
	if errors.Is(err, pgx.ErrNoRows) {
		return events.Watermark{}, false, nil
	}
	if err != nil {
		return events.Watermark{}, false, fmt.Errorf("load externalization watermark: %w", err)
	}
	return watermark, true, nil
}

func (s *Store) ListDeadLetters(ctx context.Context) ([]events.DeadLetter, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_id, event_type, occurred_at, source_version, contract_version, payload, error, failed_at
		FROM relay_ops.externalization_dead_letters ORDER BY failed_at, event_id`)
	if err != nil {
		return nil, fmt.Errorf("list externalization dead letters: %w", err)
	}
	defer rows.Close()
	result := make([]events.DeadLetter, 0)
	for rows.Next() {
		var dead events.DeadLetter
		if err := rows.Scan(&dead.Event.EventID, &dead.Event.Type, &dead.Event.OccurredAt, &dead.Event.SourceVersion,
			&dead.Event.ContractVersion, &dead.Event.Payload, &dead.Error, &dead.FailedAt); err != nil {
			return nil, fmt.Errorf("scan externalization dead letter: %w", err)
		}
		result = append(result, dead)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate externalization dead letters: %w", err)
	}
	return result, nil
}

func (s *Store) LoadAccountReadModels(ctx context.Context) ([]projection.AccountRow, error) {
	rows, err := s.projectionDB(ctx).Query(ctx, `
		SELECT account_id, status, balance::text, currency, observed_at,
			health_occurred_at, health_event_id, balance_occurred_at, balance_event_id,
			generated_at, source_watermark, freshness_seconds, completeness, calculation_version
		FROM relay_ops.account_read_models ORDER BY account_id`)
	if err != nil {
		return nil, fmt.Errorf("load account read models: %w", err)
	}
	defer rows.Close()
	result := make([]projection.AccountRow, 0)
	for rows.Next() {
		var row projection.AccountRow
		var balance, currency, healthEventID, balanceEventID *string
		var observedAt, healthAt, balanceAt *time.Time
		if err := rows.Scan(&row.AccountID, &row.Status, &balance, &currency, &observedAt,
			&healthAt, &healthEventID, &balanceAt, &balanceEventID,
			&row.Metadata.GeneratedAt, &row.Metadata.SourceWatermark, &row.Metadata.FreshnessSeconds,
			&row.Metadata.Completeness, &row.Metadata.CalculationVersion); err != nil {
			return nil, fmt.Errorf("scan account read model: %w", err)
		}
		if balance != nil {
			row.Balance = *balance
		}
		if currency != nil {
			row.Currency = *currency
		}
		if observedAt != nil {
			row.ObservedAt = observedAt.UTC()
		}
		if healthAt != nil {
			row.HealthOccurredAt = healthAt.UTC()
		}
		if healthEventID != nil {
			row.HealthEventID = *healthEventID
		}
		if balanceAt != nil {
			row.BalanceOccurredAt = balanceAt.UTC()
		}
		if balanceEventID != nil {
			row.BalanceEventID = *balanceEventID
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account read models: %w", err)
	}
	return result, nil
}

func (s *Store) UpsertAccountReadModel(ctx context.Context, row projection.AccountRow) error {
	return upsertAccountReadModel(ctx, s.projectionDB(ctx), row)
}

func (s *Store) ReplaceAccountReadModels(ctx context.Context, rows []projection.AccountRow) error {
	if tx, ok := projectionTransaction(ctx); ok {
		return replaceAccountReadModels(ctx, tx, rows)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin account read model rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := replaceAccountReadModels(ctx, tx, rows); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit account read model rebuild: %w", err)
	}
	return nil
}

func replaceAccountReadModels(ctx context.Context, executor projectionDB, rows []projection.AccountRow) error {
	if _, err := executor.Exec(ctx, `DELETE FROM relay_ops.account_read_models`); err != nil {
		return fmt.Errorf("clear account read models: %w", err)
	}
	for _, row := range rows {
		if err := upsertAccountReadModel(ctx, executor, row); err != nil {
			return err
		}
	}
	return nil
}

type projectionDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func projectionTransaction(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(projectionTransactionContextKey{}).(pgx.Tx)
	return tx, ok
}

func (s *Store) projectionDB(ctx context.Context) projectionDB {
	if tx, ok := projectionTransaction(ctx); ok {
		return tx
	}
	return s.pool
}

func upsertAccountReadModel(ctx context.Context, executor projectionDB, row projection.AccountRow) error {
	status := row.Status
	if status == "" {
		status = "unknown"
	}
	_, err := executor.Exec(ctx, `
		INSERT INTO relay_ops.account_read_models
			(account_id, status, balance, currency, observed_at,
			 health_occurred_at, health_event_id, balance_occurred_at, balance_event_id,
			 generated_at, source_watermark, freshness_seconds, completeness, calculation_version)
		VALUES ($1, $2, NULLIF($3, '')::numeric, NULLIF($4, ''), NULLIF($5, $15::timestamptz),
			NULLIF($6, $15::timestamptz), NULLIF($7, ''), NULLIF($8, $15::timestamptz), NULLIF($9, ''),
			$10, $11, $12, $13, $14)
		ON CONFLICT (account_id) DO UPDATE SET
			status=EXCLUDED.status, balance=EXCLUDED.balance, currency=EXCLUDED.currency,
			observed_at=EXCLUDED.observed_at, health_occurred_at=EXCLUDED.health_occurred_at,
			health_event_id=EXCLUDED.health_event_id, balance_occurred_at=EXCLUDED.balance_occurred_at,
			balance_event_id=EXCLUDED.balance_event_id, generated_at=EXCLUDED.generated_at,
			source_watermark=EXCLUDED.source_watermark, freshness_seconds=EXCLUDED.freshness_seconds,
			completeness=EXCLUDED.completeness, calculation_version=EXCLUDED.calculation_version`,
		row.AccountID, status, row.Balance, row.Currency, row.ObservedAt.UTC(),
		row.HealthOccurredAt.UTC(), row.HealthEventID, row.BalanceOccurredAt.UTC(), row.BalanceEventID,
		row.Metadata.GeneratedAt.UTC(), row.Metadata.SourceWatermark, row.Metadata.FreshnessSeconds,
		row.Metadata.Completeness, row.Metadata.CalculationVersion, time.Time{}.UTC())
	if err != nil {
		return fmt.Errorf("upsert account read model %d: %w", row.AccountID, err)
	}
	return nil
}

func (s *Store) LoadProfitabilityReadModels(ctx context.Context) ([]projection.ProfitabilityRow, error) {
	rows, err := s.projectionDB(ctx).Query(ctx, `
		SELECT account_id, requests, revenue::text, cost::text, profit::text, margin::text, rank,
			source_occurred_at, generated_at, source_watermark, freshness_seconds, completeness, calculation_version
		FROM relay_ops.profitability_read_models ORDER BY account_id`)
	if err != nil {
		return nil, fmt.Errorf("load profitability read models: %w", err)
	}
	defer rows.Close()
	result := make([]projection.ProfitabilityRow, 0)
	for rows.Next() {
		var row projection.ProfitabilityRow
		if err := rows.Scan(&row.AccountID, &row.Requests, &row.Revenue, &row.Cost, &row.Profit, &row.Margin, &row.Rank,
			&row.SourceOccurredAt, &row.Metadata.GeneratedAt, &row.Metadata.SourceWatermark,
			&row.Metadata.FreshnessSeconds, &row.Metadata.Completeness, &row.Metadata.CalculationVersion); err != nil {
			return nil, fmt.Errorf("scan profitability read model: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profitability read models: %w", err)
	}
	return result, nil
}

func (s *Store) ReplaceProfitabilityReadModels(ctx context.Context, rows []projection.ProfitabilityRow) error {
	if tx, ok := projectionTransaction(ctx); ok {
		return replaceProfitabilityReadModels(ctx, tx, rows)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin profitability read model replace: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := replaceProfitabilityReadModels(ctx, tx, rows); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit profitability read model replace: %w", err)
	}
	return nil
}

func replaceProfitabilityReadModels(ctx context.Context, executor projectionDB, rows []projection.ProfitabilityRow) error {
	if _, err := executor.Exec(ctx, `DELETE FROM relay_ops.profitability_read_models`); err != nil {
		return fmt.Errorf("clear profitability read models: %w", err)
	}
	for _, row := range rows {
		if _, err := executor.Exec(ctx, `
			INSERT INTO relay_ops.profitability_read_models
				(account_id, requests, revenue, cost, profit, margin, rank, source_occurred_at,
				 generated_at, source_watermark, freshness_seconds, completeness, calculation_version)
			VALUES ($1, $2, $3::numeric, $4::numeric, $5::numeric, $6::numeric, $7, $8, $9, $10, $11, $12, $13)`,
			row.AccountID, row.Requests, row.Revenue, row.Cost, row.Profit, row.Margin, row.Rank,
			row.SourceOccurredAt.UTC(), row.Metadata.GeneratedAt.UTC(), row.Metadata.SourceWatermark,
			row.Metadata.FreshnessSeconds, row.Metadata.Completeness, row.Metadata.CalculationVersion); err != nil {
			return fmt.Errorf("insert profitability read model %d: %w", row.AccountID, err)
		}
	}
	return nil
}

func (s *Store) LoadAccountingReadModel(ctx context.Context) (projection.Accounting, bool, error) {
	var model projection.Accounting
	err := s.projectionDB(ctx).QueryRow(ctx, `
		SELECT requests, revenue::text, cost::text, source_occurred_at,
			generated_at, source_watermark, freshness_seconds, completeness, calculation_version
		FROM relay_ops.accounting_read_models WHERE scope='all'`).Scan(
		&model.Requests, &model.Revenue, &model.Cost, &model.SourceOccurredAt,
		&model.Metadata.GeneratedAt, &model.Metadata.SourceWatermark, &model.Metadata.FreshnessSeconds,
		&model.Metadata.Completeness, &model.Metadata.CalculationVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return projection.Accounting{}, false, nil
	}
	if err != nil {
		return projection.Accounting{}, false, fmt.Errorf("load accounting read model: %w", err)
	}
	return model, true, nil
}

func (s *Store) SaveAccountingReadModel(ctx context.Context, model projection.Accounting) error {
	if model.Metadata.Completeness == projection.CompletenessEmpty {
		_, err := s.projectionDB(ctx).Exec(ctx, `DELETE FROM relay_ops.accounting_read_models WHERE scope='all'`)
		return err
	}
	_, err := s.projectionDB(ctx).Exec(ctx, `
		INSERT INTO relay_ops.accounting_read_models
			(scope, requests, revenue, cost, source_occurred_at, generated_at, source_watermark,
			 freshness_seconds, completeness, calculation_version)
		VALUES ('all', $1, $2::numeric, $3::numeric, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (scope) DO UPDATE SET requests=EXCLUDED.requests, revenue=EXCLUDED.revenue,
			cost=EXCLUDED.cost, source_occurred_at=EXCLUDED.source_occurred_at,
			generated_at=EXCLUDED.generated_at, source_watermark=EXCLUDED.source_watermark,
			freshness_seconds=EXCLUDED.freshness_seconds, completeness=EXCLUDED.completeness,
			calculation_version=EXCLUDED.calculation_version`,
		model.Requests, zeroNumeric(model.Revenue), zeroNumeric(model.Cost), model.SourceOccurredAt.UTC(),
		model.Metadata.GeneratedAt.UTC(), model.Metadata.SourceWatermark, model.Metadata.FreshnessSeconds,
		model.Metadata.Completeness, model.Metadata.CalculationVersion)
	if err != nil {
		return fmt.Errorf("save accounting read model: %w", err)
	}
	return nil
}

func (s *Store) LoadReconciliationReadModel(ctx context.Context) (projection.Reconciliation, bool, error) {
	var model projection.Reconciliation
	err := s.projectionDB(ctx).QueryRow(ctx, `
		SELECT matched, exceptions, source_occurred_at,
			generated_at, source_watermark, freshness_seconds, completeness, calculation_version
		FROM relay_ops.reconciliation_read_models WHERE scope='all'`).Scan(
		&model.Matched, &model.Exceptions, &model.SourceOccurredAt,
		&model.Metadata.GeneratedAt, &model.Metadata.SourceWatermark, &model.Metadata.FreshnessSeconds,
		&model.Metadata.Completeness, &model.Metadata.CalculationVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return projection.Reconciliation{}, false, nil
	}
	if err != nil {
		return projection.Reconciliation{}, false, fmt.Errorf("load reconciliation read model: %w", err)
	}
	return model, true, nil
}

func (s *Store) SaveReconciliationReadModel(ctx context.Context, model projection.Reconciliation) error {
	if model.Metadata.Completeness == projection.CompletenessEmpty {
		_, err := s.projectionDB(ctx).Exec(ctx, `DELETE FROM relay_ops.reconciliation_read_models WHERE scope='all'`)
		return err
	}
	_, err := s.projectionDB(ctx).Exec(ctx, `
		INSERT INTO relay_ops.reconciliation_read_models
			(scope, matched, exceptions, source_occurred_at, generated_at, source_watermark,
			 freshness_seconds, completeness, calculation_version)
		VALUES ('all', $1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (scope) DO UPDATE SET matched=EXCLUDED.matched, exceptions=EXCLUDED.exceptions,
			source_occurred_at=EXCLUDED.source_occurred_at, generated_at=EXCLUDED.generated_at,
			source_watermark=EXCLUDED.source_watermark, freshness_seconds=EXCLUDED.freshness_seconds,
			completeness=EXCLUDED.completeness, calculation_version=EXCLUDED.calculation_version`,
		model.Matched, model.Exceptions, model.SourceOccurredAt.UTC(), model.Metadata.GeneratedAt.UTC(),
		model.Metadata.SourceWatermark, model.Metadata.FreshnessSeconds, model.Metadata.Completeness,
		model.Metadata.CalculationVersion)
	if err != nil {
		return fmt.Errorf("save reconciliation read model: %w", err)
	}
	return nil
}

func zeroNumeric(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

// preflightBillingAccountMapping prevents migration 010 from failing with an
// opaque unique-index error when a partially migrated legacy database already
// contains duplicate active billing readers. It only reports the conflict; it
// never mutates sessions or secret references.
func (s *Store) preflightBillingAccountMapping(ctx context.Context) error {
	var billingAccountColumnExists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema='relay_ops'
				AND table_name='auth_sessions'
				AND column_name='billing_account_id'
		)`).Scan(&billingAccountColumnExists); err != nil {
		return fmt.Errorf("preflight migration 010 billing account mapping: %w", err)
	}
	if !billingAccountColumnExists {
		return nil
	}

	var billingAccountID, mappingCount int64
	err := s.pool.QueryRow(ctx, `
		SELECT billing_account_id, COUNT(*)
		FROM relay_ops.auth_sessions
		WHERE billing_account_id IS NOT NULL
			AND status='active'
			AND scope='billing_read'
		GROUP BY billing_account_id
		HAVING COUNT(*) > 1
		ORDER BY billing_account_id
		LIMIT 1`).Scan(&billingAccountID, &mappingCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("preflight migration 010 billing account mapping: %w", err)
	}
	return fmt.Errorf("migration 010 blocked: billing account %d has %d active billing_read mappings; resolve duplicate mappings before retrying migration", billingAccountID, mappingCount)
}

func (s *Store) CreateUpstream(ctx context.Context, upstream Upstream) (domain.UpstreamID, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.upstreams (display_name, role, base_url, adapter_type, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, upstream.Name, upstream.Role, upstream.BaseURL, upstream.AdapterType, upstream.Enabled).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrConflict
		}
		return 0, fmt.Errorf("create upstream: %w", err)
	}
	return domain.UpstreamID(id), nil
}

func (s *Store) CreateCandidate(ctx context.Context, record candidates.CreateRecord) (domain.UpstreamID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin candidate transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO relay_ops.secret_refs (secret_ref, kind, owner_scope, fingerprint, last_four)
		VALUES ($1, $2, $3, $4, $5)`,
		record.SecretRef.SecretRef, record.SecretRef.Kind, record.SecretRef.OwnerScope, record.SecretRef.Fingerprint, record.SecretRef.LastFour)
	if err != nil {
		return 0, candidateCreateError(err)
	}
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO relay_ops.upstreams
			(display_name, role, base_url, pricing_url, usage_url, performance_url, adapter_type, candidate_probe_secret_ref, enabled)
		VALUES ($1, 'candidate', $2, $3, $4, NULLIF($5, ''), 'unknown', $6, TRUE)
		RETURNING id`,
		record.Candidate.Name, record.Candidate.BaseURL, record.Candidate.PricingURL, record.Candidate.UsageURL,
		record.Candidate.PerformanceURL, record.SecretRef.SecretRef).Scan(&id)
	if err != nil {
		return 0, candidateCreateError(err)
	}
	afterSummary, err := json.Marshal(record.Audit.AfterSummary)
	if err != nil {
		return 0, fmt.Errorf("encode candidate audit summary: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events
			(actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`,
		record.Audit.ActorUserID, record.Audit.Action, record.Audit.ObjectType, strconv.FormatInt(id, 10), afterSummary)
	if err != nil {
		return 0, fmt.Errorf("insert candidate audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit candidate: %w", err)
	}
	return domain.UpstreamID(id), nil
}

func (s *Store) CreateProduction(ctx context.Context, record upstreams.ProductionRecord) (domain.UpstreamID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin production upstream transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO relay_ops.upstreams
			(display_name, role, base_url, pricing_url, usage_url, performance_url, adapter_type, sub2api_channel_monitor_id, enabled)
		VALUES ($1, 'production', $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, 0), TRUE)
		RETURNING id`,
		record.Source.Name, record.Source.BaseURL, record.Source.PricingURL, record.Source.UsageURL,
		record.Source.PerformanceURL, record.Source.AdapterType, record.Source.MonitorID,
	).Scan(&id)
	if err != nil {
		return 0, productionCreateError(err)
	}
	for _, groupID := range record.Source.GroupIDs {
		command, err := tx.Exec(ctx, `
			INSERT INTO relay_ops.upstream_public_groups (upstream_id, group_id)
			SELECT $1, group_id FROM relay_ops.public_groups
			WHERE group_id=$2 AND enabled=TRUE AND customer_visible=TRUE`, id, groupID)
		if err != nil {
			return 0, fmt.Errorf("link production public group: %w", err)
		}
		if command.RowsAffected() != 1 {
			return 0, upstreams.ErrGroupUnavailable
		}
	}
	afterSummary, err := json.Marshal(record.Audit.AfterSummary)
	if err != nil {
		return 0, fmt.Errorf("encode production audit summary: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events
			(actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`,
		record.Audit.ActorUserID, record.Audit.Action, record.Audit.ObjectType, strconv.FormatInt(id, 10), afterSummary,
	); err != nil {
		return 0, fmt.Errorf("insert production upstream audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit production upstream: %w", err)
	}
	return domain.UpstreamID(id), nil
}

// ProvisionBillingSource creates a production source and its active billing
// bearer mapping in one transaction. A repeated, byte-for-byte equivalent
// declaration is a no-op; any divergent pre-existing state is rejected.
func (s *Store) ProvisionBillingSource(ctx context.Context, record billing.BillingProvisionRecord) (billing.BillingProvisionResult, error) {
	source := record.Production.Source
	secretRef := strings.TrimSpace(record.Session.Secret.SecretRef)
	if source.Name == "" || source.Role != upstreams.RoleProduction || !source.Enabled || record.Session.BillingAccountID <= 0 || secretRef == "" || secretRef != record.Session.Secret.SecretRef {
		return billing.BillingProvisionResult{}, fmt.Errorf("billing provision record is invalid")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("begin billing provision: %w", err)
	}
	defer tx.Rollback(ctx)
	// Lock the secret reference before the source identity. SELECT ... FOR UPDATE
	// cannot protect an absent auth_session row, so two different new sources
	// could otherwise both bind the same bearer in parallel. Every provisioner
	// acquires these keys in this order to avoid lock cycles.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, billingProvisionLockKey("secret", secretRef)); err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("lock billing secret reference: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, billingProvisionLockKey("source", source.Name)); err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("lock billing source: %w", err)
	}

	var existing upstreams.Source
	err = tx.QueryRow(ctx, `
		SELECT id, display_name, role, base_url, COALESCE(pricing_url, ''), COALESCE(usage_url, ''),
			COALESCE(performance_url, ''), adapter_type, COALESCE(sub2api_channel_monitor_id, 0), enabled
		FROM relay_ops.upstreams WHERE display_name=$1 FOR UPDATE`, source.Name).Scan(
		&existing.ID, &existing.Name, &existing.Role, &existing.BaseURL, &existing.PricingURL, &existing.UsageURL,
		&existing.PerformanceURL, &existing.AdapterType, &existing.MonitorID, &existing.Enabled,
	)
	createdUpstream := false
	if errors.Is(err, pgx.ErrNoRows) {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO relay_ops.upstreams
				(display_name, role, base_url, pricing_url, usage_url, performance_url, adapter_type, sub2api_channel_monitor_id, enabled)
			VALUES ($1, 'production', $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, 0), TRUE)
			RETURNING id`,
			source.Name, source.BaseURL, source.PricingURL, source.UsageURL, source.PerformanceURL, source.AdapterType, source.MonitorID,
		).Scan(&id); err != nil {
			return billing.BillingProvisionResult{}, provisionStoreError(err)
		}
		existing = source
		existing.ID = domain.UpstreamID(id)
		createdUpstream = true
		if err := linkProvisionGroups(ctx, tx, existing.ID, source.GroupIDs); err != nil {
			return billing.BillingProvisionResult{}, err
		}
		if err := insertProvisionAudit(ctx, tx, record.Production.Audit, existing.ID); err != nil {
			return billing.BillingProvisionResult{}, err
		}
	} else if err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("find billing provision upstream: %w", err)
	} else {
		groups, err := provisionGroupIDs(ctx, tx, existing.ID)
		if err != nil {
			return billing.BillingProvisionResult{}, err
		}
		if !sameProvisionSource(existing, source, groups) {
			return billing.BillingProvisionResult{}, billing.ErrBillingProvisionConflict
		}
	}

	var existingSecretRef, existingAuthMode, existingStatus, existingLoginURL, existingScope string
	var existingAccountID sql.NullInt64
	err = tx.QueryRow(ctx, `
		SELECT secret_ref, auth_mode, status, login_url, scope, billing_account_id
		FROM relay_ops.auth_sessions WHERE upstream_id=$1 FOR UPDATE`, existing.ID).Scan(
		&existingSecretRef, &existingAuthMode, &existingStatus, &existingLoginURL, &existingScope, &existingAccountID,
	)
	createdSession := false
	if errors.Is(err, pgx.ErrNoRows) {
		var mappedUpstreamID int64
		err = tx.QueryRow(ctx, `
			SELECT upstream_id FROM relay_ops.auth_sessions
			WHERE billing_account_id=$1 AND status='active' AND scope='billing_read'
			FOR UPDATE`, record.Session.BillingAccountID).Scan(&mappedUpstreamID)
		if err == nil && mappedUpstreamID != int64(existing.ID) {
			return billing.BillingProvisionResult{}, billing.ErrBillingProvisionConflict
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return billing.BillingProvisionResult{}, fmt.Errorf("check billing account mapping: %w", err)
		}
		var secretUpstreamID int64
		err = tx.QueryRow(ctx, `SELECT upstream_id FROM relay_ops.auth_sessions WHERE secret_ref=$1 FOR UPDATE`, secretRef).Scan(&secretUpstreamID)
		if err == nil && secretUpstreamID != int64(existing.ID) {
			return billing.BillingProvisionResult{}, billing.ErrBillingProvisionConflict
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return billing.BillingProvisionResult{}, fmt.Errorf("check billing secret reference: %w", err)
		}
		secret := record.Session.Secret
		secret.SecretRef = secretRef
		secret.OwnerScope = strconv.FormatInt(int64(existing.ID), 10)
		if _, err := tx.Exec(ctx, `
			INSERT INTO relay_ops.secret_refs (secret_ref, kind, owner_scope, fingerprint, last_four)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (secret_ref) DO UPDATE SET kind=EXCLUDED.kind, owner_scope=EXCLUDED.owner_scope,
				fingerprint=EXCLUDED.fingerprint, last_four=EXCLUDED.last_four, status='active'`,
			secret.SecretRef, secret.Kind, secret.OwnerScope, secret.Fingerprint, secret.LastFour); err != nil {
			return billing.BillingProvisionResult{}, fmt.Errorf("store billing secret reference: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
			VALUES ($1, $2, 'bearer', 'active', $3, 'billing_read', $4)`,
			existing.ID, secret.SecretRef, record.Session.LoginURL, record.Session.BillingAccountID); err != nil {
			return billing.BillingProvisionResult{}, provisionStoreError(err)
		}
		if err := insertSessionProvisionAudit(ctx, tx, record.Session.Audit, existing.ID); err != nil {
			return billing.BillingProvisionResult{}, err
		}
		createdSession = true
	} else if err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("find billing session: %w", err)
	} else if existingSecretRef != secretRef || existingAuthMode != "bearer" || existingStatus != "active" || existingLoginURL != record.Session.LoginURL || existingScope != "billing_read" || !existingAccountID.Valid || existingAccountID.Int64 != record.Session.BillingAccountID {
		return billing.BillingProvisionResult{}, billing.ErrBillingProvisionConflict
	} else if err := verifyProvisionSecret(ctx, tx, secretRef, record.Session.Secret, existing.ID); err != nil {
		return billing.BillingProvisionResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return billing.BillingProvisionResult{}, fmt.Errorf("commit billing provision: %w", err)
	}
	return billing.BillingProvisionResult{UpstreamID: existing.ID, BillingAccountID: record.Session.BillingAccountID, AlreadyConfigured: !createdUpstream && !createdSession}, nil
}

func billingProvisionLockKey(kind, value string) string {
	return "billing-provision:" + kind + ":" + value
}

// verifyProvisionSecret makes an idempotent re-run exact: a rotated bearer
// must use a deliberate rotation flow instead of silently retaining stale
// fingerprint audit evidence.
func verifyProvisionSecret(ctx context.Context, tx pgx.Tx, secretRef string, expected billing.SessionSecretRef, upstreamID domain.UpstreamID) error {
	var kind, ownerScope, fingerprint, lastFour, status string
	err := tx.QueryRow(ctx, `
		SELECT kind, owner_scope, fingerprint, last_four, status
		FROM relay_ops.secret_refs WHERE secret_ref=$1 FOR UPDATE`, secretRef).Scan(
		&kind, &ownerScope, &fingerprint, &lastFour, &status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.ErrBillingProvisionConflict
	}
	if err != nil {
		return fmt.Errorf("find billing secret reference: %w", err)
	}
	if kind != expected.Kind || ownerScope != strconv.FormatInt(int64(upstreamID), 10) || fingerprint != expected.Fingerprint || lastFour != expected.LastFour || status != "active" {
		return billing.ErrBillingProvisionConflict
	}
	return nil
}

func linkProvisionGroups(ctx context.Context, tx pgx.Tx, upstreamID domain.UpstreamID, groupIDs []int64) error {
	for _, groupID := range groupIDs {
		command, err := tx.Exec(ctx, `
			INSERT INTO relay_ops.upstream_public_groups (upstream_id, group_id)
			SELECT $1, group_id FROM relay_ops.public_groups
			WHERE group_id=$2 AND enabled=TRUE AND customer_visible=TRUE`, upstreamID, groupID)
		if err != nil {
			return fmt.Errorf("link billing provision public group: %w", err)
		}
		if command.RowsAffected() != 1 {
			return upstreams.ErrGroupUnavailable
		}
	}
	return nil
}

func provisionGroupIDs(ctx context.Context, tx pgx.Tx, upstreamID domain.UpstreamID) ([]int64, error) {
	rows, err := tx.Query(ctx, `SELECT group_id FROM relay_ops.upstream_public_groups WHERE upstream_id=$1 ORDER BY group_id`, upstreamID)
	if err != nil {
		return nil, fmt.Errorf("list billing provision groups: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan billing provision group: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing provision groups: %w", err)
	}
	return ids, nil
}

func sameProvisionSource(existing, expected upstreams.Source, groups []int64) bool {
	if existing.Name != expected.Name || existing.Role != upstreams.RoleProduction || !existing.Enabled || existing.BaseURL != expected.BaseURL || existing.PricingURL != expected.PricingURL || existing.UsageURL != expected.UsageURL || existing.PerformanceURL != expected.PerformanceURL || existing.AdapterType != expected.AdapterType || existing.MonitorID != expected.MonitorID || len(groups) != len(expected.GroupIDs) {
		return false
	}
	for index := range groups {
		if groups[index] != expected.GroupIDs[index] {
			return false
		}
	}
	return true
}

func insertProvisionAudit(ctx context.Context, tx pgx.Tx, audit upstreams.AuditEvent, upstreamID domain.UpstreamID) error {
	afterSummary, err := json.Marshal(audit.AfterSummary)
	if err != nil {
		return fmt.Errorf("encode billing provision upstream audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events (actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`, audit.ActorUserID, audit.Action, audit.ObjectType, strconv.FormatInt(int64(upstreamID), 10), afterSummary); err != nil {
		return fmt.Errorf("insert billing provision upstream audit: %w", err)
	}
	return nil
}

func insertSessionProvisionAudit(ctx context.Context, tx pgx.Tx, audit billing.SessionAuditEvent, upstreamID domain.UpstreamID) error {
	afterSummary, err := json.Marshal(audit.AfterSummary)
	if err != nil {
		return fmt.Errorf("encode billing provision session audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events (actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`, audit.ActorUserID, audit.Action, audit.ObjectType, strconv.FormatInt(int64(upstreamID), 10), afterSummary); err != nil {
		return fmt.Errorf("insert billing provision session audit: %w", err)
	}
	return nil
}

func provisionStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return billing.ErrBillingProvisionConflict
	}
	return fmt.Errorf("persist billing provision: %w", err)
}

func (s *Store) ResolvePublicGroupIDs(ctx context.Context, names []string) ([]int64, error) {
	clean := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		clean = append(clean, name)
	}
	if len(clean) == 0 {
		return nil, upstreams.ErrGroupRequired
	}
	rows, err := s.pool.Query(ctx, `
		SELECT group_id FROM relay_ops.public_groups
		WHERE name = ANY($1::text[]) AND enabled=TRUE AND customer_visible=TRUE
		ORDER BY group_id`, clean)
	if err != nil {
		return nil, fmt.Errorf("resolve public group names: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0, len(clean))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan public group ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public group IDs: %w", err)
	}
	if len(ids) != len(clean) {
		return nil, upstreams.ErrGroupUnavailable
	}
	return ids, nil
}

func productionCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return upstreams.ErrConflict
	}
	return fmt.Errorf("create production upstream: %w", err)
}

func (s *Store) ListProduction(ctx context.Context) ([]upstreams.Source, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.display_name, u.role, u.base_url, COALESCE(u.pricing_url, ''),
			COALESCE(u.usage_url, ''), COALESCE(u.performance_url, ''), u.adapter_type,
			COALESCE(u.sub2api_channel_monitor_id, 0), u.enabled,
			COALESCE(array_agg(link.group_id ORDER BY link.group_id) FILTER (WHERE link.group_id IS NOT NULL), '{}')
		FROM relay_ops.upstreams u
		LEFT JOIN relay_ops.upstream_public_groups link ON link.upstream_id=u.id
		WHERE u.role='production'
		GROUP BY u.id
		ORDER BY u.display_name, u.id`)
	if err != nil {
		return nil, fmt.Errorf("list production upstreams: %w", err)
	}
	defer rows.Close()
	result := make([]upstreams.Source, 0)
	for rows.Next() {
		var source upstreams.Source
		if err := rows.Scan(
			&source.ID, &source.Name, &source.Role, &source.BaseURL, &source.PricingURL,
			&source.UsageURL, &source.PerformanceURL, &source.AdapterType, &source.MonitorID,
			&source.Enabled, &source.GroupIDs,
		); err != nil {
			return nil, fmt.Errorf("scan production upstream: %w", err)
		}
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate production upstreams: %w", err)
	}
	return result, nil
}

func (s *Store) DisableProduction(ctx context.Context, upstreamID domain.UpstreamID, audit upstreams.AuditEvent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin production upstream disable: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE relay_ops.upstreams SET enabled=FALSE, updated_at=NOW() WHERE id=$1 AND role='production'`, upstreamID)
	if err != nil {
		return fmt.Errorf("disable production upstream: %w", err)
	}
	if command.RowsAffected() != 1 {
		return upstreams.ErrNotFound
	}
	afterSummary, err := json.Marshal(audit.AfterSummary)
	if err != nil {
		return fmt.Errorf("encode production disable audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events (actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`, audit.ActorUserID, audit.Action, audit.ObjectType,
		strconv.FormatInt(int64(upstreamID), 10), afterSummary,
	); err != nil {
		return fmt.Errorf("insert production disable audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit production disable: %w", err)
	}
	return nil
}

func candidateCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return candidates.ErrConflict
	}
	return fmt.Errorf("create candidate: %w", err)
}

func (s *Store) ListCandidates(ctx context.Context) ([]candidates.Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, display_name, base_url, COALESCE(pricing_url, ''), COALESCE(usage_url, ''),
			COALESCE(performance_url, ''), COALESCE(candidate_probe_secret_ref, ''), enabled
		FROM relay_ops.upstreams
		WHERE role = 'candidate'
		ORDER BY display_name, id`)
	if err != nil {
		return nil, fmt.Errorf("list candidates: %w", err)
	}
	defer rows.Close()
	result := make([]candidates.Candidate, 0)
	for rows.Next() {
		var candidate candidates.Candidate
		if err := rows.Scan(
			&candidate.ID, &candidate.Name, &candidate.BaseURL, &candidate.PricingURL,
			&candidate.UsageURL, &candidate.PerformanceURL, &candidate.ProbeSecretRef, &candidate.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidates: %w", err)
	}
	return result, nil
}

func (s *Store) ListPublicGroupNames(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT name FROM relay_ops.public_groups WHERE enabled=TRUE AND customer_visible=TRUE ORDER BY name, group_id`)
	if err != nil {
		return nil, fmt.Errorf("list public group names: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan public group name: %w", err)
		}
		result = append(result, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public group names: %w", err)
	}
	return result, nil
}

func (s *Store) DisableCandidate(ctx context.Context, upstreamID domain.UpstreamID, audit candidates.AuditEvent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin candidate disable: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		UPDATE relay_ops.upstreams
		SET enabled = FALSE, updated_at = NOW()
		WHERE id = $1 AND role = 'candidate'`, upstreamID)
	if err != nil {
		return fmt.Errorf("disable candidate: %w", err)
	}
	if command.RowsAffected() != 1 {
		return candidates.ErrNotFound
	}
	afterSummary, err := json.Marshal(audit.AfterSummary)
	if err != nil {
		return fmt.Errorf("encode candidate audit summary: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events
			(actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`,
		audit.ActorUserID, audit.Action, audit.ObjectType, strconv.FormatInt(int64(upstreamID), 10), afterSummary); err != nil {
		return fmt.Errorf("insert candidate disable audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit candidate disable: %w", err)
	}
	return nil
}

func (s *Store) RecordExpired(ctx context.Context, upstreamID domain.UpstreamID, loginURL string, _ time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin usage session update: %w", err)
	}
	defer tx.Rollback(ctx)
	var storedLoginURL string
	if err := tx.QueryRow(ctx, `SELECT login_url FROM relay_ops.auth_sessions WHERE upstream_id=$1 FOR UPDATE`, upstreamID).Scan(&storedLoginURL); err != nil {
		return fmt.Errorf("find usage session: %w", err)
	}
	if storedLoginURL != loginURL {
		return fmt.Errorf("usage session login URL does not match configuration")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.auth_sessions
		SET status='expired', last_failure_reason='unauthorized'
		WHERE upstream_id=$1`, upstreamID); err != nil {
		return fmt.Errorf("record expired usage session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit usage session update: %w", err)
	}
	return nil
}

func (s *Store) UpsertUsageSession(ctx context.Context, record billing.SessionRecord) (billing.SessionConfig, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return billing.SessionConfig{}, fmt.Errorf("begin usage session configuration: %w", err)
	}
	defer tx.Rollback(ctx)
	var name, usageURL string
	if err := tx.QueryRow(ctx, `
		SELECT display_name, COALESCE(usage_url, '') FROM relay_ops.upstreams
		WHERE id=$1 AND role='production' AND enabled=TRUE`, record.Config.UpstreamID).Scan(&name, &usageURL); err != nil {
		return billing.SessionConfig{}, fmt.Errorf("find production upstream for usage session: %w", err)
	}
	if usageURL == "" {
		return billing.SessionConfig{}, fmt.Errorf("production upstream has no usage URL")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.secret_refs (secret_ref, kind, owner_scope, fingerprint, last_four)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (secret_ref) DO UPDATE SET kind=EXCLUDED.kind, owner_scope=EXCLUDED.owner_scope,
			fingerprint=EXCLUDED.fingerprint, last_four=EXCLUDED.last_four, status='active'`,
		record.Secret.SecretRef, record.Secret.Kind, record.Secret.OwnerScope, record.Secret.Fingerprint, record.Secret.LastFour); err != nil {
		return billing.SessionConfig{}, fmt.Errorf("store usage session secret reference: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
		VALUES ($1, $2, $3, 'active', $4, $5, $6)
		ON CONFLICT (upstream_id) DO UPDATE SET secret_ref=EXCLUDED.secret_ref, auth_mode=EXCLUDED.auth_mode,
			status='active', login_url=EXCLUDED.login_url, scope=EXCLUDED.scope,
			billing_account_id=EXCLUDED.billing_account_id, last_failure_reason=NULL`,
		record.Config.UpstreamID, record.Secret.SecretRef, record.Config.AuthMode, record.Config.LoginURL,
		record.Config.Scope, nullableBillingAccountID(record.Config.BillingAccountID)); err != nil {
		return billing.SessionConfig{}, fmt.Errorf("store usage session: %w", err)
	}
	afterSummary, err := json.Marshal(record.Audit.AfterSummary)
	if err != nil {
		return billing.SessionConfig{}, fmt.Errorf("encode usage session audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.audit_events (actor_user_id, action, object_type, object_id, after_summary)
		VALUES ($1, $2, $3, $4, $5)`, record.Audit.ActorUserID, record.Audit.Action, record.Audit.ObjectType,
		strconv.FormatInt(int64(record.Config.UpstreamID), 10), afterSummary); err != nil {
		return billing.SessionConfig{}, fmt.Errorf("insert usage session audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return billing.SessionConfig{}, fmt.Errorf("commit usage session: %w", err)
	}
	record.Config.UpstreamName = name
	record.Config.UsageURL = usageURL
	return record.Config, nil
}

func (s *Store) ListUsageSessions(ctx context.Context) ([]billing.SessionConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.upstream_id, u.display_name, u.usage_url, a.login_url, a.auth_mode, a.secret_ref
		FROM relay_ops.auth_sessions a
		JOIN relay_ops.upstreams u ON u.id=a.upstream_id
		WHERE u.role='production' AND u.enabled=TRUE AND COALESCE(u.usage_url, '') <> ''
		ORDER BY u.display_name, a.upstream_id`)
	if err != nil {
		return nil, fmt.Errorf("list usage sessions: %w", err)
	}
	defer rows.Close()
	result := make([]billing.SessionConfig, 0)
	for rows.Next() {
		var config billing.SessionConfig
		if err := rows.Scan(&config.UpstreamID, &config.UpstreamName, &config.UsageURL, &config.LoginURL, &config.AuthMode, &config.SecretRef); err != nil {
			return nil, fmt.Errorf("scan usage session: %w", err)
		}
		result = append(result, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage sessions: %w", err)
	}
	return result, nil
}

// ListBillingSources returns only the non-secret coordinates of enabled
// production billing sources. Credential material remains in the mounted
// secret referenced by SecretRef.
func (s *Store) ListBillingSources(ctx context.Context) ([]billing.BillingSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.billing_account_id, u.base_url, u.adapter_type, a.secret_ref
		FROM relay_ops.upstreams u
		JOIN relay_ops.auth_sessions a ON a.upstream_id=u.id
		WHERE u.role='production' AND u.enabled=TRUE AND a.status='active' AND a.auth_mode='bearer'
			AND u.adapter_type IN ('newapi', 'sub2api')
			AND a.scope='billing_read' AND a.billing_account_id IS NOT NULL
			AND COALESCE(u.base_url, '') <> '' AND COALESCE(a.secret_ref, '') <> ''
		ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("list billing sources: %w", err)
	}
	defer rows.Close()
	sources := make([]billing.BillingSource, 0)
	for rows.Next() {
		var source billing.BillingSource
		if err := rows.Scan(&source.AccountID, &source.BaseURL, &source.AdapterType, &source.SecretRef); err != nil {
			return nil, fmt.Errorf("scan billing source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing sources: %w", err)
	}
	return sources, nil
}

func nullableBillingAccountID(accountID int64) any {
	if accountID <= 0 {
		return nil
	}
	return accountID
}

func (s *Store) RecordHealthy(ctx context.Context, upstreamID domain.UpstreamID, observedAt time.Time) error {
	command, err := s.pool.Exec(ctx, `
		UPDATE relay_ops.auth_sessions
		SET status='active', last_success_at=$2, last_failure_reason=NULL
		WHERE upstream_id=$1`, upstreamID, observedAt.UTC())
	if err != nil {
		return fmt.Errorf("record healthy usage session: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("usage session not found")
	}
	return nil
}

func (s *Store) AppendCostObservation(ctx context.Context, evidence billing.UsageEvidence) error {
	var actual any
	var multiplier any
	if evidence.HasActualCost {
		actual = evidence.ActualCost
		multiplier = evidence.EffectiveMultiplier
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.cost_observations
			(upstream_id, source, standard_cost_microusd, actual_cost_microusd, effective_multiplier_bps, confidence, comparison_note, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		evidence.UpstreamID, "provider_reported", evidence.StandardCost, actual, multiplier,
		"auxiliary", nullString(evidence.Note), evidence.ObservedAt.UTC())
	if err != nil {
		return fmt.Errorf("append cost observation: %w", err)
	}
	return nil
}

func (s *Store) AppendPricingSnapshot(ctx context.Context, snapshot PricingSnapshot) (int64, error) {
	normalized := json.RawMessage(snapshot.NormalizedJSON)
	if !json.Valid(normalized) {
		return 0, fmt.Errorf("normalized pricing payload is invalid JSON")
	}
	var diff any
	if len(snapshot.DiffSummary) > 0 {
		if !json.Valid(snapshot.DiffSummary) {
			return 0, fmt.Errorf("pricing diff is invalid JSON")
		}
		diff = json.RawMessage(snapshot.DiffSummary)
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.pricing_snapshots
			(upstream_id, source_url, source_type, fetched_at, content_hash, normalized_payload, diff_summary, evidence_level)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`, snapshot.UpstreamID, snapshot.SourceURL, snapshot.SourceType, snapshot.FetchedAt.UTC(), snapshot.ContentHash, normalized, diff, snapshot.EvidenceLevel).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("append pricing snapshot: %w", err)
	}
	return id, nil
}

func (s *Store) CountPricingSnapshots(ctx context.Context, upstreamID domain.UpstreamID) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.pricing_snapshots WHERE upstream_id = $1`, upstreamID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pricing snapshots: %w", err)
	}
	return count, nil
}

func (s *Store) LatestPricingSnapshot(ctx context.Context, upstreamID domain.UpstreamID) (PricingSnapshot, bool, error) {
	var snapshot PricingSnapshot
	snapshot.UpstreamID = upstreamID
	err := s.pool.QueryRow(ctx, `SELECT source_url, source_type, fetched_at, content_hash, normalized_payload, COALESCE(diff_summary, '{}'::jsonb), evidence_level FROM relay_ops.pricing_snapshots WHERE upstream_id=$1 ORDER BY fetched_at DESC, id DESC LIMIT 1`, upstreamID).Scan(&snapshot.SourceURL, &snapshot.SourceType, &snapshot.FetchedAt, &snapshot.ContentHash, &snapshot.NormalizedJSON, &snapshot.DiffSummary, &snapshot.EvidenceLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return PricingSnapshot{}, false, nil
	}
	if err != nil {
		return PricingSnapshot{}, false, fmt.Errorf("latest pricing snapshot: %w", err)
	}
	return snapshot, true, nil
}

func (s *Store) AppendProbeRun(ctx context.Context, upstreamID domain.UpstreamID, run probes.ProbeRun, observedAt time.Time) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	fingerprintBytes := sha256.Sum256([]byte(run.RunID))
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.probe_runs
			(run_id, upstream_id, probe_kind, status, input_tokens, output_tokens,
			 estimated_standard_cost_microusd, evidence_level, request_fingerprint, redaction_version, started_at, finished_at)
		VALUES ($1, $2, 'candidate_watch', $3, $4, $5, $6, 'live_probe', $7, 'v1', $8, $8)`,
		run.RunID, upstreamID, run.Status, run.Metrics.Usage.PromptTokens, run.Metrics.Usage.CompletionTokens,
		run.ExpenseUpperBound, hex.EncodeToString(fingerprintBytes[:]), observedAt.UTC())
	if err != nil {
		return fmt.Errorf("append candidate probe run: %w", err)
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, key string, now time.Time, interval time.Duration) (bool, error) {
	if key == "" || interval <= 0 {
		return false, fmt.Errorf("scheduler claim is invalid")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin scheduler claim: %w", err)
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, key).Scan(&locked); err != nil {
		return false, fmt.Errorf("lock scheduler job: %w", err)
	}
	if !locked {
		return false, nil
	}
	var nextDue time.Time
	err = tx.QueryRow(ctx, `SELECT next_due_at FROM relay_ops.scheduler_jobs WHERE job_key=$1 FOR UPDATE`, key).Scan(&nextDue)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read scheduler due time: %w", err)
	}
	now = now.UTC()
	if err == nil && now.Before(nextDue) {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit scheduler skip: %w", err)
		}
		return false, nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO relay_ops.scheduler_jobs (job_key, next_due_at, last_started_at, last_status)
		VALUES ($1, $2, $3, 'running')
		ON CONFLICT (job_key) DO UPDATE SET next_due_at=EXCLUDED.next_due_at, last_started_at=EXCLUDED.last_started_at, last_status='running', last_error_code=NULL`,
		key, now.Add(interval), now)
	if err != nil {
		return false, fmt.Errorf("claim scheduler job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit scheduler claim: %w", err)
	}
	return true, nil
}

func (s *Store) Finish(ctx context.Context, key string, finishedAt time.Time, runErr error) error {
	status := "passed"
	var errorCode any
	if runErr != nil {
		status = "failed"
		errorCode = "job_failed"
	}
	command, err := s.pool.Exec(ctx, `UPDATE relay_ops.scheduler_jobs SET last_finished_at=$2, last_status=$3, last_error_code=$4 WHERE job_key=$1`, key, finishedAt.UTC(), status, errorCode)
	if err != nil {
		return fmt.Errorf("finish scheduler job: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("scheduler job not found")
	}
	return nil
}

func (s *Store) UpsertPublicGroup(ctx context.Context, group sub2api.PublicGroupRecord) error {
	channels, err := json.Marshal(group.ChannelIDs)
	if err != nil {
		return fmt.Errorf("encode public group channels: %w", err)
	}
	monitors, err := json.Marshal(group.MonitorIDs)
	if err != nil {
		return fmt.Errorf("encode public group monitors: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO relay_ops.public_groups
			(group_id, name, enabled, customer_visible, user_multiplier_bps, model_allowlist,
			 upstream_route_refs, sub2api_channel_monitor_ids, health_gate, source_revision, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, '[]'::jsonb, $6, $7, $8, $9, $10)
		ON CONFLICT (group_id) DO UPDATE SET
			name = EXCLUDED.name,
			enabled = EXCLUDED.enabled,
			customer_visible = EXCLUDED.customer_visible,
			user_multiplier_bps = EXCLUDED.user_multiplier_bps,
			upstream_route_refs = EXCLUDED.upstream_route_refs,
			sub2api_channel_monitor_ids = EXCLUDED.sub2api_channel_monitor_ids,
			source_revision = EXCLUDED.source_revision,
			last_seen_at = EXCLUDED.last_seen_at`,
		group.GroupID, group.Name, group.Enabled, group.CustomerVisible, group.UserMultiplierBPS,
		channels, monitors, group.HealthGate, group.SourceRevision, group.LastSeenAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert public group: %w", err)
	}
	return nil
}

func (s *Store) AppendMetricRef(ctx context.Context, ref sub2api.MetricRef) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.metric_refs
			(source_kind, external_id, window_start, window_end, payload_hash, schema_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_kind, external_id, window_start, window_end, payload_hash) DO NOTHING`,
		ref.SourceKind, ref.ExternalID, ref.WindowStart.UTC(), ref.WindowEnd.UTC(), ref.PayloadHash, ref.SchemaVersion)
	if err != nil {
		return fmt.Errorf("append metric reference: %w", err)
	}
	return nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
