package sub2api

import (
	"context"
	"encoding/json"
	"time"
)

type Reader interface {
	ListChannels(context.Context) ([]Channel, error)
	ListGroups(context.Context) ([]Group, error)
	ListChannelMonitors(context.Context) ([]ChannelMonitor, error)
	GetChannelMonitorHistory(context.Context, int64, string, int) ([]MonitorHistory, error)
	GetOpsSnapshot(context.Context, OpsQuery) (OpsSnapshot, error)
	GetUsageStats(context.Context, UsageQuery) (UsageStats, error)
}

type AccountReader interface {
	ListAccounts(context.Context) ([]Account, error)
}

type AccountMonitorReader interface {
	ListAccountMonitors(context.Context) (AccountMonitorProjection, error)
	ListAccountMonitorHistory(context.Context, int64, int) ([]AccountMonitorHistoryEntry, error)
}

type ModelDiscoveryReader interface {
	SyncUpstreamModels(context.Context, int64) ([]Model, error)
}

type OpsAlertReader interface {
	ListOpsAlertEvents(context.Context, OpsAlertEventCursor) ([]OpsAlertEvent, error)
	GetOpsAlertEvent(context.Context, int64) (OpsAlertEvent, error)
}

type Controller interface {
	GetGroup(context.Context, int64) (Group, error)
	GetAccount(context.Context, int64) (Account, error)
	GetAccountModels(context.Context, int64) ([]Model, error)
	SetAccountGroups(context.Context, int64, []int64) (Account, error)
	SetAccountSchedulable(context.Context, int64, bool) (Account, error)
}

type Account struct {
	ID                      int64           `json:"id"`
	Name                    string          `json:"name"`
	Platform                string          `json:"platform"`
	Status                  string          `json:"status"`
	Schedulable             bool            `json:"schedulable"`
	GroupIDs                []int64         `json:"group_ids"`
	CredentialsStatus       map[string]bool `json:"credentials_status"`
	ExpiresAt               *int64          `json:"expires_at"`
	AutoPauseOnExpired      bool            `json:"auto_pause_on_expired"`
	RateLimitResetAt        *time.Time      `json:"rate_limit_reset_at"`
	OverloadUntil           *time.Time      `json:"overload_until"`
	TempUnschedulableUntil  *time.Time      `json:"temp_unschedulable_until"`
	TempUnschedulableReason string          `json:"temp_unschedulable_reason"`
}

