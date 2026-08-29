package service

import (
	"context"
	"time"
)

const (
	AccountMonitorSchemaVersion           = 5
	AccountMonitorDefaultIntervalSeconds  = 300
	AccountMonitorMinIntervalSeconds      = 15
	AccountMonitorMaxIntervalSeconds      = 3600
	AccountMonitorHistoryDays             = 7
	AccountMonitorResultRetentionDays     = 30
	AccountMonitorDefaultHistoryLimit     = 50
	AccountMonitorGroupEvidenceWindow     = 5 * time.Minute
	AccountMonitorGroupEvidenceMinSamples = 3
	AccountMonitorTimelineLimit           = 24
	AccountMonitorDefaultTTFTTargetMS     = 1000
	AccountMonitorDefaultTTFTLimitMS      = 5000
	AccountMonitorDefaultLatencyTargetMS  = 10000
	AccountMonitorDefaultLatencyLimitMS   = 60000
)

type AccountMonitorRange string

const (
	AccountMonitorRange24Hours AccountMonitorRange = "24h"
	AccountMonitorRange7Days   AccountMonitorRange = "7d"
	AccountMonitorRange30Days  AccountMonitorRange = "30d"
)

var DefaultAccountMonitorScoreWeights = AccountMonitorScoreWeights{
	Cost: 15, Success: 45, TTFT: 20, Latency: 20,
	TTFTTargetMS: AccountMonitorDefaultTTFTTargetMS, TTFTLimitMS: AccountMonitorDefaultTTFTLimitMS,
	LatencyTargetMS: AccountMonitorDefaultLatencyTargetMS, LatencyLimitMS: AccountMonitorDefaultLatencyLimitMS,
}

