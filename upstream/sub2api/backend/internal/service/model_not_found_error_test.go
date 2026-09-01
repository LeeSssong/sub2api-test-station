package service

import (
	"net/http"
	"testing"
)

func TestClassifyOpenAINotFoundSeparatesModelAndRouteErrors(t *testing.T) {
	tests := []struct {
		name              string
		statusCode        int
		body              []byte
		wantKind          OpenAINotFoundKind
		wantRetryable     bool
		wantModelCooldown bool
	}{
		{
			name:              "model error",
			statusCode:        http.StatusNotFound,
			body:              []byte(`{"error":{"code":"model_not_found","message":"model not found"}}`),
			wantKind:          OpenAINotFoundModel,
			wantRetryable:     true,
			wantModelCooldown: true,
		},
		{
			name:          "endpoint error",
			statusCode:    http.StatusNotFound,
			body:          []byte(`{"error":{"message":"endpoint route not found"}}`),
			wantKind:      OpenAINotFoundRoute,
			wantRetryable: true,
		},
		{
			name:          "route wording wins over incidental model text",
			statusCode:    http.StatusNotFound,
			body:          []byte(`{"error":{"message":"model endpoint route not found"}}`),
			wantKind:      OpenAINotFoundRoute,
			wantRetryable: true,
		},
		{
			name:          "bare error",
			statusCode:    http.StatusNotFound,
			body:          []byte(`404 page not found`),
			wantKind:      OpenAINotFoundUnknown,
			wantRetryable: true,
		},
		{
			name:       "non 404 is not classified",
			statusCode: http.StatusBadGateway,
			body:       []byte(`{"error":{"code":"model_not_found"}}`),
			wantKind:   OpenAINotFoundNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOpenAINotFound(tt.statusCode, tt.body)
			if got.Kind != tt.wantKind || got.Retryable != tt.wantRetryable || got.ModelCooldown != tt.wantModelCooldown {
				t.Fatalf("ClassifyOpenAINotFound() = %+v, want kind=%q retryable=%v modelCooldown=%v", got, tt.wantKind, tt.wantRetryable, tt.wantModelCooldown)
			}
		})
	}
}

func TestIsUpstreamModelNotFoundError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{
			name:       "404 model not found message",
			statusCode: http.StatusNotFound,
			body:       []byte(`{"error":{"message":"model not found"}}`),
			want:       true,
		},
		{
			name:       "404 model_not_found code",
			statusCode: http.StatusNotFound,
			body:       []byte(`{"error":{"code":"model_not_found","message":"The requested model was not found"}}`),
			want:       true,
		},
		{
			name:       "404 unknown model message",
			statusCode: http.StatusNotFound,
			body:       []byte(`{"error":{"message":"unknown model gpt-5.4"}}`),
			want:       true,
		},
		{
			name:       "404 endpoint not found is not model specific",
			statusCode: http.StatusNotFound,
			body:       []byte(`{"error":{"message":"endpoint not found"}}`),
			want:       false,
		},
		{
			name:       "404 arbitrary body is not model specific",
			statusCode: http.StatusNotFound,
			body:       []byte(`404 page not found`),
			want:       false,
		},
		{
			name:       "non 404 does not match",
			statusCode: http.StatusBadRequest,
			body:       []byte(`{"error":{"message":"model not found"}}`),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUpstreamModelNotFoundError(tt.statusCode, tt.body); got != tt.want {
				t.Fatalf("isUpstreamModelNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAntigravityModelNotFoundKeepsBare404Fallback(t *testing.T) {
	if !isModelNotFoundError(http.StatusNotFound, []byte(`endpoint not found`)) {
		t.Fatal("antigravity model-not-found helper should keep bare 404 fallback")
	}
}

func TestIsOpenAICodexPlanGatedModelError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{
			name:       "400 codex plan gated detail payload",
			statusCode: http.StatusBadRequest,
			body:       []byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}`),
			want:       true,
		},
		{
			name:       "400 codex plan gated error message payload",
			statusCode: http.StatusBadRequest,
			body:       []byte(`{"error":{"message":"The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account."}}`),
			want:       true,
		},
		{
			name:       "400 unrelated invalid request does not match",
			statusCode: http.StatusBadRequest,
			body:       []byte(`{"error":{"message":"Invalid schema for response_format 'agentic_plan'"}}`),
			want:       false,
		},
		{
			name:       "404 with plan gated message does not match",
			statusCode: http.StatusNotFound,
			body:       []byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}`),
			want:       false,
		},
		{
			name:       "400 empty body does not match",
			statusCode: http.StatusBadRequest,
			body:       nil,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOpenAICodexPlanGatedModelError(tt.statusCode, tt.body); got != tt.want {
				t.Fatalf("isOpenAICodexPlanGatedModelError() = %v, want %v", got, tt.want)
			}
		})
	}
}
