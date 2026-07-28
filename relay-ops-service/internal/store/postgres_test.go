package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
)

func TestQualityReportsAreAppendOnlyAndRetrievable(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID, err := st.CreateUpstream(ctx, Upstream{Name: "candidate", Role: "candidate", BaseURL: "https://candidate.example/v1", AdapterType: "openai", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	report := qualityreports.Report{
		ReportID: "fast-1", ReportHash: strings.Repeat("a", 64), UpstreamID: upstreamID, UpstreamName: "candidate",
		JobKind: "health_pulse", Status: "needs_evidence", QualityScore: 85, TotalScore: 85,
		Direct: "6/6", Gateway: "unknown", Models: "3 selected", Pricing: "unknown", Capacity: "unknown",
		Record: json.RawMessage(`{"run_id":"fast-1"}`), RecordedAt: time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 7, 22, 3, 30, 0, 0, time.UTC),
	}
	if err := st.PutQualityReport(ctx, report); err != nil {
		t.Fatalf("PutQualityReport: %v", err)
	}
	if err := st.PutQualityReport(ctx, report); err != nil {
		t.Fatalf("idempotent PutQualityReport: %v", err)
	}
	changed := report
	changed.ReportHash = strings.Repeat("b", 64)
	if err := st.PutQualityReport(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed report error = %v", err)
	}
	got, found, err := st.GetQualityReport(ctx, report.ReportID)
	if err != nil || !found || got.ReportHash != report.ReportHash || got.QualityScore != 85 {
		t.Fatalf("GetQualityReport = %#v, %v, %v", got, found, err)
	}
	items, err := st.ListQualityReports(ctx, 10)
	if err != nil || len(items) != 1 || items[0].ReportID != report.ReportID {
		t.Fatalf("ListQualityReports = %#v, %v", items, err)
	}
}

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

func TestNotificationDeliveryRetriesFailedButNotDeliveredEvidence(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	incidentKey := "quality-report:17:health_pulse"
	if _, _, err := st.UpsertIncident(ctx, Incident{IncidentKey: incidentKey, Severity: "P2", State: "confirmed"}); err != nil {
		t.Fatal(err)
	}

	reservation := notify.Reservation{
		IncidentKey: incidentKey, DedupKey: "semantic-evidence",
		MessageHash: "message-one", OccurrenceNo: 1, Transition: "confirmed",
		Payload: []byte(`{"header":{"title":{"content":"P2"}},"elements":[]}`),
	}
	firstID, reserved, err := st.ReserveNotification(ctx, reservation)
	if err != nil || !reserved {
		t.Fatalf("first reservation = %d %v %v", firstID, reserved, err)
	}
	if err := st.FinishNotification(ctx, firstID, notify.DeliveryOutcome{Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	reservation.MessageHash = "message-two"
	secondID, reserved, err := st.ReserveNotification(ctx, reservation)
	if err != nil || !reserved || secondID != firstID {
		t.Fatalf("failed retry = %d %v %v", secondID, reserved, err)
	}
	payload := []byte(`{"header":{"title":{"content":"P0"}},"elements":[]}`)
	if err := st.FinishNotification(ctx, secondID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: 200, MessageID: "om-alert",
		Payload: payload, UrgentStatus: "failed",
	}); err != nil {
		t.Fatal(err)
	}
	reservation.MessageHash = "message-three"
	if _, reserved, err := st.ReserveNotification(ctx, reservation); err != nil || reserved {
		t.Fatalf("delivered duplicate = %v %v", reserved, err)
	}
	var occurrenceNo int64
	var transition, messageID, urgentStatus string
	var storedPayload []byte
	if err := st.pool.QueryRow(ctx, `
		SELECT occurrence_no, transition, message_id, urgent_status, message_payload
		FROM relay_ops.notification_deliveries WHERE id=$1`, secondID).Scan(
		&occurrenceNo, &transition, &messageID, &urgentStatus, &storedPayload,
	); err != nil {
		t.Fatal(err)
	}
	var gotPayload, wantPayload any
	if err := json.Unmarshal(storedPayload, &gotPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &wantPayload); err != nil {
		t.Fatal(err)
	}
	if occurrenceNo != 1 || transition != "confirmed" || messageID != "om-alert" ||
		urgentStatus != "failed" || !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("stored delivery = occurrence %d transition %q message %q urgency %q payload %s",
			occurrenceNo, transition, messageID, urgentStatus, storedPayload)
	}
}

func TestIncidentOccurrencePersistenceAndAcknowledgement(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	observation := incidents.Observation{
		Key:                 "group:GPT-Plus:availability",
		Severity:            "P0",
		Failing:             true,
		EvidenceHash:        "available:0/1",
		CurrentValue:        "可用 0 / 共 1",
		ConfirmationWindows: 1,
	}
	first, err := machine.Observe(ctx, observation)
	if err != nil || first.OccurrenceNo != 1 {
		t.Fatalf("first observation = %#v, %v", first, err)
	}
	acknowledgedAt := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	if err := st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
		Key: observation.Key, OccurrenceNo: 1, ActorUserID: 42, At: acknowledgedAt,
	}); err != nil {
		t.Fatalf("acknowledge current occurrence: %v", err)
	}
	var occurrenceNo, acknowledgedOccurrence int64
	var acknowledgedBy int64
	var storedAcknowledgedAt time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT occurrence_no, acknowledged_occurrence, acknowledged_by, acknowledged_at
		FROM relay_ops.incidents WHERE incident_key=$1`, observation.Key).Scan(
		&occurrenceNo, &acknowledgedOccurrence, &acknowledgedBy, &storedAcknowledgedAt,
	); err != nil {
		t.Fatal(err)
	}
	if occurrenceNo != 1 || acknowledgedOccurrence != 1 || acknowledgedBy != 42 || !storedAcknowledgedAt.Equal(acknowledgedAt) {
		t.Fatalf("stored acknowledgement = occurrence %d acknowledged %d by %d at %s",
			occurrenceNo, acknowledgedOccurrence, acknowledgedBy, storedAcknowledgedAt)
	}

	if _, err := machine.Observe(ctx, incidents.Observation{
		Key: observation.Key, Severity: "P0", Failing: false,
		EvidenceHash: "available:1/1", CurrentValue: "可用 1 / 共 1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
		Key: observation.Key, OccurrenceNo: 1, ActorUserID: 42, At: acknowledgedAt.Add(time.Minute),
	}); !errors.Is(err, incidents.ErrNotActive) {
		t.Fatalf("acknowledge recovered error = %v, want ErrNotActive", err)
	}
	second, err := machine.Observe(ctx, observation)
	if err != nil || second.OccurrenceNo != 2 {
		t.Fatalf("second occurrence = %#v, %v", second, err)
	}
	if err := st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
		Key: observation.Key, OccurrenceNo: 1, ActorUserID: 42, At: acknowledgedAt.Add(2 * time.Minute),
	}); !errors.Is(err, incidents.ErrOccurrenceConflict) {
		t.Fatalf("acknowledge stale occurrence error = %v, want ErrOccurrenceConflict", err)
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
	second := record
	second.Candidate.Name = "candidate-with-reused-key"
	second.Candidate.BaseURL = "https://candidate-2.example/v1"
	second.Candidate.ProbeSecretRef = "file:/run/secrets/candidate-2"
	second.SecretRef.SecretRef = "file:/run/secrets/candidate-2"
	second.SecretRef.OwnerScope = "candidate-with-reused-key"
	second.Audit.AfterSummary = map[string]string{"name": second.Candidate.Name}
	if _, err := st.CreateCandidate(ctx, second); !errors.Is(err, candidates.ErrConflict) {
		t.Fatalf("reused fingerprint error = %v, want ErrConflict", err)
	}
}

func TestCreateProductionUpstreamLinksOnlyCustomerVisibleGroups(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	visible := sub2api.PublicGroupRecord{
		GroupID: 3, Name: "GPT-Pro", Platform: "openai", Enabled: true, CustomerVisible: true,
		UserMultiplierBPS: 10_000, ChannelIDs: []int64{7}, MonitorIDs: []int64{9},
		HealthGate: "qualified", SourceRevision: "visible", LastSeenAt: time.Now().UTC(),
	}
	if err := st.UpsertPublicGroup(ctx, visible); err != nil {
		t.Fatal(err)
	}
	record := upstreams.ProductionRecord{
		Source: upstreams.Source{
			Name: "Neko", Role: upstreams.RoleProduction, BaseURL: "https://neko.example/v1",
			PricingURL: "https://neko.example/pricing", UsageURL: "https://neko.example/usage",
			MonitorID: 9, GroupIDs: []int64{3}, Enabled: true,
		},
		Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "Neko"}},
	}
	id, err := st.CreateProduction(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := st.ListProduction(ctx)
	if err != nil || len(sources) != 1 || sources[0].ID != id || len(sources[0].GroupIDs) != 1 || sources[0].GroupIDs[0] != 3 {
		t.Fatalf("sources = %#v, err = %v", sources, err)
	}
	var secretCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.secret_refs WHERE owner_scope = 'Neko'`).Scan(&secretCount); err != nil || secretCount != 0 {
		t.Fatalf("secret count = %d, err = %v", secretCount, err)
	}

	record.Source.Name = "Missing group"
	record.Source.BaseURL = "https://missing.example/v1"
	record.Source.GroupIDs = []int64{404}
	if _, err := st.CreateProduction(ctx, record); !errors.Is(err, upstreams.ErrGroupUnavailable) {
		t.Fatalf("missing group error = %v", err)
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

func TestSchedulerClaimIsConcurrentAndRestartSafe(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	results := make(chan bool, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claimed, err := st.Claim(ctx, "candidate-cycle:17", now, 6*time.Hour)
			if err != nil {
				t.Errorf("Claim: %v", err)
			}
			results <- claimed
		}()
	}
	wait.Wait()
	close(results)
	claimedCount := 0
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("claimed count=%d", claimedCount)
	}
	if claimed, err := st.Claim(ctx, "candidate-cycle:17", now.Add(5*time.Hour), 6*time.Hour); err != nil || claimed {
		t.Fatalf("early claim=%v err=%v", claimed, err)
	}
	if claimed, err := st.Claim(ctx, "candidate-cycle:17", now.Add(6*time.Hour), 6*time.Hour); err != nil || !claimed {
		t.Fatalf("due claim=%v err=%v", claimed, err)
	}
}

