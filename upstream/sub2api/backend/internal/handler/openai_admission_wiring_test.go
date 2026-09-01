package handler

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func handlerFunctionSource(t *testing.T, path, signature string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	start := strings.Index(string(source), signature)
	require.GreaterOrEqual(t, start, 0, "missing handler %s", signature)
	rest := string(source[start:])
	if end := strings.Index(rest[1:], "\nfunc "); end >= 0 {
		rest = rest[:end+1]
	}
	return rest
}

func TestOpenAIStreamingHandlersUseOnlyNativeAccountConcurrency(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		signature string
	}{
		{"chat completions", "openai_chat_completions.go", "func (h *OpenAIGatewayHandler) ChatCompletions"},
		{"responses", "openai_gateway_handler.go", "func (h *OpenAIGatewayHandler) Responses"},
		{"messages", "openai_gateway_handler.go", "func (h *OpenAIGatewayHandler) Messages"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := handlerFunctionSource(t, tt.path, tt.signature)

			require.Contains(t, body, "h.acquireResponsesAccountSlot(", "native account concurrency slot must remain wired")
			require.Contains(t, body, "accountReleaseFunc()", "native account concurrency slot must be released")
			require.NotContains(t, body, "AcquireOpenAIAdmission")
			require.NotContains(t, body, "openai.admission_rejected")
			require.NotContains(t, body, "WithOpenAIFirstSemanticOutputCallback")
			require.NotContains(t, body, "RecordOpenAISlowSessionGuard")
			require.NotContains(t, body, "ClassifyOpenAIAdmissionRequestShape")
			require.NotContains(t, body, "WithOpenAIAdmissionRequestShape")
		})
	}
}

func TestOpenAIHTTPHandlersApplyGroupModelAdmissionBeforeRouting(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		signature string
	}{
		{"native responses", "openai_gateway_handler.go", "func (h *OpenAIGatewayHandler) Responses"},
		{"native messages", "openai_gateway_handler.go", "func (h *OpenAIGatewayHandler) Messages"},
		{"gateway responses", "gateway_handler_responses.go", "func (h *GatewayHandler) Responses"},
		{"gateway chat completions", "gateway_handler_chat_completions.go", "func (h *GatewayHandler) ChatCompletions"},
		{"images", "openai_images.go", "func (h *OpenAIGatewayHandler) Images"},
		{"responses input tokens", "openai_gateway_count_tokens.go", "func (h *OpenAIGatewayHandler) ResponsesInputTokens"},
		{"messages count tokens", "openai_gateway_count_tokens.go", "func (h *OpenAIGatewayHandler) CountTokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := handlerFunctionSource(t, tt.path, tt.signature)
			admission := strings.Index(body, "service.GroupAllowsOpenAIModel(")
			require.GreaterOrEqual(t, admission, 0, "group model admission must be wired")
			for _, marker := range []string{
				"SelectAccount",
				"AcquireUserSlot",
				"AcquireAccount",
				"CheckBillingEligibility",
			} {
				if index := strings.Index(body, marker); index >= 0 {
					require.Less(t, admission, index, "admission must precede %s", marker)
				}
			}
		})
	}
}
