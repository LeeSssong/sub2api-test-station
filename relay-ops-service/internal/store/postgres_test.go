package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestMigrateIsIdempotentAndUpstreamIdentityIsUnique(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	first := Upstream{Name: "Neko", Role: "production", BaseURL: "https://api.neko.example/v1", AdapterType: "sub2api", Enabled: true}
	if _, err := st.CreateUpstream(ctx, first); err != nil {
		t.Fatalf("CreateUpstream: %v", err)
	}
	if _, err := st.CreateUpstream(ctx, first); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name error = %v, want ErrConflict", err)
	}
	second := first
	second.Name = "Neko duplicate"
	if _, err := st.CreateUpstream(ctx, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate base URL error = %v, want ErrConflict", err)
	}
}

func TestPricingSnapshotsAreAppendOnlyAndIncidentsDeduplicate(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID, err := st.CreateUpstream(ctx, Upstream{Name: "wawazz", Role: "candidate", BaseURL: "https://wawazz.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	for _, hash := range []string{"hash-one", "hash-two"} {
		if _, err := st.AppendPricingSnapshot(ctx, PricingSnapshot{UpstreamID: upstreamID, SourceURL: "https://wawazz.example/pricing", SourceType: "public_page", FetchedAt: fetchedAt, ContentHash: hash, NormalizedJSON: []byte(`{"multiplier_bps":500}`), EvidenceLevel: "public_page"}); err != nil {
			t.Fatalf("AppendPricingSnapshot: %v", err)
		}
	}
	count, err := st.CountPricingSnapshots(ctx, upstreamID)
	if err != nil || count != 2 {
		t.Fatalf("snapshot count = %d, err = %v", count, err)
	}

	incident := Incident{IncidentKey: "upstream:wawazz:multiplier", Severity: "P1", State: "confirmed", CurrentValue: "0.05", EvidenceRefs: []string{"snapshot:1"}}
	firstID, inserted, err := st.UpsertIncident(ctx, incident)
	if err != nil || !inserted {
		t.Fatalf("first UpsertIncident = id %d inserted %v err %v", firstID, inserted, err)
	}
	secondID, inserted, err := st.UpsertIncident(ctx, incident)
	if err != nil || inserted || secondID != firstID {
		t.Fatalf("second UpsertIncident = id %d inserted %v err %v", secondID, inserted, err)
	}
}

func TestNativeSyncPreservesQualificationAndDeduplicatesMetricRefs(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	seenAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	record := sub2api.PublicGroupRecord{
		GroupID: 3, Name: "GPT-Pro", Platform: "openai", Enabled: true, CustomerVisible: true,
		UserMultiplierBPS: 10_000, ChannelIDs: []int64{7}, MonitorIDs: []int64{9},
		HealthGate: "pending", SourceRevision: "revision-one", LastSeenAt: seenAt,
	}
	if err := st.UpsertPublicGroup(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `UPDATE relay_ops.public_groups SET health_gate = 'qualified' WHERE group_id = 3`); err != nil {
		t.Fatal(err)
	}
	record.SourceRevision = "revision-two"
	if err := st.UpsertPublicGroup(ctx, record); err != nil {
		t.Fatal(err)
	}
	var gate, revision string
	if err := st.pool.QueryRow(ctx, `SELECT health_gate, source_revision FROM relay_ops.public_groups WHERE group_id = 3`).Scan(&gate, &revision); err != nil {
		t.Fatal(err)
	}
	if gate != "qualified" || revision != "revision-two" {
		t.Fatalf("gate=%q revision=%q", gate, revision)
	}

	ref := sub2api.MetricRef{SourceKind: "sub2api_native_monitor", ExternalID: "monitor:9", WindowStart: seenAt, WindowEnd: seenAt.Add(time.Minute), PayloadHash: "abc", SchemaVersion: sub2api.NativeMetricSchemaV1}
	if err := st.AppendMetricRef(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendMetricRef(ctx, ref); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.metric_refs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("metric ref count = %d", count)
	}
}

func TestCreateCandidatePersistsSecretMetadataAndAuditAtomically(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := candidates.CreateRecord{
		Candidate: candidates.Candidate{Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing", UsageURL: "https://candidate.example/usage", ProbeSecretRef: "file:/run/secrets/candidate"},
		SecretRef: candidates.SecretRef{SecretRef: "file:/run/secrets/candidate", Kind: "candidate_probe_key", OwnerScope: "candidate", Fingerprint: "fingerprint", LastFour: "5678"},
		Audit:     candidates.AuditEvent{ActorUserID: 42, Action: "candidate.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "candidate"}},
	}
	id, err := st.CreateCandidate(ctx, record)
	if err != nil || id == 0 {
		t.Fatalf("CreateCandidate = %d, %v", id, err)
	}
	var secretRef, fingerprint, lastFour string
	if err := st.pool.QueryRow(ctx, `SELECT secret_ref, fingerprint, last_four FROM relay_ops.secret_refs WHERE owner_scope = 'candidate'`).Scan(&secretRef, &fingerprint, &lastFour); err != nil {
		t.Fatal(err)
	}
	if secretRef != "file:/run/secrets/candidate" || fingerprint != "fingerprint" || lastFour != "5678" {
		t.Fatalf("secret metadata = %q %q %q", secretRef, fingerprint, lastFour)
	}
	var auditCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE actor_user_id = 42 AND action = 'candidate.create'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
	if _, err := st.CreateCandidate(ctx, record); !errors.Is(err, candidates.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestListAndDisableCandidatePersistOnlyStateAndAudit(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := candidates.CreateRecord{
		Candidate: candidates.Candidate{Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing", UsageURL: "https://candidate.example/usage", ProbeSecretRef: "file:/run/secrets/candidate"},
		SecretRef: candidates.SecretRef{SecretRef: "file:/run/secrets/candidate", Kind: "candidate_probe_key", OwnerScope: "candidate-list", Fingerprint: "fingerprint", LastFour: "5678"},
		Audit:     candidates.AuditEvent{ActorUserID: 42, Action: "candidate.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "candidate"}},
	}
	id, err := st.CreateCandidate(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListCandidates(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != id || listed[0].ProbeSecretRef != record.SecretRef.SecretRef {
		t.Fatalf("ListCandidates = %#v, %v", listed, err)
	}
	audit := candidates.AuditEvent{ActorUserID: 99, Action: "candidate.disable", ObjectType: "upstream", AfterSummary: map[string]string{"enabled": "false"}}
	if err := st.DisableCandidate(ctx, id, audit); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := st.pool.QueryRow(ctx, `SELECT enabled FROM relay_ops.upstreams WHERE id = $1`, id).Scan(&enabled); err != nil || enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	var auditCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE actor_user_id = 99 AND action = 'candidate.disable'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}

func TestUsageSessionNotificationsSuppressForTwentyFourHoursAndCostsAppend(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateUpstream(ctx, Upstream{Name: "usage-upstream", Role: "candidate", BaseURL: "https://usage-upstream.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope) VALUES ($1, 'file:/secret', 'cookie', 'active', 'https://usage-upstream.example/login', 'usage_read')`, id); err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	notify, err := st.RecordExpired(ctx, id, "https://usage-upstream.example/login", observed)
	if err != nil || !notify {
		t.Fatalf("first expired notify=%v err=%v", notify, err)
	}
	notify, err = st.RecordExpired(ctx, id, "https://usage-upstream.example/login", observed.Add(time.Hour))
	if err != nil || notify {
		t.Fatalf("suppressed expired notify=%v err=%v", notify, err)
	}
	notify, err = st.RecordExpired(ctx, id, "https://usage-upstream.example/login", observed.Add(25*time.Hour))
	if err != nil || !notify {
		t.Fatalf("next-day expired notify=%v err=%v", notify, err)
	}
	if err := st.RecordHealthy(ctx, id, observed.Add(26*time.Hour)); err != nil {
		t.Fatal(err)
	}
	evidence := billing.UsageEvidence{UpstreamID: id, ObservedAt: observed, StandardCost: 10_000_000, ActualCost: 1_000_000, HasActualCost: true, EffectiveMultiplier: 1_000, Note: "辅助证据"}
	if err := st.AppendCostObservation(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	var status string
	var costCount int
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.auth_sessions WHERE upstream_id=$1`, id).Scan(&status); err != nil || status != "active" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.cost_observations WHERE upstream_id=$1`, id).Scan(&costCount); err != nil || costCount != 1 {
		t.Fatalf("cost count=%d err=%v", costCount, err)
	}
}

func TestIncidentMachineStatePersistsInPostgreSQL(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	observation := incidents.Observation{Key: "upstream:17:price", Severity: "P1", Failing: true, EvidenceHash: "snapshot:1", CurrentValue: "0.10"}
	if _, err := machine.Observe(ctx, observation); err != nil {
		t.Fatal(err)
	}
	transition, err := machine.Observe(ctx, observation)
	if err != nil || !transition.Notify || transition.Kind != "confirmed" {
		t.Fatalf("transition=%#v err=%v", transition, err)
	}
	record, ok, err := st.Get(ctx, observation.Key)
	if err != nil || !ok || record.State != "confirmed" || record.SampleCount != 2 {
		t.Fatalf("record=%#v ok=%v err=%v", record, ok, err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("RELAY_OPS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("RELAY_OPS_TEST_DATABASE_URL is not set")
	}
	secret := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(secret, []byte(url), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(context.Background(), secret)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(st.Close)
	if _, err := st.pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS relay_ops CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	return st
}

var _ domain.UpstreamID
