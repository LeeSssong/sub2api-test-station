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

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var initialMigration string

//go:embed migrations/004_user_impact_alerting.sql
var userImpactAlertingMigration string

//go:embed migrations/005_notification_retry.sql
var notificationRetryMigration string

//go:embed migrations/006_notification_consolidation.sql
var notificationConsolidationMigration string

//go:embed migrations/007_native_ops_alert_bridge.sql
var nativeOpsAlertBridgeMigration string

//go:embed migrations/008_accounting_ledger.sql
var accountingLedgerMigration string

//go:embed migrations/009_upstream_cost_reconciliation.sql
var upstreamCostReconciliationMigration string

//go:embed migrations/010_billing_account_mapping.sql
var billingAccountMappingMigration string

var ErrConflict = errors.New("record conflicts with existing identity")

func init() {
	initialMigration += "\n" + userImpactAlertingMigration
	initialMigration += "\n" + notificationRetryMigration
	initialMigration += "\n" + notificationConsolidationMigration
	initialMigration += "\n" + nativeOpsAlertBridgeMigration
	initialMigration += "\n" + accountingLedgerMigration
	initialMigration += "\n" + upstreamCostReconciliationMigration
}

type Store struct {
	pool *pgxpool.Pool
}

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

type Incident struct {
	IncidentKey   string
	Severity      string
	State         string
	BaselineValue string
	CurrentValue  string
	SampleCount   int64
	EvidenceRefs  []string
}

type NativeOpsAlertCursor struct {
	FiredAt time.Time
	EventID int64
}

type NativeOpsAlertSource struct {
	SourceEventID  int64
	RuleID         int64
	IncidentKey    string
	Severity       string
	SourceStatus   string
	FiredAt        time.Time
	ResolvedAt     *time.Time
	Silenced       bool
	DimensionsHash string
	LastSeenAt     time.Time
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

func (s *Store) LoadNativeOpsAlertCursor(ctx context.Context) (NativeOpsAlertCursor, bool, error) {
	var firedAt *time.Time
	var eventID *int64
	err := s.pool.QueryRow(ctx, `
		SELECT before_fired_at, before_id
		FROM relay_ops.native_ops_alert_sync_state
		WHERE singleton=TRUE`).Scan(&firedAt, &eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return NativeOpsAlertCursor{}, false, nil
	}
	if err != nil {
		return NativeOpsAlertCursor{}, false, fmt.Errorf("load native ops alert cursor: %w", err)
	}
	if firedAt == nil || eventID == nil {
		return NativeOpsAlertCursor{}, false, fmt.Errorf("stored native ops alert cursor is incomplete")
	}
	return NativeOpsAlertCursor{FiredAt: firedAt.UTC(), EventID: *eventID}, true, nil
}

func (s *Store) InitializeNativeOpsAlertSync(ctx context.Context, cursor NativeOpsAlertCursor, sources []NativeOpsAlertSource) error {
	if err := validateNativeOpsAlertPage(sources, cursor); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin native ops alert initialization: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.native_ops_alert_sync_state
			(singleton, before_fired_at, before_id, initialized_at, updated_at)
		VALUES (TRUE, $1, $2, NOW(), NOW())
		ON CONFLICT (singleton) DO NOTHING`, cursor.FiredAt.UTC(), cursor.EventID)
	if err != nil {
		return fmt.Errorf("initialize native ops alert cursor: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("native ops alert sync is already initialized")
	}
	for _, source := range sources {
		if err := upsertNativeOpsAlertSource(ctx, tx, source); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit native ops alert initialization: %w", err)
	}
	return nil
}

func (s *Store) CommitNativeOpsAlertPage(ctx context.Context, sources []NativeOpsAlertSource, cursor NativeOpsAlertCursor) error {
	if err := validateNativeOpsAlertPage(sources, cursor); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin native ops alert page: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, source := range sources {
		if err := upsertNativeOpsAlertSource(ctx, tx, source); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relay_ops.native_ops_alert_sync_state
			(singleton, before_fired_at, before_id, initialized_at, updated_at)
		VALUES (TRUE, $1, $2, NOW(), NOW())
		ON CONFLICT (singleton) DO UPDATE
		SET before_fired_at=EXCLUDED.before_fired_at,
			before_id=EXCLUDED.before_id,
			updated_at=NOW()
		WHERE (native_ops_alert_sync_state.before_fired_at, native_ops_alert_sync_state.before_id)
			<= (EXCLUDED.before_fired_at, EXCLUDED.before_id)`, cursor.FiredAt.UTC(), cursor.EventID); err != nil {
		return fmt.Errorf("commit native ops alert cursor: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit native ops alert page: %w", err)
	}
	return nil
}