func TestIncidentEscalationClaimAndRetryAreOccurrenceSafe(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:escalation-store-test:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := notify.WithDeliveryIdentity(
		notify.RenderAlert(notify.IncidentView{Title: "告警链路存储测试", Severity: "P0"}),
		transition.OccurrenceNo, transition.Kind,
	)
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "escalation-store-test-initial", MessageHash: "message",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind,
		Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, MessageID: "om-store-test",
		Payload: payload, UrgentStatus: "delivered",
	}); err != nil {
		t.Fatal(err)
	}

	var firstDeliveredAt, firstDue time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT d.delivered_at, i.next_escalation_at
		FROM relay_ops.incidents i
		JOIN relay_ops.notification_deliveries d ON d.incident_id=i.id
		WHERE i.incident_key=$1 AND d.id=$2`, key, deliveryID).Scan(&firstDeliveredAt, &firstDue); err != nil {
		t.Fatal(err)
	}
	if !firstDue.Equal(firstDeliveredAt.Add(5 * time.Minute)) {
		t.Fatalf("first due=%s delivered=%s", firstDue, firstDeliveredAt)
	}
	claim, err := st.ClaimDueEscalation(ctx, firstDue)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Key != key || claim.Severity != "P0" || claim.OccurrenceNo != 1 ||
		claim.EscalationLevel != 0 || !json.Valid(claim.MessagePayload) {
		t.Fatalf("claim=%#v", claim)
	}
	retryAt := firstDue.Add(time.Minute)
	if err := st.FinishEscalation(ctx, alerting.Result{
		Key: key, OccurrenceNo: 1, Level: 1, ClaimToken: claim.ClaimToken,
		Succeeded: false, RetryAt: retryAt,
	}); err != nil {
		t.Fatal(err)
	}
	var level int
	var next time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT escalation_level, next_escalation_at FROM relay_ops.incidents WHERE incident_key=$1`, key).Scan(&level, &next); err != nil {
		t.Fatal(err)
	}
	if level != 0 || !next.Equal(retryAt) {
		t.Fatalf("failed escalation level=%d next=%s", level, next)
	}
	claim, err = st.ClaimDueEscalation(ctx, retryAt)
	if err != nil || claim == nil || claim.EscalationLevel != 0 {
		t.Fatalf("retry claim=%#v err=%v", claim, err)
	}
	secondDue := firstDeliveredAt.Add(15 * time.Minute)
	if err := st.FinishEscalation(ctx, alerting.Result{
		Key: key, OccurrenceNo: 1, Level: 1, ClaimToken: claim.ClaimToken,
		Succeeded: true, NextEscalationAt: &secondDue,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `
		SELECT escalation_level, next_escalation_at FROM relay_ops.incidents WHERE incident_key=$1`, key).Scan(&level, &next); err != nil {
		t.Fatal(err)
	}
	if level != 1 || !next.Equal(secondDue) {
		t.Fatalf("successful escalation level=%d next=%s", level, next)
	}
	if err := st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
		Key: key, OccurrenceNo: 1, ActorUserID: 42, At: retryAt,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err = st.ClaimDueEscalation(ctx, secondDue)
	if err != nil || claim != nil {
		t.Fatalf("acknowledged claim=%#v err=%v", claim, err)
	}
}

