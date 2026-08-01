package service

import (
	"context"
	"time"
)

const (
	AccountMonitorSchemaVersion          = 2
	AccountMonitorDefaultIntervalSeconds = 300
	AccountMonitorMinIntervalSeconds     = 15
	AccountMonitorMaxIntervalSeconds     = 3600
	AccountMonitorHistoryDays            = 7
	AccountMonitorDefaultHistoryLimit    = 50
)

var DefaultAccountMonitorScoreWeights = AccountMonitorScoreWeights{
	Cost: 15, Success: 45, TTFT: 20, Latency: 20,
}

type AccountMonitorScoreWeights struct {
	Cost      int       `json:"cost"`
	Success   int       `json:"success"`
	TTFT      int       `json:"ttft"`
	Latency   int       `json:"latency"`
	UpdatedBy int64     `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AccountMonitorGroup struct {
	ID              int64                      `json:"id"`
	Name            string                     `json:"name"`
	RateMultiplier  float64                    `json:"rate_multiplier"`
	CustomerVisible bool                       `json:"customer_visible"`
	NativeOrder     int                        `json:"native_order"`
	ScoreWeights    AccountMonitorScoreWeights `json:"score_weights"`
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
	SampleCount       int
	SuccessCount      int
	ErrorCount        int
	SuccessRate       float64
	TTFTP50MS         *float64
	TTFTP95MS         *float64
	LatencyP50MS      *float64
	LatencyP95MS      *float64
	LastCheckedAt     *time.Time
	ConsecutiveFailed int
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
	Priority     int                         `json:"priority"`
	HomepageURL  string                      `json:"homepage_url,omitempty"`
	GroupIDs     []int64                     `json:"group_ids"`
	GroupNames   []string                    `json:"group_names"`
	ModelID      string                      `json:"model_id"`
	LatestStatus string                      `json:"latest_status"`
	ErrorCode    string                      `json:"error_code,omitempty"`
	SampleCount  int                         `json:"sample_count"`
	SuccessRate  float64                     `json:"success_rate"`
	TTFTP50MS    *float64                    `json:"ttft_p50_ms,omitempty"`
	TTFTP95MS    *float64                    `json:"ttft_p95_ms,omitempty"`
	LatencyP95MS *float64                    `json:"latency_p95_ms,omitempty"`
	Multiplier   AccountMonitorMultiplier    `json:"multiplier"`
	RequestCount int64                       `json:"request_count"`
	ErrorCount   int64                       `json:"error_count"`
	TodayStats   *WindowStats                `json:"today_stats,omitempty"`
	UsageWindows []AccountMonitorUsageWindow `json:"usage_windows,omitempty"`
	Latest       *AccountMonitorLatest       `json:"latest,omitempty"`
	CheckedAt    *time.Time                  `json:"checked_at,omitempty"`
	Stale        bool                        `json:"stale"`
}

type AccountMonitorProjection struct {
	SchemaVersion int                     `json:"schema_version"`
	ObservedAt    time.Time               `json:"observed_at"`
	Stale         bool                    `json:"stale"`
	Settings      AccountMonitorSettings  `json:"settings"`
	Groups        []AccountMonitorGroup   `json:"groups"`
	Accounts      []AccountMonitorAccount `json:"accounts"`
}

type AccountMonitorPage struct {
	AccountMonitorProjection
}

type AccountMonitorRepository interface {
	LoadSettings(ctx context.Context) (AccountMonitorSettings, error)
	SaveSettings(ctx context.Context, settings AccountMonitorSettings) error
	InsertResult(ctx context.Context, result AccountMonitorProbeResult, runID string) error
	ListAggregates(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]AccountMonitorAggregate, error)
	ListLatest(ctx context.Context, accountIDs []int64) (map[int64]AccountMonitorLatest, error)
	ListHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorProbeResult, error)
	DeleteBefore(ctx context.Context, before time.Time) error
	ListGroups(ctx context.Context) ([]AccountMonitorGroup, error)
	LoadGroupScoreWeights(ctx context.Context, groupID int64) (AccountMonitorScoreWeights, error)
	SaveGroupScoreWeights(ctx context.Context, groupID, actorID int64, weights AccountMonitorScoreWeights) error
	ResetGroupScoreWeights(ctx context.Context, groupID int64) error
}