func (s *Store) UpsertNativeOpsAlertSource(ctx context.Context, source NativeOpsAlertSource) error {
	if err := validateNativeOpsAlertSource(source); err != nil {
		return err
	}
	return upsertNativeOpsAlertSource(ctx, s.pool, source)
}

func (s *Store) ListFiringNativeOpsAlertSources(ctx context.Context, limit int) ([]NativeOpsAlertSource, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT source_event_id, rule_id, incident_key, severity, source_status,
			fired_at, resolved_at, silenced, dimensions_hash, last_seen_at
		FROM relay_ops.native_ops_alert_events
		WHERE source_status='firing'
		ORDER BY last_seen_at, source_event_id
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list firing native ops alert sources: %w", err)
	}
	defer rows.Close()
	result := make([]NativeOpsAlertSource, 0)
	for rows.Next() {
		var source NativeOpsAlertSource
		if err := rows.Scan(
			&source.SourceEventID, &source.RuleID, &source.IncidentKey, &source.Severity,
			&source.SourceStatus, &source.FiredAt, &source.ResolvedAt, &source.Silenced,
			&source.DimensionsHash, &source.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan firing native ops alert source: %w", err)
		}
		source.FiredAt = source.FiredAt.UTC()
		if source.ResolvedAt != nil {
			resolvedAt := source.ResolvedAt.UTC()
			source.ResolvedAt = &resolvedAt
		}
		source.LastSeenAt = source.LastSeenAt.UTC()
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate firing native ops alert sources: %w", err)
	}
	return result, nil
}

type nativeOpsAlertSourceExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func upsertNativeOpsAlertSource(ctx context.Context, executor nativeOpsAlertSourceExecer, source NativeOpsAlertSource) error {
	var resolvedAt any
	if source.ResolvedAt != nil {
		resolvedAt = source.ResolvedAt.UTC()
	}
	_, err := executor.Exec(ctx, `
		INSERT INTO relay_ops.native_ops_alert_events
			(source_event_id, rule_id, incident_key, severity, source_status,
			 fired_at, resolved_at, silenced, dimensions_hash, last_seen_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (source_event_id) DO UPDATE
		SET rule_id=EXCLUDED.rule_id,
			incident_key=EXCLUDED.incident_key,
			severity=EXCLUDED.severity,
			source_status=EXCLUDED.source_status,
			fired_at=EXCLUDED.fired_at,
			resolved_at=EXCLUDED.resolved_at,
			silenced=EXCLUDED.silenced,
			dimensions_hash=EXCLUDED.dimensions_hash,
			last_seen_at=EXCLUDED.last_seen_at,
			updated_at=NOW()`,
		source.SourceEventID, source.RuleID, source.IncidentKey, source.Severity, source.SourceStatus,
		source.FiredAt.UTC(), resolvedAt, source.Silenced, source.DimensionsHash, source.LastSeenAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert native ops alert source: %w", err)
	}
	return nil
}

func validateNativeOpsAlertPage(sources []NativeOpsAlertSource, cursor NativeOpsAlertCursor) error {
	if err := validateNativeOpsAlertCursor(cursor); err != nil {
		return err
	}
	for _, source := range sources {
		if err := validateNativeOpsAlertSource(source); err != nil {
			return err
		}
	}
	return nil
}

func validateNativeOpsAlertCursor(cursor NativeOpsAlertCursor) error {
	if cursor.EventID <= 0 || cursor.FiredAt.IsZero() {
		return fmt.Errorf("native ops alert cursor is invalid")
	}
	return nil
}

func validateNativeOpsAlertSource(source NativeOpsAlertSource) error {
	if source.SourceEventID <= 0 || source.RuleID <= 0 || strings.TrimSpace(source.IncidentKey) == "" {
		return fmt.Errorf("native ops alert source identity is invalid")
	}
	if source.Severity != "P0" && source.Severity != "P1" {
		return fmt.Errorf("native ops alert source severity is invalid")
	}
	if source.SourceStatus != "firing" && source.SourceStatus != "resolved" && source.SourceStatus != "manual_resolved" {
		return fmt.Errorf("native ops alert source status is invalid")
	}
	if !isLowerHexSHA256(source.DimensionsHash) {
		return fmt.Errorf("native ops alert source dimensions hash is invalid")
	}
	if source.FiredAt.IsZero() || source.LastSeenAt.IsZero() || source.LastSeenAt.Before(source.FiredAt) {
		return fmt.Errorf("native ops alert source timestamps are invalid")
	}
	if source.SourceStatus == "firing" && source.ResolvedAt != nil {
		return fmt.Errorf("firing native ops alert source has a resolution timestamp")
	}
	if source.SourceStatus != "firing" {
		if source.ResolvedAt == nil || source.ResolvedAt.IsZero() || source.ResolvedAt.Before(source.FiredAt) {
			return fmt.Errorf("terminal native ops alert source resolution timestamp is invalid")
		}
	}
	return nil
}

func isLowerHexSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
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

func (s *Store) RecordExpired(ctx context.Context, upstreamID domain.UpstreamID, loginURL string, observedAt time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin usage session update: %w", err)
	}
	defer tx.Rollback(ctx)
	var storedLoginURL string
	var lastNotified sql.NullTime
	if err := tx.QueryRow(ctx, `SELECT login_url, last_notified_at FROM relay_ops.auth_sessions WHERE upstream_id=$1 FOR UPDATE`, upstreamID).Scan(&storedLoginURL, &lastNotified); err != nil {
		return false, fmt.Errorf("find usage session: %w", err)
	}
	if storedLoginURL != loginURL {
		return false, fmt.Errorf("usage session login URL does not match configuration")
	}
	observedAt = observedAt.UTC()
	notify := !lastNotified.Valid || observedAt.Sub(lastNotified.Time) >= 24*time.Hour
	var notifiedAt any
	if notify {
		notifiedAt = observedAt
	} else {
		notifiedAt = lastNotified.Time
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.auth_sessions
		SET status='expired', last_failure_reason='unauthorized', last_notified_at=$2
		WHERE upstream_id=$1`, upstreamID, notifiedAt); err != nil {
		return false, fmt.Errorf("record expired usage session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit usage session update: %w", err)
	}
	return notify, nil
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

func (s *Store) UpsertIncident(ctx context.Context, incident Incident) (int64, bool, error) {
	if incident.SampleCount == 0 {
		incident.SampleCount = 1
	}
	evidence, err := json.Marshal(incident.EvidenceRefs)
	if err != nil {
		return 0, false, fmt.Errorf("encode incident evidence: %w", err)
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.incidents
			(incident_key, severity, state, baseline_value, current_value, sample_count, evidence_refs)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (incident_key) DO NOTHING
		RETURNING id`, incident.IncidentKey, incident.Severity, incident.State, nullString(incident.BaselineValue), nullString(incident.CurrentValue), incident.SampleCount, evidence).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("insert incident: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT id FROM relay_ops.incidents WHERE incident_key = $1`, incident.IncidentKey).Scan(&id); err != nil {
		return 0, false, fmt.Errorf("find existing incident: %w", err)
	}
	return id, false, nil
}

func (s *Store) Get(ctx context.Context, key string) (incidents.Record, bool, error) {
	var record incidents.Record
	var evidenceJSON []byte
	record.Key = key
	err := s.pool.QueryRow(ctx, `
		SELECT family, policy_version, source_kind, severity, state, sample_count,
		       occurrence_no, recovery_count, COALESCE(current_value, ''),
		       evidence_refs, COALESCE(material_hash, ''), latest_payload
		FROM relay_ops.incidents WHERE incident_key=$1`, key).Scan(
		&record.Family, &record.PolicyVersion, &record.SourceKind, &record.Severity,
		&record.State, &record.SampleCount, &record.OccurrenceNo, &record.RecoveryCount,
		&record.CurrentValue, &evidenceJSON, &record.MaterialHash, &record.LatestPayload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return incidents.Record{}, false, nil
	}
	if err != nil {
		return incidents.Record{}, false, fmt.Errorf("get incident state: %w", err)
	}
	var evidence []string
	if err := json.Unmarshal(evidenceJSON, &evidence); err != nil {
		return incidents.Record{}, false, fmt.Errorf("decode incident evidence: %w", err)
	}
	if len(evidence) > 0 {
		record.EvidenceHash = evidence[0]
	}
	return record, true, nil
}

