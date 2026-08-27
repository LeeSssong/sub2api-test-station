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

func TestOpenAIStreamingHandlersWireAccountOnlyAdmissionBeforeFirstSemanticOutput(t *testing.T) {
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
			admissionAt := strings.Index(body, "admissionRelease, admission := h.gatewayService.AcquireOpenAIAdmission(account.ID, shape)")
			rejectAt := strings.Index(body, "if !admission.Allowed {")
			combineAt := strings.Index(body, "priorRelease := accountReleaseFunc")
			callbackAt := strings.Index(body, "service.WithOpenAIFirstSemanticOutputCallback(attemptCtx, accountReleaseFunc)")

			require.GreaterOrEqual(t, admissionAt, 0)
			require.Greater(t, rejectAt, admissionAt)
			require.Contains(t, body[rejectAt:combineAt], "accountReleaseFunc()", "reject must release the just-acquired account slot")
			require.Contains(t, body[rejectAt:combineAt], "failedAccountIDs[account.ID] = struct{}{}", "reject must reselect another account")
			require.Contains(t, body[rejectAt:combineAt], "continue", "reject must stay in the same handler failover loop")
			require.Greater(t, callbackAt, combineAt, "first-output callback must receive the composed admission and account-slot release")
		})
	}
}