type AccountMonitorScoreWeights struct {
	Cost            int       `json:"cost"`
	Success         int       `json:"success"`
	TTFT            int       `json:"ttft"`
	Latency         int       `json:"latency"`
	TTFTTargetMS    int       `json:"ttft_target_ms"`
	TTFTLimitMS     int       `json:"ttft_limit_ms"`
	LatencyTargetMS int       `json:"latency_target_ms"`
	LatencyLimitMS  int       `json:"latency_limit_ms"`
	UpdatedBy       int64     `json:"updated_by"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AccountMonitorGlobalScoreWeightsResponse struct {
	Cost      int        `json:"cost"`
	Success   int        `json:"success"`
	TTFT      int        `json:"ttft"`
	Latency   int        `json:"latency"`
	UpdatedBy int64      `json:"updated_by"`
	UpdatedAt *time.Time `json:"updated_at"`
	IsDefault bool       `json:"is_default"`
}

type AccountMonitorGroup struct {
	ID                      int64                        `json:"id"`
	Name                    string                       `json:"name"`
	Status                  string                       `json:"status"`
	Platform                string                       `json:"platform"`
	RateMultiplier          float64                      `json:"rate_multiplier"`
	RPMLimit                int                          `json:"rpm_limit"`
	AccountCount            int64                        `json:"account_count"`
	ActiveAccountCount      int64                        `json:"active_account_count"`
	RateLimitedAccountCount int64                        `json:"rate_limited_account_count"`
	CustomerVisible         bool                         `json:"customer_visible"`
	RequirePrivacySet       bool                         `json:"require_privacy_set"`
	NativeOrder             int                          `json:"native_order"`
	ScoreWeights            AccountMonitorScoreWeights   `json:"score_weights"`
	OperationalState        string                       `json:"operational_state"`
	Health                  AccountMonitorHealthSummary  `json:"health"`
	Accounts                []AccountMonitorGroupAccount `json:"accounts,omitempty"`
}

// AccountMonitorHealthSummary is a display-only operational projection. It
// never participates in routing or scheduler selection.
type AccountMonitorHealthSummary struct {
	TotalAccounts       int      `json:"total_accounts"`
	MonitoringAccounts  int      `json:"monitoring_accounts"`
	AvailableAccounts   int      `json:"available_accounts"`
	UnavailableAccounts int      `json:"unavailable_accounts"`
	PendingAccounts     int      `json:"pending_accounts"`
	PausedAccounts      int      `json:"paused_accounts"`
	SuccessRate         float64  `json:"success_rate"`
	SuccessSampleCount  int      `json:"success_sample_count"`
	TTFTSampleCount     int      `json:"ttft_sample_count"`
	LatencySampleCount  int      `json:"latency_sample_count"`
	TTFTP50MS           *float64 `json:"ttft_p50_ms,omitempty"`
	LatencyP95MS        *float64 `json:"latency_p95_ms,omitempty"`
}

type AccountMonitorQualityEvidence struct {
	Source                    string    `json:"source"`
	Known                     bool      `json:"known"`
	Freshness                 string    `json:"freshness"`
	UnknownReason             string    `json:"unknown_reason,omitempty"`
	RealRequestSamples        int       `json:"real_request_sample_count"`
	ProbeSamples              int       `json:"probe_sample_count"`
	RealRequestWeight         float64   `json:"real_request_weight"`
	ProbeWeight               float64   `json:"probe_weight"`
	SampleCount               int       `json:"sample_count"`
	SuccessSampleCount        int       `json:"success_sample_count"`
	TTFTSampleCount           int       `json:"ttft_sample_count"`
	LatencySampleCount        int       `json:"latency_sample_count"`
	SuccessRate               float64   `json:"success_rate"`
	TTFTP50MS                 *float64  `json:"ttft_p50_ms,omitempty"`
	LatencyP95MS              *float64  `json:"latency_p95_ms,omitempty"`
	OutputRateTokensPerSecond *float64  `json:"output_rate_tokens_per_second,omitempty"`
	OutputRateSampleCount     int       `json:"output_rate_sample_count,omitempty"`
	ObservedAt                time.Time `json:"observed_at"`
}

type AccountMonitorScoreBreakdown struct {
	Cost    float64 `json:"cost"`
	Success float64 `json:"success"`
	TTFT    float64 `json:"ttft"`
	Latency float64 `json:"latency"`
}

type AccountMonitorReasonCode string

const (
	AccountMonitorReasonStrategy      AccountMonitorReasonCode = "strategy"
	AccountMonitorReasonQualityGate   AccountMonitorReasonCode = "quality_gate"
	AccountMonitorReasonRuntimeLoad   AccountMonitorReasonCode = "runtime_load"
	AccountMonitorReasonCooldown      AccountMonitorReasonCode = "cooldown"
	AccountMonitorReasonTieBreak      AccountMonitorReasonCode = "tie_break"
	AccountMonitorReasonDataFreshness AccountMonitorReasonCode = "data_freshness"
	AccountMonitorReasonNotEligible   AccountMonitorReasonCode = "not_eligible"
)

type AccountMonitorQualityScoreComponent struct {
	Score float64 `json:"score"`
	Max   float64 `json:"max"`
}

type AccountMonitorQualityExplanationBreakdown struct {
	Cost    AccountMonitorQualityScoreComponent `json:"cost"`
	Success AccountMonitorQualityScoreComponent `json:"success"`
	TTFT    AccountMonitorQualityScoreComponent `json:"ttft"`
	Latency AccountMonitorQualityScoreComponent `json:"latency"`
}

type AccountMonitorQualityExplanation struct {
	Score           *float64                                  `json:"score,omitempty"`
	Rank            *int                                      `json:"rank,omitempty"`
	RankTotal       int                                       `json:"rank_total,omitempty"`
	Breakdown       AccountMonitorQualityExplanationBreakdown `json:"breakdown,omitempty"`
	Window          string                                    `json:"window"`
	SampleCount     int                                       `json:"sample_count"`
	Source          string                                    `json:"source"`
	ObservedAt      time.Time                                 `json:"observed_at"`
	ExperienceLabel string                                    `json:"experience_label,omitempty"`
}

type AccountMonitorSchedulerExplanation struct {
	Rank           *int   `json:"rank,omitempty"`
	RankTotal      int    `json:"rank_total,omitempty"`
	CandidateTotal int    `json:"candidate_total,omitempty"`
	Eligible       bool   `json:"eligible"`
	PolicyKey      string `json:"policy_key,omitempty"`
	PolicyLabel    string `json:"policy_label"`
	// EffectiveWeights is retained for in-process compatibility. It is not
	// serialized because its keys are scheduler implementation details.
	EffectiveWeights   map[string]float64            `json:"-"`
	EffectiveFacts     []AccountMonitorSchedulerFact `json:"effective_facts,omitempty"`
	ModelQuotaParity   string                        `json:"model_quota_parity,omitempty"`
	CandidateScope     string                        `json:"candidate_scope,omitempty"`
	SnapshotAt         *time.Time                    `json:"snapshot_at,omitempty"`
	PrimaryReasonCode  AccountMonitorReasonCode      `json:"primary_reason_code,omitempty"`
	PrimaryReasonLabel string                        `json:"primary_reason_label,omitempty"`
}

type AccountMonitorSchedulerFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type AccountMonitorGroupAccount struct {
	AccountMonitorAccount
	Evidence AccountMonitorQualityEvidence `json:"evidence"`
}

type AccountMonitorSettings struct {
	IntervalSeconds int       `json:"interval_seconds"`
	UpdatedBy       int64     `json:"updated_by"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AccountMonitorProbeResult struct {
	AccountID  int64
	ModelID    string
	Status     string
	ErrorCode  string
	HTTPStatus *int
	TTFTMS     *float64
	LatencyMS  *float64
	CheckedAt  time.Time
}

type AccountMonitorAggregate struct {
	SampleCount        int
	SuccessCount       int
	ErrorCount         int
	SuccessRate        float64
	SuccessSampleCount int
	TTFTSampleCount    int
	LatencySampleCount int
	TTFTP50MS          *float64
	TTFTP95MS          *float64
	LatencyP50MS       *float64
	LatencyP95MS       *float64
	LastCheckedAt      *time.Time
	ConsecutiveFailed  int
}

// AccountMonitorWindowAggregate contains real request evidence from usage_logs.
// Probe observations are intentionally kept separate from these fields.
type AccountMonitorWindowAggregate struct {
	RequestCount              int64
	SuccessCount              int64
	ErrorCount                int64
	BaseCost                  float64
	Revenue                   float64
	AccountCost               float64
	CostComplete              bool
	SuccessRate               float64
	TTFTSampleCount           int
	LatencySampleCount        int
	TTFTP50MS                 *float64
	TTFTP95MS                 *float64
	LatencyP95MS              *float64
	OutputRateTokensPerSecond *float64
	OutputRateSampleCount     int
	LastObservedAt            *time.Time
}

type AccountMonitorRealRequestTimelineRepository interface {
	ListRealRequestTimelines(ctx context.Context, accountIDs []int64, since, until time.Time, bucketCount int) (map[int64][]AccountMonitorRealRequestTimelinePoint, error)
}

type AccountMonitorRealRequestRepository interface {
	ListRealRequestAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error)
}

