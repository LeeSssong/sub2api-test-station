package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stickyReferenceCountingCache struct {
	count int
}

func (c *stickyReferenceCountingCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, ErrStickySessionNotFound
}
func (*stickyReferenceCountingCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}
func (*stickyReferenceCountingCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}
func (*stickyReferenceCountingCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}
func (c *stickyReferenceCountingCache) CountStickyAccountReferences(context.Context, int64) (int, error) {
	return c.count, nil
}

func TestOpenAIAccountModelRuntimeCountsLiveRedisStickyReferences(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &stickyReferenceCountingCache{count: 3}}
	now := time.Date(2032, 1, 2, 0, 0, 0, 0, time.UTC)
	svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
		AccountID: 92, CanonicalModel: "gpt-5.5", StatusCode: 502, ErrorType: "transient_upstream", Now: now,
	})

	snapshots := svc.SnapshotOpenAIAccountModelRuntime(now)
	require.Len(t, snapshots, 1)
	require.Equal(t, 3, snapshots[0].StickyReferenceCount)
}
