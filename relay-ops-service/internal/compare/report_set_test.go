package compare

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func completeSetInput(page Page, comparedAt time.Time) ReportSetInput {
	inputs := make([]ComparisonInput, 0, len(RequiredWindowDefinitions()))
	for _, definition := range RequiredWindowDefinitions() {
		input := completeComparisonInput(definition.Kind, comparedAt)
		input.Legacy.Page = page
		input.External.Page = page
		input.Legacy.WindowStart = comparedAt.Add(-definition.Duration)
		input.External.WindowStart = input.Legacy.WindowStart
		input.Legacy.WindowEnd = comparedAt
		input.External.WindowEnd = comparedAt
		input.Legacy.RunID = "run-42"
		input.External.RunID = "run-42"
		input.Legacy.EvidenceLineage = "fixture:lineage:42"
		input.External.EvidenceLineage = "fixture:lineage:42"
		input.Legacy.SnapshotID = "legacy:" + string(definition.Kind)
		input.External.SnapshotID = "external:" + string(definition.Kind)
		input.Legacy.SnapshotDigest = "sha256:legacy:" + string(definition.Kind)
		input.External.SnapshotDigest = "sha256:external:" + string(definition.Kind)
		input.Legacy.Freshness.GeneratedAt = comparedAt.Add(-time.Minute)
		input.External.Freshness.GeneratedAt = comparedAt.Add(-time.Minute)
		input.Legacy.Freshness.FreshUntil = comparedAt.Add(5 * time.Minute)
		input.External.Freshness.FreshUntil = comparedAt.Add(5 * time.Minute)
		inputs = append(inputs, input)
	}
	return ReportSetInput{SetID: "set-42", RunID: "run-42", Page: page, Operator: "operator@example.com", EvidenceLineage: "fixture:lineage:42", ComparedAt: comparedAt, Comparisons: inputs}
}

func TestCompareAndPersistSetStoresOneCoherentImmutableRecord(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "sets.jsonl"))
	set, err := NewReportSetService(repository).CompareAndPersistSet(context.Background(), completeSetInput(PageAccountMonitor, now))
	if err != nil {
		t.Fatal(err)
	}
	if set.ID != "set-42" || set.RunID != "run-42" || len(set.Reports) != 3 || !set.Eligible() {
		t.Fatalf("set=%#v", set)
	}
	for _, report := range set.Reports {
		if report.ReportSetID != set.ID || report.RunID != set.RunID || report.EvidenceLineage != set.EvidenceLineage || !report.PersistedAt.Equal(set.PersistedAt) {
			t.Fatalf("report is not bound to its set: %#v", report)
		}
	}
	loaded, err := repository.LoadReportSets(context.Background(), PageAccountMonitor)
	if err != nil || len(loaded) != 1 || loaded[0].ID != set.ID {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	if err := repository.SaveReportSet(context.Background(), set); err == nil {
		t.Fatal("duplicate immutable set id accepted")
	}
}

func TestReportSetRejectsDetachedChildIdentity(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	service := NewReportSetService(NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "sets.jsonl")))
	set, err := service.CompareAndPersistSet(context.Background(), completeSetInput(PageAccountMonitor, now))
	if err != nil {
		t.Fatal(err)
	}
	set.Reports[0].EvidenceLineage = "detached-lineage"
	if set.Eligible() {
		t.Fatal("report set with detached child lineage was eligible")
	}
	set.Reports[0].EvidenceLineage = set.EvidenceLineage
	set.Reports[1].ReportSetID = "other-set"
	if set.Eligible() {
		t.Fatal("report set with detached child set id was eligible")
	}
}

