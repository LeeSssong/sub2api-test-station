//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitService_HandleUpstreamError_DoesNotUseDeterministicBalanceClassifier(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	blocker := &runtimeBlockRecorder{}
	svc.SetAccountRuntimeBlocker(blocker)
	account := &Account{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusPaymentRequired, http.Header{}, []byte(`{"error":{"code":"insufficient_user_quota","message":"balance exhausted"}}`))

	require.True(t, shouldDisable)
<<<<<<< HEAD
	require.Equal(t, 1, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
	require.Contains(t, repo.lastTempReason, `"failure_class":"balance_exhausted"`)
	require.Contains(t, repo.lastTempReason, `"recovery_policy":"probe_required"`)
	require.Equal(t, 0, len(repo.modelRateLimitCalls))
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, account.ID, blocker.accounts[0].ID)
	require.Equal(t, "deterministic_balance_exhausted", blocker.reasons[0])
}

func TestRateLimitService_DeterministicBalancePreemptsPoolMode(t *testing.T) {
	repo := &deterministicIsolationRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 34, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"pool_mode": true,
	}}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, []byte(`{"error":{"code":"insufficient_user_quota"}}`))

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestRateLimitService_DeterministicBalancePreemptsCustomErrorCodeSkip(t *testing.T) {
	repo := &deterministicIsolationRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 35, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"custom_error_codes_enabled": true,
		"custom_error_codes":         []any{float64(http.StatusUnauthorized)},
	}}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusPaymentRequired, http.Header{}, []byte(`{"error":{"code":"E44001"}}`))

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestRateLimitService_DeterministicModelUsesCanonicalProbeRequiredLimit(t *testing.T) {
	repo := &deterministicIsolationRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}, Credentials: map[string]any{"model_mapping": map[string]any{"alias": "gpt-5.6-sol"}}}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusNotFound, http.Header{}, []byte(`{"error":{"code":"model_not_found"}}`), "alias")

	require.True(t, shouldDisable)
	require.Empty(t, repo.tempCalls)
	require.Empty(t, repo.setErrorCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.6-sol", repo.modelRateLimitCalls[0].scope)
	require.Contains(t, repo.modelRateLimitCalls[0].reason, `"recovery_policy":"probe_required"`)
}

func TestRateLimitService_GenericForbiddenDoesNotCreateDeterministicIsolation(t *testing.T) {
	repo := &deterministicIsolationRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 33, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_ = svc.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, []byte(`{"error":{"message":"forbidden"}}`), "gpt-5.6-sol")

=======
>>>>>>> codex/p1-task4
	require.Zero(t, repo.tempCalls)
	require.Equal(t, 1, repo.setErrorCalls)
	require.NotContains(t, repo.lastErrorMsg, "deterministic_failure_isolation")
	require.Contains(t, repo.lastErrorMsg, "Payment required (402)")
}

func TestRateLimitService_HandleUpstreamError_403UsesNativeHandler(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, []byte(`{"error":{"message":"forbidden"}}`))

	require.True(t, shouldDisable)
	require.Zero(t, repo.tempCalls)
	require.Equal(t, 1, repo.setErrorCalls)
	require.NotContains(t, repo.lastErrorMsg, "deterministic_failure_isolation")
}

func TestRateLimitService_HandleUpstreamError_ModelNotFoundUsesNativeHandler(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := openAIModelNotFoundTempAccount()

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusNotFound, http.Header{}, []byte(`{"error":{"code":"model_not_found"}}`), "gpt-5.4")

	require.True(t, shouldDisable)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, upstreamModelNotFoundReason, repo.modelRateLimitCalls[0].reason)
	require.NotContains(t, repo.modelRateLimitCalls[0].reason, "deterministic_failure_isolation")
}

func TestRateLimitService_HandleUpstreamError_429UsesNativeHandler(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 34, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(`{"error":{"message":"rate limit exceeded"}}`))

	require.False(t, shouldDisable)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
}

func TestRateLimitService_HandleUpstreamError_5xxUsesNativeHandler(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 35, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"upstream failure"}}`))

	require.False(t, shouldDisable)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
}