func (s *Store) Put(ctx context.Context, record incidents.Record) error {
	if record.OccurrenceNo <= 0 {
		record.OccurrenceNo = 1
	}
	if record.Family == "" {
		record.Family = "legacy"
	}
	if record.SourceKind == "" {
		record.SourceKind = "legacy"
	}
	if record.PolicyVersion < 0 || record.RecoveryCount < 0 {
		return fmt.Errorf("incident notification metadata is invalid")
	}
	if record.State == "recovered" {
		record.RecoveryCount = 0
	}
	var latestPayload any
	if len(record.LatestPayload) > 0 {
		if len(record.LatestPayload) > 30<<10 || !json.Valid(record.LatestPayload) {
			return fmt.Errorf("incident latest payload is invalid")
		}
		latestPayload = string(record.LatestPayload)
	}
	evidence := []string{}
	if record.EvidenceHash != "" {
		evidence = append(evidence, record.EvidenceHash)
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("encode incident state evidence: %w", err)
	}
	for {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin incident state update: %w", err)
		}
		var previousOccurrence int64
		var claimToken sql.NullString
		err = tx.QueryRow(ctx, `
			SELECT occurrence_no, escalation_claim_token
			FROM relay_ops.incidents
			WHERE incident_key=$1
			FOR UPDATE`, record.Key).Scan(&previousOccurrence, &claimToken)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return fmt.Errorf("lock incident state: %w", err)
		}
		if claimToken.Valid && claimToken.String != "" {
			tx.Rollback(ctx)
			if err := waitForIncidentOperation(ctx); err != nil {
				return err
			}
			continue
		}
		inFlight, err := hasInFlightNotificationDelivery(ctx, tx, record.Key, record.OccurrenceNo)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if inFlight {
			tx.Rollback(ctx)
			if err := waitForIncidentOperation(ctx); err != nil {
				return err
			}
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO relay_ops.incidents
				(incident_key, family, policy_version, source_kind, severity, state,
				 current_value, sample_count, occurrence_no, recovery_count,
				 evidence_refs, material_hash, latest_payload)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10,
			        $11, NULLIF($12, ''), $13::jsonb)
			ON CONFLICT (incident_key) DO UPDATE SET
				severity=EXCLUDED.severity,
				state=EXCLUDED.state,
				current_value=EXCLUDED.current_value,
				sample_count=EXCLUDED.sample_count,
				occurrence_no=EXCLUDED.occurrence_no,
				family=EXCLUDED.family,
				policy_version=EXCLUDED.policy_version,
				source_kind=EXCLUDED.source_kind,
				recovery_count=EXCLUDED.recovery_count,
				evidence_refs=EXCLUDED.evidence_refs,
				material_hash=EXCLUDED.material_hash,
				latest_payload=EXCLUDED.latest_payload,
				escalation_level=CASE
					WHEN relay_ops.incidents.occurrence_no<>EXCLUDED.occurrence_no OR EXCLUDED.state='recovered' THEN 0
					ELSE relay_ops.incidents.escalation_level
				END,
				next_escalation_at=CASE
					WHEN relay_ops.incidents.occurrence_no<>EXCLUDED.occurrence_no OR EXCLUDED.state='recovered' THEN NULL
					ELSE relay_ops.incidents.next_escalation_at
				END,
				escalation_claim_token=CASE
					WHEN relay_ops.incidents.occurrence_no<>EXCLUDED.occurrence_no OR EXCLUDED.state='recovered' THEN NULL
					ELSE relay_ops.incidents.escalation_claim_token
				END,
				escalation_claimed_at=CASE
					WHEN relay_ops.incidents.occurrence_no<>EXCLUDED.occurrence_no OR EXCLUDED.state='recovered' THEN NULL
					ELSE relay_ops.incidents.escalation_claimed_at
				END,
				last_seen_at=NOW()`,
			record.Key, record.Family, record.PolicyVersion, record.SourceKind,
			record.Severity, record.State, record.CurrentValue, record.SampleCount,
			record.OccurrenceNo, record.RecoveryCount, evidenceJSON,
			record.MaterialHash, latestPayload)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("put incident state: %w", err)
		}
		newOccurrence := previousOccurrence > 0 && previousOccurrence != record.OccurrenceNo
		if record.State == "recovered" || record.State == "suppressed" || record.State == "closed" || newOccurrence {
			if _, err := tx.Exec(ctx, `
				UPDATE relay_ops.notification_deliveries
				SET delivery_status='canceled', next_attempt_at=NULL
				WHERE incident_id=(SELECT id FROM relay_ops.incidents WHERE incident_key=$1)
				  AND delivery_status IN ('failed', 'reserved')
				  AND (
				    ($3 AND occurrence_no=$2)
				    OR ($4 AND occurrence_no<>$2)
				  )`,
				record.Key, record.OccurrenceNo,
				record.State == "recovered" || record.State == "suppressed" || record.State == "closed",
				newOccurrence); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("cancel inactive incident notification retries: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit incident state: %w", err)
		}
		return nil
	}
}

func waitForIncidentOperation(ctx context.Context) error {
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func hasInFlightNotificationDelivery(ctx context.Context, tx pgx.Tx, key string, occurrenceNo int64) (bool, error) {
	var inFlight bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM relay_ops.notification_deliveries d
			JOIN relay_ops.incidents i ON i.id=d.incident_id
			WHERE i.incident_key=$1
			  AND d.occurrence_no=$2
			  AND d.delivery_status='reserved'
			  AND d.created_at>NOW()-INTERVAL '2 minutes'
		)`, key, occurrenceNo).Scan(&inFlight); err != nil {
		return false, fmt.Errorf("read in-flight incident notification: %w", err)
	}
	return inFlight, nil
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