func TestRecoveryReservationRequiresDeliveredAlertForOccurrence(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:recovery-delivery-gate:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "recovery-before-alert", MessageHash: "recovery",
		OccurrenceNo: transition.OccurrenceNo, Transition: "recovered",
		Payload: []byte(`{"header":{"title":{"content":"recovered"}},"elements":[]}`),
	}); err != nil || reserved {
		t.Fatalf("recovery before alert reserved=%v err=%v", reserved, err)
	}

	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "initial-alert", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: "confirmed",
		Payload: []byte(`{"header":{"title":{"content":"alert"}},"elements":[]}`),
	})
	if err != nil || !reserved {
		t.Fatalf("initial reserve=%d %v %v", deliveryID, reserved, err)
	}
	payload, err := notify.RenderAlert(notify.IncidentView{Title: "test", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, MessageID: "om-recovery-gate",
		Payload: payload, UrgentStatus: "delivered",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: false, EvidenceHash: "1/1",
		CurrentValue: "可用 1 / 共 1", ConfirmationWindows: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "recovery-after-alert", MessageHash: "recovery",
		OccurrenceNo: transition.OccurrenceNo, Transition: "recovered",
		Payload: []byte(`{"header":{"title":{"content":"recovered"}},"elements":[]}`),
	}); err != nil || !reserved {
		t.Fatalf("recovery after alert reserved=%v err=%v", reserved, err)
	}
}

