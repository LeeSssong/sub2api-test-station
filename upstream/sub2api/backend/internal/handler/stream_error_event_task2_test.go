package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteOpenAIStreamRecoverySSE_EmitsOneStructuredRetryableEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, _ = c.Writer.WriteString("data: {\"id\":\"chunk_1\"}\n\n")

	require.True(t, writeOpenAIStreamRecoverySSE(c, &service.UpstreamFailoverError{
		OutputStarted: true,
		ResponseID:    "resp_123",
		Recovery: &service.OpenAIStreamRecoveryPayload{
			Type: "upstream_stream_error", Message: "Upstream response stream was interrupted",
			Retryable: true, ResumeSupported: true, RetryAfterSeconds: 10,
		},
	}))

	frames := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n\n")
	require.Len(t, frames, 2)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(strings.Split(frames[1], "\n")[1], "data: ")), &payload))
	errorObject, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "upstream_temporarily_unavailable", errorObject["type"])
	require.Equal(t, "当前上游暂时不可用，请稍后继续", errorObject["message"])
	require.Equal(t, true, errorObject["retryable"])
	require.Equal(t, false, errorObject["resume_supported"])
	require.NotContains(t, errorObject, "response_id")
	require.Equal(t, float64(10), errorObject["retry_after_seconds"])
}

func TestWriteOpenAIStreamRecoverySSE_NormalizesResponseIDAndResumeFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, _ = c.Writer.WriteString("data: partial\n\n")

	require.True(t, writeOpenAIStreamRecoverySSE(c, &service.UpstreamFailoverError{
		OutputStarted: true,
		ResponseID:    " resp_outer ",
		Recovery: &service.OpenAIStreamRecoveryPayload{
			Type: "upstream_stream_error", Message: "interrupted", Retryable: true,
			ResumeSupported: true, ResponseID: "stale_payload",
		},
	}))
	var payload map[string]any
	frame := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n\n")[1]
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(strings.Split(frame, "\n")[1], "data: ")), &payload))
	errorObject, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, errorObject, "response_id")
	require.Equal(t, false, errorObject["resume_supported"])
}

func TestWriteOpenAIStreamRecoverySSE_ExactContractWithoutResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	require.True(t, writeOpenAIStreamRecoverySSE(c, &service.UpstreamFailoverError{OutputStarted: true}))
	require.Equal(t, "event: error\ndata: {\"error\":{\"type\":\"upstream_temporarily_unavailable\",\"message\":\"当前上游暂时不可用，请稍后继续\",\"retryable\":true,\"resume_supported\":false,\"retry_after_seconds\":10}}\n\n", w.Body.String())
}

func TestWriteAnthropicStreamRecoverySSE_UsesMessagesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	_, _ = c.Writer.WriteString("event: content_block_delta\ndata: {}\n\n")

	require.True(t, writeAnthropicStreamRecoverySSE(c, &service.UpstreamFailoverError{OutputStarted: true, ResponseID: "resp_msg"}))
	frame := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n\n")[1]
	require.True(t, strings.HasPrefix(frame, "event: error\n"))
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(strings.Split(frame, "\n")[1], "data: ")), &payload))
	require.Equal(t, "error", payload["type"])
	require.NotContains(t, payload, "response_id")
	errorObject, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "api_error", errorObject["type"])
	require.Equal(t, "当前上游暂时不可用，请稍后继续", errorObject["message"])
	require.NotContains(t, errorObject, "response_id")
}

func TestHandleAnthropicFailoverExhausted_UsesMessagesRecoveryEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	h := &OpenAIGatewayHandler{}

	h.handleAnthropicFailoverExhausted(c, &service.UpstreamFailoverError{OutputStarted: true}, true)

	require.Equal(t, "event: error\ndata: {\"error\":{\"message\":\"当前上游暂时不可用，请稍后继续\",\"resume_supported\":false,\"retry_after_seconds\":10,\"retryable\":true,\"type\":\"api_error\"},\"type\":\"error\"}\n\n", w.Body.String())
}
