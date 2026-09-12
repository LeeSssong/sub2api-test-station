package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIDFromHeaders_UnconfiguredAccountRecordsNothing(t *testing.T) {
	h := http.Header{}
	h.Set("X-Client-Request-ID", "sub2api-client")
	h.Set("X-Request-ID", "sub2api-local")
	h.Set("X-Oneapi-Request-Id", "oneapi-1")
	h.Set("Request-Id", "req_official")
	h.Set("xai-request-id", "xai-1")
	h.Set("x-goog-request-id", "goog-1")

	require.Equal(t, "", UpstreamRequestIDFromHeaders(nil, h))
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		require.Equal(t, "", UpstreamRequestIDFromHeaders(&Account{Platform: platform}, h), platform)
	}
	blank := &Account{Platform: PlatformOpenAI, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}}
	require.Equal(t, "", UpstreamRequestIDFromHeaders(blank, h))
}

func TestUpstreamRequestIDFromHeaders_ReadsOnlyConfiguredHeader(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Extra:    map[string]any{AccountExtraUpstreamRequestIDHeader: " x-oneapi-request-id "},
	}
	h := http.Header{}
	h.Set("X-Request-ID", "passthrough-from-real-upstream")
	require.Equal(t, "", UpstreamRequestIDFromHeaders(account, h))

	h.Set("X-Oneapi-Request-Id", " oneapi-2 ")
	require.Equal(t, "oneapi-2", UpstreamRequestIDFromHeaders(account, h))
	require.Equal(t, "", UpstreamRequestIDFromHeaders(account, nil))

	official := &Account{Platform: PlatformAnthropic, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "request-id"}}
	only := http.Header{}
	only.Set("Request-Id", "req_official")
	require.Equal(t, "req_official", UpstreamRequestIDFromHeaders(official, only))
}

func TestUpstreamRequestIDHeaderNameInfersOneAPIConservatively(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name:    "custom OpenAI API key base URL",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}},
			want:    "X-Oneapi-Request-Id",
		},
		{
			name:    "trusted NewAPI identity",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{AccountMonitorBalanceExtraKey: AccountMonitorBalance{Version: AccountMonitorBalanceVersion, Status: AccountMonitorBalanceStatusOK, Source: AccountMonitorBalanceSourceNewAPI}}},
			want:    "X-Oneapi-Request-Id",
		},
		{
			name:    "native probe unsupported",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported}}},
			want:    "X-Oneapi-Request-Id",
		},
		{
			name:    "official OpenAI base URL",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://api.openai.com/v1"}},
		},
		{
			name:    "empty base URL",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		},
		{
			name:    "OpenAI OAuth",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://relay.example/v1"}},
		},
		{
			name:    "non OpenAI platform",
			account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}},
		},
		{
			name:    "invalid base URL",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "://bad"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, UpstreamRequestIDHeaderName(tt.account))
		})
	}
}

func TestUpstreamRequestIDHeaderNameExplicitConfigurationWinsOverInference(t *testing.T) {
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://relay.example/v1"},
		Extra:       map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Custom-Request-Id"},
	}
	require.Equal(t, "X-Custom-Request-Id", UpstreamRequestIDHeaderName(account))
}

func TestUpstreamRequestIDFromHeadersUsesInferredOneAPIHeader(t *testing.T) {
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://relay.example/v1"},
	}
	h := http.Header{}
	h.Set("X-Oneapi-Request-Id", " oneapi-inferred ")
	require.Equal(t, "oneapi-inferred", UpstreamRequestIDFromHeaders(account, h))
}

func TestUsageUpstreamRequestIDPtr(t *testing.T) {
	account := &Account{Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Request-ID"}}
	h := http.Header{}
	h.Set("X-Request-ID", strings.Repeat("a", 200))
	require.Nil(t, usageUpstreamRequestIDPtr(account, h, true))
	require.Nil(t, usageUpstreamRequestIDPtr(account, http.Header{}, false))
	require.Nil(t, usageUpstreamRequestIDPtr(nil, h, false))
	require.Nil(t, usageUpstreamRequestIDPtr(&Account{}, h, false))

	got := usageUpstreamRequestIDPtr(account, h, false)
	require.NotNil(t, got)
	require.Len(t, *got, maxUsageUpstreamRequestIDLen)
}

func TestValidateUpstreamRequestIDHeaderExtra(t *testing.T) {
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(nil))
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{}))

	blank := map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(blank))
	_, present := blank[AccountExtraUpstreamRequestIDHeader]
	require.False(t, present, "blank header name must be removed")

	valid := map[string]any{AccountExtraUpstreamRequestIDHeader: " X-Oneapi-Request-Id "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(valid))
	require.Equal(t, "X-Oneapi-Request-Id", valid[AccountExtraUpstreamRequestIDHeader])

	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: 1}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: "X Request Id"}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Request-Id:"}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: strings.Repeat("x", 65)}))
}
