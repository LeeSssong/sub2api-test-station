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
	result, err := EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount{
		{AccountID: 11, Name: "active-one", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://UPSTREAM.example/", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &valueOld, ObservedAt: &older}},
		{AccountID: 12, Name: "active-two", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceNewAPI, Status: AccountMonitorBalanceStatusOK, ValueUSD: &valueNew, ObservedAt: &newer}},
		{AccountID: 13, Name: "inactive", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "error", BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &failedValue, ObservedAt: &newer}},
		{AccountID: 14, Name: "failed", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusFailed, ValueUSD: &failedValue, ObservedAt: &newer}},
		{AccountID: 15, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, BaseURL: "https://upstream.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &failedValue, ObservedAt: &newer}},
	})
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

func TestEvaluateUpstreamBaseURLBalanceRejectsAmbiguousAndInvalidData(t *testing.T) {
	now := time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC)
	zero := 0.0
	conflicting := 1.0
	result, err := EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount{
		{AccountID: 1, Name: "one", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://same.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &zero, ObservedAt: &now}},
		{AccountID: 2, Name: "two", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://same.example/", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceNewAPI, Status: AccountMonitorBalanceStatusOK, ValueUSD: &conflicting, ObservedAt: &now}},
	})
	if err == nil || result != nil {
		t.Fatalf("ambiguous/invalid result = %#v, %v, want fail-closed error", result, err)
	}

	healthy := 7.0
	result, err = EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount{{AccountID: 3, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, BaseURL: "https://healthy.example", Snapshot: &AccountMonitorBalance{Version: 1, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK, ValueUSD: &healthy, ObservedAt: &now}}})
	if err != nil || len(result) != 1 || result[0].State != UpstreamBalanceStateHealthy {
		t.Fatalf("healthy result = %#v, %v", result, err)
	}
	if result[0].ValueUSD == nil || *result[0].ValueUSD != healthy {
		t.Fatalf("healthy value = %#v", result[0].ValueUSD)
	}
}