func TestCompareAndPersistSetRejectsMixedRunOrWindowDefinitions(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*ReportSetInput){
		"mixed run":      func(input *ReportSetInput) { input.Comparisons[1].External.RunID = "other-run" },
		"mixed operator": func(input *ReportSetInput) { input.Comparisons[1].Operator = "other@example.com" },
		"wrong window":   func(input *ReportSetInput) { input.Comparisons[2].External.WindowStart = now.Add(-7 * time.Hour) },
		"mixed lineage":  func(input *ReportSetInput) { input.Comparisons[0].Legacy.EvidenceLineage = "other-lineage" },
	} {
		t.Run(name, func(t *testing.T) {
			input := completeSetInput(PageAccountMonitor, now)
			mutate(&input)
			_, err := NewReportSetService(NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "sets.jsonl"))).CompareAndPersistSet(context.Background(), input)
			if err == nil {
				t.Fatal("incoherent set accepted")
			}
		})
	}
}

func TestEvaluateLatestValidSetRejectsFutureAndAncientSourceEvidence(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	service := NewReportSetService(NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "sets.jsonl")))
	valid, err := service.CompareAndPersistSet(context.Background(), completeSetInput(PageAccountMonitor, now.Add(-2*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	futureInput := completeSetInput(PageAccountMonitor, now.Add(-time.Minute))
	futureInput.SetID = "set-future"
	futureInput.RunID = "run-future"
	for index := range futureInput.Comparisons {
		futureInput.Comparisons[index].Legacy.RunID = "run-future"
		futureInput.Comparisons[index].External.RunID = "run-future"
		futureInput.Comparisons[index].Legacy.Freshness.GeneratedAt = now.Add(time.Minute)
	}
	future, err := service.CompareAndPersistSet(context.Background(), futureInput)
	if err != nil {
		t.Fatal(err)
	}
	decision := EvaluateLatestPageCutover(ExternalPrimary, PageAccountMonitor, []CompareReportSet{valid, future}, now, 10*time.Minute, nil)
	if !decision.UseExternal || decision.ReportSetID != valid.ID {
		t.Fatalf("decision=%#v", decision)
	}
	decision = EvaluateLatestPageCutover(ExternalPrimary, PageAccountMonitor, []CompareReportSet{valid}, now.Add(20*time.Minute), 10*time.Minute, nil)
	if decision.UseExternal || decision.Reason != "comparison_gate_failed" {
		t.Fatalf("ancient decision=%#v", decision)
	}
}

func TestBalanceVarianceRequiresBoundSnapshotsSkewAndReconciliation(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	base := completeComparisonInput(WindowMinimum, now)
	base.Legacy.SnapshotID = "legacy-snapshot"
	base.External.SnapshotID = "external-snapshot"
	base.Legacy.SnapshotDigest = "sha256:legacy"
	base.External.SnapshotDigest = "sha256:external"
	base.External.DecimalAmounts[MetricBalance] = "81.49"
	base.External.BalanceObservedAt = now.Add(-time.Minute)
	base.BalanceReconciliation = BalanceReconciliationEvidence{
		EvidenceRef: "reconciliation:42", LegacySnapshotID: "legacy-snapshot", ExternalSnapshotID: "external-snapshot",
	}

	for _, testCase := range []struct {
		name     string
		mutate   func(*ComparisonInput)
		wantPass bool
	}{
		{"bounded variance", func(*ComparisonInput) {}, true},
		{"arbitrary drift", func(input *ComparisonInput) { input.External.DecimalAmounts[MetricBalance] = "80.50" }, false},
		{"unbound snapshot", func(input *ComparisonInput) { input.BalanceReconciliation.ExternalSnapshotID = "other" }, false},
		{"excessive skew", func(input *ComparisonInput) { input.External.BalanceObservedAt = now.Add(-10 * time.Minute) }, false},
		{"missing reconciliation", func(input *ComparisonInput) { input.BalanceReconciliation.EvidenceRef = "" }, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := base
			input.Legacy.DecimalAmounts = cloneStringMap(base.Legacy.DecimalAmounts)
			input.External.DecimalAmounts = cloneStringMap(base.External.DecimalAmounts)
			testCase.mutate(&input)
			report, err := NewReportService(&recordingReportRepository{}).CompareAndPersist(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed != testCase.wantPass {
				t.Fatalf("passed=%v comparison=%#v", report.Passed, report.DecimalAmounts[MetricBalance])
			}
		})
	}
}

func TestComparisonRequiresCurrencyRanksReconciliationAndPerMetricVersions(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	base := completeComparisonInput(WindowDefault, now)
	for _, snapshot := range []*SourceSnapshot{&base.Legacy, &base.External} {
		snapshot.CurrencyAmounts = map[string]map[string]string{
			MetricRawCost:         {"USD": "1.23", "CNY": "8.84"},
			MetricRevenue:         {"USD": "2.00", "CNY": "14.36"},
			MetricProcurementCost: {"USD": "0.75", "CNY": "5.39"},
			MetricProfit:          {"USD": "1.25", "CNY": "8.97"},
			MetricBalance:         {"USD": "81.50", "CNY": "585.17"},
		}
		snapshot.Ranks = map[string]int64{"account-1": 1, "account-2": 2}
		snapshot.ReconciliationCounts = map[string]int64{"matched": 10, "pending": 2, "conflict": 1, "unattributed": 3, "exception": 4}
		snapshot.MetricVersions = map[string]MetricVersionEvidence{
			MetricRawCost:         {RateVersion: "rate-v4", CalculationVersion: "cost-v2"},
			MetricRevenue:         {RateVersion: "rate-v4", CalculationVersion: "revenue-v2"},
			MetricProcurementCost: {RateVersion: "rate-v4", CalculationVersion: "procurement-v2"},
			MetricProfit:          {RateVersion: "rate-v4", CalculationVersion: "profit-v3"},
			MetricProfitMargin:    {RateVersion: "rate-v4", CalculationVersion: "margin-v3"},
			MetricMultiplier:      {RateVersion: "rate-v4", CalculationVersion: "multiplier-v2"},
			MetricScore:           {RateVersion: "rate-v4", CalculationVersion: "score-v2"},
			MetricRank:            {RateVersion: "rate-v4", CalculationVersion: "rank-v2"},
		}
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*ComparisonInput)
	}{
		{"currency drift", func(input *ComparisonInput) { input.External.CurrencyAmounts[MetricRevenue]["CNY"] = "14.35" }},
		{"rank swap", func(input *ComparisonInput) {
			input.External.Ranks["account-1"], input.External.Ranks["account-2"] = 2, 1
		}},
		{"missing reconciliation dimension", func(input *ComparisonInput) { delete(input.External.ReconciliationCounts, "unattributed") }},
		{"metric version drift", func(input *ComparisonInput) {
			input.External.MetricVersions[MetricProfit] = MetricVersionEvidence{RateVersion: "rate-v4", CalculationVersion: "profit-v4"}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := cloneComparisonInput(base)
			testCase.mutate(&input)
			report, err := NewReportService(&recordingReportRepository{}).CompareAndPersist(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed {
				t.Fatalf("schema drift passed: %#v", report)
			}
		})
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneComparisonInput(source ComparisonInput) ComparisonInput {
	result := source
	result.Legacy.CurrencyAmounts = cloneNestedStringMap(source.Legacy.CurrencyAmounts)
	result.External.CurrencyAmounts = cloneNestedStringMap(source.External.CurrencyAmounts)
	result.Legacy.Ranks = cloneIntMap(source.Legacy.Ranks)
	result.External.Ranks = cloneIntMap(source.External.Ranks)
	result.Legacy.ReconciliationCounts = cloneIntMap(source.Legacy.ReconciliationCounts)
	result.External.ReconciliationCounts = cloneIntMap(source.External.ReconciliationCounts)
	result.Legacy.MetricVersions = cloneVersionMap(source.Legacy.MetricVersions)
	result.External.MetricVersions = cloneVersionMap(source.External.MetricVersions)
	return result
}

func cloneNestedStringMap(source map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string, len(source))
	for key, value := range source {
		result[key] = cloneStringMap(value)
	}
	return result
}

func cloneIntMap(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneVersionMap(source map[string]MetricVersionEvidence) map[string]MetricVersionEvidence {
	result := make(map[string]MetricVersionEvidence, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