type AccountMonitorGroupRealRequestRepository interface {
	ListGroupRealRequestAggregates(ctx context.Context, groupIDs, accountIDs []int64, since, until time.Time) (map[int64]map[int64]AccountMonitorWindowAggregate, error)
}

type AccountMonitorRank struct {
	Rank  int `json:"rank"`
	Total int `json:"total"`
}

type AccountMonitorRealRequestEvidence struct {
	RequestCount    int64     `json:"request_count"`
	SuccessCount    int64     `json:"success_count"`
	FailureCount    int64     `json:"failure_count"`
	SuccessRate     float64   `json:"success_rate"`
	TTFTP95MS       *float64  `json:"ttft_p95_ms,omitempty"`
	TTFTSampleCount int       `json:"ttft_sample_count"`
	ObservedAt      time.Time `json:"observed_at"`
}

type AccountMonitorGlobalRanks struct {
	SuccessRate *AccountMonitorRank `json:"success_rate,omitempty"`
	TTFTP95     *AccountMonitorRank `json:"ttft_p95,omitempty"`
}

type AccountMonitorGroupProfitability struct {
	GroupID     int64               `json:"group_id"`
	GroupName   string              `json:"group_name"`
	ProfitRate  *float64            `json:"profit_rate,omitempty"`
	Status      string              `json:"status"`
	Reason      string              `json:"reason,omitempty"`
	Rank        *AccountMonitorRank `json:"rank,omitempty"`
	Revenue     float64             `json:"revenue"`
	AccountCost float64             `json:"account_cost"`
	SampleCount int64               `json:"sample_count"`
}

