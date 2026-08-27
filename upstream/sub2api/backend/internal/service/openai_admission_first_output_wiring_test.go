package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamingForwardersCarryAdmissionReleaseToSemanticOutput(t *testing.T) {
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
