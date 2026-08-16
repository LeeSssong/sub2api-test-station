//go:build unit

package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthServiceRefreshTokenPair_ReplaysSameRotationAfterLostResponse(t *testing.T) {
	ctx := context.Background()
	user := &service.User{
		ID:           71,
		Email:        "admin@example.com",
		PasswordHash: "password-hash",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
	}
	refreshTokenCache := newEmailBindRefreshTokenCacheStub()
	cfg := &config.Config{JWT: config.JWTConfig{
		Secret:                   "refresh-replay-test-secret",
		ExpireHour:               1,
		AccessTokenExpireMinutes: 60,
		RefreshTokenExpireDays:   7,
	}}
	svc := service.NewAuthService(
		nil,
		newEmailBindUserRepoStub(user),
		nil,
		refreshTokenCache,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	original, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)

	type refreshResult struct {
		pair *service.TokenPairWithUser
		err  error
	}
	start := make(chan struct{})
	results := make(chan refreshResult, 2)
	for range 2 {
		go func() {
			<-start
			pair, refreshErr := svc.RefreshTokenPair(ctx, original.RefreshToken)
			results <- refreshResult{pair: pair, err: refreshErr}
		}()
	}
	close(start)
	firstResult := <-results
	secondResult := <-results
	require.NoError(t, firstResult.err)
	require.NoError(t, secondResult.err)
	first := firstResult.pair
	replayed := secondResult.pair
	require.Equal(t, first, replayed)

	refreshTokenCache.mu.Lock()
	var replayMarkerJSON []byte
	var replayTTL time.Duration
	for tokenHash, data := range refreshTokenCache.tokens {
		if data.Rotation != nil {
			replayMarkerJSON, err = json.Marshal(data)
			require.NoError(t, err)
			replayTTL = refreshTokenCache.ttls[tokenHash]
			break
		}
	}
	refreshTokenCache.mu.Unlock()
	require.NotEmpty(t, replayMarkerJSON)
	require.NotContains(t, string(replayMarkerJSON), first.RefreshToken)
	require.False(t, strings.Contains(string(replayMarkerJSON), original.RefreshToken))
	require.Positive(t, replayTTL)
	require.LessOrEqual(t, replayTTL, 10*time.Second)

	require.NoError(t, svc.RevokeRefreshToken(ctx, replayed.RefreshToken))
	_, err = svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.ErrorIs(t, err, service.ErrRefreshTokenInvalid)
}
