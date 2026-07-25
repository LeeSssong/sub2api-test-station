package service

import (
	"context"
	"encoding/json"
	"errors"
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

type accountMonitorConnectionProbeStub struct {
	result AccountMonitorProbeResult
	err    error
}

func (s *accountMonitorConnectionProbeStub) ProbeAccountConnection(
	context.Context,
	int64,
	string,
	string,
	string,
) (AccountMonitorProbeResult, error) {
	return s.result, s.err
}

type accountMonitorMultiplierRefreshStub struct {
	calls []bool
	err   error
}

func (s *accountMonitorMultiplierRefreshStub) RefreshAccount(
	_ context.Context,
	_ int64,
	force bool,
) error {
	s.calls = append(s.calls, force)
	return s.err
}

type accountMonitorRepositoryRecording struct {
	AccountMonitorRepository
	inserted []AccountMonitorProbeResult
}

func (r *accountMonitorRepositoryRecording) InsertResult(
	_ context.Context,
	result AccountMonitorProbeResult,
	_ string,
) error {
	r.inserted = append(r.inserted, result)
	return nil
}

func (r *accountMonitorRepositoryRecording) DeleteBefore(context.Context, time.Time) error {
	return nil
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

func TestResolveAccountMonitorMultiplierPrefersFreshDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	declaredAt := now.Add(-time.Minute)
	declaredUntil := now.Add(time.Hour)
	measuredAt := now.Add(-2 * time.Minute)
	measuredUntil := now.Add(23 * time.Hour)
	account := &Account{Extra: map[string]any{
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
			Status:     UpstreamBillingProbeStatusOK,
			Data:       accountMonitorDeclaredMultiplierData(0.16, declaredAt),
			ReceivedAt: &declaredAt,
			FreshUntil: &declaredUntil,
		},
		UpstreamMultiplierMeasurementExtraKey: UpstreamMultiplierMeasurementSnapshot{
			SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
			Status:        AccountMonitorMultiplierStatusOK,
			Multiplier:    numberPointer(0.08),
			ObservedAt:    measuredAt,
			FreshUntil:    measuredUntil,
			Model:         "gpt-5.4",
			SampleCount:   3,
		},
	}}

	got := resolveAccountMonitorMultiplier(account, now)

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceDeclared ||
		got.Value == nil || *got.Value != 0.16 || got.ObservedAt == nil || !got.ObservedAt.Equal(declaredAt) {
		t.Fatalf("multiplier = %#v", got)
	}
}

func TestResolveAccountMonitorMultiplierUsesFreshMeasurementAfterUnsupportedDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	measuredAt := now.Add(-time.Hour)
	account := &Account{Extra: map[string]any{
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
			Status:     UpstreamBillingProbeStatusUnsupported,
			HTTPStatus: 404,
		},
		UpstreamMultiplierMeasurementExtraKey: UpstreamMultiplierMeasurementSnapshot{
			SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
			Status:        AccountMonitorMultiplierStatusOK,
			Multiplier:    numberPointer(0.09),
			ObservedAt:    measuredAt,
			FreshUntil:    measuredAt.Add(24 * time.Hour),
			Model:         "gpt-5.4",
			SampleCount:   3,
		},
	}}

	got := resolveAccountMonitorMultiplier(account, now)

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceMeasured ||
		got.Value == nil || *got.Value != 0.09 || got.ObservedAt == nil || !got.ObservedAt.Equal(measuredAt) {
		t.Fatalf("multiplier = %#v", got)
	}
}

func TestResolveAccountMonitorMultiplierRejectsStaleAndInvalidValues(t *testing.T) {
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	observedAt := now.Add(-25 * time.Hour)
	tests := []struct {
		name   string
		extra  map[string]any
		status string
		source string
	}{
		{
			name: "stale measurement",
			extra: map[string]any{
				UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
				UpstreamMultiplierMeasurementExtraKey: UpstreamMultiplierMeasurementSnapshot{
					SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
					Status:        AccountMonitorMultiplierStatusOK,
					Multiplier:    numberPointer(0.12),
					ObservedAt:    observedAt,
					FreshUntil:    now.Add(-time.Hour),
					Model:         "gpt-5.4",
					SampleCount:   3,
				},
			},
			status: AccountMonitorMultiplierStatusStale,
			source: AccountMonitorMultiplierSourceMeasured,
		},
		{
			name: "unsupported declaration",
			extra: map[string]any{
				UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
					Status: UpstreamBillingProbeStatusUnsupported,
				},
			},
			status: AccountMonitorMultiplierStatusUnsupported,
			source: AccountMonitorMultiplierSourceDeclared,
		},
		{
			name: "failed measurement",
			extra: map[string]any{
				UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
				UpstreamMultiplierMeasurementExtraKey: UpstreamMultiplierMeasurementSnapshot{
					SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
					Status:        AccountMonitorMultiplierStatusFailed,
					ObservedAt:    observedAt,
				},
			},
			status: AccountMonitorMultiplierStatusFailed,
			source: AccountMonitorMultiplierSourceMeasured,
		},
		{
			name:   "unavailable",
			extra:  nil,
			status: AccountMonitorMultiplierStatusUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAccountMonitorMultiplier(&Account{Extra: tt.extra}, now)
			if got.Status != tt.status || got.Source != tt.source || got.Value != nil {
				t.Fatalf("multiplier = %#v", got)
			}
		})
	}
}

func TestAccountMonitorMultiplierProjectionIsSanitized(t *testing.T) {
	observedAt := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	row := AccountMonitorAccount{
		AccountID: 7,
		Multiplier: AccountMonitorMultiplier{
			Value:      numberPointer(0.08),
			Source:     AccountMonitorMultiplierSourceMeasured,
			Status:     AccountMonitorMultiplierStatusOK,
			ObservedAt: &observedAt,
		},
	}

	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(encoded))
	for _, expected := range []string{`"multiplier":{"value":0.08`, `"source":"measured"`, `"status":"ok"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("projection %s missing %s", text, expected)
		}
	}
	for _, forbidden := range []string{"base_url", "api_key", "quota", "request_body", "response_body", "last_error"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projection contains forbidden field %q: %s", forbidden, text)
		}
	}
}

func TestAccountMonitorRunOnePersistsConnectivityWhenMultiplierRefreshFails(t *testing.T) {
	monitorRepo := &accountMonitorRepositoryRecording{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          17,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
	}}}
	connection := &accountMonitorConnectionProbeStub{result: AccountMonitorProbeResult{
		Status:    "success",
		CheckedAt: time.Now().UTC(),
	}}
	multiplier := &accountMonitorMultiplierRefreshStub{err: errors.New("measurement failed")}
	service := NewAccountMonitorService(monitorRepo, accountRepo, connection, nil, multiplier)

	result, err := service.RunOne(context.Background(), 99, 17)

	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || len(monitorRepo.inserted) != 1 ||
		monitorRepo.inserted[0].Status != "success" {
		t.Fatalf("result=%#v inserted=%#v", result, monitorRepo.inserted)
	}
	if len(multiplier.calls) != 1 || !multiplier.calls[0] {
		t.Fatalf("multiplier calls = %#v", multiplier.calls)
	}
}

func accountMonitorDeclaredMultiplierData(multiplier float64, observedAt time.Time) map[string]any {
	return map[string]any{
		"billing_scope":             "token",
		"resolved_rate_multiplier":  multiplier,
		"peak_rate_enabled":         false,
		"effective_rate_multiplier": multiplier,
		"observed_at":               observedAt.Format(time.RFC3339Nano),
	}
}

func numberPointer(value float64) *float64 {
	return &value
}