func (s *Store) Find(ctx context.Context, incidentKey string) (agent.Analysis, bool, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT analysis.result FROM relay_ops.agent_analyses analysis JOIN relay_ops.incidents incident ON incident.id=analysis.incident_id WHERE incident.incident_key=$1 ORDER BY analysis.id DESC LIMIT 1`, incidentKey).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.Analysis{}, false, nil
	}
	if err != nil {
		return agent.Analysis{}, false, fmt.Errorf("find agent analysis: %w", err)
	}
	var result agent.Analysis
	if err := json.Unmarshal(payload, &result); err != nil {
		return agent.Analysis{}, false, fmt.Errorf("decode agent analysis: %w", err)
	}
	return result, true, nil
}

func (s *Store) Save(ctx context.Context, incidentKey string, analysis agent.Analysis, fallback bool) error {
	payload, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("encode agent analysis: %w", err)
	}
	hashBytes := sha256.Sum256(payload)
	analysisIDBytes := sha256.Sum256([]byte("relay-ops-analysis:" + incidentKey))
	provider := "agent"
	delivery := "ready"
	if fallback {
		provider = "deterministic"
		delivery = "fallback"
	}
	command, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.agent_analyses
			(analysis_id, incident_id, model_provider, prompt_contract_version, result, confidence, requires_human_approval, delivery_status, output_hash)
		SELECT $2, id, $3, 'relay-ops-incident-v1', $4, $5, $6, $7, $8
		FROM relay_ops.incidents WHERE incident_key=$1
		ON CONFLICT (analysis_id) DO NOTHING`, incidentKey, hex.EncodeToString(analysisIDBytes[:]), provider, payload, analysis.Confidence, analysis.RequiresHumanApproval, delivery, hex.EncodeToString(hashBytes[:]))
	if err != nil {
		return fmt.Errorf("save agent analysis: %w", err)
	}
	if command.RowsAffected() == 0 {
		if _, found, findErr := s.Find(ctx, incidentKey); findErr != nil || !found {
			return fmt.Errorf("agent incident not found")
		}
	}
	return nil
}

func (s *Store) ListIncidentSummaries(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT severity, state, incident_key, COALESCE(current_value, '')
		FROM relay_ops.incidents
		WHERE state NOT IN ('recovered', 'muted')
		ORDER BY last_seen_at DESC, id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list incident summaries: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var severity, state, key, current string
		if err := rows.Scan(&severity, &state, &key, &current); err != nil {
			return nil, fmt.Errorf("scan incident summary: %w", err)
		}
		if current == "" {
			result = append(result, severity+" "+state+" "+key)
		} else {
			result = append(result, severity+" "+state+" "+key+"："+current)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident summaries: %w", err)
	}
	return result, nil
}

func (s *Store) ListAgentSummaries(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT incident.incident_key, analysis.result
		FROM relay_ops.agent_analyses analysis
		JOIN relay_ops.incidents incident ON incident.id=analysis.incident_id
		ORDER BY analysis.created_at DESC, analysis.id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list Agent summaries: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var key string
		var payload []byte
		if err := rows.Scan(&key, &payload); err != nil {
			return nil, fmt.Errorf("scan Agent summary: %w", err)
		}
		var analysis agent.Analysis
		if err := json.Unmarshal(payload, &analysis); err != nil {
			continue
		}
		text := analysis.Summary
		if text == "" {
			text = analysis.RecommendedAction
		}
		result = append(result, key+"："+text)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Agent summaries: %w", err)
	}
	return result, nil
}

