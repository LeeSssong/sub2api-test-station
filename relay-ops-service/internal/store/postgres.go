package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/sub2api"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var initialMigration string

var ErrConflict = errors.New("record conflicts with existing identity")

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

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, initialMigration); err != nil {
		return fmt.Errorf("migrate relay ops schema: %w", err)
	}
	return nil
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
