package compare

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

type recordingReportRepository struct {
	reports []CompareReport
	err     error
}

func (r *recordingReportRepository) SaveCompareReport(_ context.Context, report CompareReport) error {
	r.reports = append(r.reports, report)
	return r.err
}

func completeSnapshot(page Page, window WindowKind, observedAt time.Time) SourceSnapshot {
	return SourceSnapshot{
		Page:        page,
		Window:      window,
		WindowStart: observedAt.Add(-time.Hour),
		WindowEnd:   observedAt,
		Counts: map[string]int64{
			MetricAccountCount:                 2,
			MetricRequestCount:                 3,
			MetricBillCount:                    2,
			MetricTokenCount:                   144,
			MetricRank:                         1,
			MetricReconciliationExceptionCount: 1,
		},
		Identifiers: map[string][]string{
			MetricAccountIDs:                 {"account-1", "account-2"},
			MetricRequestIDs:                 {"request-1", "request-2", "request-3"},
			MetricBillIDs:                    {"bill-1", "bill-2"},
			MetricReconciliationExceptionIDs: {"exception-1"},
		},
		DecimalAmounts: map[string]string{
			MetricRawCost:         "1.2300",
			MetricRevenue:         "2.0000",
			MetricProcurementCost: "0.7500",
			MetricProfit:          "1.2500",
			MetricProfitMargin:    "0.625",
			MetricBalance:         "81.5000",
			MetricMultiplier:      "0.5800",
			MetricScore:           "91.2500",
		},
		CurrencyAmounts: map[string]map[string]string{
			MetricRawCost:         {"USD": "1.23", "CNY": "8.84"},
			MetricRevenue:         {"USD": "2.00", "CNY": "14.36"},
			MetricProcurementCost: {"USD": "0.75", "CNY": "5.39"},
			MetricProfit:          {"USD": "1.25", "CNY": "8.97"},
			MetricBalance:         {"USD": "81.50", "CNY": "585.17"},
		},
		Ranks: map[string]int64{"account-1": 1, "account-2": 2},
		ReconciliationCounts: map[string]int64{
			"matched": 10, "pending": 2, "conflict": 1, "unattributed": 3, "exception": 4,
		},
		MetricVersions: map[string]MetricVersionEvidence{
			MetricRawCost:         {RateVersion: "rate-v4", CalculationVersion: "cost-v2"},
			MetricRevenue:         {RateVersion: "rate-v4", CalculationVersion: "revenue-v2"},
			MetricProcurementCost: {RateVersion: "rate-v4", CalculationVersion: "procurement-v2"},
			MetricProfit:          {RateVersion: "rate-v4", CalculationVersion: "profit-v3"},
			MetricProfitMargin:    {RateVersion: "rate-v4", CalculationVersion: "margin-v3"},
			MetricMultiplier:      {RateVersion: "rate-v4", CalculationVersion: "multiplier-v2"},
			MetricScore:           {RateVersion: "rate-v4", CalculationVersion: "score-v2"},
			MetricRank:            {RateVersion: "rate-v4", CalculationVersion: "rank-v2"},
		},
		RateVersions: map[string]string{
			MetricRateVersion:        "rate-v4",
			MetricCalculationVersion: "profit-v3",
		},
		Freshness: FreshnessEvidence{
			GeneratedAt:     observedAt,
			SourceWatermark: "event-144",
			Complete:        true,
			FreshUntil:      observedAt.Add(5 * time.Minute),
		},
		BalanceObservedAt:     observedAt,
		BalanceSourceEvidence: "collector:balance:144",
		ContractComplete:      true,
	}
}

func completeComparisonInput(window WindowKind, comparedAt time.Time) ComparisonInput {
	legacy := completeSnapshot(PageAccountMonitor, window, comparedAt)
	external := completeSnapshot(PageAccountMonitor, window, comparedAt)
	for metric, value := range external.DecimalAmounts {
		external.DecimalAmounts[metric] = value + "0"
	}
	return ComparisonInput{
		Legacy:     legacy,
		External:   external,
		Permission: CheckEvidence{Passed: true, EvidenceRef: "permission:test:admin"},
		Export:     CheckEvidence{Passed: true, EvidenceRef: "export:sha256:abc"},
		Rollback:   CheckEvidence{Passed: true, EvidenceRef: "rollback:rehearsal:42"},
		Operator:   "operator@example.com",
		ComparedAt: comparedAt,
	}
}

func TestReportServiceComparesAndPersistsEveryRequiredWindowExactly(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &recordingReportRepository{}
	service := NewReportService(repository)

	for _, window := range []WindowKind{WindowMinimum, WindowDefault, WindowMaximum} {
		report, err := service.CompareAndPersist(context.Background(), completeComparisonInput(window, now))
		if err != nil {
			t.Fatalf("window %s: %v", window, err)
		}
		if !report.Passed || report.Operator != "operator@example.com" || !report.ComparedAt.Equal(now) {
			t.Fatalf("window %s report = %#v", window, report)
		}
		if comparison := report.DecimalAmounts[MetricRawCost]; !comparison.Matched || comparison.Legacy != "1.2300" || comparison.External != "1.23000" {
			t.Fatalf("decimal comparison = %#v", comparison)
		}
	}

	if len(repository.reports) != 3 {
		t.Fatalf("persisted reports = %d", len(repository.reports))
	}
}

