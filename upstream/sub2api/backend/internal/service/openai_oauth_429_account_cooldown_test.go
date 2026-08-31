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
	setCalls    int
	extendCalls int
	lastResetAt time.Time
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

func TestPersistOpenAIOAuth429CooldownUsesFiveMinuteFallback(t *testing.T) {
	repo := &openAIOAuth429CooldownRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 701, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	before := time.Now()

	svc.PersistOpenAIOAuth429Cooldown(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"try again"}}`))

	require.Equal(t, 0, repo.setCalls)
	require.Equal(t, 1, repo.extendCalls)
	require.InDelta(t, float64(openAIOAuth429PersistentCooldown), float64(repo.lastResetAt.Sub(before)), float64(2*time.Second))
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