type AccountMonitorSettings struct {
	IntervalSeconds int        `json:"interval_seconds"`
	UpdatedBy       int64      `json:"updated_by,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
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

type AccountMonitorTodayStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

type AccountMonitorMultiplier struct {
	Value      *float64   `json:"value,omitempty"`
	Source     string     `json:"source,omitempty"`
	Status     string     `json:"status"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type AccountMonitorAccount struct {
	AccountID    int64                       `json:"account_id"`
	Name         string                      `json:"name"`
	Platform     string                      `json:"platform"`
	AccountType  string                      `json:"account_type"`
	Status       string                      `json:"status"`
	Schedulable  bool                        `json:"schedulable"`
	GroupIDs     []int64                     `json:"group_ids"`
	GroupNames   []string                    `json:"group_names"`
	ModelID      string                      `json:"model_id"`
	LatestStatus string                      `json:"latest_status"`
	ErrorCode    string                      `json:"error_code"`
	SampleCount  int                         `json:"sample_count"`
	SuccessRate  float64                     `json:"success_rate"`
	TTFTP50MS    *float64                    `json:"ttft_p50_ms"`
	TTFTP95MS    *float64                    `json:"ttft_p95_ms"`
	LatencyP95MS *float64                    `json:"latency_p95_ms"`
	Multiplier   AccountMonitorMultiplier    `json:"multiplier"`
	RequestCount int64                       `json:"request_count"`
	ErrorCount   int64                       `json:"error_count"`
	TodayStats   *AccountMonitorTodayStats   `json:"today_stats,omitempty"`
	UsageWindows []AccountMonitorUsageWindow `json:"usage_windows"`
	Latest       *AccountMonitorLatest       `json:"latest,omitempty"`
	CheckedAt    *time.Time                  `json:"checked_at,omitempty"`
	Stale        bool                        `json:"stale"`
}
type AccountMonitorProjection struct {
	SchemaVersion int                     `json:"schema_version"`
	ObservedAt    time.Time               `json:"observed_at"`
	Stale         bool                    `json:"stale"`
	Settings      AccountMonitorSettings  `json:"settings"`
	Accounts      []AccountMonitorAccount `json:"accounts"`
}

type AccountMonitorHistoryEntry struct {
	AccountID  int64     `json:"account_id"`
	ModelID    string    `json:"model_id"`
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type accountMonitorHistoryPage struct {
	Items []AccountMonitorHistoryEntry `json:"items"`
}

type Model struct {
	ID string `json:"id"`
}

type Channel struct {
	ID           int64                        `json:"id"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Status       string                       `json:"status"`
	GroupIDs     []int64                      `json:"group_ids"`
	ModelPricing []ChannelModelPrice          `json:"model_pricing"`
	ModelMapping map[string]map[string]string `json:"model_mapping"`
}

type ChannelModelPrice struct {
	Platform        string                      `json:"platform"`
	Models          []string                    `json:"models"`
	BillingMode     string                      `json:"billing_mode"`
	InputPrice      *float64                    `json:"input_price"`
	OutputPrice     *float64                    `json:"output_price"`
	CacheWritePrice *float64                    `json:"cache_write_price"`
	CacheReadPrice  *float64                    `json:"cache_read_price"`
	PerRequestPrice *float64                    `json:"per_request_price"`
	Intervals       []ChannelModelPriceInterval `json:"intervals"`
}

type ChannelModelPriceInterval struct {
	MinTokens       int64    `json:"min_tokens"`
	MaxTokens       *int64   `json:"max_tokens"`
	TierLabel       string   `json:"tier_label"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}

type Group struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`
	IsExclusive    bool    `json:"is_exclusive"`
	Status         string  `json:"status"`
}

func (g Group) CustomerVisible() bool {
	return g.Status == "active" && !g.IsExclusive
}

type ChannelMonitor struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	GroupName        string  `json:"group_name"`
	Enabled          bool    `json:"enabled"`
	PrimaryModel     string  `json:"primary_model"`
	PrimaryStatus    string  `json:"primary_status"`
	PrimaryLatencyMS int64   `json:"primary_latency_ms"`
	Availability7D   float64 `json:"availability_7d"`
}

type MonitorHistory struct {
	ID            int64  `json:"id"`
	Model         string `json:"model"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	PingLatencyMS int64  `json:"ping_latency_ms"`
	CheckedAt     string `json:"checked_at"`
}

type OpsQuery struct {
	TimeRange string
	GroupID   int64
	AccountID int64
	Platform  string
	Mode      string
}

type OpsAlertEventCursor struct {
	Limit         int
	BeforeFiredAt *time.Time
	BeforeID      *int64
}

type OpsAlertEvent struct {
	ID             int64          `json:"id"`
	RuleID         int64          `json:"rule_id"`
	Severity       string         `json:"severity"`
	Status         string         `json:"status"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	MetricValue    *float64       `json:"metric_value"`
	ThresholdValue *float64       `json:"threshold_value"`
	Dimensions     map[string]any `json:"dimensions"`
	FiredAt        time.Time      `json:"fired_at"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
}

type Percentiles struct {
	P50MS float64 `json:"p50_ms"`
	P90MS float64 `json:"p90_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
}

type OpsOverview struct {
	StartTime         string      `json:"start_time"`
	EndTime           string      `json:"end_time"`
	SuccessCount      int64       `json:"success_count"`
	ErrorCountTotal   int64       `json:"error_count_total"`
	RequestCountTotal int64       `json:"request_count_total"`
	RequestCountSLA   int64       `json:"request_count_sla"`
	SLA               float64     `json:"sla"`
	ErrorRate         float64     `json:"error_rate"`
	UpstreamErrorRate float64     `json:"upstream_error_rate"`
	Duration          Percentiles `json:"duration"`
	TTFT              Percentiles `json:"ttft"`
}

type OpsSnapshot struct {
	GeneratedAt string      `json:"generated_at"`
	Overview    OpsOverview `json:"overview"`
}

type UsageQuery struct {
	GroupID   int64
	AccountID int64
	Model     string
	Period    string
	StartDate string
	EndDate   string
	Timezone  string
}

type UsageStats struct {
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheTokens         int64   `json:"total_cache_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`
	TotalActualCost          float64 `json:"total_actual_cost"`
	TotalAccountCost         float64 `json:"total_account_cost"`
	AverageDurationMS        float64 `json:"average_duration_ms"`
	CacheMetricsPresent      bool    `json:"-"`
}

func (s *UsageStats) UnmarshalJSON(data []byte) error {
	type alias UsageStats
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = UsageStats(decoded)
	for _, name := range []string{"total_cache_tokens", "total_cache_creation_tokens", "total_cache_read_tokens", "total_tokens"} {
		if _, ok := fields[name]; !ok {
			return nil
		}
	}
	s.CacheMetricsPresent = true
	return nil
}
