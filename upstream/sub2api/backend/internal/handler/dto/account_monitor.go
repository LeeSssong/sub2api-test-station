package dto

type AccountMonitorHistoryItem struct {
	AccountID  int64    `json:"account_id"`
	ModelID    string   `json:"model_id"`
	Status     string   `json:"status"`
	ErrorCode  string   `json:"error_code,omitempty"`
	HTTPStatus *int     `json:"http_status,omitempty"`
	TTFTMS     *float64 `json:"ttft_ms,omitempty"`
	LatencyMS  *float64 `json:"latency_ms,omitempty"`
	CheckedAt  string   `json:"checked_at"`
}

// AccountModelRuntimeItem exposes transient OpenAI account-model scheduling state.
type AccountModelRuntimeItem struct {
	AccountID                int64  `json:"account_id"`
	CanonicalSchedulingModel string `json:"canonical_scheduling_model"`
	State                    string `json:"state"`
	FailureStreak            int    `json:"failure_streak"`
	LastFailureAt            string `json:"last_failure_at,omitempty"`
	CooldownUntil            string `json:"cooldown_until,omitempty"`
	HalfOpenInFlight         bool   `json:"half_open_in_flight"`
	LastStatusCode           int    `json:"last_status_code"`
	LastErrorType            string `json:"last_error_type,omitempty"`
	OutputStarted            bool   `json:"output_started"`
	StickyReferenceCount     int    `json:"sticky_reference_count"`
}
