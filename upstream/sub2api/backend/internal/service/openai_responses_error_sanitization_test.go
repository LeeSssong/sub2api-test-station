package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenAIResponseFailedEventRemovesUpstreamIdentifiers(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"Service temporarily unavailable request id req_secret at https://internal.invalid/v1"}}}`)
	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", true, &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}})

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"upstream_unavailable"`)
	require.Contains(t, string(got), `"message":"Upstream response failed"`)
	require.NotContains(t, strings.ToLower(string(got)), "req_secret")
	require.NotContains(t, strings.ToLower(string(got)), "internal.invalid")
}

func TestSanitizeOpenAIBareErrorRemovesUpstreamIdentifiers(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"code":"server_error","message":"openai_error Ray ID abc-secret"}}`)
	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}})

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"upstream_unavailable"`)
	require.Contains(t, string(got), `"message":"Upstream response failed"`)
	require.NotContains(t, strings.ToLower(string(got)), "abc-secret")
}

func TestSanitizeOpenAINativePassthroughPreservesCapacityError(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"type":"server_error","code":"server_is_overloaded","message":"The model is currently overloaded. Please try again later."}}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", false, account)

	require.False(t, changed)
	require.JSONEq(t, string(payload), string(got))
	require.NotContains(t, string(got), `"output"`)
}

func TestSanitizeOpenAINativePassthroughPreservesModelSelectionError(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"invalid_request_error","code":"model_not_found","message":"The model 'gpt-5.6-sol' does not exist or you do not have access to it."}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, account)

	require.False(t, changed)
	require.JSONEq(t, string(payload), string(got))
}

func TestSanitizeOpenAINativePassthroughPreservesRateLimitError(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"Rate limit reached for the model."}}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", false, account)

	require.False(t, changed)
	require.JSONEq(t, string(payload), string(got))
}

func TestSanitizeOpenAINativePassthroughStillSanitizesUnknownProviderError(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"server_error","code":"vendor_internal_error","message":"The upstream vendor rejected this request."}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, account)

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"upstream_unavailable"`)
	require.Contains(t, string(got), `"message":"Upstream response failed"`)
}

func TestSanitizeOpenAINonPassthroughStillRewritesCapacityCode(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"server_error","code":"server_is_overloaded","message":"The model is currently overloaded."}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_passthrough": false}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, account)

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"server_error"`)
	require.NotContains(t, string(got), `"code":"server_is_overloaded"`)
}

func TestSanitizeOpenAINativePassthroughRequiresOpenAIAccount(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"server_error","code":"server_is_overloaded","message":"The model is currently overloaded."}}`)
	account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{"openai_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, account)

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"server_error"`)
	require.NotContains(t, string(got), `"code":"server_is_overloaded"`)
}

func TestSanitizeOpenAINativePassthroughAcceptsLegacySwitch(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"Rate limit reached."}}`)
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_oauth_passthrough": true}}

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false, account)

	require.False(t, changed)
	require.JSONEq(t, string(payload), string(got))
}