type AccountMonitorRealRequestTimelinePoint struct {
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	RequestCount int64     `json:"request_count"`
	SuccessCount int64     `json:"success_count"`
	FailureCount int64     `json:"failure_count"`
	TTFTP95MS    *float64  `json:"ttft_p95_ms,omitempty"`
}

type AccountMonitorUsageWindow struct {
	Name        string     `json:"name"`
	Utilization float64    `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Requests    int64      `json:"requests"`
	Tokens      int64      `json:"tokens"`
}

type AccountMonitorLatest struct {
	ModelID    string    `json:"model_id"`
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

const (
	AccountMonitorGroupRecommendationStatusRecommended    = "recommended"
	AccountMonitorGroupRecommendationStatusObserve        = "observe"
	AccountMonitorGroupRecommendationStatusBlocked        = "blocked"
	AccountMonitorGroupRecommendationStatusNotRecommended = "not_recommended"

	AccountMonitorGroupRecommendationTargetPro     = "gpt_pro"
	AccountMonitorGroupRecommendationTargetPlus    = "gpt_plus"
	AccountMonitorGroupRecommendationTargetSpecial = "gpt_special"

	AccountMonitorGroupRecommendationActionKeep    = "keep"
	AccountMonitorGroupRecommendationActionMigrate = "migrate"
	AccountMonitorGroupRecommendationActionHold    = "hold"
	AccountMonitorGroupRecommendationActionNone    = "none"
)

type AccountMonitorGroupRecommendation struct {
	Status      string    `json:"status"`
	Target      string    `json:"target"`
	TargetName  string    `json:"target_name"`
	Action      string    `json:"action"`
	ReasonCodes []string  `json:"reason_codes"`
	SampleCount int       `json:"sample_count"`
	ObservedAt  time.Time `json:"observed_at"`
	Source      string    `json:"source"`
}

type AccountMonitorTimelinePoint struct {
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type MonitorV2GroupAccountScope struct {
	GroupID   int64
	AccountID int64
}

type MonitorV2NativeTimelinePoint struct {
	BucketStart time.Time
	Status      string
	LatencyMS   *float64
	HasResult   bool
}

type MonitorV2NativeGroupProjection struct {
	Status                 string
	SourceUpdatedAt        *time.Time
	OperationalBucketCount int
	TotalBucketCount       int
	TTFTP50MS              *float64
	AverageLatencyMS       *float64
	TTFTSampleCount        int
	LatencySampleCount     int
	Timeline               []MonitorV2NativeTimelinePoint
}

// MonitorV4GroupProjection is the request-weighted real-request projection
// used by the hybrid performance monitor. Probe fallback rows are already
// reduced to one logical request per selected bucket by the repository.
type MonitorV4GroupProjection struct {
	SuccessRate               *float64
	RequestCount              int
	SuccessCount              int
	RealRequestCount          int
	RealSuccessCount          int
	ProbeFallbackBucketCount  int
	ProbeFallbackRequestCount int
	MissingProbeTerminalCount int
	TTFTP95MS                 *float64
	TTFTSampleCount           int
	LatencyP95MS              *float64
	LatencySampleCount        int
	SourceUpdatedAt           *time.Time
	CurrentOperational        bool
}

// AccountMonitorGroupProbeRepository is the native read path used by Monitor V2.
// It is optional so existing Account Monitor repository adapters remain source-compatible.
type AccountMonitorGroupProbeRepository interface {
	ProjectMonitorV2Groups(
		ctx context.Context,
		scopes []MonitorV2GroupAccountScope,
		start, end, freshSince time.Time,
		bucketSize time.Duration,
	) (map[int64]MonitorV2NativeGroupProjection, error)
}

// AccountMonitorHybridProjectionRepository is optional so existing account
// monitor adapters remain source-compatible while the v4 read path rolls out.
type AccountMonitorHybridProjectionRepository interface {
	ProjectMonitorV4Groups(
		ctx context.Context,
		scopes []MonitorV2GroupAccountScope,
		start, end time.Time,
		bucketSize time.Duration,
	) (map[int64]MonitorV4GroupProjection, error)
}

// AccountMonitorHybridProjectionGroupsRepository is the V4 read path variant
// that receives the visible group ids separately from account facts. This
// lets the repository construct a complete group × closed-bucket matrix even
// when a currently visible group has no account scope rows.
type AccountMonitorHybridProjectionGroupsRepository interface {
	ProjectMonitorV4GroupsForGroups(
		ctx context.Context,
		groupIDs []int64,
		scopes []MonitorV2GroupAccountScope,
		start, end time.Time,
		bucketSize time.Duration,
	) (map[int64]MonitorV4GroupProjection, error)
}

type AccountMonitorMultiplier struct {
	Value       *float64   `json:"value,omitempty"`
	Source      string     `json:"source,omitempty"`
	Status      string     `json:"status"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	SampleCount int        `json:"sample_count"`
}

