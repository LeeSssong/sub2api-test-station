package accountquality

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxResultBytes = 2 << 20
	staleAfter     = 20 * time.Minute
)

var (
	lowercaseSHA256 = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
	modelIDPattern  = regexp.MustCompile(`\A[a-z0-9][a-z0-9._-]{0,127}\z`)
	forbiddenKey    = regexp.MustCompile(`(?i)(?:api[_-]?key|token|cookie|authorization|password|secret|credential|response|base[_-]?url|request[_-]?header)`)
)

type Result struct {
	SchemaVersion    int       `json:"schema_version"`
	SnapshotID       string    `json:"snapshot_id"`
	ObservedAt       time.Time `json:"observed_at"`
	AccountSetSHA256 string    `json:"account_set_sha256"`
	Accounts         []Account `json:"accounts"`
}

type Account struct {
	AccountID      int64     `json:"account_id"`
	ModelID        string    `json:"model_id"`
	RateMultiplier float64   `json:"rate_multiplier"`
	SampleCount    int64     `json:"sample_count"`
	SuccessCount   int64     `json:"success_count"`
	SuccessRate    float64   `json:"success_rate"`
	TTFTP50MS      *float64  `json:"ttft_p50_ms"`
	TTFTP95MS      *float64  `json:"ttft_p95_ms"`
	LastResult     string    `json:"last_result"`
	LastErrorCode  string    `json:"last_error_code"`
	LastObservedAt time.Time `json:"last_observed_at"`
}

type View struct {
	Available        bool
	Stale            bool
	SnapshotID       string
	ObservedAt       string
	AccountSetSHA256 string
	Accounts         []AccountView
}

type AccountView struct {
	AccountID, ModelID, Stability, SuccessRate, TTFTP50, TTFTP95, Multiplier, LastResult string
}

type FileSource struct {
	Path string
}

func (s FileSource) Read(now time.Time) (Result, error) {
	return Load(s.Path, now)
}

func Load(path string, now time.Time) (Result, error) {
	if strings.TrimSpace(path) == "" {
		return Result{}, fmt.Errorf("account quality evidence is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("account quality evidence is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxResultBytes {
		return Result{}, fmt.Errorf("account quality evidence is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxResultBytes+1))
	if err != nil || len(data) > maxResultBytes {
		return Result{}, fmt.Errorf("account quality evidence is invalid")
	}

	generic, err := decodeGeneric(data)
	if err != nil || containsForbiddenKey(generic) {
		return Result{}, fmt.Errorf("account quality evidence is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil || ensureEOF(decoder) != nil || validate(result, now.UTC()) != nil {
		return Result{}, fmt.Errorf("account quality evidence is invalid")
	}
	return result, nil
}

func (r Result) View(now time.Time) View {
	accounts := make([]AccountView, 0, len(r.Accounts))
	for _, account := range r.Accounts {
		accounts = append(accounts, AccountView{
			AccountID:   strconv.FormatInt(account.AccountID, 10),
			ModelID:     emptyAs(account.ModelID, "未选择文本模型"),
			Stability:   fmt.Sprintf("成功 %d/%d", account.SuccessCount, account.SampleCount),
			SuccessRate: fmt.Sprintf("%.1f%%", account.SuccessRate*100),
			TTFTP50:     formatMS(account.TTFTP50MS),
			TTFTP95:     formatMS(account.TTFTP95MS),
			Multiplier:  strconv.FormatFloat(account.RateMultiplier, 'f', -1, 64) + "x",
			LastResult:  resultLabel(account.LastResult),
		})
	}
	return View{
		Available: true, Stale: now.UTC().Sub(r.ObservedAt) > staleAfter,
		SnapshotID: r.SnapshotID, ObservedAt: r.ObservedAt.UTC().Format(time.RFC3339),
		AccountSetSHA256: r.AccountSetSHA256, Accounts: accounts,
	}
}

func validate(result Result, now time.Time) error {
	if result.SchemaVersion != 1 || strings.TrimSpace(result.SnapshotID) == "" || result.ObservedAt.IsZero() ||
		result.ObservedAt.After(now) || !lowercaseSHA256.MatchString(result.AccountSetSHA256) {
		return fmt.Errorf("metadata is invalid")
	}
	previousID := int64(0)
	for _, account := range result.Accounts {
		if account.AccountID <= previousID || !validModelID(account.ModelID, account.LastResult) ||
			!finiteNonNegative(account.RateMultiplier) || account.SampleCount <= 0 || account.SuccessCount < 0 ||
			account.SuccessCount > account.SampleCount || account.SuccessRate < 0 || account.SuccessRate > 1 ||
			!finite(account.SuccessRate) || !allowedResult(account.LastResult) || account.LastObservedAt.IsZero() ||
			account.LastObservedAt.After(result.ObservedAt) || !validErrorCode(account.LastErrorCode, account.LastResult) ||
			!validTTFT(account.TTFTP50MS) || !validTTFT(account.TTFTP95MS) ||
			(account.SuccessCount == 0 && (account.TTFTP50MS != nil || account.TTFTP95MS != nil)) ||
			(account.SuccessCount > 0 && (account.TTFTP50MS == nil || account.TTFTP95MS == nil)) ||
			(account.TTFTP50MS != nil && account.TTFTP95MS != nil && *account.TTFTP50MS > *account.TTFTP95MS) ||
			math.Abs(account.SuccessRate-float64(account.SuccessCount)/float64(account.SampleCount)) > 0.000001 {
			return fmt.Errorf("account is invalid")
		}
		previousID = account.AccountID
	}
	return nil
}

func validModelID(value, result string) bool {
	return modelIDPattern.MatchString(value) || (value == "" && result != "passed")
}

func validErrorCode(value, result string) bool {
	if result == "passed" {
		return value == ""
	}
	return value == result
}

func allowedResult(value string) bool {
	switch value {
	case "passed", "balance_exhausted", "account_test_error", "http_error", "timeout", "malformed_stream", "model_unavailable":
		return true
	default:
		return false
	}
}

func validTTFT(value *float64) bool {
	return value == nil || finiteNonNegative(*value)
}

func finiteNonNegative(value float64) bool {
	return finite(value) && value >= 0
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func formatMS(value *float64) string {
	if value == nil {
		return "无成功样本"
	}
	return fmt.Sprintf("%.0fms", *value)
}

func resultLabel(value string) string {
	labels := map[string]string{
		"passed":             "通过",
		"balance_exhausted":  "余额不足",
		"account_test_error": "账号测试错误",
		"http_error":         "HTTP 错误",
		"timeout":            "超时",
		"malformed_stream":   "流格式错误",
		"model_unavailable":  "无可用文本模型",
	}
	return labels[value]
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func decodeGeneric(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || ensureEOF(decoder) != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	return document, nil
}

func containsForbiddenKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if forbiddenKey.MatchString(key) || containsForbiddenKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsForbiddenKey(child) {
				return true
			}
		}
	}
	return false
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
