//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIOAuth429CooldownRepo struct {
	mockAccountRepoForGemini
	setCalls      int
	extendCalls   int
	clearCalls    int
	lastResetAt   time.Time
	groupAccounts []Account
}

func (r *openAIOAuth429CooldownRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.setCalls++
	r.lastResetAt = resetAt
	return nil
}

func (r *openAIOAuth429CooldownRepo) SetRateLimitedIfLater(_ context.Context, _ int64, resetAt time.Time) error {
	r.extendCalls++
	r.lastResetAt = resetAt
	return nil
}

func (r *openAIOAuth429CooldownRepo) ListByGroup(_ context.Context, _ int64) ([]Account, error) {
	return r.groupAccounts, nil
}

func (r *openAIOAuth429CooldownRepo) ClearRateLimit(_ context.Context, _ int64) error {
	r.clearCalls++
	return nil
}

func TestPersistOpenAIOAuth429CooldownUsesFiveMinuteFallback(t *testing.T) {
	repo := &openAIOAuth429CooldownRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 701, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	before := time.Now()

	svc.PersistOpenAIOAuth429Cooldown(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"try again"}}`))

	require.Equal(t, 0, repo.setCalls)
	require.Equal(t, 1, repo.extendCalls)
	require.InDelta(t, float64(openAIOAuth429FallbackCooldown), float64(repo.lastResetAt.Sub(before)), float64(2*time.Second))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestPersistOpenAIOAuth429CooldownHonorsReliableReset(t *testing.T) {
	repo := &openAIOAuth429CooldownRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 702, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	headers := http.Header{"Retry-After": []string{"90"}}

	svc.PersistOpenAIOAuth429Cooldown(context.Background(), account, headers, []byte(`{"error":{"type":"rate_limit_error","message":"try again"}}`))

	require.Equal(t, 1, repo.extendCalls)
	require.InDelta(t, float64(90*time.Second), float64(repo.lastResetAt.Sub(time.Now())), float64(2*time.Second))
}

func TestPersistOpenAIOAuth429CooldownIgnoresNonTransientAndNonOAuth(t *testing.T) {
	repo := &openAIOAuth429CooldownRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	quotaAccount := &Account{ID: 703, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 704, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	quotaHeaders := http.Header{}
	quotaHeaders.Set("x-codex-primary-used-percent", "100")
	quotaHeaders.Set("x-codex-primary-reset-after-seconds", "60")
	quotaHeaders.Set("x-codex-primary-window-minutes", "300")
	svc.PersistOpenAIOAuth429Cooldown(context.Background(), quotaAccount, quotaHeaders, []byte(`{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	svc.PersistOpenAIOAuth429Cooldown(context.Background(), apiKeyAccount, http.Header{}, nil)

	require.Zero(t, repo.extendCalls)
}

func TestRefreshOpenAIOAuth429GroupClearsOnlyShortExcludedCooldowns(t *testing.T) {
	now := time.Now()
	shortStarted := now.Add(-time.Minute)
	shortReset := now.Add(4 * time.Minute)
	sevenDayReset := now.Add(24 * time.Hour)
	repo := &openAIOAuth429CooldownRepo{groupAccounts: []Account{
		{ID: 705, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &shortStarted, RateLimitResetAt: &shortReset},
		{ID: 706, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &shortStarted, RateLimitResetAt: &sevenDayReset, Extra: map[string]any{"codex_7d_used_percent": 100.0, "codex_7d_reset_at": sevenDayReset.Format(time.RFC3339)}},
		{ID: 707, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateLimitedAt: &shortStarted, RateLimitResetAt: &shortReset},
	}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	svc.BlockAccountScheduling(&repo.groupAccounts[0], shortReset, "429")

	cleared, err := svc.RefreshOpenAIOAuth429Group(context.Background(), 99, map[int64]struct{}{705: {}, 706: {}, 707: {}})

	require.NoError(t, err)
	require.Equal(t, 1, cleared)
	require.Equal(t, 1, repo.clearCalls)
	_, blocked := svc.openaiAccountRuntimeBlockUntil.Load(int64(705))
	require.False(t, blocked)
}

func TestRefreshOpenAIOAuth429GroupRequiresExcludedAccounts(t *testing.T) {
	repo := &openAIOAuth429CooldownRepo{groupAccounts: []Account{{ID: 708, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	cleared, err := svc.RefreshOpenAIOAuth429Group(context.Background(), 99, nil)

	require.NoError(t, err)
	require.Zero(t, cleared)
	require.Zero(t, repo.clearCalls)
}
