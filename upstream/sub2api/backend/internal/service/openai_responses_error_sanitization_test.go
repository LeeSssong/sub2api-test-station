package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenAIResponseFailedEventRemovesUpstreamIdentifiers(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"Service temporarily unavailable request id req_secret at https://internal.invalid/v1"}}}`)
	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", true)

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"upstream_unavailable"`)
	require.Contains(t, string(got), `"message":"Upstream response failed"`)
	require.NotContains(t, strings.ToLower(string(got)), "req_secret")
	require.NotContains(t, strings.ToLower(string(got)), "internal.invalid")
}

func TestSanitizeOpenAIBareErrorRemovesUpstreamIdentifiers(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"code":"server_error","message":"openai_error Ray ID abc-secret"}}`)
	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "error", false)

	require.True(t, changed)
	require.Contains(t, string(got), `"code":"upstream_unavailable"`)
	require.Contains(t, string(got), `"message":"Upstream response failed"`)
	require.NotContains(t, strings.ToLower(string(got)), "abc-secret")
}