func (s *Store) ReserveNotification(ctx context.Context, reservation notify.Reservation) (int64, bool, error) {
	if reservation.IncidentKey == "" || reservation.DedupKey == "" || reservation.MessageHash == "" ||
		reservation.OccurrenceNo <= 0 || reservation.Transition == "" || !json.Valid(reservation.Payload) {
		return 0, false, fmt.Errorf("notification identity is incomplete")
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		WITH target_incident AS (
			SELECT id
			FROM relay_ops.incidents
			WHERE incident_key=$1
			  AND occurrence_no=$4
			  AND (
			    ($5='recovered' AND state='recovered')
			    OR ($5='manual_resolved' AND state='closed')
			    OR
			    ($5 NOT IN ('recovered', 'manual_resolved') AND state IN ('confirmed', 'escalated', 'degraded'))
			  )
			FOR UPDATE
		)
		INSERT INTO relay_ops.notification_deliveries
			(incident_id, dedup_key, message_hash, delivery_status, occurrence_no, transition,
			 message_payload, attempt_count)
		SELECT id, $2, $3, 'reserved', $4, $5, $6::jsonb, 1
		FROM target_incident
		WHERE (
		    $5 NOT IN ('recovered', 'manual_resolved')
		    OR EXISTS (
		      SELECT 1
		      FROM relay_ops.notification_deliveries delivered
		      WHERE delivered.incident_id=target_incident.id
		        AND delivered.occurrence_no=$4
		        AND delivered.delivery_status='delivered'
		        AND delivered.transition NOT IN ('recovered', 'manual_resolved')
		    )
		  )
		ON CONFLICT (dedup_key) DO UPDATE
		SET message_hash=EXCLUDED.message_hash,
		    delivery_status='reserved',
		    occurrence_no=EXCLUDED.occurrence_no,
		    transition=EXCLUDED.transition,
		    response_code=NULL,
		    delivered_at=NULL,
		    message_payload=EXCLUDED.message_payload,
		    message_id=NULL,
		    urgent_status=NULL,
		    urgent_response_code=NULL,
		    attempt_count=notification_deliveries.attempt_count+1,
		    next_attempt_at=NULL,
		    created_at=NOW()
		WHERE notification_deliveries.delivery_status='failed'
		RETURNING id`, reservation.IncidentKey, reservation.DedupKey, reservation.MessageHash,
		reservation.OccurrenceNo, reservation.Transition, string(reservation.Payload)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if scanErr := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay_ops.notification_deliveries WHERE dedup_key=$1)`, reservation.DedupKey).Scan(&exists); scanErr != nil {
			return 0, false, fmt.Errorf("check notification reservation: %w", scanErr)
		}
		if exists {
			return 0, false, nil
		}
		var incidentExists bool
		if scanErr := s.pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM relay_ops.incidents
				WHERE incident_key=$1 AND occurrence_no=$2
			)`, reservation.IncidentKey, reservation.OccurrenceNo).Scan(&incidentExists); scanErr != nil {
			return 0, false, fmt.Errorf("check notification incident: %w", scanErr)
		}
		if incidentExists {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("incident not found for notification")
	}
	if err != nil {
		return 0, false, fmt.Errorf("reserve notification: %w", err)
	}
	return id, true, nil
}

func (s *Store) FinishNotification(ctx context.Context, deliveryID int64, outcome notify.DeliveryOutcome) error {
	if deliveryID <= 0 || (outcome.Status != "delivered" && outcome.Status != "failed") {
		return fmt.Errorf("notification delivery status is invalid")
	}
	var code any
	if outcome.ResponseCode > 0 {
		code = outcome.ResponseCode
	}
	var deliveredAt any
	if outcome.Status == "delivered" {
		deliveredAt = time.Now().UTC()
	}
	var payload any
	if len(outcome.Payload) > 0 {
		if !json.Valid(outcome.Payload) {
			return fmt.Errorf("notification delivery payload is invalid")
		}
		payload = string(outcome.Payload)
	}
	var urgentCode any
	if outcome.UrgentResponseCode > 0 {
		urgentCode = outcome.UrgentResponseCode
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification delivery finish: %w", err)
	}
	defer tx.Rollback(ctx)
	var incidentID int64
	if err := tx.QueryRow(ctx, `
		SELECT i.id
		FROM relay_ops.notification_deliveries d
		JOIN relay_ops.incidents i ON i.id=d.incident_id
		WHERE d.id=$1
		FOR UPDATE OF i`, deliveryID).Scan(&incidentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("notification delivery not found")
		}
		return fmt.Errorf("lock notification incident: %w", err)
	}
	command, err := tx.Exec(ctx, `
		UPDATE relay_ops.notification_deliveries
		SET delivery_status=$2, response_code=$3, delivered_at=$4,
		    message_payload=$5::jsonb, message_id=NULLIF($6, ''),
		    urgent_status=NULLIF($7, ''), urgent_response_code=$8,
		    next_attempt_at=CASE
		      WHEN $2='failed' AND attempt_count<5 THEN
		        NOW() + CASE attempt_count
		          WHEN 1 THEN INTERVAL '1 minute'
		          WHEN 2 THEN INTERVAL '2 minutes'
		          WHEN 3 THEN INTERVAL '5 minutes'
		          ELSE INTERVAL '10 minutes'
		        END
		      ELSE NULL
		    END
		WHERE id=$1 AND delivery_status='reserved'`, deliveryID, outcome.Status, code, deliveredAt, payload,
		outcome.MessageID, outcome.UrgentStatus, urgentCode)
	if err != nil {
		return fmt.Errorf("finish notification delivery: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery not found")
	}
	if outcome.Status == "delivered" {
		var occurrenceNo, deliveryOccurrence int64
		var severity, state, transition string
		var escalationLevel int
		var firstDeliveredAt time.Time
		err = tx.QueryRow(ctx, `
			SELECT i.id, i.severity, i.state, i.occurrence_no, i.escalation_level,
			       d.occurrence_no, d.transition,
			       MIN(history.delivered_at)
			FROM relay_ops.notification_deliveries d
			JOIN relay_ops.incidents i ON i.id=d.incident_id
			JOIN relay_ops.notification_deliveries history
			  ON history.incident_id=i.id
			 AND history.occurrence_no=d.occurrence_no
			 AND history.delivery_status='delivered'
			 AND history.transition<>'recovered'
			WHERE d.id=$1
			GROUP BY i.id, d.occurrence_no, d.transition`, deliveryID).Scan(
			&incidentID, &severity, &state, &occurrenceNo, &escalationLevel,
			&deliveryOccurrence, &transition, &firstDeliveredAt,
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read initial escalation schedule: %w", err)
		}
		active := state == "confirmed" || state == "escalated" || state == "degraded"
		if err == nil && active && transition != "recovered" && deliveryOccurrence == occurrenceNo {
			var deadline any
			if next, found := alerting.NextEscalationAt(severity, escalationLevel, firstDeliveredAt); found {
				deadline = next
			}
			if _, err := tx.Exec(ctx, `
				UPDATE relay_ops.incidents
				SET next_escalation_at=$2
				WHERE id=$1 AND occurrence_no=$3`, incidentID, deadline, occurrenceNo); err != nil {
				return fmt.Errorf("schedule initial incident escalation: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification delivery finish: %w", err)
	}
	return nil
}

func (s *Store) ClaimNotificationRetry(ctx context.Context, now time.Time) (*notify.RetryDelivery, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("notification retry claim time is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin notification retry claim: %w", err)
	}
	defer tx.Rollback(ctx)
	var delivery notify.RetryDelivery
	err = tx.QueryRow(ctx, `
		SELECT d.id, i.incident_key, i.severity, d.occurrence_no, d.transition, d.message_payload
		FROM relay_ops.notification_deliveries d
		JOIN relay_ops.incidents i ON i.id=d.incident_id
		WHERE d.attempt_count<5
		  AND d.message_payload IS NOT NULL
		  AND (
		    (d.delivery_status='failed' AND d.next_attempt_at<=$1)
		    OR (d.delivery_status='reserved' AND d.created_at<=$1-INTERVAL '2 minutes')
		  )
		  AND i.occurrence_no=d.occurrence_no
		  AND (
		    (d.transition='recovered' AND i.state='recovered')
		    OR (d.transition='manual_resolved' AND i.state='closed')
		    OR (
		      d.transition NOT IN ('recovered', 'manual_resolved')
		      AND i.state IN ('confirmed', 'escalated', 'degraded')
		    )
		  )
		ORDER BY COALESCE(d.next_attempt_at, d.created_at), d.id
		FOR UPDATE OF i SKIP LOCKED
		LIMIT 1`, now.UTC()).Scan(
		&delivery.ID, &delivery.IncidentKey, &delivery.Severity,
		&delivery.OccurrenceNo, &delivery.Transition, &delivery.Payload,
	)
	if err == nil {
		delivery.Kind = "incident"
		if _, err := tx.Exec(ctx, `
			UPDATE relay_ops.notification_deliveries
			SET delivery_status='reserved', attempt_count=attempt_count+1,
			    next_attempt_at=NULL, created_at=$2
			WHERE id=$1`, delivery.ID, now.UTC()); err != nil {
			return nil, fmt.Errorf("lease notification retry: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit notification retry claim: %w", err)
		}
		return &delivery, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("claim notification retry: %w", err)
	}

	delivery = notify.RetryDelivery{}
	err = tx.QueryRow(ctx, `
		SELECT id, notification_key, 'P2', message_payload
		FROM relay_ops.notification_messages
		WHERE attempt_count<5
		  AND message_payload IS NOT NULL
		  AND (
		    (delivery_status='failed' AND next_attempt_at<=$1)
		    OR (delivery_status='reserved' AND updated_at<=$1-INTERVAL '2 minutes')
		  )
		ORDER BY COALESCE(next_attempt_at, updated_at), id
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, now.UTC()).Scan(
		&delivery.ID, &delivery.NotificationKey, &delivery.Severity, &delivery.Payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty notification retry claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim one-shot notification retry: %w", err)
	}
	delivery.Kind = "one_shot"
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.notification_messages
		SET delivery_status='reserved', attempt_count=attempt_count+1,
		    next_attempt_at=NULL, updated_at=$2
		WHERE id=$1`, delivery.ID, now.UTC()); err != nil {
		return nil, fmt.Errorf("lease one-shot notification retry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit one-shot notification retry claim: %w", err)
	}
	return &delivery, nil
}

func (s *Store) ClaimDueEscalation(ctx context.Context, now time.Time) (*alerting.Incident, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("incident escalation claim time is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin incident escalation claim: %w", err)
	}
	defer tx.Rollback(ctx)
	claimToken, err := newEscalationClaimToken()
	if err != nil {
		return nil, err
	}
	var id int64
	var incident alerting.Incident
	err = tx.QueryRow(ctx, `
		SELECT i.id, i.incident_key, i.severity, i.occurrence_no, i.escalation_level,
		       first_delivery.delivered_at, i.current_value
		FROM relay_ops.incidents i
		JOIN LATERAL (
			SELECT MIN(d.delivered_at) AS delivered_at
			FROM relay_ops.notification_deliveries d
			WHERE d.incident_id=i.id
			  AND d.occurrence_no=i.occurrence_no
			  AND d.delivery_status='delivered'
			  AND d.transition<>'recovered'
		) first_delivery ON first_delivery.delivered_at IS NOT NULL
		WHERE i.next_escalation_at<=$1
		  AND i.severity IN ('P0', 'P1')
		  AND i.state IN ('confirmed', 'escalated', 'degraded')
		  AND (
		    i.escalation_claim_token IS NULL
		    OR i.escalation_claimed_at<=$1-INTERVAL '2 minutes'
		  )
		ORDER BY i.next_escalation_at, i.id
		FOR UPDATE OF i SKIP LOCKED
		LIMIT 1`, now.UTC()).Scan(
		&id, &incident.Key, &incident.Severity, &incident.OccurrenceNo,
		&incident.EscalationLevel, &incident.FirstDeliveredAt, &incident.CurrentValue,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit empty incident escalation claim: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim due incident escalation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET next_escalation_at=$2, escalation_claim_token=$3, escalation_claimed_at=$4
		WHERE id=$1`, id, now.UTC().Add(time.Minute), claimToken, now.UTC()); err != nil {
		return nil, fmt.Errorf("lease incident escalation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit incident escalation claim: %w", err)
	}
	incident.ClaimToken = claimToken
	return &incident, nil
}