func TestNonRecoveryReservationIsRejectedAfterIncidentRecovery(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:recovered-reservation-gate:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: false, EvidenceHash: "1/1",
		CurrentValue: "可用 1 / 共 1", ConfirmationWindows: 1,
	}); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"header":{"title":{"content":"stale alert"}},"elements":[]}`)
	if _, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "stale-alert-after-recovery", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	}); err != nil || reserved {
		t.Fatalf("stale alert after recovery reserved=%v err=%v", reserved, err)
	}
}

func TestFailedNotificationBecomesDueForLeasedRetry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:failed-delivery-retry:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := notify.RenderAlert(notify.IncidentView{Title: "test", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "failed-delivery-retry", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "failed", Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	var nextAttempt time.Time
	var attempts int
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at, attempt_count
		FROM relay_ops.notification_deliveries
		WHERE id=$1`, deliveryID).Scan(&nextAttempt, &attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d", attempts)
	}
	if claim, err := st.ClaimNotificationRetry(ctx, nextAttempt.Add(-time.Second)); err != nil || claim != nil {
		t.Fatalf("early claim=%#v err=%v", claim, err)
	}
	claim, err := st.ClaimNotificationRetry(ctx, nextAttempt)
	if err != nil || claim == nil || claim.ID != deliveryID || claim.IncidentKey != key ||
		claim.OccurrenceNo != transition.OccurrenceNo || claim.Transition != transition.Kind ||
		!json.Valid(claim.Payload) {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	if err := st.pool.QueryRow(ctx, `
		SELECT attempt_count FROM relay_ops.notification_deliveries WHERE id=$1`,
		deliveryID).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts after claim=%d err=%v", attempts, err)
	}
}

