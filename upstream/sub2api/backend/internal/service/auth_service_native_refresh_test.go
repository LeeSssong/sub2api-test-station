//go:build unit

package service_test

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthServiceRefreshTokenPair_RejectsReusedRotatedToken(t *testing.T) {
	ctx := context.Background()
	user := &service.User{
		ID:           72,
		Email:        "admin@example.com",
		PasswordHash: "password-hash",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
	}
	refreshTokenCache := newEmailBindRefreshTokenCacheStub()
	cfg := &config.Config{JWT: config.JWTConfig{
		Secret:                   "native-refresh-test-secret",
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

	_, err = svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.NoError(t, err)

	_, err = svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.ErrorIs(t, err, service.ErrRefreshTokenInvalid)
}
