package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIAccountQualityRepoStub struct {
	mu    sync.Mutex
	rows  []OpenAIAccountQuality
	err   error
	calls int
}

func (r *openAIAccountQualityRepoStub) ListOpenAIAccountQuality(context.Context, time.Time, time.Time) ([]OpenAIAccountQuality, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]OpenAIAccountQuality(nil), r.rows...), nil
}

func (r *openAIAccountQualityRepoStub) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestOpenAIAccountQualitySnapshotProviderCachesAndServesStaleData(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	repo := &openAIAccountQualityRepoStub{rows: []OpenAIAccountQuality{{AccountID: 7, AttemptCount: 2, SuccessCount: 2}}}
	provider := NewOpenAIAccountQualitySnapshotProvider(repo, time.Minute, clock)

	first := provider.Snapshot(context.Background())
	second := provider.Snapshot(context.Background())
	require.Equal(t, 1, repo.callCount())
	require.False(t, first.Stale)
	require.Equal(t, first, second)

	now = now.Add(61 * time.Second)
	repo.err = errors.New("db unavailable")
	stale := provider.Snapshot(context.Background())
	require.Equal(t, 2, repo.callCount())
	require.True(t, stale.Stale)
	require.Equal(t, int64(2), stale.Accounts[7].AttemptCount)
}

func TestOpenAIAccountQualitySnapshotProviderRefreshesAfterExpiry(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	repo := &openAIAccountQualityRepoStub{rows: []OpenAIAccountQuality{{AccountID: 7, AttemptCount: 1}}}
	provider := NewOpenAIAccountQualitySnapshotProvider(repo, time.Minute, func() time.Time { return now })

	_ = provider.Snapshot(context.Background())
	now = now.Add(61 * time.Second)
	repo.mu.Lock()
	repo.rows = []OpenAIAccountQuality{{AccountID: 8, AttemptCount: 3}}
	repo.mu.Unlock()
	refreshed := provider.Snapshot(context.Background())
	require.Equal(t, 2, repo.callCount())
	require.False(t, refreshed.Stale)
	require.Contains(t, refreshed.Accounts, int64(8))
	require.NotContains(t, refreshed.Accounts, int64(7))
}

func TestOpenAIAccountQualitySnapshotProviderColdStartFailureIsNonBlocking(t *testing.T) {
	repo := &openAIAccountQualityRepoStub{err: errors.New("db unavailable")}
	provider := NewOpenAIAccountQualitySnapshotProvider(repo, time.Minute, time.Now)

	snapshot := provider.Snapshot(context.Background())
	require.True(t, snapshot.Stale)
	require.Empty(t, snapshot.Accounts)
	require.Empty(t, snapshot.WindowStart)
	require.Empty(t, snapshot.WindowEnd)
}
