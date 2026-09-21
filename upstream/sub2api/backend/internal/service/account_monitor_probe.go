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
	"github.com/google/uuid"
)

// accountMonitorProbePrompt returns a short, non-sensitive prompt with a
// per-request nonce so scheduled probes do not repeatedly send identical
// content upstream.
func accountMonitorProbePrompt() string {
	return "ping " + uuid.NewString()
}

type accountMonitorProbeObserverKey struct{}
type accountMonitorFirstTokenTimeoutKey struct{}

var accountMonitorHTTPStatusPattern = regexp.MustCompile(`(?i)(?:returned|返回|status(?:\s+code)?|http|failed)\s*(?:\(|:)?\s*([1-5][0-9]{2})(?:[^0-9]|$)`)

type accountMonitorProbeObserver struct {
	firstContentAt time.Time
	completedAt    time.Time
	firstContent   func()
}

func (o *accountMonitorProbeObserver) observe(event TestEvent, now time.Time) {
	if o == nil {
		return
	}
	if event.Type == "test_complete" && event.Success {
		o.completedAt = now
	}
	if o.firstContentAt.IsZero() && event.Type == "content" && strings.TrimSpace(event.Text) != "" {
		o.firstContentAt = now
		if o.firstContent != nil {
			o.firstContent()
		}
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
	ctx, observer, cleanup := newAccountMonitorProbeContext(ctx)
	defer cleanup()

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/account-monitor-probe", nil).WithContext(ctx)

	testErr := s.TestAccountConnectionWithProbeKind(ginCtx, accountID, modelID, prompt, mode, ProbeKindMonitor)
	finishedAt := time.Now()
	if errors.Is(context.Cause(ctx), errAccountMonitorFirstTokenTimeout) {
		testErr = context.DeadlineExceeded
	} else if ctx.Err() != nil && ctx.Value(accountMonitorFirstTokenTimeoutKey{}) != nil {
		// A caller/batch deadline is not evidence that the 15s first-token timer fired.
		testErr = context.Canceled
	}
	result := buildAccountMonitorProbeResult(accountID, modelID, startedAt, finishedAt, observer, testErr)
	if usageObserver, ok := ginCtx.Request.Context().Value(accountProbeUsageObserverKey{}).(*accountProbeUsageObserver); ok {
		observation := usageObserver.observation(modelID, ProbeOutcomeSuccess, "")
		result.InputTokens = observation.Tokens.InputTokens
		result.CacheCreationTokens = observation.Tokens.CacheCreationTokens
		result.CacheReadTokens = observation.Tokens.CacheReadTokens
		result.UsageCompleteness = observation.Completeness
	}
	return result, nil
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
		// A successful terminal event is sufficient availability evidence. Some
		// Responses-compatible upstreams legally complete without a text delta;
		// keep TTFT absent instead of turning that completion into a false outage.
		if observer == nil || observer.completedAt.IsZero() {
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
		strings.Contains(message, "api returned"),
		strings.Contains(message, "api 返回"):
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
	case strings.Contains(message, "host is not allowed"),
		strings.Contains(message, "invalid base url"):
		return "account_test_error"
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

var errAccountMonitorFirstTokenTimeout = errors.New("first token timeout")

func newAccountMonitorProbeContext(ctx context.Context) (context.Context, *accountMonitorProbeObserver, func()) {
	observer := &accountMonitorProbeObserver{}
	cleanup := func() {}
	if limit, ok := ctx.Value(accountMonitorFirstTokenTimeoutKey{}).(time.Duration); ok && limit > 0 {
		var cancel context.CancelCauseFunc
		ctx, cancel = context.WithCancelCause(ctx)
		timer := time.AfterFunc(limit, func() { cancel(errAccountMonitorFirstTokenTimeout) })
		observer.firstContent = func() { timer.Stop() }
		cleanup = func() { timer.Stop(); cancel(nil) }
	}
	return context.WithValue(ctx, accountMonitorProbeObserverKey{}, observer), observer, cleanup
}
