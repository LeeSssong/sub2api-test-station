package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/qualityreports"
)

func TestCandidateServiceUsesConfiguredManagedSecretDirectory(t *testing.T) {
	service := configuredCandidateService(config.Config{CandidateSecretDir: "/var/lib/relay-ops/candidate-keys"}, nil)
	store, ok := service.SecretStore.(candidates.FileSecretStore)
	if !ok {
		t.Fatalf("SecretStore = %T", service.SecretStore)
	}
	if store.Directory != "/var/lib/relay-ops/candidate-keys" {
		t.Fatalf("Directory = %q", store.Directory)
	}
}

func TestConfiguredTrustedProxyResolvesAtStartupAndForEveryTrustCheck(t *testing.T) {
	lookups := 0
	policy, err := configuredTrustedProxy(config.Config{TrustedProxyHost: "caddy"}, func(string) ([]net.IP, error) {
		lookups++
		return []net.IP{net.ParseIP("172.20.0.4")}, nil
	})
	if err != nil {
		t.Fatalf("configuredTrustedProxy: %v", err)
	}
	if !policy.Trusted("172.20.0.4") || policy.Trusted("172.20.0.9") {
		t.Fatal("configured proxy policy did not retain the exact resolved Caddy peer")
	}
	if lookups != 3 {
		t.Fatalf("lookups = %d, want one startup resolution and one per trust check", lookups)
	}
	_, err = configuredTrustedProxy(config.Config{TrustedProxyHost: "caddy"}, func(string) ([]net.IP, error) {
		return nil, errors.New("Docker DNS unavailable")
	})
	if err == nil || !strings.Contains(err.Error(), "resolve trusted proxy") {
		t.Fatalf("startup error = %v", err)
	}
}

func TestAccountingIsNotMountedWhenDisabled(t *testing.T) {
	app := newAccountingVerificationApp(t, false, time.Time{})
	t.Cleanup(app.Close)

	recorder := httptest.NewRecorder()
	app.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/relay-ops/accounting", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAccountingEnabledBuildsZeroBaselineUntilNewUsageArrives(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	startDate := time.Date(2026, 8, 2, 0, 0, 0, 0, shanghai)
	app := newAccountingVerificationApp(t, true, startDate)
	t.Cleanup(app.Close)
	if app.Accounting == nil {
		t.Fatal("enabled accounting service = nil")
	}

	snapshot, err := app.Accounting.RecomputeDate(context.Background(), startDate)
	if err != nil {
		t.Fatalf("RecomputeDate: %v", err)
	}
	if got := snapshot.ExternalRevenueCNY.StringFixed(2); got != "0.00" {
		t.Fatalf("external revenue = %s, want 0.00", got)
	}
	if got := snapshot.ResourceCostCNY.StringFixed(2); got != "0.00" {
		t.Fatalf("resource cost = %s, want 0.00", got)
	}
}

func TestConfiguredAccountingServiceIsEnabledOnlyAndUsesRuntimeConfiguration(t *testing.T) {
	if service := configuredAccountingService(config.Config{}, nil); service != nil {
		t.Fatalf("disabled accounting service = %#v, want nil", service)
	}

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	startDate := time.Date(2026, 8, 2, 0, 0, 0, 0, shanghai)
	service := configuredAccountingService(config.Config{
		AccountingEnabled:           true,
		Timezone:                    shanghai,
		AccountingLedgerStartDate:   startDate,
		AccountingInternalUserIDs:   []int64{7, 9},
		AccountingInternalAPIKeyIDs: []int64{11},
	}, nil)
	if service == nil {
		t.Fatal("enabled accounting service = nil")
	}
	if service.Timezone != shanghai || !service.StartDate.Equal(startDate) {
		t.Fatalf("accounting schedule = timezone %v, start %v", service.Timezone, service.StartDate)
	}
	if len(service.Exclusions.InternalUserIDs) != 2 ||
		service.Exclusions.InternalUserIDs[0] != 7 ||
		service.Exclusions.InternalUserIDs[1] != 9 ||
		len(service.Exclusions.InternalAPIKeyIDs) != 1 ||
		service.Exclusions.InternalAPIKeyIDs[0] != 11 {
		t.Fatalf("accounting exclusions = %#v", service.Exclusions)
	}
}

// newAccountingVerificationApp builds the same App root route composition as
// production, backed by an in-memory accounting repository. It deliberately
// avoids New because New migrates and writes to its configured database.
func newAccountingVerificationApp(t *testing.T, enabled bool, startDate time.Time) *App {
	t.Helper()
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	service := configuredAccountingService(config.Config{
		Mode:                      config.ModeClosed,
		Timezone:                  shanghai,
		AccountingEnabled:         enabled,
		AccountingLedgerStartDate: startDate,
	}, &accountingVerificationRepository{snapshots: make(map[time.Time]accounting.DailySnapshot)})
	operations, err := newOperationsServer(httpserver.Dependencies{
		BaseOrigin: "https://api.example.test",
		Auth:       accountingVerificationAdminVerifier{}, Pricing: accountingVerificationPricing{},
	}, service)
	if err != nil {
		t.Fatal(err)
	}
	root := http.NewServeMux()
	root.Handle("/", operations)
	return &App{Handler: root, Accounting: service}
}

type accountingVerificationPricing struct{}