// AccountMonitorRefreshOptions keeps declaration and balance refreshes explicit.
type AccountMonitorRefreshOptions struct {
	RefreshDeclaration bool
	RefreshBalance     bool
}

type AccountMonitorAccount struct {
	AccountID                  int64                                    `json:"account_id"`
	Name                       string                                   `json:"name"`
	Platform                   string                                   `json:"platform"`
	AccountType                string                                   `json:"account_type"`
	Status                     string                                   `json:"status"`
	Schedulable                bool                                     `json:"schedulable"`
	Priority                   int                                      `json:"priority"`
	HomepageURL                string                                   `json:"homepage_url,omitempty"`
	GroupIDs                   []int64                                  `json:"group_ids"`
	GroupNames                 []string                                 `json:"group_names"`
	ModelID                    string                                   `json:"model_id"`
	ConnectionProbeModel       string                                   `json:"connection_probe_model,omitempty"`
	ModelDetection             *AccountModelDetectionProjection         `json:"model_detection,omitempty"`
	LatestStatus               string                                   `json:"latest_status"`
	ErrorCode                  string                                   `json:"error_code,omitempty"`
	ProbeSampleCount           int                                      `json:"probe_sample_count"`
	ProbeSuccessCount          int                                      `json:"probe_success_count"`
	ProbeSuccessRate           float64                                  `json:"probe_success_rate"`
	ProbeTTFTP50MS             *float64                                 `json:"probe_ttft_p50_ms,omitempty"`
	ProbeLatencyP95MS          *float64                                 `json:"probe_latency_p95_ms,omitempty"`
	AvailabilityStatus         string                                   `json:"availability_status"`
	ScoreStatus                string                                   `json:"score_status"`
	SampleCount                int                                      `json:"sample_count"`
	SuccessSampleCount         int                                      `json:"success_sample_count"`
	TTFTSampleCount            int                                      `json:"ttft_sample_count"`
	LatencySampleCount         int                                      `json:"latency_sample_count"`
	SuccessRate                float64                                  `json:"success_rate"`
	TTFTP50MS                  *float64                                 `json:"ttft_p50_ms,omitempty"`
	TTFTP95MS                  *float64                                 `json:"ttft_p95_ms,omitempty"`
	LatencyP95MS               *float64                                 `json:"latency_p95_ms,omitempty"`
	OutputRateTokensPerSecond  *float64                                 `json:"output_rate_tokens_per_second,omitempty"`
	OutputRateSampleCount      int                                      `json:"output_rate_sample_count,omitempty"`
	Multiplier                 AccountMonitorMultiplier                 `json:"multiplier"`
	Balance                    *AccountMonitorBalance                   `json:"balance,omitempty"`
	ProcurementCostCNY         *float64                                 `json:"procurement_cost_cny"`
	EstimatedUsableQuotaUSD    *float64                                 `json:"estimated_usable_quota_usd"`
	ProcurementCostEffectiveAt *time.Time                               `json:"procurement_cost_effective_at"`
	ExpiresAt                  *time.Time                               `json:"expires_at"`
	RequestCount               int64                                    `json:"request_count"`
	ErrorCount                 int64                                    `json:"error_count"`
	Range                      AccountMonitorRange                      `json:"range,omitempty"`
	BaseCost                   float64                                  `json:"base_cost"`
	EffectiveMultiplier        *float64                                 `json:"effective_multiplier,omitempty"`
	EquivalentSiteMultiplier   *float64                                 `json:"equivalent_site_multiplier"`
	CostMode                   string                                   `json:"cost_mode,omitempty"`
	CostScore                  float64                                  `json:"cost_score"`
	QualityScore               *float64                                 `json:"quality_score,omitempty"`
	ScoreBreakdown             *AccountMonitorScoreBreakdown            `json:"score_breakdown,omitempty"`
	EvidenceSource             string                                   `json:"evidence_source,omitempty"`
	GroupRank                  *int                                     `json:"group_rank,omitempty"`
	QualityRank                *int                                     `json:"quality_rank,omitempty"`
	QualityRankTotal           int                                      `json:"quality_rank_total,omitempty"`
	SchedulerRank              *int                                     `json:"scheduler_rank,omitempty"`
	SchedulerRankTotal         int                                      `json:"scheduler_rank_total,omitempty"`
	QualityExplanation         *AccountMonitorQualityExplanation        `json:"quality_explanation,omitempty"`
	SchedulerExplanation       *AccountMonitorSchedulerExplanation      `json:"scheduler_explanation,omitempty"`
	SchedulerUnavailable       bool                                     `json:"scheduler_unavailable,omitempty"`
	Eligible                   bool                                     `json:"eligible"`
	TodayStats                 *WindowStats                             `json:"today_stats,omitempty"`
	UsageWindows               []AccountMonitorUsageWindow              `json:"usage_windows,omitempty"`
	Latest                     *AccountMonitorLatest                    `json:"latest,omitempty"`
	Timeline                   []AccountMonitorTimelinePoint            `json:"timeline"`
	CheckedAt                  *time.Time                               `json:"checked_at,omitempty"`
	Stale                      bool                                     `json:"stale"`
	ManagementState            string                                   `json:"management_state"`
	ServiceState               string                                   `json:"service_state"`
	GroupEligibility           string                                   `json:"group_eligibility"`
	MonitorBucket              string                                   `json:"monitor_bucket"`
	GroupRecommendation        *AccountMonitorGroupRecommendation       `json:"group_recommendation,omitempty"`
	RealRequestEvidence        *AccountMonitorRealRequestEvidence       `json:"real_request_evidence,omitempty"`
	GlobalRanks                AccountMonitorGlobalRanks                `json:"global_ranks,omitempty"`
	GroupProfitability         *AccountMonitorGroupProfitability        `json:"group_profitability,omitempty"`
	UpstreamMultiplier         *AccountMonitorMultiplier                `json:"upstream_multiplier,omitempty"`
	RealRequestTimeline        []AccountMonitorRealRequestTimelinePoint `json:"real_request_timeline,omitempty"`
}

