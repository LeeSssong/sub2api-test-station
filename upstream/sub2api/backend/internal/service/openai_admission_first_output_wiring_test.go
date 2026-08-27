package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This supplements the behavior tests below by ensuring the known forwarding
// entry points retain a callback reference when future paths are added.
func TestOpenAIStreamingForwarderSourceWiringSupplement(t *testing.T) {
	tests := []struct {
		path        string
		expectation string
	}{
		{"openai_gateway_response_handling.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"openai_gateway_chat_completions.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"openai_gateway_messages.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"gateway_forward_as_chat_completions.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"openai_gateway_chat_completions_raw.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"openai_gateway_cc_pipeline.go", "notifyOpenAIFirstSemanticOutput(ctx)"},
		{"openai_gateway_messages_chat_fallback.go", "s.scanCCStream(ctx,"},
		{"openai_gateway_responses_chat_fallback.go", "s.scanCCStream(ctx,"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			source, err := os.ReadFile(tt.path)
			require.NoError(t, err)
			require.Contains(t, string(source), tt.expectation)
		})
	}
}

func TestHandleStreamingResponseNotifiesAfterFlushedSemanticSSEFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	semanticFrame := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			semanticFrame +
				`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n",
		)),
	}

	callbackFrames := make([]string, 0, 1)
	ctx := WithOpenAIFirstSemanticOutputCallback(context.Background(), func() {
		callbackFrames = append(callbackFrames, recorder.Body.String())
	})
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	result, err := svc.handleStreamingResponse(ctx, resp, c, &Account{ID: 153, Platform: PlatformOpenAI}, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{semanticFrame}, callbackFrames)
	require.NotNil(t, result.firstTokenMs)
}

func TestHandleAnthropicStreamingResponseStructuralOnlyCompletionDoesNotArmSlowSessionGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","response":{"id":"resp_structural","status":"in_progress","output":[]}}`,
			``,
			`event: response.in_progress`,
			`data: {"type":"response.in_progress","response":{"id":"resp_structural","status":"in_progress","output":[]}}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_structural","status":"completed","output":[]}}`,
			``,
		}, "\n"))),
	}
	store := newOpenAISharedHealthStoreStub()
	cfg := &config.Config{}
	cfg.Gateway.OpenAISharedHealth = DefaultOpenAISharedHealthConfig()
	cfg.Gateway.OpenAISharedHealth.SlowTTFTMS = 1
	svc := &OpenAIGatewayService{cfg: cfg}
	svc.SetOpenAISharedHealthStore(store)

	result, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, &Account{ID: 153, Platform: PlatformOpenAI}, "gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-sol", time.Now().Add(-time.Minute))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.FirstTokenMs)
	svc.RecordOpenAISlowSessionGuard(153, result, false)
	store.mu.Lock()
	_, guarded := store.slowGuard[153]
	store.mu.Unlock()
	require.False(t, guarded)
}

func TestScanCCStreamNotifiesAfterEmittingFirstSemanticChunk(t *testing.T) {
	order := make([]string, 0, 2)
	ctx := WithOpenAIFirstSemanticOutputCallback(context.Background(), func() {
		order = append(order, "callback")
	})
	resp := &http.Response{
		Body: http.NoBody,
	}
	resp.Body = io.NopCloser(bytes.NewBufferString("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))

	state := (&OpenAIGatewayService{}).scanCCStream(ctx, resp, "test", "request", time.Now(), func(_ *apicompat.ChatCompletionsChunk) {
		order = append(order, "emit")
	})

	require.NoError(t, state.Err)
	require.Equal(t, []string{"emit", "callback"}, order)
}
