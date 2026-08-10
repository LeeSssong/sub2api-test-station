package compare

import "time"

type ReadMode string

const (
	LegacyOnly        ReadMode = "legacy_only"
	ShadowBuilding    ReadMode = "shadow_building"
	DualReadComparing ReadMode = "dual_read_comparing"
	ExternalPrimary   ReadMode = "external_primary"
	LegacyRetired     ReadMode = "legacy_retired"
)

type Page string

const (
	PageAccountMonitor Page = "monitor"
	PageProfitability  Page = "profitability"
	PageAccounting     Page = "accounting"
	PageReconciliation Page = "reconciliation"
)

type WindowKind string

const (
	WindowMinimum WindowKind = "minimum"
	WindowDefault WindowKind = "default"
	WindowMaximum WindowKind = "maximum"
)

const (
	MetricAccountCount                 = "account_count"
	MetricRequestCount                 = "request_count"
	MetricBillCount                    = "bill_count"
	MetricTokenCount                   = "token_count"
	MetricRank                         = "rank"
	MetricReconciliationExceptionCount = "reconciliation_exception_count"
	MetricAccountIDs                   = "account_ids"
	MetricRequestIDs                   = "request_ids"
	MetricBillIDs                      = "bill_ids"
	MetricReconciliationExceptionIDs   = "reconciliation_exception_ids"
	MetricRawCost                      = "raw_cost"
	MetricRevenue                      = "revenue"
	MetricProcurementCost              = "procurement_cost"
	MetricProfit                       = "profit"
	MetricProfitMargin                 = "profit_margin"
	MetricBalance                      = "balance"
	MetricMultiplier                   = "multiplier"
	MetricScore                        = "score"
	MetricRateVersion                  = "rate_version"
	MetricCalculationVersion           = "calculation_version"
)

var requiredCountMetrics = []string{
	MetricAccountCount,
	MetricRequestCount,
	MetricBillCount,
	MetricTokenCount,
}

var requiredIdentifierMetrics = []string{
	MetricAccountIDs,
	MetricRequestIDs,
	MetricBillIDs,
	MetricReconciliationExceptionIDs,
}

var requiredDecimalMetrics = []string{
	MetricRawCost,
	MetricRevenue,
	MetricProcurementCost,
	MetricProfit,
	MetricProfitMargin,
	MetricBalance,
	MetricMultiplier,
	MetricScore,
}

var requiredCurrencyMetrics = []string{MetricRawCost, MetricRevenue, MetricProcurementCost, MetricProfit, MetricBalance}
var requiredCurrencies = []string{"USD", "CNY"}
var requiredReconciliationDimensions = []string{"matched", "pending", "conflict", "unattributed", "exception"}
var requiredDerivedMetrics = []string{MetricRawCost, MetricRevenue, MetricProcurementCost, MetricProfit, MetricProfitMargin, MetricMultiplier, MetricScore, MetricRank}

var requiredRateVersionMetrics = []string{MetricRateVersion, MetricCalculationVersion}

type FreshnessEvidence struct {
	GeneratedAt     time.Time `json:"generated_at"`
	SourceWatermark string    `json:"source_watermark"`
	Complete        bool      `json:"complete"`
	FreshUntil      time.Time `json:"fresh_until"`
}

type SourceSnapshot struct {
	Page                  Page                             `json:"page"`
	Window                WindowKind                       `json:"window"`
	RunID                 string                           `json:"run_id"`
	EvidenceLineage       string                           `json:"evidence_lineage"`
	SnapshotID            string                           `json:"snapshot_id"`
	SnapshotDigest        string                           `json:"snapshot_digest"`
	WindowStart           time.Time                        `json:"window_start"`
	WindowEnd             time.Time                        `json:"window_end"`
	Counts                map[string]int64                 `json:"counts"`
	Identifiers           map[string][]string              `json:"identifiers"`
	DecimalAmounts        map[string]string                `json:"decimal_amounts"`
	CurrencyAmounts       map[string]map[string]string     `json:"currency_amounts"`
	Ranks                 map[string]int64                 `json:"ranks"`
	ReconciliationCounts  map[string]int64                 `json:"reconciliation_counts"`
	MetricVersions        map[string]MetricVersionEvidence `json:"metric_versions"`
	RateVersions          map[string]string                `json:"rate_versions"`
	Freshness             FreshnessEvidence                `json:"freshness"`
	BalanceObservedAt     time.Time                        `json:"balance_observed_at"`
	BalanceSourceEvidence string                           `json:"balance_source_evidence"`
	ContractComplete      bool                             `json:"contract_complete"`
	Degraded              bool                             `json:"degraded"`
	DegradedReason        string                           `json:"degraded_reason,omitempty"`
}

