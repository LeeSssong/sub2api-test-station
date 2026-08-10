package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"example.invalid/relay-ops-service/internal/compare"
)

type windowSummary struct {
	Kind     compare.WindowKind `json:"kind"`
	Start    time.Time          `json:"start"`
	End      time.Time          `json:"end"`
	ReportID string             `json:"report_id"`
	Passed   bool               `json:"passed"`
}

type pageSummary struct {
	Page            compare.Page     `json:"page"`
	ReportSetID     string           `json:"report_set_id"`
	RunID           string           `json:"run_id"`
	Operator        string           `json:"operator"`
	ComparedAt      time.Time        `json:"compared_at"`
	Windows         []windowSummary  `json:"windows"`
	PromotionResult string           `json:"promotion_result"`
	RollbackResult  string           `json:"rollback_result"`
	ActiveMode      compare.ReadMode `json:"active_mode"`
}

type rehearsalSummary struct {
	SchemaVersion int           `json:"schema_version"`
	Environment   string        `json:"environment"`
	GeneratedAt   time.Time     `json:"generated_at"`
	Pages         []pageSummary `json:"pages"`
}

func main() {
	output := flag.String("output", "", "absolute output directory")
	rollback := flag.Bool("rollback", false, "exercise rollback after ordered promotion")
	flag.Parse()
	if *output == "" || !filepath.IsAbs(*output) {
		fatal("an absolute --output directory is required")
	}
	if err := os.MkdirAll(*output, 0o700); err != nil {
		fatal("create output directory: %v", err)
	}
	setPath := filepath.Join(*output, "report-sets.jsonl")
	auditPath := filepath.Join(*output, "cutover-audit.jsonl")
	summaryPath := filepath.Join(*output, "summary.json")
	for _, path := range []string{setPath, auditPath, summaryPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fatal("reset %s: %v", filepath.Base(path), err)
		}
	}

	ctx := context.Background()
	comparedAt := time.Now().UTC().Truncate(time.Second)
	repository := compare.NewJSONLReportSetRepository(setPath)
	service := compare.NewReportSetService(repository)
	pages := []compare.Page{compare.PageAccountMonitor, compare.PageProfitability, compare.PageAccounting, compare.PageReconciliation}
	summary := rehearsalSummary{SchemaVersion: 1, Environment: "isolated_local_fixture", GeneratedAt: comparedAt, Pages: make([]pageSummary, 0, len(pages))}
	for _, page := range pages {
		input := reportSetInput(page, comparedAt)
		set, err := service.CompareAndPersistSet(ctx, input)
		if err != nil {
			fatal("persist %s report set: %v", page, err)
		}
		item := pageSummary{Page: page, ReportSetID: set.ID, RunID: set.RunID, Operator: set.Operator, ComparedAt: set.ComparedAt}
		for _, report := range set.Reports {
			item.Windows = append(item.Windows, windowSummary{Kind: report.Window, Start: report.WindowStart, End: report.WindowEnd, ReportID: report.ID, Passed: report.Passed})
		}
		summary.Pages = append(summary.Pages, item)
	}

	authority := compare.NewJSONLCutoverAuthority(auditPath, repository, func() time.Time { return comparedAt }, 10*time.Minute)
	for index, page := range pages {
		record, err := authority.SetMode(ctx, page, compare.ExternalPrimary, 9001, fmt.Sprintf("rehearsal:%s:promote", page), nil)
		if err != nil {
			fatal("promote %s: %v", page, err)
		}
		summary.Pages[index].PromotionResult = record.Result
		summary.Pages[index].ActiveMode = record.EffectiveMode
	}
	if *rollback {
		for index := len(pages) - 1; index >= 0; index-- {
			page := pages[index]
			record, err := authority.SetMode(ctx, page, compare.LegacyOnly, 9001, fmt.Sprintf("rehearsal:%s:rollback", page), nil)
			if err != nil {
				fatal("rollback %s: %v", page, err)
			}
			summary.Pages[index].RollbackResult = record.Result
			summary.Pages[index].ActiveMode = record.EffectiveMode
		}
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fatal("encode summary: %v", err)
	}
	if err := os.WriteFile(summaryPath, append(encoded, '\n'), 0o600); err != nil {
		fatal("write summary: %v", err)
	}
	fmt.Printf("externalization_rehearsal=passed output=%s\n", *output)
}

