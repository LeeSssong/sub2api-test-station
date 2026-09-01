package service

import (
	"testing"
	"time"
)

func TestNormalizeNotificationBaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "canonicalizes host and slash", input: " HTTPS://Example.COM/api/// ", want: "https://example.com/api", ok: true},
		{name: "rejects userinfo", input: "https://user:pass@example.com", ok: false},
		{name: "rejects query", input: "https://example.com/api?x=1", ok: false},
		{name: "rejects fragment", input: "https://example.com/api#x", ok: false},
		{name: "rejects plain text", input: "example.com", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNotificationBaseURL(tt.input)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("NormalizeNotificationBaseURL() = %q, %v, want %q", got, err, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("NormalizeNotificationBaseURL() error = nil, want rejection for %q", tt.input)
			}
		})
	}
}

func TestEvaluateUpstreamBaseURLBalanceUsesLatestValidSnapshot(t *testing.T) {
	older := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	valueOld := 12.0
	valueNew := 4.5
	failedValue := 0.0
	oldFingerprint, newFingerprint := accountMonitorBalanceCredentialFingerprint("old"), accountMonitorBalanceCredentialFingerprint("new")
	result, err := EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount{
		{AccountID: 11, Name: "active-one", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://UPSTREAM.example/", CredentialFingerprint: oldFingerprint, Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &valueOld, ObservedAt: &older, CredentialFingerprint: oldFingerprint}},
		{AccountID: 12, Name: "active-two", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://upstream.example", CredentialFingerprint: newFingerprint, Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceNewAPI, Status: AccountMonitorBalanceStatusOK, ValueUSD: &valueNew, ObservedAt: &newer, CredentialFingerprint: newFingerprint}},
		{AccountID: 13, Name: "inactive", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "error", BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &failedValue, ObservedAt: &newer}},
		{AccountID: 14, Name: "failed", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusFailed, ValueUSD: &failedValue, ObservedAt: &newer}},
		{AccountID: 15, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &failedValue, ObservedAt: &newer}},
	}, newer)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d evaluations, want 1", len(result))
	}
	got := result[0]
	if got.NormalizedBaseURL != "https://upstream.example" || got.ValueUSD == nil || *got.ValueUSD != valueNew || got.State != UpstreamBalanceStateLow {
		t.Fatalf("evaluation = %#v, want latest low snapshot", got)
	}
	if len(got.Accounts) != 3 || got.Accounts[0].AccountID != 11 || got.Accounts[1].AccountID != 12 || got.Accounts[2].AccountID != 14 {
		t.Fatalf("accounts = %#v, want all active API-key accounts in ID order", got.Accounts)
	}
}

func TestEvaluateUpstreamBaseURLBalanceSkipsAmbiguousScope(t *testing.T) {
	now := time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC)
	zero := 0.0
	conflicting := 1.0
	healthy := 7.0
	fingerprint := accountMonitorBalanceCredentialFingerprint("key")
	result, err := EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount{
		{AccountID: 1, Name: "one", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://same.example", CredentialFingerprint: fingerprint, Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &zero, ObservedAt: &now, CredentialFingerprint: fingerprint}},
		{AccountID: 2, Name: "two", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://same.example/", CredentialFingerprint: fingerprint, Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceNewAPI, Status: AccountMonitorBalanceStatusOK, ValueUSD: &conflicting, ObservedAt: &now, CredentialFingerprint: fingerprint}},
		{AccountID: 3, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://healthy.example", CredentialFingerprint: fingerprint, Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &healthy, ObservedAt: &now, CredentialFingerprint: fingerprint}},
	}, now)
	if err != nil {
		t.Fatalf("ambiguous scope should not block other scopes: %v", err)
	}
	if len(result) != 1 || result[0].NormalizedBaseURL != "https://healthy.example" || result[0].State != UpstreamBalanceStateHealthy {
		t.Fatalf("result = %#v, want only unaffected healthy scope", result)
	}
	if result[0].ValueUSD == nil || *result[0].ValueUSD != healthy {
		t.Fatalf("healthy value = %#v", result[0].ValueUSD)
	}
}

func TestBuildUpstreamBalanceEvaluationsRejectsOldCredentialSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	value := 0.0
	oldFingerprint := accountMonitorBalanceCredentialFingerprint("old-key")
	accounts := []Account{{
		ID: 1, Name: "rotated", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"base_url": "https://upstream.example", "api_key": "new-key"},
	}}
	page := AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		ObservedAt: now,
		Accounts: []AccountMonitorAccount{{AccountID: 1, Balance: &AccountMonitorBalance{
			Version: AccountMonitorBalanceVersion, Status: AccountMonitorBalanceStatusOK,
			Source: AccountMonitorBalanceSourceSub2API, ValueUSD: &value, ObservedAt: &now,
			CredentialFingerprint: oldFingerprint,
		}}},
	}}

	evaluations, err := buildUpstreamBalanceEvaluations(accounts, page)

	if err != nil {
		t.Fatal(err)
	}
	if len(evaluations) != 0 {
		t.Fatalf("old-key snapshot produced evaluations: %#v", evaluations)
	}
}

func TestBuildUpstreamBalanceEvaluationsRejectsStaleZeroSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	observedAt := now.Add(-11 * time.Minute)
	value := 0.0
	fingerprint := accountMonitorBalanceCredentialFingerprint("current-key")
	accounts := []Account{{
		ID: 1, Name: "recharged", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"base_url": "https://upstream.example", "api_key": "current-key"},
	}}
	page := AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		ObservedAt: now,
		Accounts: []AccountMonitorAccount{{AccountID: 1, Balance: &AccountMonitorBalance{
			Version: AccountMonitorBalanceVersion, Status: AccountMonitorBalanceStatusOK,
			Source: AccountMonitorBalanceSourceSub2API, ValueUSD: &value, ObservedAt: &observedAt,
			CredentialFingerprint: fingerprint,
		}}},
	}}

	evaluations, err := buildUpstreamBalanceEvaluations(accounts, page)

	if err != nil {
		t.Fatal(err)
	}
	if len(evaluations) != 0 {
		t.Fatalf("stale zero snapshot produced evaluations: %#v", evaluations)
	}
}
