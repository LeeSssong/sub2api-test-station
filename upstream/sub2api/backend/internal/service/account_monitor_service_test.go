package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type accountMonitorAccountRepoStub struct {
	accounts              []Account
	schedulableAccounts   []Account
	listAllStatus         string
	listAllCalled         bool
	listSchedulableCalled bool
}

func (s *accountMonitorAccountRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	s.listSchedulableCalled = true
	return append([]Account(nil), s.schedulableAccounts...), nil
}

func (s *accountMonitorAccountRepoStub) ListAllWithFilters(
	_ context.Context,
	_ string,
	_ string,
	status string,
	_ string,
	_ int64,
	_ string,
) ([]Account, error) {
	s.listAllCalled = true
	s.listAllStatus = status
	return append([]Account(nil), s.accounts...), nil
}

func TestAccountMonitorListPoolUsesPersistedActiveSchedulableFlags(t *testing.T) {
	future := time.Now().Add(time.Hour)
	repo := &accountMonitorAccountRepoStub{
		accounts: []Account{
			{ID: 9, Status: StatusActive, Schedulable: true, RateLimitResetAt: &future},
			{ID: 2, Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future},
			{ID: 4, Status: StatusDisabled, Schedulable: true},
			{ID: 6, Status: StatusActive, Schedulable: false},
		},
		schedulableAccounts: []Account{
			{ID: 9, Status: StatusActive, Schedulable: true},
		},
	}
	service := NewAccountMonitorService(nil, repo, nil, nil)

	accounts, err := service.listPool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 2 || accounts[1].ID != 9 {
		t.Fatalf("accounts = %#v", accounts)
	}
	if !repo.listAllCalled || repo.listAllStatus != "" {
		t.Fatalf("ListAllWithFilters must not use the composite active filter: %#v", repo)
	}
	if repo.listSchedulableCalled {
		t.Fatal("account monitor must not use runtime scheduler eligibility")
	}
}

func TestAccountMonitorModelUsesMappedTextModel(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gpt-4o-mini":            "upstream-small",
			"gpt-5.2-codex":          "upstream-codex",
			"gpt-5.4":                "upstream-latest",
			"gpt-*":                  "upstream-wildcard",
			"gpt-image-1":            "upstream-image",
			"text-embedding-3-large": "upstream-embedding",
		}},
	}

	if got := monitorModelForAccount(account); got != "gpt-5.4" {
		t.Fatalf("monitorModelForAccount() = %q, want gpt-5.4", got)
	}
}

func TestAccountMonitorModelFallsBackToNativePlatformDefaults(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "anthropic", platform: PlatformAnthropic, want: claude.DefaultTestModel},
		{name: "openai", platform: PlatformOpenAI, want: openai.DefaultTestModel},
		{name: "gemini", platform: PlatformGemini, want: geminicli.DefaultTestModel},
		{name: "grok", platform: PlatformGrok, want: xai.DefaultModels()[0].ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Credentials: map[string]any{"model_mapping": map[string]any{
					"*":                      "wildcard",
					"gpt-image-1":            "image",
					"text-embedding-3-small": "embedding",
					"grok-imagine-video-1.5": "video",
				}},
			}
			if got := monitorModelForAccount(account); got != tt.want {
				t.Fatalf("monitorModelForAccount() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccountMonitorListPoolKeepsOnlyActiveSchedulableAccounts(t *testing.T) {
	service := NewAccountMonitorService(nil, &accountMonitorAccountRepoStub{accounts: []Account{
		{ID: 9, Status: StatusActive, Schedulable: true},
		{ID: 2, Status: StatusActive, Schedulable: true},
		{ID: 4, Status: StatusDisabled, Schedulable: true},
		{ID: 6, Status: StatusActive, Schedulable: false},
	}}, nil, nil)

	accounts, err := service.listPool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 2 || accounts[1].ID != 9 {
		t.Fatalf("accounts = %#v", accounts)
	}
}

func TestAccountMonitorUsageWindowNormalizesNativePercentage(t *testing.T) {
	progress := &UsageProgress{
		Utilization: 42,
		WindowStats: &WindowStats{Requests: 12, Tokens: 340},
	}
	window, ok := accountMonitorUsageWindow("5h", progress)
	if !ok {
		t.Fatal("expected usage window")
	}
	if window.Utilization != 0.42 || window.Requests != 12 || window.Tokens != 340 {
		t.Fatalf("window = %#v", window)
	}
}

func TestAccountMonitorProjectionIncludesReusableTodayStatsWithoutSecrets(t *testing.T) {
	row := AccountMonitorAccount{
		AccountID:   7,
		AccountType: "oauth",
		TodayStats:  &WindowStats{Requests: 3, Tokens: 120, Cost: 0.5, UserCost: 0.8},
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{`"account_type":"oauth"`, `"today_stats":`, `"tokens":120`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("projection %s missing %s", text, expected)
		}
	}
	for _, forbidden := range []string{"credential", "token\"", "cookie", "authorization", "password", "secret"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("projection contains forbidden field %q: %s", forbidden, text)
		}
	}
}