func TestStaleReservedNotificationIsReclaimed(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:stale-reservation-retry:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: "1/2",
		CurrentValue: "可用 1 / 共 2", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := notify.RenderAlert(notify.IncidentView{Title: "test", Severity: "P1"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "stale-reservation-retry", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.notification_deliveries
		SET created_at=$2
		WHERE id=$1`, deliveryID, now.Add(-3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	claim, err := st.ClaimNotificationRetry(ctx, now)
	if err != nil || claim == nil || claim.ID != deliveryID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
}

func TestAcknowledgementAndRecoveryCancelPendingNotificationRetries(t *testing.T) {
	for _, test := range []struct {
		name string
		stop func(context.Context, *Store, string, int64) error
	}{
		{
			name: "acknowledgement",
			stop: func(ctx context.Context, st *Store, key string, occurrenceNo int64) error {
				return st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
					Key: key, OccurrenceNo: occurrenceNo, ActorUserID: 42, At: time.Now().UTC(),
				})
			},
		},
		{
			name: "recovery",
			stop: func(ctx context.Context, st *Store, key string, occurrenceNo int64) error {
				return st.Put(ctx, incidents.Record{
					Key: key, Severity: "P0", State: "recovered", OccurrenceNo: occurrenceNo,
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := openTestStore(t)
			ctx := context.Background()
			if err := st.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			key := "group:cancel-retry-" + test.name + ":availability"
			machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
			transition, err := machine.Observe(ctx, incidents.Observation{
				Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
				CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			payload := []byte(`{"header":{"title":{"content":"alert"}},"elements":[]}`)
			deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
				IncidentKey: key, DedupKey: "cancel-retry-" + test.name, MessageHash: "alert",
				OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
			})
			if err != nil || !reserved {
				t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
			}
			if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
				Status: "failed", Payload: payload,
			}); err != nil {
				t.Fatal(err)
			}
			if err := test.stop(ctx, st, key, transition.OccurrenceNo); err != nil {
				t.Fatal(err)
			}
			var status string
			var nextAttempt *time.Time
			if err := st.pool.QueryRow(ctx, `
				SELECT delivery_status, next_attempt_at
				FROM relay_ops.notification_deliveries
				WHERE id=$1`, deliveryID).Scan(&status, &nextAttempt); err != nil {
				t.Fatal(err)
			}
			if status != "canceled" || nextAttempt != nil {
				t.Fatalf("status=%q next_attempt=%v", status, nextAttempt)
			}
		})
	}
}

func TestAcknowledgementAndRecoveryWaitForInFlightNotificationDelivery(t *testing.T) {
	for _, test := range []struct {
		name string
		stop func(context.Context, *Store, string, int64) error
	}{
		{
			name: "acknowledgement",
			stop: func(ctx context.Context, st *Store, key string, occurrenceNo int64) error {
				return st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
					Key: key, OccurrenceNo: occurrenceNo, ActorUserID: 42, At: time.Now().UTC(),
				})
			},
		},
		{
			name: "recovery",
			stop: func(ctx context.Context, st *Store, key string, occurrenceNo int64) error {
				return st.Put(ctx, incidents.Record{
					Key: key, Severity: "P0", State: "recovered", OccurrenceNo: occurrenceNo,
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := openTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := st.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			key := "group:in-flight-notification-" + test.name + ":availability"
			machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
			transition, err := machine.Observe(ctx, incidents.Observation{
				Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
				CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			payload := []byte(`{"header":{"title":{"content":"alert"}},"elements":[]}`)
			deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
				IncidentKey: key, DedupKey: "in-flight-notification-" + test.name, MessageHash: "alert",
				OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
			})
			if err != nil || !reserved {
				t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
			}

			done := make(chan error, 1)
			go func() {
				done <- test.stop(ctx, st, key, transition.OccurrenceNo)
			}()
			select {
			case err := <-done:
				t.Fatalf("%s returned before notification delivery finished: %v", test.name, err)
			case <-time.After(100 * time.Millisecond):
			}

			if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
				Status: "delivered", ResponseCode: http.StatusOK, Payload: payload,
				MessageID: "om-in-flight", UrgentStatus: "delivered",
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatalf("%s did not resume after notification delivery finished", test.name)
			}
		})
	}
}

func TestRecoveryWaitsForInFlightNotificationRetry(t *testing.T) {
	st := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:in-flight-notification-retry:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"header":{"title":{"content":"alert"}},"elements":[]}`)
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "in-flight-notification-retry", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "failed", Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	var nextAttempt time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at
		FROM relay_ops.notification_deliveries
		WHERE id=$1`, deliveryID).Scan(&nextAttempt); err != nil {
		t.Fatal(err)
	}
	claim, err := st.ClaimNotificationRetry(ctx, nextAttempt)
	if err != nil || claim == nil || claim.ID != deliveryID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Put(ctx, incidents.Record{
			Key: key, Severity: "P0", State: "recovered", OccurrenceNo: transition.OccurrenceNo,
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("recovery returned before notification retry finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, Payload: payload,
		MessageID: "om-retry", UrgentStatus: "delivered",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("recovery did not resume after notification retry finished")
	}
}

func TestAcknowledgementWaitsForInFlightEscalation(t *testing.T) {
	st, key, occurrenceNo, firstDue := prepareDueEscalation(t, "ack-race")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	claim, err := st.ClaimDueEscalation(ctx, firstDue)
	if err != nil || claim == nil || claim.ClaimToken == "" {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	done := make(chan error, 1)
	go func() {
		done <- st.AcknowledgeIncident(ctx, incidents.Acknowledgement{
			Key: key, OccurrenceNo: occurrenceNo, ActorUserID: 42, At: firstDue,
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("acknowledgement returned before escalation finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := st.FinishEscalation(ctx, alerting.Result{
		Key: key, OccurrenceNo: occurrenceNo, Level: 1, ClaimToken: claim.ClaimToken,
		Succeeded: false, RetryAt: firstDue.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("acknowledgement did not resume after escalation finished")
	}
}

func TestRecoveryWaitsForInFlightEscalation(t *testing.T) {
	st, key, occurrenceNo, firstDue := prepareDueEscalation(t, "recovery-race")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	claim, err := st.ClaimDueEscalation(ctx, firstDue)
	if err != nil || claim == nil || claim.ClaimToken == "" {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	done := make(chan error, 1)
	go func() {
		done <- st.Put(ctx, incidents.Record{
			Key: key, Severity: "P0", State: "recovered", OccurrenceNo: occurrenceNo,
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("recovery returned before escalation finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := st.FinishEscalation(ctx, alerting.Result{
		Key: key, OccurrenceNo: occurrenceNo, Level: 1, ClaimToken: claim.ClaimToken,
		Succeeded: false, RetryAt: firstDue.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("recovery did not resume after escalation finished")
	}
}

func prepareDueEscalation(t *testing.T, suffix string) (*Store, string, int64, time.Time) {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:" + suffix + ":availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P0", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"header":{"title":{"content":"alert"}},"elements":[]}`)
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: suffix, MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, Payload: payload, UrgentStatus: "delivered",
	}); err != nil {
		t.Fatal(err)
	}
	var firstDue time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_escalation_at FROM relay_ops.incidents WHERE incident_key=$1`, key).Scan(&firstDue); err != nil {
		t.Fatal(err)
	}
	return st, key, transition.OccurrenceNo, firstDue
}

func TestGroupSignalsReplaceByIdentityAndExpire(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	signal := GroupSignal{
		GroupName: "GPT Plus 内测", SourceKind: "capacity", SourceKey: "current",
		Payload:          json.RawMessage(`{"available":1,"total":2}`),
		SourceObservedAt: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute),
	}
	if err := st.UpsertGroupSignal(ctx, signal); err != nil {
		t.Fatalf("UpsertGroupSignal: %v", err)
	}
	signal.Payload = json.RawMessage(`{"available":0,"total":2}`)
	signal.SourceObservedAt = now
	if err := st.UpsertGroupSignal(ctx, signal); err != nil {
		t.Fatalf("replace UpsertGroupSignal: %v", err)
	}
	if err := st.UpsertGroupSignal(ctx, GroupSignal{
		GroupName: "GPT Plus 内测", SourceKind: "native_monitor", SourceKey: "7:gpt-5",
		Payload:          json.RawMessage(`{"status":"abnormal"}`),
		SourceObservedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatalf("expired UpsertGroupSignal: %v", err)
	}

	signals, err := st.ListFreshGroupSignals(ctx, "GPT Plus 内测", now)
	if err != nil {
		t.Fatalf("ListFreshGroupSignals: %v", err)
	}
	if len(signals) != 1 || signals[0].SourceKind != "capacity" {
		t.Fatalf("fresh signals = %#v", signals)
	}
	var payload map[string]int
	if err := json.Unmarshal(signals[0].Payload, &payload); err != nil ||
		payload["available"] != 0 || payload["total"] != 2 {
		t.Fatalf("fresh signal payload = %s, err=%v", signals[0].Payload, err)
	}
}

func TestOneShotReservationIsIdempotentAndRetriesFailure(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	reservation := notify.OneShotReservation{
		NotificationKey: "pricing:7:semantic-hash",
		Family:          "pricing_notice",
		PolicyVersion:   1,
		SourceKind:      "public_pricing",
		DedupKey:        "dedup-pricing-7",
		MessageHash:     "message-one",
		Payload:         []byte(`{"header":{"title":{"content":"价格变更"}},"elements":[]}`),
	}
	firstID, reserved, err := st.ReserveOneShot(ctx, reservation)
	if err != nil || !reserved {
		t.Fatalf("first reservation = %d %v %v", firstID, reserved, err)
	}
	if err := st.FinishOneShot(ctx, firstID, notify.DeliveryOutcome{Status: "failed", Payload: reservation.Payload}); err != nil {
		t.Fatal(err)
	}
	reservation.MessageHash = "message-two"
	secondID, reserved, err := st.ReserveOneShot(ctx, reservation)
	if err != nil || !reserved || secondID != firstID {
		t.Fatalf("failed retry = %d %v %v", secondID, reserved, err)
	}
	if err := st.FinishOneShot(ctx, secondID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, MessageID: "om-pricing",
		Payload: reservation.Payload, UrgentStatus: "not_supported",
	}); err != nil {
		t.Fatal(err)
	}
	if _, reserved, err := st.ReserveOneShot(ctx, reservation); err != nil || reserved {
		t.Fatalf("delivered duplicate = %v %v", reserved, err)
	}
	var status string
	var attempts int
	if err := st.pool.QueryRow(ctx, `
		SELECT delivery_status, attempt_count
		FROM relay_ops.notification_messages WHERE id=$1`, firstID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" || attempts != 2 {
		t.Fatalf("stored one-shot = status %q attempts %d", status, attempts)
	}
}

func TestNotificationDecisionUpsertsLastSeenAndCount(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	firstSeen := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	record := DecisionRecord{
		DecisionKey: "capacity:GPT Plus 内测:source-unavailable",
		Family:      "group_capacity", PolicyVersion: 1, SourceKind: "capacity",
		Decision: "suppressed", Reason: "source_unavailable",
		Details: json.RawMessage(`{"source":"accounts"}`), ObservedAt: firstSeen,
	}
	if err := st.RecordNotificationDecision(ctx, record); err != nil {
		t.Fatal(err)
	}
	record.Reason = "policy_disabled"
	record.Details = json.RawMessage(`{"mode":"disabled"}`)
	record.ObservedAt = firstSeen.Add(5 * time.Minute)
	if err := st.RecordNotificationDecision(ctx, record); err != nil {
		t.Fatal(err)
	}
	var reason string
	var count int64
	var lastSeen time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT reason, observation_count, last_seen_at
		FROM relay_ops.notification_decisions WHERE decision_key=$1`, record.DecisionKey).Scan(
		&reason, &count, &lastSeen,
	); err != nil {
		t.Fatal(err)
	}
	if reason != "policy_disabled" || count != 2 || !lastSeen.Equal(record.ObservedAt) {
		t.Fatalf("stored decision = reason %q count %d last_seen %s", reason, count, lastSeen)
	}
}

