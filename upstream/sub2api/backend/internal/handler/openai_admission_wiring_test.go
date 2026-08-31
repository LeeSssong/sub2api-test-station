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
