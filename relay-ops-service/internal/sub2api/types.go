package sub2api

import (
	"context"
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
	Platform  string
	Mode      string
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
	Model     string
	Period    string
	StartDate string
	EndDate   string
	Timezone  string
}

type UsageStats struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	TotalAccountCost  float64 `json:"total_account_cost"`
	AverageDurationMS float64 `json:"average_duration_ms"`
}