func TestOperationalBaselineRoundTrips(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	want := Baseline{
		Key: "multiplier:account:17", CurrentValue: "0.07",
		EvidenceHash: "sha256:one",
		UpdatedAt:    time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC),
	}
	if err := st.PutOperationalBaseline(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := st.GetOperationalBaseline(ctx, want.Key)
	if err != nil || !found {
		t.Fatalf("GetOperationalBaseline = %#v %v %v", got, found, err)
	}
	if got.Key != want.Key || got.CurrentValue != want.CurrentValue ||
		got.EvidenceHash != want.EvidenceHash || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("baseline = %#v, want %#v", got, want)
	}
	if _, found, err := st.GetOperationalBaseline(ctx, "missing"); err != nil || found {
		t.Fatalf("missing baseline found=%v err=%v", found, err)
	}
}

func TestIncidentNotificationMetadataRoundTripsAndNewOccurrenceResetsLifecycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := incidents.Record{
		Key: "group:7:user-impact", Family: "group_runtime", PolicyVersion: 1,
		SourceKind: "site_monitor", Severity: "P1", State: "confirmed",
		SampleCount: 2, OccurrenceNo: 1, RecoveryCount: 1,
		EvidenceHash: "metrics-one", MaterialHash: "partial-request-failures",
		CurrentValue:  `{"latest_fact":"错误率 10%"}`,
		LatestPayload: []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`),
	}
	if err := st.Put(ctx, record); err != nil {
		t.Fatal(err)
	}
	got, found, err := st.Get(ctx, record.Key)
	if err != nil || !found {
		t.Fatalf("Get = %#v %v %v", got, found, err)
	}
	var gotPayload, wantPayload any
	if err := json.Unmarshal(got.LatestPayload, &gotPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.LatestPayload, &wantPayload); err != nil {
		t.Fatal(err)
	}
	got.LatestPayload = nil
	record.LatestPayload = nil
	if !reflect.DeepEqual(got, record) || !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("record = %#v, want %#v", got, record)
	}
	record.LatestPayload = []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`)

	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: record.Key, DedupKey: "old-occurrence-delivery",
		MessageHash: "old-card", OccurrenceNo: 1, Transition: "confirmed",
		Payload: record.LatestPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("ReserveNotification = %d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "failed", Payload: record.LatestPayload,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET acknowledged_occurrence=1, acknowledged_at=NOW(), acknowledged_by=42,
		    escalation_level=1, next_escalation_at=NOW()+INTERVAL '5 minutes'
		WHERE incident_key=$1`, record.Key); err != nil {
		t.Fatal(err)
	}

	record.OccurrenceNo = 2
	record.RecoveryCount = 0
	record.EvidenceHash = "metrics-two"
	record.MaterialHash = "lost-redundancy"
	if err := st.Put(ctx, record); err != nil {
		t.Fatal(err)
	}
	var acknowledged sql.NullInt64
	var nextEscalation sql.NullTime
	var oldDeliveryStatus string
	if err := st.pool.QueryRow(ctx, `
		SELECT acknowledged_occurrence, next_escalation_at
		FROM relay_ops.incidents WHERE incident_key=$1`, record.Key).Scan(
		&acknowledged, &nextEscalation,
	); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `
		SELECT delivery_status FROM relay_ops.notification_deliveries WHERE id=$1`,
		deliveryID).Scan(&oldDeliveryStatus); err != nil {
		t.Fatal(err)
	}
	if acknowledged.Valid || nextEscalation.Valid || oldDeliveryStatus != "canceled" {
		t.Fatalf("new occurrence lifecycle = ack %#v escalation %#v delivery %q",
			acknowledged, nextEscalation, oldDeliveryStatus)
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
