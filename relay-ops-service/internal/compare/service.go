package compare

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"github.com/shopspring/decimal"
)

const MetricSchemaV1 = "relay-ops-comparison-v1"

type Metrics struct {
	SourceKind        string
	SourceRef         string
	ModelID           string
	WindowStart       time.Time
	WindowEnd         time.Time
	SampleCount       int64
	SuccessRate       float64
	SSECompletionRate float64
	TTFTP95MS         float64
	LatencyP95MS      float64
	Rate429           float64
	Rate5XX           float64
	TimeoutRate       float64
	CostMultiplier    domain.MultiplierBPS
}

type NativeMetrics Metrics
type ProbeMetrics Metrics

type ComparisonWindow struct {
	SchemaVersion string
	ModelID       string
	WindowStart   time.Time
	WindowEnd     time.Time
	Production    Metrics
	Candidate     Metrics
}

func Materialize(production NativeMetrics, candidate ProbeMetrics) (ComparisonWindow, error) {
	left := Metrics(production)
	right := Metrics(candidate)
	if left.ModelID == "" || left.ModelID != right.ModelID {
		return ComparisonWindow{}, fmt.Errorf("comparison models are incompatible")
	}
	if left.SourceKind == "" || right.SourceKind == "" || left.SourceKind == right.SourceKind {
		return ComparisonWindow{}, fmt.Errorf("comparison sources must remain distinct")
	}
	start := latest(left.WindowStart, right.WindowStart)
	end := earliest(left.WindowEnd, right.WindowEnd)
	if !end.After(start) {
		return ComparisonWindow{}, fmt.Errorf("comparison windows do not overlap")
	}
	if left.SampleCount <= 0 || right.SampleCount <= 0 {
		return ComparisonWindow{}, fmt.Errorf("comparison requires samples")
	}
	return ComparisonWindow{SchemaVersion: MetricSchemaV1, ModelID: left.ModelID, WindowStart: start.UTC(), WindowEnd: end.UTC(), Production: left, Candidate: right}, nil
}

type Policy struct {
	TTFTImprovementRate float64
	CostImprovementRate float64
	QualityTolerance    float64
}

func DefaultPolicy() Policy {
	return Policy{TTFTImprovementRate: 0.20, CostImprovementRate: 0.10, QualityTolerance: 0.005}
}

type CandidateComparison struct {
	MoreStable    bool
	FasterTTFT    bool
	Cheaper       bool
	OverallBetter bool
	Status        string
}

const (
	StatusBlocked                 = "blocked"
	StatusNeedsEvidence           = "needs_evidence"
	StatusNotBetter               = "not_better"
	StatusReviewRecommended       = "review_recommended"
	StatusEligibleForManualSwitch = "eligible_for_manual_switch"
)

type QualityEvidence struct {
	TechnicalPassed       bool
	EvidenceFresh         bool
	BaselineKnown         bool
	GatewayKnown          bool
	PricingKnown          bool
	ReliabilityScore      int
	LatencyScore          int
	GenerationScore       int
	CapacityScore         int
	PriceScore            int
	ReliabilityRegression bool
	LatencyRegression     bool
	MaterialImprovement   bool
}

type QualityDecision struct {
	Status          string
	Eligible        bool
	HardGateReasons []string
	MissingEvidence []string
	QualityScore    int
	TotalScore      int
}

func EvaluateQualityFirst(evidence QualityEvidence) QualityDecision {
	reasons := make([]string, 0, 3)
	if !evidence.TechnicalPassed {
		reasons = append(reasons, "technical_failure")
	}
	if !evidence.EvidenceFresh {
		reasons = append(reasons, "stale_evidence")
	}
	if evidence.ReliabilityRegression {
		reasons = append(reasons, "reliability_regression")
	}
	if evidence.LatencyRegression {
		reasons = append(reasons, "latency_regression")
	}
	missing := make([]string, 0, 3)
	if !evidence.BaselineKnown {
		missing = append(missing, "production_baseline")
	}
	if !evidence.GatewayKnown {
		missing = append(missing, "gateway_measurement")
	}
	if !evidence.PricingKnown {
		missing = append(missing, "verified_pricing")
	}
	quality := bounded(evidence.ReliabilityScore, 40) + bounded(evidence.LatencyScore, 25) +
		bounded(evidence.GenerationScore, 10) + bounded(evidence.CapacityScore, 15)
	total := quality + bounded(evidence.PriceScore, 10)
	status := StatusNotBetter
	switch {
	case len(reasons) > 0:
		status = StatusBlocked
	case quality < 80:
		status = StatusNotBetter
	case len(missing) > 0:
		status = StatusNeedsEvidence
	case !evidence.MaterialImprovement:
		status = StatusNotBetter
	default:
		status = StatusEligibleForManualSwitch
	}
	return QualityDecision{
		Status: status, Eligible: status == StatusEligibleForManualSwitch,
		HardGateReasons: reasons, MissingEvidence: missing, QualityScore: quality, TotalScore: total,
	}
}

