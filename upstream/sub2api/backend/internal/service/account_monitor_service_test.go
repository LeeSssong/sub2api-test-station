package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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

type accountMonitorRepoStub struct {
	AccountMonitorRepository
	mu      sync.Mutex
	results []AccountMonitorProbeResult
}

func (s *accountMonitorRepoStub) InsertResult(_ context.Context, result AccountMonitorProbeResult, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, result)
	return nil
}

func (s *accountMonitorRepoStub) DeleteBefore(context.Context, time.Time) error {
	return nil
}

type accountMonitorMultiplierStub struct {
	mu     sync.Mutex
	calls  []accountMonitorMultiplierCall
	err    error
	result AccountMonitorMultiplier
}

type accountMonitorMultiplierCall struct {
	accountID int64
	force     bool
}

func (s *accountMonitorMultiplierStub) Resolve(*Account, time.Time) AccountMonitorMultiplier {
	return s.result
}

func (s *accountMonitorMultiplierStub) Refresh(_ context.Context, account *Account, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, accountMonitorMultiplierCall{accountID: account.ID, force: force})
	return s.err
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
	service := NewAccountMonitorService(nil, repo, nil, nil, nil)

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
	}}, nil, nil, nil)

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

func TestAccountMonitorServiceRunAllRefreshesDueMultiplierWithoutFailingConnectivity(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          21,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	multiplier := &accountMonitorMultiplierStub{err: errors.New("measurement failed")}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

	completed, err := service.RunAll(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if completed != 1 || len(monitorRepo.results) != 1 {
		t.Fatalf("completed=%d results=%#v", completed, monitorRepo.results)
	}
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 21, force: false}) {
		t.Fatalf("multiplier calls = %#v", multiplier.calls)
	}
}

func TestAccountMonitorServiceRunOneForcesMultiplierWithoutFailingConnectivity(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          23,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	multiplier := &accountMonitorMultiplierStub{err: errors.New("measurement failed")}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

	result, err := service.RunOne(context.Background(), 1, 23)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountID != 23 || len(monitorRepo.results) != 1 {
		t.Fatalf("result=%#v persisted=%#v", result, monitorRepo.results)
	}
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 23, force: true}) {
		t.Fatalf("multiplier calls = %#v", multiplier.calls)
	}
}