type CheckEvidence struct {
	Passed      bool   `json:"passed"`
	EvidenceRef string `json:"evidence_ref"`
}

func (e CheckEvidence) valid() bool {
	return e.Passed && e.EvidenceRef != ""
}

type CountComparison struct {
	Legacy   int64 `json:"legacy"`
	External int64 `json:"external"`
	Matched  bool  `json:"matched"`
	Missing  bool  `json:"missing"`
}

type IdentifierComparison struct {
	Legacy   []string `json:"legacy"`
	External []string `json:"external"`
	Matched  bool     `json:"matched"`
	Missing  bool     `json:"missing"`
}

type DecimalComparison struct {
	Legacy                  string    `json:"legacy"`
	External                string    `json:"external"`
	Matched                 bool      `json:"matched"`
	Missing                 bool      `json:"missing"`
	ObservationGapExplained bool      `json:"observation_gap_explained"`
	LegacyObservedAt        time.Time `json:"legacy_observed_at,omitempty"`
	ExternalObservedAt      time.Time `json:"external_observed_at,omitempty"`
	LegacySourceEvidence    string    `json:"legacy_source_evidence,omitempty"`
	ExternalSourceEvidence  string    `json:"external_source_evidence,omitempty"`
}

type VersionComparison struct {
	Legacy   string `json:"legacy"`
	External string `json:"external"`
	Matched  bool   `json:"matched"`
	Missing  bool   `json:"missing"`
}

type MetricVersionEvidence struct {
	RateVersion        string `json:"rate_version"`
	CalculationVersion string `json:"calculation_version"`
}

type MetricVersionComparison struct {
	Legacy   MetricVersionEvidence `json:"legacy"`
	External MetricVersionEvidence `json:"external"`
	Matched  bool                  `json:"matched"`
	Missing  bool                  `json:"missing"`
}

type FreshnessComparison struct {
	Legacy   FreshnessEvidence `json:"legacy"`
	External FreshnessEvidence `json:"external"`
	Passed   bool              `json:"passed"`
}

type DegradationEvidence struct {
	Degraded bool   `json:"degraded"`
	Reason   string `json:"reason,omitempty"`
}

type CompareReport struct {
	ID                   string                                  `json:"id"`
	ReportSetID          string                                  `json:"report_set_id,omitempty"`
	RunID                string                                  `json:"run_id,omitempty"`
	EvidenceLineage      string                                  `json:"evidence_lineage,omitempty"`
	Page                 Page                                    `json:"page"`
	Window               WindowKind                              `json:"window"`
	WindowStart          time.Time                               `json:"window_start"`
	WindowEnd            time.Time                               `json:"window_end"`
	Counts               map[string]CountComparison              `json:"counts"`
	Identifiers          map[string]IdentifierComparison         `json:"identifiers"`
	DecimalAmounts       map[string]DecimalComparison            `json:"decimal_amounts"`
	CurrencyAmounts      map[string]map[string]DecimalComparison `json:"currency_amounts"`
	Ranks                map[string]CountComparison              `json:"ranks"`
	ReconciliationCounts map[string]CountComparison              `json:"reconciliation_counts"`
	MetricVersions       map[string]MetricVersionComparison      `json:"metric_versions"`
	RateVersions         map[string]VersionComparison            `json:"rate_versions"`
	Freshness            FreshnessComparison                     `json:"freshness"`
	Permission           CheckEvidence                           `json:"permission"`
	Export               CheckEvidence                           `json:"export"`
	Degraded             DegradationEvidence                     `json:"degraded"`
	Rollback             CheckEvidence                           `json:"rollback"`
	ContractComplete     bool                                    `json:"contract_complete"`
	Passed               bool                                    `json:"passed"`
	MismatchReasons      []string                                `json:"mismatch_reasons,omitempty"`
	Operator             string                                  `json:"operator"`
	ComparedAt           time.Time                               `json:"compared_at"`
	PersistedAt          time.Time                               `json:"persisted_at"`
}

