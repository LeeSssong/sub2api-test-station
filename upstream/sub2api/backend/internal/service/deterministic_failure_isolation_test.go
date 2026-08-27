package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestClassifyDeterministicUpstreamFailure(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Extra: map[string]any{
		"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"},
	}}
	cases := []struct {
		name       string
		statusCode int
		body       string
		model      string
		class      string
		scope      string
		canonical  string
		classified bool
	}{
		{name: "explicit balance code", statusCode: 402, body: `{"error":{"code":"insufficient_balance"}}`, class: "balance_exhausted", scope: "account", classified: true},
		{name: "generic payment required is not balance evidence", statusCode: 402, body: `{"error":{"message":"request rejected"}}`},
		{name: "explicit model not found", statusCode: 404, body: `{"error":{"code":"model_not_found"}}`, model: "gpt-5.6-sol", class: "model_unsupported", scope: "account_model", canonical: "gpt-5.6-sol", classified: true},
		{name: "api key unauthorized", statusCode: 401, body: `{"error":{"code":"invalid_api_key"}}`, class: "credential_invalid", scope: "account", classified: true},
		{name: "api key bare unauthorized", statusCode: 401, body: `{"detail":"Unauthorized"}`, class: "credential_invalid", scope: "account", classified: true},
		{name: "deactivated workspace is not balance", statusCode: 402, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{name: "generic forbidden", statusCode: 403, body: `{"error":{"message":"forbidden"}}`},
		{name: "empty catalog", statusCode: 200, body: `{"data":[]}`, model: "gpt-5.6-sol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDeterministicUpstreamFailure(account, tc.statusCode, []byte(tc.body), tc.model)
			require.Equal(t, tc.classified, got.Classified)
			require.Equal(t, tc.class, got.FailureClass)
			require.Equal(t, tc.scope, got.Scope)
			require.Equal(t, tc.canonical, got.CanonicalModel)
		})
	}
}

func TestBuildDeterministicFailureReasonIsBoundedAndOwned(t *testing.T) {
	message := "long-body-" + strings.Repeat("x", 400)
	reason := buildDeterministicFailureReason(DeterministicFailureDecision{
		Classified: true, FailureClass: "balance_exhausted", Scope: "account", EvidenceCode: "insufficient_balance",
		RecoveryPolicy: "expires",
	}, message, time.Unix(100, 0))
	require.Contains(t, reason, `"source":"deterministic_failure_isolation"`)
	require.Contains(t, reason, `"failure_class":"balance_exhausted"`)
	require.LessOrEqual(t, len(reason), 700)
}

func TestDeterministicBalanceIsolationDurationBounds(t *testing.T) {
	for _, tc := range []struct {
		configured int
		want       time.Duration
	}{
		{60, 3650 * 24 * time.Hour}, {90, 3650 * 24 * time.Hour}, {120, 3650 * 24 * time.Hour}, {59, 3650 * 24 * time.Hour}, {121, 3650 * 24 * time.Hour},
	} {
		cfg := &config.Config{}
		cfg.RateLimit.BalanceExhaustedIsolationMinutes = tc.configured
		require.Equal(t, tc.want, deterministicBalanceIsolationDuration(cfg))
	}
}
