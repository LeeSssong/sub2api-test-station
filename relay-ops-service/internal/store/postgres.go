package store

import (
	"context"
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
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/probes"
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

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

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
		SELECT severity, state, sample_count, COALESCE(current_value, ''), evidence_refs
		FROM relay_ops.incidents WHERE incident_key=$1`, key).Scan(
		&record.Severity, &record.State, &record.SampleCount, &record.CurrentValue, &evidenceJSON,
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
	evidence := []string{}
	if record.EvidenceHash != "" {
		evidence = append(evidence, record.EvidenceHash)
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("encode incident state evidence: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO relay_ops.incidents
			(incident_key, severity, state, current_value, sample_count, evidence_refs)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		ON CONFLICT (incident_key) DO UPDATE SET
			severity=EXCLUDED.severity,
			state=EXCLUDED.state,
			current_value=EXCLUDED.current_value,
			sample_count=EXCLUDED.sample_count,
			evidence_refs=EXCLUDED.evidence_refs,
			last_seen_at=NOW()`,
		record.Key, record.Severity, record.State, record.CurrentValue, record.SampleCount, evidenceJSON)
	if err != nil {
		return fmt.Errorf("put incident state: %w", err)
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