func reportSetInput(page compare.Page, comparedAt time.Time) compare.ReportSetInput {
	runID := "local-rehearsal-run-" + string(page)
	lineage := "isolated-fixture:task-9:" + string(page)
	result := compare.ReportSetInput{
		SetID: "local-rehearsal-set-" + string(page), RunID: runID, Page: page,
		Operator: "local-rehearsal-operator", EvidenceLineage: lineage, ComparedAt: comparedAt,
	}
	for _, definition := range compare.RequiredWindowDefinitions() {
		legacy := snapshot(page, definition, comparedAt, runID, lineage, "legacy")
		external := snapshot(page, definition, comparedAt, runID, lineage, "external")
		result.Comparisons = append(result.Comparisons, compare.ComparisonInput{
			Legacy: legacy, External: external,
			Permission: compare.CheckEvidence{Passed: true, EvidenceRef: "fixture:permission:admin"},
			Export:     compare.CheckEvidence{Passed: true, EvidenceRef: "fixture:export:csv"},
			Rollback:   compare.CheckEvidence{Passed: true, EvidenceRef: "fixture:rollback:local"},
			Operator:   result.Operator, ComparedAt: comparedAt,
		})
	}
	return result
}

func snapshot(page compare.Page, definition compare.WindowDefinition, comparedAt time.Time, runID, lineage, source string) compare.SourceSnapshot {
	return compare.SourceSnapshot{
		Page: page, Window: definition.Kind, RunID: runID, EvidenceLineage: lineage,
		SnapshotID:     source + ":" + string(page) + ":" + string(definition.Kind),
		SnapshotDigest: "sha256:fixture:" + source + ":" + string(page) + ":" + string(definition.Kind),
		WindowStart:    comparedAt.Add(-definition.Duration), WindowEnd: comparedAt,
		Counts: map[string]int64{
			compare.MetricAccountCount: 2, compare.MetricRequestCount: 3, compare.MetricBillCount: 2, compare.MetricTokenCount: 144,
		},
		Identifiers: map[string][]string{
			compare.MetricAccountIDs: {"account-1", "account-2"}, compare.MetricRequestIDs: {"request-1", "request-2", "request-3"},
			compare.MetricBillIDs: {"bill-1", "bill-2"}, compare.MetricReconciliationExceptionIDs: {"exception-1"},
		},
		DecimalAmounts: map[string]string{
			compare.MetricRawCost: "1.23", compare.MetricRevenue: "2.00", compare.MetricProcurementCost: "0.75", compare.MetricProfit: "1.25",
			compare.MetricProfitMargin: "0.625", compare.MetricBalance: "81.50", compare.MetricMultiplier: "0.58", compare.MetricScore: "91.25",
		},
		CurrencyAmounts: map[string]map[string]string{
			compare.MetricRawCost: {"USD": "1.23", "CNY": "8.84"}, compare.MetricRevenue: {"USD": "2.00", "CNY": "14.36"},
			compare.MetricProcurementCost: {"USD": "0.75", "CNY": "5.39"}, compare.MetricProfit: {"USD": "1.25", "CNY": "8.97"},
			compare.MetricBalance: {"USD": "81.50", "CNY": "585.17"},
		},
		Ranks:                map[string]int64{"account-1": 1, "account-2": 2},
		ReconciliationCounts: map[string]int64{"matched": 10, "pending": 2, "conflict": 1, "unattributed": 3, "exception": 4},
		MetricVersions:       metricVersions(),
		RateVersions:         map[string]string{compare.MetricRateVersion: "rate-v4", compare.MetricCalculationVersion: "fixture-v1"},
		Freshness:            compare.FreshnessEvidence{GeneratedAt: comparedAt.Add(-time.Minute), SourceWatermark: "fixture:event:144", Complete: true, FreshUntil: comparedAt.Add(5 * time.Minute)},
		BalanceObservedAt:    comparedAt.Add(-time.Minute), BalanceSourceEvidence: "fixture:balance:" + source,
		ContractComplete: true,
	}
}

func metricVersions() map[string]compare.MetricVersionEvidence {
	return map[string]compare.MetricVersionEvidence{
		compare.MetricRawCost:         {RateVersion: "rate-v4", CalculationVersion: "cost-v2"},
		compare.MetricRevenue:         {RateVersion: "rate-v4", CalculationVersion: "revenue-v2"},
		compare.MetricProcurementCost: {RateVersion: "rate-v4", CalculationVersion: "procurement-v2"},
		compare.MetricProfit:          {RateVersion: "rate-v4", CalculationVersion: "profit-v3"},
		compare.MetricProfitMargin:    {RateVersion: "rate-v4", CalculationVersion: "margin-v3"},
		compare.MetricMultiplier:      {RateVersion: "rate-v4", CalculationVersion: "multiplier-v2"},
		compare.MetricScore:           {RateVersion: "rate-v4", CalculationVersion: "score-v2"},
		compare.MetricRank:            {RateVersion: "rate-v4", CalculationVersion: "rank-v2"},
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "externalization rehearsal failed: "+format+"\n", args...)
	os.Exit(1)
}
