package d04readiness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

type FileSource struct {
	Path string
}

type Result struct {
	PolicyID                string           `json:"policy_id"`
	SnapshotID              string           `json:"snapshot_id"`
	AccountSetSHA256        string           `json:"account_set_sha256"`
	EvaluatedAt             time.Time        `json:"evaluated_at"`
	Decision                string           `json:"decision"`
	BlockingReasons         []string         `json:"blocking_reasons"`
	RequiredActions         []string         `json:"required_actions"`
	Upstreams               []ResultUpstream `json:"upstreams"`
	RealActionExecuted      bool             `json:"real_action_executed"`
	ExternalSystemContacted bool             `json:"external_system_contacted"`
}

type ResultUpstream struct {
	AccountID           int64      `json:"account_id"`
	DisplayName         string     `json:"display_name"`
	GroupIDs            []int64    `json:"group_ids"`
	Status              string     `json:"status"`
	Schedulable         bool       `json:"schedulable"`
	RuntimeAvailable    bool       `json:"runtime_available"`
	BalanceUSD          *float64   `json:"balance_usd"`
	FinancialRecordedAt *time.Time `json:"financial_recorded_at"`
	QualityRecordedAt   *time.Time `json:"quality_recorded_at"`
	SampleCount         *int64     `json:"sample_count"`
	SuccessRate         *float64   `json:"success_rate"`
	ErrorRate           *float64   `json:"error_rate"`
	TTFTP95MS           *float64   `json:"ttft_p95_ms"`
	TotalLatencyP95MS   *float64   `json:"total_latency_p95_ms"`
	Decision            string     `json:"decision"`
	BlockingReasons     []string   `json:"blocking_reasons"`
}

var lowercaseSHA256 = regexp.MustCompile(`\A[0-9a-f]{64}\z`)

func (s FileSource) Read() (Result, error) {
	if strings.TrimSpace(s.Path) == "" {
		return Result{}, fmt.Errorf("D04 readiness evidence is unavailable")
	}
	file, err := os.Open(s.Path)
	if err != nil {
		return Result{}, fmt.Errorf("D04 readiness evidence is unavailable")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 2<<20))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil || ensureJSONEOF(decoder) != nil {
		return Result{}, fmt.Errorf("D04 readiness evidence is invalid")
	}
	if err := validateResult(result); err != nil {
		return Result{}, fmt.Errorf("D04 readiness evidence is invalid")
	}
	return result, nil
}

func validateResult(result Result) error {
	if result.PolicyID != "D04-LIGHTWEIGHT-LAUNCH-v3" || strings.TrimSpace(result.SnapshotID) == "" ||
		!lowercaseSHA256.MatchString(result.AccountSetSHA256) || result.EvaluatedAt.IsZero() ||
		(result.Decision != "go" && result.Decision != "no_go") || result.RealActionExecuted || result.ExternalSystemContacted {
		return fmt.Errorf("result metadata is invalid")
	}
	previousID := int64(0)
	for _, upstream := range result.Upstreams {
		if upstream.AccountID <= previousID || upstream.Status != "active" || !upstream.Schedulable ||
			(upstream.Decision != "go" && upstream.Decision != "no_go") {
			return fmt.Errorf("upstream result is invalid")
		}
		for _, groupID := range upstream.GroupIDs {
			if groupID <= 0 {
				return fmt.Errorf("upstream result is invalid")
			}
		}
		previousID = upstream.AccountID
	}
	return nil
}