func (accountingVerificationPricing) PublicPricing(context.Context) ([]httpserver.PublicGroup, error) {
	return nil, nil
}

type accountingVerificationAdminVerifier struct{}

func (accountingVerificationAdminVerifier) VerifyAdminSession(context.Context, adminauth.Session) (adminauth.Identity, error) {
	return adminauth.Identity{UserID: 1, Role: "admin", Status: "active"}, nil
}

type accountingVerificationRepository struct {
	snapshots map[time.Time]accounting.DailySnapshot
}

func (*accountingVerificationRepository) ReadUsageTotals(context.Context, accounting.DayWindow, accounting.ExclusionPolicy) (accounting.UsageTotals, error) {
	return accounting.UsageTotals{}, nil
}

func (*accountingVerificationRepository) ReadCashEventTotals(context.Context, accounting.DayWindow) (accounting.CashEventTotals, error) {
	return accounting.CashEventTotals{}, nil
}

func (repository *accountingVerificationRepository) UpsertDailySnapshot(_ context.Context, snapshot accounting.DailySnapshot) error {
	repository.snapshots[snapshot.ReportDate] = snapshot
	return nil
}

func (*accountingVerificationRepository) CreateCashEvent(context.Context, domain.AdminActor, accounting.CashEventInput, string) (accounting.CashEvent, bool, error) {
	return accounting.CashEvent{}, false, errors.New("cash events are outside this verification")
}

func (repository *accountingVerificationRepository) ReadDailySnapshot(_ context.Context, date time.Time) (accounting.DailySnapshot, bool, error) {
	snapshot, found := repository.snapshots[date]
	return snapshot, found, nil
}

func (*accountingVerificationRepository) ListCashEvents(context.Context, time.Time, time.Time, int) ([]accounting.CashEvent, error) {
	return nil, nil
}

func TestExecuteFastCandidatePersistsQualityReportWithoutNotificationDependencies(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	record := []byte(`{"schema_version":1,"run_id":"fast-1","channel_id":"candidate-17","profile_id":"quality-first-fast-v1","job_kind":"health_pulse","recorded_at":"2026-07-22T03:00:00Z","status":"passed","metrics":{"selected_models":["gpt-a"],"direct":{"request_count":2,"success_count":2,"success_rate":1,"latency":{"p95_ms":1200},"ttft":{"p95_ms":500}},"gateway":{"status":"unknown"}},"errors":[]}`)
	runner := &fakeFastRunner{result: probes.FastResult{RunID: "fast-1", JobKind: "health_pulse", Status: "passed", RecordedAt: now, Record: record}}
	sink := &fakeQualitySink{}
	repository := fakeCandidateRepository{items: []candidates.Candidate{{ID: 17, Name: "Candidate", BaseURL: "https://candidate.example/v1", Enabled: true}}}

	err := executeFastCandidate(
		context.Background(), domain.UpstreamID(17), "health_pulse",
		repository, runner, sink,
	)
	if err != nil {
		t.Fatalf("executeFastCandidate: %v", err)
	}
	if runner.candidate.ID != 17 || sink.report.ReportID != "fast-1" || len(sink.report.ReportHash) != 64 {
		t.Fatalf("runner=%#v report=%#v", runner.candidate, sink.report)
	}
}

func TestQualityReviewAdapterMapsStaleEvidenceWithoutWrites(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	repository := &fakeQualityRepository{report: qualityreports.Report{
		ReportID: "fast-1", ReportHash: strings.Repeat("a", 64), ExpiresAt: now.Add(time.Minute), Status: "needs_evidence",
	}}
	adapter := qualityReviewAdapter{Service: qualityreports.Service{Repository: repository, Clock: func() time.Time { return now }}}

	preview, err := adapter.Preview(context.Background(), domain.AdminActor{UserID: 1}, httpserver.QualityPreviewInput{ReportID: "fast-1", ReportHash: repository.report.ReportHash})
	if err != nil || preview.Writes != 0 || preview.Status != "dry_run" {
		t.Fatalf("Preview = %#v, %v", preview, err)
	}
	_, err = adapter.Preview(context.Background(), domain.AdminActor{UserID: 1}, httpserver.QualityPreviewInput{ReportID: "fast-1", ReportHash: strings.Repeat("b", 64)})
	if !errors.Is(err, httpserver.ErrQualityReportStale) {
		t.Fatalf("stale error = %v", err)
	}
}

type fakeCandidateRepository struct{ items []candidates.Candidate }

func (f fakeCandidateRepository) ListCandidates(context.Context) ([]candidates.Candidate, error) {
	return f.items, nil
}

type fakeFastRunner struct {
	result    probes.FastResult
	candidate candidates.Candidate
}

func (f *fakeFastRunner) Fast(_ context.Context, candidate candidates.Candidate, _ string) (probes.FastResult, error) {
	f.candidate = candidate
	return f.result, nil
}

type fakeQualitySink struct{ report qualityreports.Report }

func (f *fakeQualitySink) PutQualityReport(_ context.Context, report qualityreports.Report) error {
	f.report = report
	return nil
}

type fakeQualityRepository struct{ report qualityreports.Report }

func (f *fakeQualityRepository) Get(context.Context, string) (qualityreports.Report, bool, error) {
	return f.report, true, nil
}
func (f *fakeQualityRepository) Put(context.Context, qualityreports.Report) error { return nil }
