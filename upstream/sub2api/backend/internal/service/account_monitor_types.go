package service

import (
	"context"
	"time"
)

const (
	AccountMonitorSchemaVersion           = 3
	AccountMonitorDefaultIntervalSeconds  = 300
	AccountMonitorMinIntervalSeconds      = 15
	AccountMonitorMaxIntervalSeconds      = 3600
	AccountMonitorHistoryDays             = 7
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
	Source             string    `json:"source"`
	SampleCount        int       `json:"sample_count"`
	SuccessSampleCount int       `json:"success_sample_count"`
	TTFTSampleCount    int       `json:"ttft_sample_count"`
	LatencySampleCount int       `json:"latency_sample_count"`
	SuccessRate        float64   `json:"success_rate"`
	TTFTP50MS          *float64  `json:"ttft_p50_ms,omitempty"`
	LatencyP95MS       *float64  `json:"latency_p95_ms,omitempty"`
	ObservedAt         time.Time `json:"observed_at"`
}

type AccountMonitorGroupAccount struct {
	AccountMonitorAccount
	QualityScore *float64                      `json:"quality_score,omitempty"`
	GroupRank    *int                          `json:"group_rank,omitempty"`
	Eligible     bool                          `json:"eligible"`
	Evidence     AccountMonitorQualityEvidence `json:"evidence"`
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
	RequestCount       int64
	ErrorCount         int64
	BaseCost           float64
	SuccessRate        float64
	TTFTSampleCount    int
	LatencySampleCount int
	TTFTP50MS          *float64
	LatencyP95MS       *float64
	LastObservedAt     *time.Time
}

type AccountMonitorUsageWindow struct {
	Name        string     `json:"name"`
	Utilization float64    `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Requests    int64      `json:"requests"`
	Tokens      int64      `json:"tokens"`
}

type AccountMonitorLatest struct {
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type AccountMonitorTimelinePoint struct {
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type AccountMonitorMultiplier struct {
	Value       *float64   `json:"value,omitempty"`
	Source      string     `json:"source,omitempty"`
	Status      string     `json:"status"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	SampleCount int        `json:"sample_count"`
}

type AccountMonitorAccount struct {
	AccountID           int64                         `json:"account_id"`
	Name                string                        `json:"name"`
	Platform            string                        `json:"platform"`
	AccountType         string                        `json:"account_type"`
	Status              string                        `json:"status"`
	Schedulable         bool                          `json:"schedulable"`
	Priority            int                           `json:"priority"`
	HomepageURL         string                        `json:"homepage_url,omitempty"`
	GroupIDs            []int64                       `json:"group_ids"`
	GroupNames          []string                      `json:"group_names"`
	ModelID             string                        `json:"model_id"`
	LatestStatus        string                        `json:"latest_status"`
	ErrorCode           string                        `json:"error_code,omitempty"`
	SampleCount         int                           `json:"sample_count"`
	SuccessSampleCount  int                           `json:"success_sample_count"`
	TTFTSampleCount     int                           `json:"ttft_sample_count"`
	LatencySampleCount  int                           `json:"latency_sample_count"`
	SuccessRate         float64                       `json:"success_rate"`
	TTFTP50MS           *float64                      `json:"ttft_p50_ms,omitempty"`
	TTFTP95MS           *float64                      `json:"ttft_p95_ms,omitempty"`
	LatencyP95MS        *float64                      `json:"latency_p95_ms,omitempty"`
	Multiplier          AccountMonitorMultiplier      `json:"multiplier"`
	RequestCount        int64                         `json:"request_count"`
	ErrorCount          int64                         `json:"error_count"`
	Range               AccountMonitorRange           `json:"range,omitempty"`
	BaseCost            float64                       `json:"base_cost"`
	EffectiveMultiplier *float64                      `json:"effective_multiplier,omitempty"`
	CostMode            string                        `json:"cost_mode,omitempty"`
	CostScore           float64                       `json:"cost_score"`
	TodayStats          *WindowStats                  `json:"today_stats,omitempty"`
	UsageWindows        []AccountMonitorUsageWindow   `json:"usage_windows,omitempty"`
	Latest              *AccountMonitorLatest         `json:"latest,omitempty"`
	Timeline            []AccountMonitorTimelinePoint `json:"timeline"`
	CheckedAt           *time.Time                    `json:"checked_at,omitempty"`
	Stale               bool                          `json:"stale"`
	ManagementState     string                        `json:"management_state"`
	ServiceState        string                        `json:"service_state"`
	GroupEligibility    string                        `json:"group_eligibility"`
	MonitorBucket       string                        `json:"monitor_bucket"`
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
	ListAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorAggregate, error)
	ListWindowAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error)
	ListLatest(ctx context.Context, accountIDs []int64) (map[int64]AccountMonitorLatest, error)
	ListTimelines(ctx context.Context, accountIDs []int64, perAccountLimit int) (map[int64][]AccountMonitorTimelinePoint, error)
	ListHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorProbeResult, error)
	DeleteBefore(ctx context.Context, before time.Time) error
	ListGroups(ctx context.Context) ([]AccountMonitorGroup, error)
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

// AccountMonitorCombinedAggregateRepository provides percentiles calculated
// across the underlying observations. Per-account percentiles cannot be
// averaged to produce a valid global or group percentile.
type AccountMonitorCombinedAggregateRepository interface {
	LoadAggregate(ctx context.Context, accountIDs []int64, since time.Time) (AccountMonitorAggregate, error)
	LoadGroupAggregate(ctx context.Context, groupID int64, accountIDs []int64, since time.Time) (AccountMonitorAggregate, error)
}