type AccountMonitorProjection struct {
	SchemaVersion int                         `json:"schema_version"`
	Range         AccountMonitorRange         `json:"range,omitempty"`
	ObservedAt    time.Time                   `json:"observed_at"`
	Stale         bool                        `json:"stale"`
	Settings      AccountMonitorSettings      `json:"settings"`
	Health        AccountMonitorHealthSummary `json:"health"`
	Groups        []AccountMonitorGroup       `json:"groups"`
	Accounts      []AccountMonitorAccount     `json:"accounts"`
}

type AccountMonitorPage struct {
	AccountMonitorProjection
}

type AccountMonitorRepository interface {
	LoadSettings(ctx context.Context) (AccountMonitorSettings, error)
	SaveSettings(ctx context.Context, settings AccountMonitorSettings) error
	InsertResult(ctx context.Context, result AccountMonitorProbeResult, runID string) error
	// EnsureProbeBucketTerminal atomically records one group/bucket probe
	// terminal. Existing account-level probe rows are reduced to one logical
	// result; an empty/failed set becomes a persisted failure terminal.
	EnsureProbeBucketTerminal(ctx context.Context, groupID int64, accountIDs []int64, bucketStart time.Time, runID string) error
	ListAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorAggregate, error)
	ListWindowAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error)
	ListLatest(ctx context.Context, accountIDs []int64) (map[int64]AccountMonitorLatest, error)
	ListTimelines(ctx context.Context, accountIDs []int64, perAccountLimit int) (map[int64][]AccountMonitorTimelinePoint, error)
	ListHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorProbeResult, error)
	DeleteBefore(ctx context.Context, before time.Time) error
	ListGroups(ctx context.Context) ([]AccountMonitorGroup, error)
	LoadGlobalScoreWeights(ctx context.Context) (AccountMonitorScoreWeights, error)
	SaveGlobalScoreWeights(ctx context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorScoreWeights, error)
	ResetGlobalScoreWeights(ctx context.Context) error
	LoadGroupScoreWeights(ctx context.Context, groupID int64) (AccountMonitorScoreWeights, error)
	SaveGroupScoreWeights(ctx context.Context, groupID, actorID int64, weights AccountMonitorScoreWeights) error
	ResetGroupScoreWeights(ctx context.Context, groupID int64) error
}

// AccountMonitorGroupAggregateRepository is optional so existing repository
// adapters can continue serving the global projection while group evidence is
// rolled out.
type AccountMonitorGroupAggregateRepository interface {
	ListGroupAggregates(ctx context.Context, groupID int64, accountIDs []int64, since time.Time) (map[int64]AccountMonitorAggregate, error)
}

// AccountMonitorGroupWindowAggregateRepository is optional so legacy monitor
// repositories can continue serving the global window projection.
type AccountMonitorGroupWindowAggregateRepository interface {
	ListGroupWindowAggregates(ctx context.Context, groupID int64, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error)
}

// AccountMonitorCombinedAggregateRepository provides percentiles calculated
// across the underlying observations. Per-account percentiles cannot be
// averaged to produce a valid global or group percentile.
type AccountMonitorCombinedAggregateRepository interface {
	LoadAggregate(ctx context.Context, accountIDs []int64, since time.Time) (AccountMonitorAggregate, error)
	LoadGroupAggregate(ctx context.Context, groupID int64, accountIDs []int64, since time.Time) (AccountMonitorAggregate, error)
}