func (r CompareReport) Eligible() bool {
	return r.Passed && r.Permission.valid() && r.Export.valid() && r.Rollback.valid() &&
		r.Freshness.Passed && r.ContractComplete && !r.Degraded.Degraded
}

type ComparisonInput struct {
	Legacy                SourceSnapshot
	External              SourceSnapshot
	Permission            CheckEvidence
	Export                CheckEvidence
	Rollback              CheckEvidence
	Operator              string
	ComparedAt            time.Time
	BalanceReconciliation BalanceReconciliationEvidence
}

type BalanceReconciliationEvidence struct {
	EvidenceRef        string `json:"evidence_ref"`
	LegacySnapshotID   string `json:"legacy_snapshot_id"`
	ExternalSnapshotID string `json:"external_snapshot_id"`
}

type WindowDefinition struct {
	Kind     WindowKind    `json:"kind"`
	Duration time.Duration `json:"duration"`
}

type ReportSetInput struct {
	SetID           string            `json:"set_id"`
	RunID           string            `json:"run_id"`
	Page            Page              `json:"page"`
	Operator        string            `json:"operator"`
	EvidenceLineage string            `json:"evidence_lineage"`
	ComparedAt      time.Time         `json:"compared_at"`
	Comparisons     []ComparisonInput `json:"comparisons"`
}

type CompareReportSet struct {
	ID              string          `json:"id"`
	RunID           string          `json:"run_id"`
	Page            Page            `json:"page"`
	Operator        string          `json:"operator"`
	EvidenceLineage string          `json:"evidence_lineage"`
	ComparedAt      time.Time       `json:"compared_at"`
	PersistedAt     time.Time       `json:"persisted_at"`
	Reports         []CompareReport `json:"reports"`
}

func (s CompareReportSet) Eligible() bool {
	if s.ID == "" || s.RunID == "" || s.Page == "" || s.Operator == "" || s.EvidenceLineage == "" || s.ComparedAt.IsZero() || s.PersistedAt.IsZero() || len(s.Reports) != 3 {
		return false
	}
	seen := make(map[WindowKind]bool, 3)
	for _, report := range s.Reports {
		if report.ReportSetID != s.ID || report.RunID != s.RunID || report.EvidenceLineage != s.EvidenceLineage ||
			report.PersistedAt.IsZero() || !report.PersistedAt.Equal(s.PersistedAt) ||
			report.Page != s.Page || report.Operator != s.Operator || !report.ComparedAt.Equal(s.ComparedAt) ||
			!report.Eligible() || seen[report.Window] {
			return false
		}
		seen[report.Window] = true
	}
	return seen[WindowMinimum] && seen[WindowDefault] && seen[WindowMaximum]
}

type RetirementEvidence struct {
	Passed      bool      `json:"passed"`
	EvidenceRef string    `json:"evidence_ref"`
	Operator    string    `json:"operator"`
	RecordedAt  time.Time `json:"recorded_at"`
}

func (e *RetirementEvidence) valid() bool {
	return e != nil && e.Passed && e.EvidenceRef != "" && e.Operator != "" && !e.RecordedAt.IsZero()
}

type CutoverDecision struct {
	Page          Page      `json:"page"`
	RequestedMode ReadMode  `json:"requested_mode"`
	EffectiveMode ReadMode  `json:"effective_mode"`
	UseExternal   bool      `json:"use_external"`
	Degraded      bool      `json:"degraded"`
	Reason        string    `json:"reason"`
	ReportSetID   string    `json:"report_set_id,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	Operator      string    `json:"operator,omitempty"`
	ComparedAt    time.Time `json:"compared_at,omitempty"`
}