func bounded(value, maximum int) int {
	if value < 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

func Classify(window ComparisonWindow, policy Policy) CandidateComparison {
	production := window.Production
	candidate := window.Candidate
	qualityOK := candidate.SuccessRate+policy.QualityTolerance >= production.SuccessRate &&
		candidate.SSECompletionRate+policy.QualityTolerance >= production.SSECompletionRate &&
		candidate.Rate429 <= production.Rate429+policy.QualityTolerance &&
		candidate.Rate5XX <= production.Rate5XX+policy.QualityTolerance &&
		candidate.TimeoutRate <= production.TimeoutRate+policy.QualityTolerance
	moreStable := qualityOK && (candidate.SuccessRate > production.SuccessRate || candidate.SSECompletionRate > production.SSECompletionRate || candidate.Rate429 < production.Rate429 || candidate.Rate5XX < production.Rate5XX || candidate.TimeoutRate < production.TimeoutRate)
	faster := production.TTFTP95MS > 0 && candidate.TTFTP95MS <= production.TTFTP95MS*(1-policy.TTFTImprovementRate)
	cheaper := production.CostMultiplier > 0 && float64(candidate.CostMultiplier) <= float64(production.CostMultiplier)*(1-policy.CostImprovementRate)
	overall := qualityOK && (faster || cheaper)
	status := "not_better"
	if overall {
		status = "consider_recheck"
	} else if !qualityOK {
		status = "quality_regression"
	}
	return CandidateComparison{MoreStable: moreStable, FasterTTFT: faster, Cheaper: cheaper, OverallBetter: overall, Status: status}
}

func latest(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
func earliest(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}

type ReportRepository interface {
	SaveCompareReport(context.Context, CompareReport) error
}

type ReportSetRepository interface {
	SaveReportSet(context.Context, CompareReportSet) error
	LoadReportSets(context.Context, Page) ([]CompareReportSet, error)
}

type JSONLReportRepository struct {
	path string
	mu   sync.Mutex
}

func NewJSONLReportRepository(path string) *JSONLReportRepository {
	return &JSONLReportRepository{path: path}
}

func (r *JSONLReportRepository) SaveCompareReport(ctx context.Context, report CompareReport) error {
	if r == nil || r.path == "" {
		return fmt.Errorf("comparison report path is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode comparison report: %w", err)
	}
	encoded = append(encoded, '\n')
	r.mu.Lock()
	defer r.mu.Unlock()
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open comparison report: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat comparison report: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("comparison report must be a regular 0600 file")
	}
	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("append comparison report: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync comparison report: %w", err)
	}
	return nil
}

