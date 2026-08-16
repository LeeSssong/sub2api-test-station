package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenRotationLock_OnlyOwnerCanRelease(t *testing.T) {
	server := miniredis.RunT(t)
	cache := NewRefreshTokenCache(redis.NewClient(&redis.Options{Addr: server.Addr()}))
	locker := cache.(service.RefreshTokenRotationLocker)
	ctx := context.Background()

	acquired, err := locker.AcquireRefreshTokenRotation(ctx, "old-token-hash", "owner-a", 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = locker.AcquireRefreshTokenRotation(ctx, "old-token-hash", "owner-b", 30*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, locker.ReleaseRefreshTokenRotation(ctx, "old-token-hash", "owner-b"))
	acquired, err = locker.AcquireRefreshTokenRotation(ctx, "old-token-hash", "owner-b", 30*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, locker.ReleaseRefreshTokenRotation(ctx, "old-token-hash", "owner-a"))
	acquired, err = locker.AcquireRefreshTokenRotation(ctx, "old-token-hash", "owner-b", 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
}