func (s *Store) FinishEscalation(ctx context.Context, result alerting.Result) error {
	if result.Key == "" || result.OccurrenceNo <= 0 || result.Level <= 0 || result.ClaimToken == "" {
		return fmt.Errorf("incident escalation result is invalid")
	}
	var next any
	if result.Succeeded {
		if result.NextEscalationAt != nil {
			next = result.NextEscalationAt.UTC()
		}
	} else {
		if result.RetryAt.IsZero() {
			return fmt.Errorf("incident escalation retry time is required")
		}
		next = result.RetryAt.UTC()
	}
	if result.Succeeded {
		command, err := s.pool.Exec(ctx, `
			UPDATE relay_ops.incidents
			SET escalation_level=$3, next_escalation_at=$4,
			    escalation_claim_token=NULL, escalation_claimed_at=NULL
			WHERE incident_key=$1 AND occurrence_no=$2
			  AND escalation_level=$3-1
			  AND escalation_claim_token=$5
			  AND state IN ('confirmed', 'escalated', 'degraded')`,
			result.Key, result.OccurrenceNo, result.Level, next, result.ClaimToken)
		if err != nil {
			return fmt.Errorf("finish successful incident escalation: %w", err)
		}
		if command.RowsAffected() != 1 {
			return fmt.Errorf("incident escalation claim is no longer current")
		}
		return nil
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET next_escalation_at=$4, escalation_claim_token=NULL, escalation_claimed_at=NULL
		WHERE incident_key=$1 AND occurrence_no=$2
		  AND escalation_level=$3-1
		  AND escalation_claim_token=$5
		  AND state IN ('confirmed', 'escalated', 'degraded')`,
		result.Key, result.OccurrenceNo, result.Level, next, result.ClaimToken)
	if err != nil {
		return fmt.Errorf("finish failed incident escalation: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("incident escalation claim is no longer current")
	}
	return nil
}

func newEscalationClaimToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate incident escalation claim token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
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
