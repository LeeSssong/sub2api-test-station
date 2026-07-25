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