func TestReportServicePersistsExactIdentifierAndDecimalMismatches(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &recordingReportRepository{}
	service := NewReportService(repository)
	input := completeComparisonInput(WindowDefault, now)
	input.External.Identifiers[MetricRequestIDs] = []string{"request-1", "request-2", "request-other"}
	input.External.DecimalAmounts[MetricRawCost] = "1.2300000001"

	report, err := service.CompareAndPersist(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Identifiers[MetricRequestIDs].Matched || report.DecimalAmounts[MetricRawCost].Matched {
		t.Fatalf("mismatch report = %#v", report)
	}
	if len(repository.reports) != 1 || repository.reports[0].Passed {
		t.Fatalf("failed report was not persisted = %#v", repository.reports)
	}
}

func TestReportServiceRequiresBoundedSnapshotReconciliationForBalanceDifferences(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &recordingReportRepository{}
	service := NewReportService(repository)
	input := completeComparisonInput(WindowMinimum, now)
	input.Legacy.SnapshotID = "legacy-snapshot"
	input.Legacy.SnapshotDigest = "sha256:legacy"
	input.External.SnapshotID = "external-snapshot"
	input.External.SnapshotDigest = "sha256:external"
	input.External.DecimalAmounts[MetricBalance] = "81.49"
	input.External.BalanceObservedAt = time.Time{}
	input.External.BalanceSourceEvidence = ""

	missingEvidence, err := service.CompareAndPersist(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if missingEvidence.Passed || missingEvidence.DecimalAmounts[MetricBalance].ObservationGapExplained {
		t.Fatalf("missing balance evidence = %#v", missingEvidence.DecimalAmounts[MetricBalance])
	}

	input.External.BalanceObservedAt = now.Add(-time.Minute)
	input.External.BalanceSourceEvidence = "collector:balance:143"
	input.BalanceReconciliation = BalanceReconciliationEvidence{
		EvidenceRef: "reconciliation:42", LegacySnapshotID: "legacy-snapshot", ExternalSnapshotID: "external-snapshot",
	}
	explained, err := service.CompareAndPersist(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !explained.Passed || !explained.DecimalAmounts[MetricBalance].ObservationGapExplained {
		t.Fatalf("explained balance gap = %#v", explained)
	}
	comparison := explained.DecimalAmounts[MetricBalance]
	if comparison.LegacyObservedAt.IsZero() || comparison.ExternalObservedAt.IsZero() ||
		comparison.LegacySourceEvidence == "" || comparison.ExternalSourceEvidence == "" {
		t.Fatalf("balance evidence was not persisted = %#v", comparison)
	}

	input.External.DecimalAmounts[MetricBalance] = "not-a-decimal"
	invalid, err := service.CompareAndPersist(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Passed || invalid.DecimalAmounts[MetricBalance].ObservationGapExplained {
		t.Fatalf("invalid balance was explained = %#v", invalid.DecimalAmounts[MetricBalance])
	}
}

func TestJSONLReportRepositoryPersistsAndReloadsAuditableReports(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "comparison-reports.jsonl")
	repository := NewJSONLReportRepository(path)
	report, err := NewReportService(repository).CompareAndPersist(context.Background(), completeComparisonInput(WindowDefault, now))
	if err != nil {
		t.Fatal(err)
	}

	stored, err := repository.LoadCompareReports(context.Background(), PageAccountMonitor)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != report.ID || stored[0].Operator != "operator@example.com" || stored[0].Rollback.EvidenceRef != "rollback:rehearsal:42" {
		t.Fatalf("stored reports = %#v", stored)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %o", info.Mode().Perm())
	}
}

func TestEvaluatePageCutoverRequiresFreshCompleteThreeWindowEvidenceAndRetirementProof(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	reports := make([]CompareReport, 0, 3)
	for _, window := range []WindowKind{WindowMinimum, WindowDefault, WindowMaximum} {
		report, err := NewReportService(&recordingReportRepository{}).CompareAndPersist(context.Background(), completeComparisonInput(window, now))
		if err != nil {
			t.Fatal(err)
		}
		reports = append(reports, report)
	}

	decision := EvaluatePageCutover(ExternalPrimary, PageAccountMonitor, reports, now, 10*time.Minute, nil)
	if !decision.UseExternal || decision.Degraded || decision.EffectiveMode != ExternalPrimary {
		t.Fatalf("external decision = %#v", decision)
	}

	missingWindow := EvaluatePageCutover(ExternalPrimary, PageAccountMonitor, reports[:2], now, 10*time.Minute, nil)
	if missingWindow.UseExternal || !missingWindow.Degraded || missingWindow.EffectiveMode != LegacyOnly {
		t.Fatalf("missing-window decision = %#v", missingWindow)
	}

	stale := EvaluatePageCutover(ExternalPrimary, PageAccountMonitor, reports, now.Add(11*time.Minute), 10*time.Minute, nil)
	if stale.UseExternal || !stale.Degraded {
		t.Fatalf("stale decision = %#v", stale)
	}

	withoutRetirement := EvaluatePageCutover(LegacyRetired, PageAccountMonitor, reports, now, 10*time.Minute, nil)
	if withoutRetirement.UseExternal || withoutRetirement.EffectiveMode == LegacyRetired {
		t.Fatalf("legacy retirement was reachable = %#v", withoutRetirement)
	}

	retired := EvaluatePageCutover(LegacyRetired, PageAccountMonitor, reports, now, 10*time.Minute, &RetirementEvidence{
		Passed: true, EvidenceRef: "retirement:review:9", Operator: "operator@example.com", RecordedAt: now,
	})
	if !retired.UseExternal || retired.EffectiveMode != LegacyRetired {
		t.Fatalf("retired decision = %#v", retired)
	}
}

func TestMaterializeKeepsSourcesSeparateAndRejectsIncompatibleWindows(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	production := NativeMetrics{SourceKind: "sub2api_native_monitor", SourceRef: "monitor:9", ModelID: "gpt-a", WindowStart: start, WindowEnd: start.Add(time.Hour), SampleCount: 60, SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 2000, CostMultiplier: 1_000}
	candidate := ProbeMetrics{SourceKind: "relay_ops_candidate_probe", SourceRef: "probe:17", ModelID: "gpt-a", WindowStart: start, WindowEnd: start.Add(time.Hour), SampleCount: 4, SuccessRate: 1, SSECompletionRate: 1, TTFTP95MS: 1500, CostMultiplier: 800}
	window, err := Materialize(production, candidate)
	if err != nil || window.Production.SourceKind == window.Candidate.SourceKind || window.SchemaVersion != MetricSchemaV1 {
		t.Fatalf("window=%#v err=%v", window, err)
	}
	candidate.ModelID = "gpt-b"
	if _, err := Materialize(production, candidate); err == nil {
		t.Fatal("incompatible models accepted")
	}
}

func TestClassifyRequiresMeaningfulImprovementWithoutQualityRegression(t *testing.T) {
	t.Parallel()
	window := ComparisonWindow{
		Production: Metrics{SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 2000, CostMultiplier: domain.MultiplierBPS(1_000)},
		Candidate:  Metrics{SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 1500, CostMultiplier: domain.MultiplierBPS(850)},
	}
	result := Classify(window, DefaultPolicy())
	if !result.FasterTTFT || !result.Cheaper || !result.OverallBetter {
		t.Fatalf("comparison=%#v", result)
	}
	window.Candidate.SuccessRate = 0.90
	result = Classify(window, DefaultPolicy())
	if result.OverallBetter || result.MoreStable {
		t.Fatalf("regressed comparison=%#v", result)
	}
}

func TestEvaluateQualityFirstAppliesHardGatesBeforeScore(t *testing.T) {
	t.Parallel()
	evidence := QualityEvidence{
		TechnicalPassed: false, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		MaterialImprovement: true,
	}

	decision := EvaluateQualityFirst(evidence)

	if decision.Status != StatusBlocked || decision.Eligible || decision.TotalScore != 100 || !contains(decision.HardGateReasons, "technical_failure") {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestEvaluateQualityFirstRequiresQualityAndMandatoryEvidence(t *testing.T) {
	t.Parallel()
	unknown := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true,
		ReliabilityScore: 40, LatencyScore: 15, GenerationScore: 10, CapacityScore: 15,
	})
	if unknown.Status != StatusNeedsEvidence || unknown.QualityScore != 80 || unknown.Eligible ||
		!contains(unknown.MissingEvidence, "production_baseline") || !contains(unknown.MissingEvidence, "gateway_measurement") || !contains(unknown.MissingEvidence, "verified_pricing") {
		t.Fatalf("unknown=%#v", unknown)
	}

	low := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 0, PriceScore: 10,
		MaterialImprovement: true,
	})
	if low.Status != StatusNotBetter || low.QualityScore != 75 || low.Eligible {
		t.Fatalf("low=%#v", low)
	}
}

func TestEvaluateQualityFirstMakesOnlyCompleteBetterEvidenceEligible(t *testing.T) {
	t.Parallel()
	decision := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		MaterialImprovement: true,
	})
	if decision.Status != StatusEligibleForManualSwitch || decision.QualityScore != 90 || decision.TotalScore != 100 || !decision.Eligible {
		t.Fatalf("decision=%#v", decision)
	}

	regressed := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 20, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		LatencyRegression: true, MaterialImprovement: true,
	})
	if regressed.Status != StatusBlocked || !contains(regressed.HardGateReasons, "latency_regression") {
		t.Fatalf("regressed=%#v", regressed)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
