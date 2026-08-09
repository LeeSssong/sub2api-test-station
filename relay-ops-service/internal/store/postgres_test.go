package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/events"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/projection"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
)

func int64Pointer(value int64) *int64 { return &value }

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

func TestProjectionStorePersistsConsumerCheckpointDeadLettersAndReadModels(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	event := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440001",
		Type:            events.RequestCompleted,
		OccurredAt:      at,
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"request_id":"request-1","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"1.25","actual_cost":"0.40","cost_usd":"0.40","currency":"USD"}`),
	}
	profitability := projection.NewProfitabilityWithRepository(st)
	accounting := projection.NewAccountingWithRepository(st)
	consumer, err := events.NewPersistentConsumer(st, profitability, accounting)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}

	restarted, err := events.NewPersistentConsumer(st, events.HandlerFunc(func(context.Context, events.Event) error {
		t.Fatal("persisted duplicate was dispatched after restart")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Handle(ctx, event); err != nil {
		t.Fatalf("restart duplicate: %v", err)
	}
	next := event
	next.EventID = "550e8400-e29b-41d4-a716-446655440002"
	next.OccurredAt = at.Add(time.Second)
	next.Payload = []byte(`{"request_id":"request-2","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"0.75","actual_cost":"0.10","cost_usd":"0.10","currency":"USD"}`)
	resumedProfitability := projection.NewProfitabilityWithRepository(st)
	resumedAccounting := projection.NewAccountingWithRepository(st)
	resumed, err := events.NewPersistentConsumer(st, resumedProfitability, resumedAccounting)
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Handle(ctx, next); err != nil {
		t.Fatalf("resume projection from persisted state: %v", err)
	}
	w, found, err := restarted.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || w.LastEventID != next.EventID || w.Completeness != events.CompletenessComplete {
		t.Fatalf("watermark=%+v found=%v err=%v", w, found, err)
	}

	var requestCount int64
	var revenue, cost, profit, completeness, version string
	if err := st.pool.QueryRow(ctx, `
		SELECT requests, revenue::text, cost::text, profit::text, completeness, calculation_version
		FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(
		&requestCount, &revenue, &cost, &profit, &completeness, &version,
	); err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || revenue != "2" || cost != "0.5" || profit != "1.5" || completeness != projection.CompletenessComplete || version != projection.ProfitabilityCalculationVersion {
		t.Fatalf("persisted profitability requests=%d revenue=%s cost=%s profit=%s completeness=%s version=%s", requestCount, revenue, cost, profit, completeness, version)
	}
	if err := st.pool.QueryRow(ctx, `SELECT requests FROM relay_ops.accounting_read_models WHERE scope='all'`).Scan(&requestCount); err != nil || requestCount != 2 {
		t.Fatalf("persisted accounting requests=%d err=%v", requestCount, err)
	}
	accountEvent := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440003",
		Type:            events.AccountHealthChanged,
		OccurredAt:      at.Add(2 * time.Second),
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:02Z"}`),
	}
	accounts := projection.NewAccountsWithRepository(st)
	accountConsumer, err := events.NewPersistentConsumer(st, accounts)
	if err != nil {
		t.Fatal(err)
	}
	if err := accountConsumer.Handle(ctx, accountEvent); err != nil {
		t.Fatalf("persist account projection: %v", err)
	}
	var status, sourceWatermark string
	var generatedAt time.Time
	var freshness int64
	if err := st.pool.QueryRow(ctx, `
		SELECT status, generated_at, source_watermark, freshness_seconds
		FROM relay_ops.account_read_models WHERE account_id=7`).Scan(&status, &generatedAt, &sourceWatermark, &freshness); err != nil {
		t.Fatal(err)
	}
	if status != "healthy" || generatedAt.IsZero() || sourceWatermark != accountEvent.EventID || freshness < 0 {
		t.Fatalf("persisted account status=%s generated_at=%s watermark=%s freshness=%d", status, generatedAt, sourceWatermark, freshness)
	}

	deadEvent := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440004",
		Type:            events.AccountHealthChanged,
		OccurredAt:      at.Add(time.Second),
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:01Z"}`),
	}
	failing, err := events.NewPersistentConsumer(st, events.HandlerFunc(func(context.Context, events.Event) error {
		return errors.New("projection rejected event")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := failing.Handle(ctx, deadEvent); err == nil {
		t.Fatal("expected projection failure")
	}
	dead, err := restarted.ListDeadLetters(ctx)
	if err != nil || len(dead) != 1 || dead[0].Event.EventID != deadEvent.EventID || dead[0].Error != "projection rejected event" {
		t.Fatalf("dead letters=%+v err=%v", dead, err)
	}
}

func TestConsumerAtomicProjectionRollbackFencingAndCompleteness(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440011", at, "1.00", "0.25")
	consumer, err := events.NewPersistentConsumer(st,
		projection.NewProfitabilityWithRepository(st),
		events.HandlerFunc(func(context.Context, events.Event) error { return errors.New("second projection failed") }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err == nil {
		t.Fatal("expected second handler failure")
	}
	var profitabilityRows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.profitability_read_models`).Scan(&profitabilityRows); err != nil {
		t.Fatal(err)
	}
	if profitabilityRows != 0 {
		t.Fatalf("partial projection rows=%d, want 0", profitabilityRows)
	}
	var status string
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.externalization_events WHERE event_id=$1`, event.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "dead" {
		t.Fatalf("failed event status=%q", status)
	}

	good := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440012", at.Add(time.Second), "0.50", "0.10")
	goodConsumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := goodConsumer.Handle(ctx, good); err != nil {
		t.Fatal(err)
	}
	watermark, found, err := goodConsumer.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found {
		t.Fatalf("watermark found=%v err=%v", found, err)
	}
	if watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("watermark completeness=%q with dead gap, want partial", watermark.Completeness)
	}

	fenced := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440013", at.Add(2*time.Second), "0.25", "0.05")
	oldClaim, err := st.ClaimEvent(ctx, fenced)
	if err != nil || !oldClaim.Acquired {
		t.Fatalf("old claim=%+v err=%v", oldClaim, err)
	}
	if _, err := st.pool.Exec(ctx, `UPDATE relay_ops.externalization_events SET lease_until=NOW()-INTERVAL '1 second' WHERE event_id=$1`, fenced.EventID); err != nil {
		t.Fatal(err)
	}
	newClaim, err := st.ClaimEvent(ctx, fenced)
	if err != nil || !newClaim.Acquired || newClaim.Generation <= oldClaim.Generation || newClaim.Token == oldClaim.Token {
		t.Fatalf("new claim=%+v old=%+v err=%v", newClaim, oldClaim, err)
	}
	_, err = st.ApplyEvent(ctx, fenced, oldClaim, time.Now().UTC(), func(context.Context) error { return nil })
	if !errors.Is(err, events.ErrStaleClaim) {
		t.Fatalf("old worker apply error=%v, want ErrStaleClaim", err)
	}
	if _, err := st.ApplyEvent(ctx, fenced, newClaim, time.Now().UTC(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("new worker apply: %v", err)
	}
}

func TestConsumerCompleteFailureRollsBackProjectionAndRetryDoesNotDoubleCount(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION relay_ops.reject_externalization_watermark() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'forced watermark failure'; END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_externalization_watermark
		BEFORE INSERT OR UPDATE ON relay_ops.externalization_watermarks
		FOR EACH ROW EXECUTE FUNCTION relay_ops.reject_externalization_watermark();`); err != nil {
		t.Fatal(err)
	}
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440021", time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC), "1.00", "0.40")
	consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err == nil {
		t.Fatal("expected forced completion failure")
	}
	var rows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.profitability_read_models`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("projection rows after completion rollback=%d err=%v", rows, err)
	}
	if _, err := st.pool.Exec(ctx, `
		DROP TRIGGER reject_externalization_watermark ON relay_ops.externalization_watermarks;
		DROP FUNCTION relay_ops.reject_externalization_watermark();`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.externalization_events SET lease_until=NOW()-INTERVAL '1 second' WHERE event_id=$1`, event.EventID); err != nil {
		t.Fatal(err)
	}
	retry, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	var requests int64
	var revenue string
	if err := st.pool.QueryRow(ctx, `SELECT requests, revenue::text FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(&requests, &revenue); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || revenue != "1" {
		t.Fatalf("retried projection requests=%d revenue=%s", requests, revenue)
	}
}

func TestConsumerConcurrentInstancesSerializeProjectionUpdates(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	input := []events.Event{
		externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440031", at, "0.10", "0.01"),
		externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440032", at, "0.20", "0.02"),
	}
	start := make(chan struct{})
	result := make(chan error, len(input))
	for _, event := range input {
		event := event
		go func() {
			consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
			if err == nil {
				<-start
				err = consumer.Handle(ctx, event)
			}
			result <- err
		}()
	}
	close(start)
	for range input {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	var requests int64
	var revenue string
	if err := st.pool.QueryRow(ctx, `SELECT requests, revenue::text FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(&requests, &revenue); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || revenue != "0.3" {
		t.Fatalf("concurrent projection requests=%d revenue=%s", requests, revenue)
	}
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessComplete {
		t.Fatalf("completed watermark=%+v found=%v err=%v", watermark, found, err)
	}
	pending := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440033", at.Add(time.Second), "0.30", "0.03")
	claim, err := st.ClaimEvent(ctx, pending)
	if err != nil || !claim.Acquired {
		t.Fatalf("processing gap claim=%+v err=%v", claim, err)
	}
	watermark, found, err = st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("processing-gap watermark=%+v found=%v err=%v", watermark, found, err)
	}
}

func TestProjectionCompletenessTracksProcessingAndDeadGapsInSameTransaction(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 30, 0, 0, time.UTC)
	accounts := projection.NewAccountsWithRepository(st)
	profitability := projection.NewProfitabilityWithRepository(st)
	accounting := projection.NewAccountingWithRepository(st)
	reconciliation := projection.NewReconciliationWithRepository(st)
	consumer, err := events.NewPersistentConsumer(st, accounts, profitability, accounting, reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	accountEvent := events.Event{
		EventID: "550e8400-e29b-41d4-a716-446655440034", Type: events.AccountHealthChanged,
		OccurredAt: at, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-10T03:30:00Z"}`),
	}
	requestEvent := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440035", at.Add(time.Second), "1.00", "0.25")
	if err := consumer.Handle(ctx, accountEvent); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, requestEvent); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessComplete)

	processing := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440036", at.Add(2*time.Second), "0.50", "0.10")
	claim, err := st.ClaimEvent(ctx, processing)
	if err != nil || !claim.Acquired {
		t.Fatalf("processing claim=%+v err=%v", claim, err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessPartial)
	if _, err := st.ApplyEvent(ctx, processing, claim, time.Now().UTC(), func(applyCtx context.Context) error {
		for _, handler := range []events.Handler{accounts, profitability, accounting, reconciliation} {
			if err := handler.Handle(applyCtx, processing); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessComplete)

	dead := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440037", at.Add(3*time.Second), "0.25", "0.05")
	deadClaim, err := st.ClaimEvent(ctx, dead)
	if err != nil || !deadClaim.Acquired {
		t.Fatalf("dead claim=%+v err=%v", deadClaim, err)
	}
	if err := st.FailEvent(ctx, dead, deadClaim, time.Now().UTC(), errors.New("permanent projection failure")); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessPartial)
}

func TestClaimAndFirstWatermarkCompletionShareProjectionLock(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 45, 0, 0, time.UTC)
	first := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440038", at, "1.00", "0.25")
	firstClaim, err := st.ClaimEvent(ctx, first)
	if err != nil || !firstClaim.Acquired {
		t.Fatalf("first claim=%+v err=%v", firstClaim, err)
	}

	blocker, err := st.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	blockerTx, err := blocker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blockerTx.Rollback(ctx)
	if _, err := blockerTx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('relay-ops:externalization-projections', 0))`); err != nil {
		t.Fatal(err)
	}

	applyResult := make(chan error, 1)
	go func() {
		_, err := st.ApplyEvent(ctx, first, firstClaim, time.Now().UTC(), func(context.Context) error { return nil })
		applyResult <- err
	}()
	waitForAdvisoryWaiters(t, st, 1)

	second := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440039", at.Add(time.Second), "0.50", "0.10")
	type claimResult struct {
		claim events.Claim
		err   error
	}
	secondResult := make(chan claimResult, 1)
	go func() {
		claim, err := st.ClaimEvent(ctx, second)
		secondResult <- claimResult{claim: claim, err: err}
	}()
	waitForAdvisoryWaiters(t, st, 2)
	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-applyResult; err != nil {
		t.Fatal(err)
	}
	claimed := <-secondResult
	if claimed.err != nil || !claimed.claim.Acquired {
		t.Fatalf("second claim=%+v err=%v", claimed.claim, claimed.err)
	}
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("watermark=%+v found=%v err=%v, want partial", watermark, found, err)
	}
	var status string
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.externalization_events WHERE event_id=$1`, second.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("second event status=%q, want processing", status)
	}
}

func assertExternalizationCompleteness(t *testing.T, st *Store, want string) {
	t.Helper()
	ctx := context.Background()
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != want {
		t.Fatalf("watermark=%+v found=%v err=%v, want completeness %q", watermark, found, err, want)
	}
	for _, table := range []string{
		"account_read_models", "profitability_read_models", "accounting_read_models", "reconciliation_read_models",
	} {
		var values []string
		rows, err := st.pool.Query(ctx, `SELECT completeness FROM relay_ops.`+table)
		if err != nil {
			t.Fatalf("query %s completeness: %v", table, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			values = append(values, value)
		}
		rows.Close()
		if len(values) == 0 {
			t.Fatalf("%s has no projection rows", table)
		}
		for _, value := range values {
			if value != want {
				t.Fatalf("%s completeness=%q, want %q", table, value, want)
			}
		}
	}
}

func waitForAdvisoryWaiters(t *testing.T, st *Store, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("advisory lock waiters=%d, want at least %d", waiting, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestProjectionMigrationUpgradesPlaceholderSchemaInPlace(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.pool.Exec(ctx, `
		CREATE SCHEMA relay_ops;
		CREATE TABLE relay_ops.externalization_events (
		 event_id TEXT PRIMARY KEY, source_version TEXT NOT NULL, event_type TEXT NOT NULL,
		 occurred_at TIMESTAMPTZ NOT NULL, processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), payload JSONB NOT NULL
		);
		CREATE TABLE relay_ops.externalization_watermarks (
		 source TEXT PRIMARY KEY, last_event_id TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL,
		 processed_at TIMESTAMPTZ NOT NULL, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
		);
		CREATE TABLE relay_ops.externalization_dead_letters (
		 event_id TEXT PRIMARY KEY, error TEXT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE relay_ops.account_read_models (
		 account_id BIGINT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'unknown', balance NUMERIC, currency TEXT,
		 observed_at TIMESTAMPTZ, generated_at TIMESTAMPTZ NOT NULL, source_watermark TEXT NOT NULL,
		 freshness_seconds BIGINT NOT NULL DEFAULT 0, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
		);
		INSERT INTO relay_ops.externalization_events
			(event_id, source_version, event_type, occurred_at, payload)
		VALUES ('legacy-event', 'legacy-v1', 'request.completed', '2026-08-09T00:00:00Z', '{"legacy":true}');
		INSERT INTO relay_ops.externalization_watermarks
			(source, last_event_id, occurred_at, processed_at, completeness, calculation_version)
		VALUES ('legacy-v1', 'legacy-event', '2026-08-09T00:00:00Z', '2026-08-09T00:00:01Z', 'complete', 'legacy-v1');
		INSERT INTO relay_ops.externalization_dead_letters
			(event_id, error, payload)
		VALUES ('legacy-dead', 'legacy failure', '{"legacy":true}');
			INSERT INTO relay_ops.account_read_models
				(account_id, status, balance, currency, observed_at, generated_at, source_watermark, completeness, calculation_version)
			VALUES (99, 'healthy', 12.50, 'USD', '2026-08-09T00:00:00Z', '2026-08-09T00:00:01Z', 'legacy-event', 'complete', 'accounts-v0');`); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("upgrade placeholder schema: %v", err)
	}
	var legacyStatus string
	var legacyGeneration int64
	if err := st.pool.QueryRow(ctx, `
		SELECT status, claim_generation FROM relay_ops.externalization_events WHERE event_id='legacy-event'`).Scan(&legacyStatus, &legacyGeneration); err != nil {
		t.Fatal(err)
	}
	if legacyStatus != "processed" || legacyGeneration != 0 {
		t.Fatalf("legacy event status=%s generation=%d", legacyStatus, legacyGeneration)
	}
	var legacyAccounts, legacyDead int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&legacyAccounts); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.externalization_dead_letters WHERE event_id='legacy-dead'`).Scan(&legacyDead); err != nil {
		t.Fatal(err)
	}
	if legacyAccounts != 1 || legacyDead != 1 {
		t.Fatalf("legacy rows accounts=%d dead=%d", legacyAccounts, legacyDead)
	}
	var healthAt, balanceAt *time.Time
	var healthEventID, balanceEventID *string
	if err := st.pool.QueryRow(ctx, `
		SELECT health_occurred_at, health_event_id, balance_occurred_at, balance_event_id
		FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&healthAt, &healthEventID, &balanceAt, &balanceEventID); err != nil {
		t.Fatal(err)
	}
	legacyObservedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if healthAt == nil || !healthAt.Equal(legacyObservedAt) || healthEventID == nil || *healthEventID != "legacy-event" ||
		balanceAt == nil || !balanceAt.Equal(legacyObservedAt) || balanceEventID == nil || *balanceEventID != "legacy-event" {
		t.Fatalf("legacy positions health=(%v,%v) balance=(%v,%v)", healthAt, healthEventID, balanceAt, balanceEventID)
	}
	accounts := projection.NewAccountsWithRepository(st)
	accountConsumer, err := events.NewPersistentConsumer(st, accounts)
	if err != nil {
		t.Fatal(err)
	}
	for _, delayed := range []events.Event{
		{
			EventID: "legacy-health-delayed", Type: events.AccountHealthChanged,
			OccurredAt: legacyObservedAt.Add(-time.Second), SourceVersion: "legacy-v1", ContractVersion: events.ContractVersion,
			Payload: []byte(`{"account_id":99,"status":"unhealthy","checked_at":"2026-08-08T23:59:59Z"}`),
		},
		{
			EventID: "legacy-balance-delayed", Type: events.AccountBalanceSnapshot,
			OccurredAt: legacyObservedAt.Add(-time.Second), SourceVersion: "legacy-v1", ContractVersion: events.ContractVersion,
			Payload: []byte(`{"account_id":99,"balance":"1.25","currency":"USD","captured_at":"2026-08-08T23:59:59Z"}`),
		},
	} {
		if err := accountConsumer.Handle(ctx, delayed); err != nil {
			t.Fatalf("consume delayed legacy account event: %v", err)
		}
	}
	var legacyAccountStatus, legacyBalance string
	if err := st.pool.QueryRow(ctx, `SELECT status, balance::text FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&legacyAccountStatus, &legacyBalance); err != nil {
		t.Fatal(err)
	}
	if legacyAccountStatus != "healthy" || legacyBalance != "12.50" {
		t.Fatalf("delayed event overwrote legacy account status=%q balance=%q", legacyAccountStatus, legacyBalance)
	}
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440041", time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC), "1.00", "0.25")
	consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err != nil {
		t.Fatalf("consume after placeholder migration upgrade: %v", err)
	}
}

func externalizationRequestEvent(id string, occurredAt time.Time, charge, cost string) events.Event {
	return events.Event{
		EventID: id, Type: events.RequestCompleted, OccurredAt: occurredAt, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"request_id":"request-` + id + `","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"` + charge + `","actual_cost":"` + cost + `","cost_usd":"` + cost + `","currency":"USD"}`),
	}
}

func TestMigrateRejectsDuplicateActiveBillingAccountMappings(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.pool.Exec(ctx, initialMigration); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `ALTER TABLE relay_ops.auth_sessions ADD COLUMN billing_account_id BIGINT`); err != nil {
		t.Fatalf("add legacy billing account mapping column: %v", err)
	}
	for _, name := range []string{"duplicate-billing-one", "duplicate-billing-two"} {
		upstreamID, err := st.CreateUpstream(ctx, Upstream{
			Name: name, Role: "production", BaseURL: "https://" + name + ".example/v1", AdapterType: "newapi", Enabled: true,
		})
		if err != nil {
			t.Fatalf("CreateUpstream %q: %v", name, err)
		}
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
			VALUES ($1, 'file:/run/secrets/upstream-sessions/redacted', 'bearer', 'active', 'https://billing.example/login', 'billing_read', 8123)`, upstreamID); err != nil {
			t.Fatalf("insert legacy duplicate mapping %q: %v", name, err)
		}
	}

	err := st.Migrate(ctx)
	if err == nil {
		t.Fatal("Migrate succeeded with duplicate active billing mappings")
	}
	for _, want := range []string{"migration 010 blocked", "billing account 8123", "2 active billing_read mappings"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Migrate error = %q, want actionable diagnostic containing %q", err, want)
		}
	}
	var count int
	if err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM relay_ops.auth_sessions
		WHERE billing_account_id=8123 AND status='active' AND scope='billing_read'`).Scan(&count); err != nil {
		t.Fatalf("count legacy duplicate mappings: %v", err)
	}
	if count != 2 {
		t.Fatalf("legacy duplicate mappings = %d, want 2 (preflight must not mutate data)", count)
	}
}

