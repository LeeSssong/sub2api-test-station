package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type accountMonitorProbeObserverKey struct{}

var accountMonitorHTTPStatusPattern = regexp.MustCompile(`(?i)(?:returned|status(?:\s+code)?|http|failed)\s*(?:\(|:)?\s*([1-5][0-9]{2})(?:[^0-9]|$)`)

type accountMonitorProbeObserver struct {
	firstContentAt time.Time
}

func (o *accountMonitorProbeObserver) observe(event TestEvent, now time.Time) {
	if o == nil || !o.firstContentAt.IsZero() {
		return
	}
	if event.Type == "content" && strings.TrimSpace(event.Text) != "" {
		o.firstContentAt = now
	}
}

// ProbeAccountConnection reuses the native admin account test path while
// returning only bounded metrics and stable error classifications.
func (s *AccountTestService) ProbeAccountConnection(
	ctx context.Context,
	accountID int64,
	modelID string,
	prompt string,
	mode string,
) (AccountMonitorProbeResult, error) {
	startedAt := time.Now()
	observer := &accountMonitorProbeObserver{}
	ctx = context.WithValue(ctx, accountMonitorProbeObserverKey{}, observer)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/account-monitor-probe", nil).WithContext(ctx)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, prompt, mode)
	finishedAt := time.Now()
	return buildAccountMonitorProbeResult(accountID, modelID, startedAt, finishedAt, observer, testErr), nil
}

func buildAccountMonitorProbeResult(
	accountID int64,
	modelID string,
	startedAt time.Time,
	finishedAt time.Time,
	observer *accountMonitorProbeObserver,
	testErr error,
) AccountMonitorProbeResult {
	result := AccountMonitorProbeResult{
		AccountID: accountID,
		ModelID:   modelID,
		Status:    "success",
		CheckedAt: finishedAt.UTC(),
	}
	latency := float64(finishedAt.Sub(startedAt).Microseconds()) / 1000
	if latency < 0 {
		latency = 0
	}
	result.LatencyMS = &latency
	if observer != nil && !observer.firstContentAt.IsZero() {
		ttft := float64(observer.firstContentAt.Sub(startedAt).Microseconds()) / 1000
		if ttft < 0 {
			ttft = 0
		}
		result.TTFTMS = &ttft
	}

	if testErr == nil {
		if result.TTFTMS == nil {
			result.Status = "failed"
			result.ErrorCode = "malformed_stream"
		}
		return result
	}

	result.Status = "failed"
	result.ErrorCode = classifyAccountMonitorProbeError(testErr)
	result.HTTPStatus = extractAccountMonitorProbeHTTPStatus(testErr)
	return result
}

func classifyAccountMonitorProbeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(message, "timeout"):
		return "timeout"
	case strings.Contains(message, "balance"),
		strings.Contains(message, "quota"),
		strings.Contains(message, "insufficient"):
		return "balance_exhausted"
	case strings.Contains(message, "returned "),
		strings.Contains(message, "status"),
		strings.Contains(message, "api returned"):
		return "http_error"
	case strings.Contains(message, "api key"),
		strings.Contains(message, "access token"),
		strings.Contains(message, "authentication"),
		strings.Contains(message, "unauthorized"),
		strings.Contains(message, "forbidden"),
		strings.Contains(message, "credential"):
		return "invalid_auth"
	case strings.Contains(message, "model"),
		strings.Contains(message, "unsupported"):
		return "model_unavailable"
	case strings.Contains(message, "stream"),
		strings.Contains(message, "sse"),
		strings.Contains(message, "invalid"):
		return "malformed_stream"
	default:
		return "account_test_error"
	}
}

func extractAccountMonitorProbeHTTPStatus(err error) *int {
	if err == nil {
		return nil
	}
	match := accountMonitorHTTPStatusPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return nil
	}
	status, parseErr := strconv.Atoi(match[1])
	if parseErr != nil {
		return nil
	}
	return &status
}