func (r *JSONLReportRepository) LoadCompareReports(ctx context.Context, page Page) ([]CompareReport, error) {
	if r == nil || r.path == "" {
		return nil, fmt.Errorf("comparison report path is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	file, err := os.Open(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open comparison report: %w", err)
	}
	defer file.Close()
	result := make([]CompareReport, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var report CompareReport
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		if err := decoder.Decode(&report); err != nil {
			return nil, fmt.Errorf("decode comparison report: %w", err)
		}
		if page == "" || report.Page == page {
			result = append(result, report)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan comparison report: %w", err)
	}
	return result, nil
}

type ReportService struct {
	repository ReportRepository
}

func NewReportService(repository ReportRepository) *ReportService {
	return &ReportService{repository: repository}
}

func (s *ReportService) CompareAndPersist(ctx context.Context, input ComparisonInput) (CompareReport, error) {
	if s == nil || s.repository == nil {
		return CompareReport{}, fmt.Errorf("compare report repository is required")
	}
	report, err := buildCompareReport(input)
	if err != nil {
		return CompareReport{}, err
	}
	if err := s.repository.SaveCompareReport(ctx, report); err != nil {
		return CompareReport{}, fmt.Errorf("persist compare report: %w", err)
	}
	return report, nil
}

func buildCompareReport(input ComparisonInput) (CompareReport, error) {
	if input.Operator == "" || input.ComparedAt.IsZero() {
		return CompareReport{}, fmt.Errorf("comparison operator and timestamp are required")
	}
	if input.Legacy.Page == "" || input.Legacy.Page != input.External.Page ||
		input.Legacy.Window == "" || input.Legacy.Window != input.External.Window ||
		!input.Legacy.WindowStart.Equal(input.External.WindowStart) ||
		!input.Legacy.WindowEnd.Equal(input.External.WindowEnd) ||
		!input.Legacy.WindowEnd.After(input.Legacy.WindowStart) {
		return CompareReport{}, fmt.Errorf("comparison page and window must match exactly")
	}

	report := CompareReport{
		ID:                   fmt.Sprintf("%s:%s:%d", input.Legacy.Page, input.Legacy.Window, input.ComparedAt.UnixNano()),
		Page:                 input.Legacy.Page,
		Window:               input.Legacy.Window,
		WindowStart:          input.Legacy.WindowStart.UTC(),
		WindowEnd:            input.Legacy.WindowEnd.UTC(),
		Counts:               make(map[string]CountComparison, len(requiredCountMetrics)),
		Identifiers:          make(map[string]IdentifierComparison, len(requiredIdentifierMetrics)),
		DecimalAmounts:       make(map[string]DecimalComparison, len(requiredDecimalMetrics)),
		CurrencyAmounts:      make(map[string]map[string]DecimalComparison, len(requiredCurrencyMetrics)),
		Ranks:                make(map[string]CountComparison),
		ReconciliationCounts: make(map[string]CountComparison, len(requiredReconciliationDimensions)),
		MetricVersions:       make(map[string]MetricVersionComparison, len(requiredDerivedMetrics)),
		RateVersions:         make(map[string]VersionComparison, len(requiredRateVersionMetrics)),
		Permission:           input.Permission,
		Export:               input.Export,
		Rollback:             input.Rollback,
		ContractComplete:     input.Legacy.ContractComplete && input.External.ContractComplete,
		Operator:             input.Operator,
		ComparedAt:           input.ComparedAt.UTC(),
		PersistedAt:          time.Now().UTC(),
	}

	passed := true
	addMismatch := func(reason string) {
		passed = false
		report.MismatchReasons = append(report.MismatchReasons, reason)
	}
	for _, metric := range requiredCountMetrics {
		legacy, legacyOK := input.Legacy.Counts[metric]
		external, externalOK := input.External.Counts[metric]
		comparison := CountComparison{Legacy: legacy, External: external, Matched: legacyOK && externalOK && legacy == external, Missing: !legacyOK || !externalOK}
		report.Counts[metric] = comparison
		if !comparison.Matched {
			addMismatch("count:" + metric)
		}
	}
	for _, metric := range requiredIdentifierMetrics {
		legacy, legacyOK := input.Legacy.Identifiers[metric]
		external, externalOK := input.External.Identifiers[metric]
		legacy = sortedCopy(legacy)
		external = sortedCopy(external)
		comparison := IdentifierComparison{Legacy: legacy, External: external, Matched: legacyOK && externalOK && slices.Equal(legacy, external), Missing: !legacyOK || !externalOK}
		report.Identifiers[metric] = comparison
		if !comparison.Matched {
			addMismatch("identifier:" + metric)
		}
	}
	for _, metric := range requiredDecimalMetrics {
		legacyText, legacyOK := input.Legacy.DecimalAmounts[metric]
		externalText, externalOK := input.External.DecimalAmounts[metric]
		legacyValue, legacyErr := decimal.NewFromString(legacyText)
		externalValue, externalErr := decimal.NewFromString(externalText)
		matched := legacyOK && externalOK && legacyErr == nil && externalErr == nil && legacyValue.Equal(externalValue)
		comparison := DecimalComparison{Legacy: legacyText, External: externalText, Matched: matched, Missing: !legacyOK || !externalOK}
		if metric == MetricBalance {
			comparison.LegacyObservedAt = input.Legacy.BalanceObservedAt
			comparison.ExternalObservedAt = input.External.BalanceObservedAt
			comparison.LegacySourceEvidence = input.Legacy.BalanceSourceEvidence
			comparison.ExternalSourceEvidence = input.External.BalanceSourceEvidence
			if !matched && legacyOK && externalOK && legacyErr == nil && externalErr == nil {
				comparison.ObservationGapExplained = balanceGapExplained(input.Legacy, input.External, input.BalanceReconciliation)
			}
		}
		report.DecimalAmounts[metric] = comparison
		if !matched && !comparison.ObservationGapExplained {
			addMismatch("decimal:" + metric)
		}
	}
	for _, metric := range requiredCurrencyMetrics {
		report.CurrencyAmounts[metric] = make(map[string]DecimalComparison, len(requiredCurrencies))
		for _, currency := range requiredCurrencies {
			legacyText, legacyOK := input.Legacy.CurrencyAmounts[metric][currency]
			externalText, externalOK := input.External.CurrencyAmounts[metric][currency]
			legacyValue, legacyErr := decimal.NewFromString(legacyText)
			externalValue, externalErr := decimal.NewFromString(externalText)
			matched := legacyOK && externalOK && legacyErr == nil && externalErr == nil && legacyValue.Equal(externalValue)
			comparison := DecimalComparison{Legacy: legacyText, External: externalText, Matched: matched, Missing: !legacyOK || !externalOK}
			report.CurrencyAmounts[metric][currency] = comparison
			if !matched {
				addMismatch("currency:" + metric + ":" + currency)
			}
		}
	}
	if len(input.Legacy.Ranks) == 0 || len(input.External.Ranks) == 0 {
		addMismatch("ranks:missing")
	}
	for entityID, legacy := range input.Legacy.Ranks {
		external, ok := input.External.Ranks[entityID]
		comparison := CountComparison{Legacy: legacy, External: external, Matched: ok && legacy == external, Missing: !ok}
		report.Ranks[entityID] = comparison
		if !comparison.Matched {
			addMismatch("rank:" + entityID)
		}
	}
	for entityID, external := range input.External.Ranks {
		if _, ok := input.Legacy.Ranks[entityID]; !ok {
			report.Ranks[entityID] = CountComparison{External: external, Missing: true}
			addMismatch("rank:" + entityID)
		}
	}
	for _, dimension := range requiredReconciliationDimensions {
		legacy, legacyOK := input.Legacy.ReconciliationCounts[dimension]
		external, externalOK := input.External.ReconciliationCounts[dimension]
		comparison := CountComparison{Legacy: legacy, External: external, Matched: legacyOK && externalOK && legacy == external, Missing: !legacyOK || !externalOK}
		report.ReconciliationCounts[dimension] = comparison
		if !comparison.Matched {
			addMismatch("reconciliation:" + dimension)
		}
	}
	for _, metric := range requiredDerivedMetrics {
		legacy, legacyOK := input.Legacy.MetricVersions[metric]
		external, externalOK := input.External.MetricVersions[metric]
		matched := legacyOK && externalOK && legacy.RateVersion != "" && legacy.CalculationVersion != "" && legacy == external
		report.MetricVersions[metric] = MetricVersionComparison{Legacy: legacy, External: external, Matched: matched, Missing: !legacyOK || !externalOK}
		if !matched {
			addMismatch("metric_version:" + metric)
		}
	}
	for _, metric := range requiredRateVersionMetrics {
		legacy, legacyOK := input.Legacy.RateVersions[metric]
		external, externalOK := input.External.RateVersions[metric]
		comparison := VersionComparison{Legacy: legacy, External: external, Matched: legacyOK && externalOK && legacy != "" && legacy == external, Missing: !legacyOK || !externalOK}
		report.RateVersions[metric] = comparison
		if !comparison.Matched {
			addMismatch("version:" + metric)
		}
	}

	report.Freshness = FreshnessComparison{
		Legacy:   input.Legacy.Freshness,
		External: input.External.Freshness,
		Passed:   freshnessPassed(input.Legacy.Freshness, input.ComparedAt) && freshnessPassed(input.External.Freshness, input.ComparedAt),
	}
	if !report.Freshness.Passed {
		addMismatch("freshness")
	}
	if !report.ContractComplete {
		addMismatch("contract_incomplete")
	}
	if !input.Permission.valid() {
		addMismatch("permission")
	}
	if !input.Export.valid() {
		addMismatch("export")
	}
	if !input.Rollback.valid() {
		addMismatch("rollback")
	}
	report.Degraded = DegradationEvidence{
		Degraded: input.Legacy.Degraded || input.External.Degraded,
		Reason:   firstNonEmpty(input.Legacy.DegradedReason, input.External.DegradedReason),
	}
	if report.Degraded.Degraded {
		addMismatch("degraded")
	}
	report.Passed = passed
	return report, nil
}

func EvaluatePageCutover(requested ReadMode, page Page, reports []CompareReport, now time.Time, maxAge time.Duration, retirement *RetirementEvidence) CutoverDecision {
	decision := CutoverDecision{Page: page, RequestedMode: requested, EffectiveMode: LegacyOnly, Reason: "legacy_default"}
	switch requested {
	case LegacyOnly:
		return decision
	case ShadowBuilding, DualReadComparing:
		decision.EffectiveMode = requested
		decision.Reason = "legacy_visible_during_comparison"
		return decision
	case ExternalPrimary, LegacyRetired:
	default:
		decision.Degraded = true
		decision.Reason = "invalid_mode"
		return decision
	}

	byWindow := make(map[WindowKind]CompareReport, len(reports))
	for _, report := range reports {
		if report.Page == page {
			byWindow[report.Window] = report
		}
	}
	for _, window := range []WindowKind{WindowMinimum, WindowDefault, WindowMaximum} {
		report, ok := byWindow[window]
		if !ok || !report.Eligible() || maxAge <= 0 || now.Before(report.ComparedAt) || now.Sub(report.ComparedAt) > maxAge || now.After(report.Freshness.External.FreshUntil) {
			decision.Degraded = true
			decision.Reason = "comparison_gate_failed"
			return decision
		}
	}
	if requested == LegacyRetired && !retirement.valid() {
		decision.Degraded = true
		decision.Reason = "retirement_evidence_missing"
		return decision
	}
	decision.EffectiveMode = requested
	decision.UseExternal = true
	decision.Reason = "comparison_gate_passed"
	return decision
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	slices.Sort(result)
	return result
}

const (
	maximumBalanceObservationSkew = 2 * time.Minute
	maximumBalanceVariance        = "0.01"
)

func balanceGapExplained(legacy, external SourceSnapshot, reconciliation BalanceReconciliationEvidence) bool {
	legacyValue, legacyErr := decimal.NewFromString(legacy.DecimalAmounts[MetricBalance])
	externalValue, externalErr := decimal.NewFromString(external.DecimalAmounts[MetricBalance])
	limit, limitErr := decimal.NewFromString(maximumBalanceVariance)
	skew := legacy.BalanceObservedAt.Sub(external.BalanceObservedAt)
	if skew < 0 {
		skew = -skew
	}
	return !legacy.BalanceObservedAt.IsZero() && !external.BalanceObservedAt.IsZero() &&
		!legacy.BalanceObservedAt.Equal(external.BalanceObservedAt) &&
		skew <= maximumBalanceObservationSkew &&
		legacy.SnapshotID != "" && external.SnapshotID != "" &&
		legacy.SnapshotDigest != "" && external.SnapshotDigest != "" &&
		legacy.BalanceSourceEvidence != "" && external.BalanceSourceEvidence != "" &&
		reconciliation.EvidenceRef != "" &&
		reconciliation.LegacySnapshotID == legacy.SnapshotID &&
		reconciliation.ExternalSnapshotID == external.SnapshotID &&
		legacyErr == nil && externalErr == nil && limitErr == nil &&
		legacyValue.Sub(externalValue).Abs().LessThanOrEqual(limit)
}

func freshnessPassed(evidence FreshnessEvidence, comparedAt time.Time) bool {
	return evidence.Complete && !evidence.GeneratedAt.IsZero() && evidence.SourceWatermark != "" &&
		!evidence.FreshUntil.IsZero() && !evidence.GeneratedAt.After(comparedAt) &&
		!comparedAt.After(evidence.FreshUntil)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