func TestProvisionBillingSourceIsAtomicIdempotentAndRejectsConflicts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO relay_ops.public_groups
			(group_id, name, enabled, customer_visible, user_multiplier_bps, source_revision, last_seen_at)
		VALUES (71, 'billing-public', TRUE, TRUE, 10000, 'test', NOW())`); err != nil {
		t.Fatal(err)
	}
	record := billing.BillingProvisionRecord{
		Production: upstreams.ProductionRecord{
			Source: upstreams.Source{
				Name: "billing-primary", Role: upstreams.RoleProduction, BaseURL: "https://billing-primary.example/v1",
				PricingURL: "https://billing-primary.example/pricing", UsageURL: "https://billing-primary.example/usage",
				AdapterType: upstreams.AdapterSub2API, GroupIDs: []int64{71}, Enabled: true,
			},
			Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "billing-primary"}},
		},
		Session: billing.BillingReadSessionRecord{
			LoginURL: "https://billing-primary.example/login", BillingAccountID: 8123,
			Secret: billing.SessionSecretRef{SecretRef: "file:/run/secrets/upstream-sessions/billing-primary", Kind: "upstream_usage_session", Fingerprint: strings.Repeat("a", 64), LastFour: "token"},
			Audit:  billing.SessionAuditEvent{ActorUserID: 42, Action: "upstream.billing_session.provision", ObjectType: "auth_session", AfterSummary: map[string]string{"scope": "billing_read"}},
		},
	}
	first, err := st.ProvisionBillingSource(ctx, record)
	if err != nil || first.AlreadyConfigured || first.UpstreamID <= 0 || first.BillingAccountID != 8123 {
		t.Fatalf("first provision=%#v err=%v", first, err)
	}
	second, err := st.ProvisionBillingSource(ctx, record)
	if err != nil || !second.AlreadyConfigured || second.UpstreamID != first.UpstreamID {
		t.Fatalf("idempotent provision=%#v err=%v", second, err)
	}
	rotatedSecret := record
	rotatedSecret.Session.Secret.Fingerprint = strings.Repeat("d", 64)
	if _, err := st.ProvisionBillingSource(ctx, rotatedSecret); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("rotated bearer declaration error=%v, want billing provision conflict", err)
	}
	var sessions, audits int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.auth_sessions WHERE billing_account_id=8123`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE object_id=$1`, strconv.FormatInt(int64(first.UpstreamID), 10)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || audits != 2 {
		t.Fatalf("sessions=%d audits=%d", sessions, audits)
	}
	conflict := record
	conflict.Production.Source.Name = "billing-secondary"
	conflict.Production.Source.BaseURL = "https://billing-secondary.example/v1"
	conflict.Production.Source.PricingURL = "https://billing-secondary.example/pricing"
	conflict.Production.Source.UsageURL = "https://billing-secondary.example/usage"
	conflict.Session.Secret.SecretRef = "file:/run/secrets/upstream-sessions/billing-secondary"
	conflict.Session.Secret.Fingerprint = strings.Repeat("b", 64)
	if _, err := st.ProvisionBillingSource(ctx, conflict); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("duplicate billing account error=%v", err)
	}
	secretConflict := record
	secretConflict.Production.Source.Name = "billing-secret-reuse"
	secretConflict.Production.Source.BaseURL = "https://billing-secret-reuse.example/v1"
	secretConflict.Production.Source.PricingURL = "https://billing-secret-reuse.example/pricing"
	secretConflict.Production.Source.UsageURL = "https://billing-secret-reuse.example/usage"
	secretConflict.Session.BillingAccountID = 9123
	if _, err := st.ProvisionBillingSource(ctx, secretConflict); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("reused billing secret reference error=%v", err)
	}
}

func TestProvisionBillingSourceConcurrentSecretReferenceConflicts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION relay_ops.delay_billing_secret_insert() RETURNS trigger AS $$
		BEGIN
			IF NEW.secret_ref = 'file:/run/secrets/upstream-sessions/shared-billing-token' THEN
				PERFORM pg_sleep(0.2);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER delay_billing_secret_insert
			BEFORE INSERT ON relay_ops.secret_refs
			FOR EACH ROW EXECUTE FUNCTION relay_ops.delay_billing_secret_insert();`); err != nil {
		t.Fatalf("install concurrent secret reference test barrier: %v", err)
	}

	record := func(name string, billingAccountID int64) billing.BillingProvisionRecord {
		return billing.BillingProvisionRecord{
			Production: upstreams.ProductionRecord{
				Source: upstreams.Source{
					Name: name, Role: upstreams.RoleProduction, BaseURL: "https://" + name + ".example/v1",
					PricingURL: "https://" + name + ".example/pricing", UsageURL: "https://" + name + ".example/usage",
					AdapterType: upstreams.AdapterSub2API, Enabled: true,
				},
				Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": name}},
			},
			Session: billing.BillingReadSessionRecord{
				LoginURL: "https://" + name + ".example/login", BillingAccountID: billingAccountID,
				Secret: billing.SessionSecretRef{SecretRef: "file:/run/secrets/upstream-sessions/shared-billing-token", Kind: "upstream_usage_session", Fingerprint: strings.Repeat("c", 64), LastFour: "token"},
				Audit:  billing.SessionAuditEvent{ActorUserID: 42, Action: "upstream.billing_session.provision", ObjectType: "auth_session", AfterSummary: map[string]string{"scope": "billing_read"}},
			},
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, input := range []billing.BillingProvisionRecord{record("billing-concurrent-one", 8123), record("billing-concurrent-two", 9123)} {
		go func(input billing.BillingProvisionRecord) {
			<-start
			_, err := st.ProvisionBillingSource(ctx, input)
			results <- err
		}(input)
	}
	close(start)

	var success, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, billing.ErrBillingProvisionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent billing provision error = %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent shared secret results: success=%d conflicts=%d, want one of each", success, conflicts)
	}
	var sessions int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.auth_sessions WHERE secret_ref=$1`, "file:/run/secrets/upstream-sessions/shared-billing-token").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("sessions using shared secret reference=%d, want 1", sessions)
	}
}

func TestBillingProvisionLockKeysUseSeparateNamespaces(t *testing.T) {
	secret := billingProvisionLockKey("secret", "X")
	source := billingProvisionLockKey("source", "X")
	if secret == source {
		t.Fatalf("secret and source lock keys must differ: %q", secret)
	}
	if secret != "billing-provision:secret:X" || source != "billing-provision:source:X" {
		t.Fatalf("billing lock keys = %q, %q", secret, source)
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

func TestIncidentOccurrencePersistence(t *testing.T) {
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

	if _, err := machine.Observe(ctx, incidents.Observation{
		Key: observation.Key, Severity: "P0", Failing: false,
		EvidenceHash: "available:1/1", CurrentValue: "可用 1 / 共 1",
	}); err != nil {
		t.Fatal(err)
	}
	second, err := machine.Observe(ctx, observation)
	if err != nil || second.OccurrenceNo != 2 {
		t.Fatalf("second occurrence = %#v, %v", second, err)
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

func TestListBillingSourcesUsesEnabledProductionActiveSessions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	eligible, err := st.CreateUpstream(ctx, Upstream{Name: "billing-active", Role: "production", BaseURL: "https://billing-active.example/v1", AdapterType: "sub2api", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := st.CreateUpstream(ctx, Upstream{Name: "billing-disabled", Role: "production", BaseURL: "https://billing-disabled.example/v1", AdapterType: "newapi", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	unsupported, err := st.CreateUpstream(ctx, Upstream{Name: "billing-unsupported", Role: "production", BaseURL: "https://billing-unsupported.example/v1", AdapterType: "openai", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cookieSession, err := st.CreateUpstream(ctx, Upstream{Name: "billing-cookie", Role: "production", BaseURL: "https://billing-cookie.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	usageBearer, err := st.CreateUpstream(ctx, Upstream{Name: "billing-usage-only", Role: "production", BaseURL: "https://billing-usage-only.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id               domain.UpstreamID
		status           string
		authMode         string
		ref              string
		scope            string
		billingAccountID *int64
	}{
		{eligible, "active", "bearer", "file:/run/secrets/upstream-sessions/active", "billing_read", int64Pointer(731)},
		{disabled, "active", "bearer", "file:/run/secrets/upstream-sessions/disabled", "billing_read", int64Pointer(732)},
		{unsupported, "active", "bearer", "file:/run/secrets/upstream-sessions/unsupported", "billing_read", int64Pointer(733)},
		{cookieSession, "active", "cookie", "file:/run/secrets/upstream-sessions/cookie", "usage_read", nil},
		{usageBearer, "active", "bearer", "file:/run/secrets/upstream-sessions/usage-only", "usage_read", int64Pointer(734)},
	} {
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
			VALUES ($1, $2, $3, $4, 'https://billing.example/login', $5, $6)`, row.id, row.ref, row.authMode, row.status, row.scope, row.billingAccountID); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := st.ListBillingSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources=%#v", sources)
	}
	got := sources[0]
	if got.AccountID != 731 || got.BaseURL != "https://billing-active.example/v1" || got.AdapterType != "sub2api" || got.SecretRef != "file:/run/secrets/upstream-sessions/active" {
		t.Fatalf("source=%#v", got)
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
		CurrentValue: `{"group_name":"Public","headline":"全部请求持续失败",` +
			`"latest_fact":"最近窗口请求全部失败。","capacity":"当前可用账号 0 / 1。"}`,
		ConfirmationWindows: 1,
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
		claim.EscalationLevel != 0 || !json.Valid([]byte(claim.CurrentValue)) ||
		!strings.Contains(claim.CurrentValue, "最近窗口请求全部失败") {
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
	claim, err = st.ClaimDueEscalation(ctx, secondDue)
	if err != nil || claim == nil || claim.OccurrenceNo != 1 || claim.EscalationLevel != 1 {
		t.Fatalf("second reminder claim=%#v err=%v", claim, err)
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
		CurrentValue: `{"group_name":"Public","headline":"全部请求持续失败",` +
			`"latest_fact":"最近窗口请求全部失败。","capacity":"当前可用账号 0 / 1。"}`,
		ConfirmationWindows: 1,
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
		CurrentValue: `{"group_name":"Public","headline":"全部请求持续失败",` +
			`"latest_fact":"最近窗口请求全部失败。","capacity":"当前可用账号 0 / 1。"}`,
		ConfirmationWindows: 1,
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
	if err != nil || claim == nil || claim.Kind != "incident" ||
		claim.ID != deliveryID || claim.IncidentKey != key ||
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

func TestFailedRecoveryNotificationBecomesDueForLeasedRetry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:failed-recovery-retry:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	alert, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: "0/1",
		CurrentValue: "可用 0 / 共 1", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	alertPayload := []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`)
	alertID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "failed-recovery-retry-alert", MessageHash: "alert",
		OccurrenceNo: alert.OccurrenceNo, Transition: alert.Kind, Payload: alertPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve alert=%d %v %v", alertID, reserved, err)
	}
	if err := st.FinishNotification(ctx, alertID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, Payload: alertPayload,
	}); err != nil {
		t.Fatal(err)
	}

	recovery, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: false, EvidenceHash: "1/1",
		CurrentValue: "可用 1 / 共 1",
	})
	if err != nil || recovery.Kind != "recovered" {
		t.Fatalf("recovery=%#v err=%v", recovery, err)
	}
	recoveryPayload := []byte(`{"header":{"title":{"content":"恢复"}},"elements":[]}`)
	recoveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "failed-recovery-retry-recovery", MessageHash: "recovery",
		OccurrenceNo: recovery.OccurrenceNo, Transition: recovery.Kind, Payload: recoveryPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve recovery=%d %v %v", recoveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, recoveryID, notify.DeliveryOutcome{
		Status: "failed", Payload: recoveryPayload,
	}); err != nil {
		t.Fatal(err)
	}
	var due time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at FROM relay_ops.notification_deliveries WHERE id=$1`,
		recoveryID,
	).Scan(&due); err != nil {
		t.Fatal(err)
	}

	claim, err := st.ClaimNotificationRetry(ctx, due)

	if err != nil || claim == nil || claim.ID != recoveryID ||
		claim.IncidentKey != key || claim.OccurrenceNo != recovery.OccurrenceNo ||
		claim.Transition != "recovered" || !json.Valid(claim.Payload) {
		t.Fatalf("claim=%#v err=%v", claim, err)
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
	if err != nil || claim == nil || claim.Kind != "incident" ||
		claim.ID != deliveryID || claim.Severity != "P1" ||
		claim.OccurrenceNo != transition.OccurrenceNo ||
		claim.Transition != transition.Kind {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
}

func TestSupersededIncidentRetryIsNotClaimed(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "site:group:retry-superseded:availability"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: "partial",
		CurrentValue: "legacy capacity warning", ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`)
	deliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "retry-superseded",
		MessageHash: "message", OccurrenceNo: transition.OccurrenceNo,
		Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "failed", Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	var due time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at FROM relay_ops.notification_deliveries WHERE id=$1`,
		deliveryID,
	).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if updated, err := st.SupersedeLegacyNotificationIncidents(ctx, due); err != nil || updated != 1 {
		t.Fatalf("supersede=%d %v", updated, err)
	}
	claim, err := st.ClaimNotificationRetry(ctx, due)
	if err != nil || claim != nil {
		t.Fatalf("superseded claim=%#v err=%v", claim, err)
	}
}

func TestNotificationRetryPrefersIncidentThenClaimsOneShot(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	oneShotPayload := []byte(`{"header":{"title":{"content":"价格变化"}},"elements":[]}`)
	oneShotID, reserved, err := st.ReserveOneShot(ctx, notify.OneShotReservation{
		NotificationKey: "pricing:retry-priority",
		Family:          "pricing_notice",
		PolicyVersion:   1,
		SourceKind:      "public_pricing",
		DedupKey:        "pricing-retry-priority",
		MessageHash:     "pricing-message",
		Payload:         oneShotPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("one-shot reserve=%d %v %v", oneShotID, reserved, err)
	}
	if err := st.FinishOneShot(ctx, oneShotID, notify.DeliveryOutcome{
		Status: "failed", Payload: oneShotPayload,
	}); err != nil {
		t.Fatal(err)
	}
	var oneShotDue time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at FROM relay_ops.notification_messages WHERE id=$1`,
		oneShotID,
	).Scan(&oneShotDue); err != nil {
		t.Fatal(err)
	}

	key := "group:retry-priority:user-impact"
	machine := incidents.Machine{Repository: st, Policy: incidents.DefaultPolicy()}
	transition, err := machine.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: "partial",
		CurrentValue: `{"group_name":"Public","headline":"部分请求失败",` +
			`"latest_fact":"最近窗口仍有请求失败。"}`,
		ConfirmationWindows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	incidentPayload := []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`)
	incidentID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: key, DedupKey: "incident-retry-priority",
		MessageHash: "incident-message", OccurrenceNo: transition.OccurrenceNo,
		Transition: transition.Kind, Payload: incidentPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("incident reserve=%d %v %v", incidentID, reserved, err)
	}
	if err := st.FinishNotification(ctx, incidentID, notify.DeliveryOutcome{
		Status: "failed", Payload: incidentPayload,
	}); err != nil {
		t.Fatal(err)
	}
	var incidentDue time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at FROM relay_ops.notification_deliveries WHERE id=$1`,
		incidentID,
	).Scan(&incidentDue); err != nil {
		t.Fatal(err)
	}
	now := incidentDue
	if oneShotDue.After(now) {
		now = oneShotDue
	}
	claim, err := st.ClaimNotificationRetry(ctx, now)
	if err != nil || claim == nil || claim.Kind != "incident" ||
		claim.ID != incidentID || claim.IncidentKey != key ||
		claim.OccurrenceNo != transition.OccurrenceNo ||
		claim.Transition != transition.Kind {
		t.Fatalf("incident claim=%#v err=%v", claim, err)
	}
	if err := st.FinishNotification(ctx, incidentID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK, Payload: incidentPayload,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err = st.ClaimNotificationRetry(ctx, now)
	if err != nil || claim == nil || claim.Kind != "one_shot" ||
		claim.ID != oneShotID ||
		claim.NotificationKey != "pricing:retry-priority" ||
		claim.IncidentKey != "" || claim.OccurrenceNo != 0 ||
		claim.Transition != "" || claim.Severity != "P2" {
		t.Fatalf("one-shot claim=%#v err=%v", claim, err)
	}
}

func TestNotificationRetrySkipsTerminalAndMaxAttemptOneShots(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 7, 0, 0, 0, time.UTC)
	for index, state := range []struct {
		status   string
		attempts int
	}{
		{status: "delivered", attempts: 1},
		{status: "expired", attempts: 2},
		{status: "failed", attempts: 5},
	} {
		key := fmt.Sprintf("terminal-one-shot-%d", index)
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO relay_ops.notification_messages
				(notification_key, family, policy_version, source_kind, dedup_key,
				 message_hash, message_payload, delivery_status, attempt_count,
				 next_attempt_at, updated_at)
			VALUES ($1, 'pricing_notice', 1, 'public_pricing', $2, 'message',
			        '{"header":{"title":{"content":"P2"}},"elements":[]}'::jsonb,
			        $3, $4, $5, $5)`,
			key, key+"-dedup", state.status, state.attempts, now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	claim, err := st.ClaimNotificationRetry(ctx, now)
	if err != nil || claim != nil {
		t.Fatalf("terminal claim=%#v err=%v", claim, err)
	}
}

func TestRecoveryCancelsPendingNotificationRetries(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:cancel-retry-recovery:availability"
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
		IncidentKey: key, DedupKey: "cancel-retry-recovery", MessageHash: "alert",
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
	if err := st.Put(ctx, incidents.Record{
		Key: key, Severity: "P0", State: "recovered", OccurrenceNo: transition.OccurrenceNo,
	}); err != nil {
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
}

func TestRecoveryWaitsForInFlightNotificationDelivery(t *testing.T) {
	st := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key := "group:in-flight-notification-recovery:availability"
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
		IncidentKey: key, DedupKey: "in-flight-notification-recovery", MessageHash: "alert",
		OccurrenceNo: transition.OccurrenceNo, Transition: transition.Kind, Payload: payload,
	})
	if err != nil || !reserved {
		t.Fatalf("reserve=%d %v %v", deliveryID, reserved, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Put(ctx, incidents.Record{
			Key: key, Severity: "P0", State: "recovered", OccurrenceNo: transition.OccurrenceNo,
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("recovery returned before notification delivery finished: %v", err)
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
		t.Fatal("recovery did not resume after notification delivery finished")
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

func TestActiveReminderClaimIgnoresHistoricalAcknowledgement(t *testing.T) {
	st, _, _, firstDue := prepareDueEscalation(t, "historical-ack")
	ctx := context.Background()
	observation := incidents.Observation{
		Key: "group:historical-ack:availability",
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET acknowledged_occurrence=occurrence_no,
		    acknowledged_at=$2,
		    acknowledged_by=42,
		    next_escalation_at=$3
		WHERE incident_key=$1`,
		observation.Key, firstDue.Add(-time.Minute), firstDue.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}

	claim, err := st.ClaimDueEscalation(ctx, firstDue)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Key != observation.Key {
		t.Fatalf("active reminder claim = %#v, want incident %q", claim, observation.Key)
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

func TestOneShotReservationIsIdempotentAndDefersFailureToRetryWorker(t *testing.T) {
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
	if secondID, reserved, err := st.ReserveOneShot(ctx, reservation); err != nil || reserved {
		t.Fatalf("failed duplicate = %d %v %v", secondID, reserved, err)
	}
	var nextAttempt time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT next_attempt_at
		FROM relay_ops.notification_messages WHERE id=$1`, firstID,
	).Scan(&nextAttempt); err != nil {
		t.Fatal(err)
	}
	claim, err := st.ClaimNotificationRetry(ctx, nextAttempt)
	if err != nil || claim == nil || claim.Kind != "one_shot" ||
		claim.ID != firstID || claim.NotificationKey != reservation.NotificationKey {
		t.Fatalf("retry claim=%#v err=%v", claim, err)
	}
	if err := st.FinishOneShot(ctx, firstID, notify.DeliveryOutcome{
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

func TestSupersedeLegacyNotificationIncidentsPreservesRowsAndClearsLifecycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	legacyKeys := []string{
		"daily-report:2026-07-28",
		"native-monitor:7:account:1",
		"site:account:1:paused",
		"site:account:2:balance_exhausted",
		"site:group:7:availability",
		"site:group:7:error_rate",
		"site:group:7:ttft_p95",
		"upstream:1:pricing",
		"candidate:17:quality",
		"quality-report:17:health_pulse",
		"synthetic:relay-ops:acceptance:v1",
		"upstream:1:usage_session",
	}
	for _, key := range legacyKeys {
		if err := st.Put(ctx, incidents.Record{
			Key: key, Family: "legacy", SourceKind: "legacy",
			Severity: "P1", State: "confirmed", SampleCount: 1, OccurrenceNo: 1,
		}); err != nil {
			t.Fatalf("Put(%q): %v", key, err)
		}
	}
	if err := st.Put(ctx, incidents.Record{
		Key: "legacy:already-recovered", Family: "legacy", SourceKind: "legacy",
		Severity: "P1", State: "recovered", SampleCount: 2, OccurrenceNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, incidents.Record{
		Key: "group:7:user-impact", Family: "group_runtime", PolicyVersion: 1,
		SourceKind: "site_monitor", Severity: "P1", State: "confirmed",
		SampleCount: 2, OccurrenceNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET next_escalation_at=$1,
		    escalation_claim_token='legacy-claim',
		    escalation_claimed_at=$2
		WHERE family='legacy' AND state='confirmed'`,
		now.Add(-time.Minute), now.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	updated, err := st.SupersedeLegacyNotificationIncidents(ctx, now)
	if err != nil || updated != int64(len(legacyKeys)) {
		t.Fatalf("SupersedeLegacyNotificationIncidents = %d, %v", updated, err)
	}
	for _, key := range legacyKeys {
		var state string
		var nextEscalation, claimedAt sql.NullTime
		var claimToken sql.NullString
		if err := st.pool.QueryRow(ctx, `
			SELECT state, next_escalation_at, escalation_claim_token, escalation_claimed_at
			FROM relay_ops.incidents WHERE incident_key=$1`, key).
			Scan(&state, &nextEscalation, &claimToken, &claimedAt); err != nil {
			t.Fatalf("read %q: %v", key, err)
		}
		if state != "superseded" || nextEscalation.Valid || claimToken.Valid || claimedAt.Valid {
			t.Fatalf("%q lifecycle = state %q next %#v token %#v claimed %#v",
				key, state, nextEscalation, claimToken, claimedAt)
		}
	}
	for key, wantState := range map[string]string{
		"legacy:already-recovered": "recovered",
		"group:7:user-impact":      "confirmed",
	} {
		var state string
		if err := st.pool.QueryRow(ctx,
			`SELECT state FROM relay_ops.incidents WHERE incident_key=$1`, key,
		).Scan(&state); err != nil || state != wantState {
			t.Fatalf("%q state = %q, %v", key, state, err)
		}
	}
	var incidentCount, deliveryCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.incidents`).
		Scan(&incidentCount); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.notification_deliveries`).
		Scan(&deliveryCount); err != nil {
		t.Fatal(err)
	}
	if incidentCount != len(legacyKeys)+2 || deliveryCount != 0 {
		t.Fatalf("preserved rows=%d deliveries=%d", incidentCount, deliveryCount)
	}
}

func TestDailyNotificationSummaryReadsOnlyConsolidatedProductionFacts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.UpsertPublicGroup(ctx, sub2api.PublicGroupRecord{
		GroupID: 7, Name: "Public", Enabled: true, CustomerVisible: true,
		UserMultiplierBPS: 10_000, SourceRevision: "summary-test",
		LastSeenAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertGroupSignal(ctx, GroupSignal{
		GroupName: "Public", SourceKind: "capacity", SourceKey: "current",
		Payload:          json.RawMessage(`{"available":2,"total":2}`),
		SourceObservedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	for _, record := range []incidents.Record{
		{
			Key: "group:7:user-impact", Family: "group_runtime",
			PolicyVersion: 1, SourceKind: "site_monitor",
			Severity: "P0", State: "confirmed", SampleCount: 2, OccurrenceNo: 1,
		},
		{
			Key: "group:8:user-impact", Family: "group_runtime",
			PolicyVersion: 1, SourceKind: "site_monitor",
			Severity: "P1", State: "escalated", SampleCount: 3, OccurrenceNo: 1,
		},
		{
			Key: "group:9:user-impact", Family: "group_runtime",
			PolicyVersion: 1, SourceKind: "site_monitor",
			Severity: "P1", State: "confirmed", SampleCount: 2, OccurrenceNo: 1,
		},
		{
			Key: "candidate:17:quality", Family: "legacy",
			Severity: "P1", State: "confirmed", SampleCount: 1, OccurrenceNo: 1,
		},
	} {
		if err := st.Put(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	initialPayload := []byte(`{"header":{"title":{"content":"P1"}},"elements":[]}`)
	initialDeliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: "group:9:user-impact", DedupKey: "summary-initial",
		MessageHash: "summary-initial-message", Transition: "confirmed",
		OccurrenceNo: 1, Payload: initialPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("ReserveNotification initial=%d %v %v", initialDeliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, initialDeliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK,
		MessageID: "om-initial-summary", Payload: initialPayload,
		UrgentStatus: "not_supported",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, incidents.Record{
		Key: "group:9:user-impact", Family: "group_runtime",
		PolicyVersion: 1, SourceKind: "site_monitor",
		Severity: "P1", State: "recovered", SampleCount: 4, OccurrenceNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	recoveryPayload := []byte(`{"header":{"title":{"content":"恢复"}},"elements":[]}`)
	recoveryDeliveryID, reserved, err := st.ReserveNotification(ctx, notify.Reservation{
		IncidentKey: "group:9:user-impact", DedupKey: "summary-recovery",
		MessageHash: "summary-recovery-message", Transition: "recovered",
		OccurrenceNo: 1, Payload: recoveryPayload,
	})
	if err != nil || !reserved {
		t.Fatalf("ReserveNotification=%d %v %v", recoveryDeliveryID, reserved, err)
	}
	if err := st.FinishNotification(ctx, recoveryDeliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK,
		MessageID: "om-recovery-summary", Payload: recoveryPayload,
		UrgentStatus: "not_supported",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, incidents.Record{
		Key: "group:9:user-impact", Family: "group_runtime",
		PolicyVersion: 1, SourceKind: "site_monitor",
		Severity: "P1", State: "observed", SampleCount: 1, OccurrenceNo: 2,
	}); err != nil {
		t.Fatal(err)
	}
	upstreamID, err := st.CreateUpstream(ctx, Upstream{
		Name: "Neko", Role: "production",
		BaseURL:     "https://neko-summary.example/v1",
		AdapterType: "openai", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.upstreams
		SET pricing_url='https://neko-summary.example/pricing'
		WHERE id=$1`, upstreamID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendPricingSnapshot(ctx, PricingSnapshot{
		UpstreamID: upstreamID, SourceURL: "https://neko-summary.example/pricing",
		SourceType: "public_page", FetchedAt: now, ContentHash: "summary-price",
		NormalizedJSON: []byte(`{"schema_version":"pricing-evidence-v2","confidence":"structured_json","models":[]}`),
		EvidenceLevel:  "structured_json",
	}); err != nil {
		t.Fatal(err)
	}
	reservation := notify.OneShotReservation{
		NotificationKey: "pricing:summary:test", Family: "pricing_notice",
		PolicyVersion: 1, SourceKind: "public_pricing",
		DedupKey: "pricing-summary-test", MessageHash: "pricing-summary-message",
		Payload: []byte(`{"header":{"title":{"content":"价格变更"}},"elements":[]}`),
	}
	deliveryID, reserved, err := st.ReserveOneShot(ctx, reservation)
	if err != nil || !reserved {
		t.Fatalf("ReserveOneShot=%d %v %v", deliveryID, reserved, err)
	}
	if err := st.FinishOneShot(ctx, deliveryID, notify.DeliveryOutcome{
		Status: "delivered", ResponseCode: http.StatusOK,
		MessageID: "om-summary", Payload: reservation.Payload,
		UrgentStatus: "not_supported",
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := st.ReadDailyNotificationSummary(
		ctx,
		now.Add(-time.Hour),
		now.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.PublicGroups != 1 ||
		summary.ActiveP0 != 1 ||
		summary.ActiveP1 != 1 ||
		summary.Recovered != 1 ||
		summary.PricingEvents != 1 ||
		summary.FreshCapacityGroups != 1 ||
		summary.PricingSources != 1 ||
		summary.TrackedPricingSources != 1 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestIncidentNotificationMetadataRoundTripsAndNewOccurrencePreservesHistoricalAcknowledgement(t *testing.T) {
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
	acknowledgedAt := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.incidents
		SET acknowledged_occurrence=1, acknowledged_at=$2, acknowledged_by=42,
		    escalation_level=1, next_escalation_at=NOW()+INTERVAL '5 minutes'
		WHERE incident_key=$1`, record.Key, acknowledgedAt); err != nil {
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
	var storedAcknowledgedAt sql.NullTime
	var acknowledgedBy sql.NullInt64
	var nextEscalation sql.NullTime
	var oldDeliveryStatus string
	if err := st.pool.QueryRow(ctx, `
		SELECT acknowledged_occurrence, acknowledged_at, acknowledged_by, next_escalation_at
		FROM relay_ops.incidents WHERE incident_key=$1`, record.Key).Scan(
		&acknowledged, &storedAcknowledgedAt, &acknowledgedBy, &nextEscalation,
	); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `
		SELECT delivery_status FROM relay_ops.notification_deliveries WHERE id=$1`,
		deliveryID).Scan(&oldDeliveryStatus); err != nil {
		t.Fatal(err)
	}
	if !acknowledged.Valid || acknowledged.Int64 != 1 ||
		!storedAcknowledgedAt.Valid || !storedAcknowledgedAt.Time.Equal(acknowledgedAt) ||
		!acknowledgedBy.Valid || acknowledgedBy.Int64 != 42 ||
		nextEscalation.Valid || oldDeliveryStatus != "canceled" {
		t.Fatalf("new occurrence lifecycle = ack %#v at %#v by %#v escalation %#v delivery %q",
			acknowledged, storedAcknowledgedAt, acknowledgedBy, nextEscalation, oldDeliveryStatus)
	}
}

func TestNativeOpsAlertMigrationCursorAndSourceRoundTrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	got, found, err := st.LoadNativeOpsAlertCursor(ctx)
	if err != nil {
		t.Fatalf("LoadNativeOpsAlertCursor before initialization: %v", err)
	}
	if found {
		t.Fatalf("LoadNativeOpsAlertCursor before initialization found %v, cursor %#v", found, got)
	}

	cursor41 := NativeOpsAlertCursor{
		FiredAt: time.Date(2026, 7, 30, 13, 48, 0, 0, time.UTC),
		EventID: 41,
	}
	source41 := nativeOpsAlertSource(41, "firing")
	if err := st.InitializeNativeOpsAlertSync(ctx, cursor41, []NativeOpsAlertSource{source41}); err != nil {
		t.Fatalf("InitializeNativeOpsAlertSync: %v", err)
	}

	got, found, err = st.LoadNativeOpsAlertCursor(ctx)
	if err != nil {
		t.Fatalf("LoadNativeOpsAlertCursor after initialization: %v", err)
	}
	if !found || !got.FiredAt.Equal(cursor41.FiredAt) || got.EventID != cursor41.EventID {
		t.Fatalf("LoadNativeOpsAlertCursor after initialization = %#v, %v, want %#v, true", got, found, cursor41)
	}

	firing, err := st.ListFiringNativeOpsAlertSources(ctx, 10)
	if err != nil {
		t.Fatalf("ListFiringNativeOpsAlertSources: %v", err)
	}
	if len(firing) != 1 || !reflect.DeepEqual(firing[0], source41) {
		t.Fatalf("ListFiringNativeOpsAlertSources = %#v, want %#v", firing, []NativeOpsAlertSource{source41})
	}
}

func TestNativeOpsAlertPageRejectsInvalidSourceWithoutUpdatingCursor(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	cursor41 := NativeOpsAlertCursor{FiredAt: time.Date(2026, 7, 30, 13, 48, 0, 0, time.UTC), EventID: 41}
	if err := st.InitializeNativeOpsAlertSync(ctx, cursor41, []NativeOpsAlertSource{nativeOpsAlertSource(41, "firing")}); err != nil {
		t.Fatal(err)
	}

	valid42 := nativeOpsAlertSource(42, "firing")
	invalid43 := nativeOpsAlertSource(43, "firing")
	invalid43.RuleID = 0
	cursor43 := NativeOpsAlertCursor{FiredAt: time.Date(2026, 7, 30, 13, 50, 0, 0, time.UTC), EventID: 43}
	if err := st.CommitNativeOpsAlertPage(ctx, []NativeOpsAlertSource{valid42, invalid43}, cursor43); err == nil {
		t.Fatal("CommitNativeOpsAlertPage accepted an invalid source")
	}

	got, found, err := st.LoadNativeOpsAlertCursor(ctx)
	if err != nil || !found || !got.FiredAt.Equal(cursor41.FiredAt) || got.EventID != cursor41.EventID {
		t.Fatalf("cursor after rejected page = %#v, %v, %v; want %#v, true, nil", got, found, err, cursor41)
	}
	var rows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.native_ops_alert_events WHERE source_event_id=42`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("valid source from rejected page persisted %d rows", rows)
	}
}

func TestNativeOpsAlertSourceLedgerIsIdempotentAndResolvedSourcesAreExcluded(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	source41 := nativeOpsAlertSource(41, "firing")
	cursor41 := NativeOpsAlertCursor{FiredAt: time.Date(2026, 7, 30, 13, 48, 0, 0, time.UTC), EventID: 41}
	if err := st.CommitNativeOpsAlertPage(ctx, []NativeOpsAlertSource{source41}, cursor41); err != nil {
		t.Fatalf("first CommitNativeOpsAlertPage: %v", err)
	}
	if err := st.CommitNativeOpsAlertPage(ctx, []NativeOpsAlertSource{source41}, cursor41); err != nil {
		t.Fatalf("idempotent CommitNativeOpsAlertPage: %v", err)
	}
	var rows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.native_ops_alert_events WHERE source_event_id=41`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("source event row count = %d, want 1", rows)
	}

	resolved := source41
	resolved.SourceStatus = "resolved"
	resolvedAt := time.Date(2026, 7, 30, 14, 0, 0, 0, time.UTC)
	resolved.ResolvedAt = &resolvedAt
	resolved.LastSeenAt = resolvedAt
	if err := st.UpsertNativeOpsAlertSource(ctx, resolved); err != nil {
		t.Fatalf("UpsertNativeOpsAlertSource resolved: %v", err)
	}
	firing, err := st.ListFiringNativeOpsAlertSources(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(firing) != 0 {
		t.Fatalf("ListFiringNativeOpsAlertSources after resolved update = %#v, want none", firing)
	}
}

func nativeOpsAlertSource(eventID int64, status string) NativeOpsAlertSource {
	firedAt := time.Date(2026, 7, 30, 13, 48, 0, 0, time.UTC).Add(time.Duration(eventID-41) * time.Minute)
	return NativeOpsAlertSource{
		SourceEventID:  eventID,
		RuleID:         7,
		IncidentKey:    "native-ops-alert:7:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Severity:       "P0",
		SourceStatus:   status,
		FiredAt:        firedAt,
		Silenced:       false,
		DimensionsHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		LastSeenAt:     firedAt,
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
