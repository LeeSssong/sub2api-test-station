package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newOpenAISharedHealthUnitStore(t *testing.T) (service.OpenAISharedHealthStore, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewOpenAISharedHealthStore(rdb), rdb, mr
}

func mustOpenAISharedHealthKey(t *testing.T) service.OpenAISharedHealthKey {
	t.Helper()
	key, err := service.NewOpenAISharedHealthKey(153, " GPT-5.6-SOL ")
	require.NoError(t, err)
	return key
}

func TestOpenAISharedHealthRecordAttemptIsIdempotentAndResetsOnSuccess(t *testing.T) {
	store, rdb, _ := newOpenAISharedHealthUnitStore(t)
	ctx := context.Background()
	key := mustOpenAISharedHealthKey(t)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	domain := service.OpenAIFailureDomain{Type: service.OpenAIFailureDomainQuotaPool, ID: "openai:quota_pool:org-secret"}

	failure := service.OpenAISharedHealthEvent{
		ID:            "attempt-1",
		Key:           key,
		Domains:       []service.OpenAIFailureDomain{domain},
		StatusCode:    503,
		ErrorType:     "transient_upstream",
		TTFT:          1500 * time.Millisecond,
		ObservedAt:    now,
		CooldownUntil: now.Add(10 * time.Second),
	}
	snapshot, err := store.RecordAttempt(ctx, failure)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.FailureStreak)
	require.Equal(t, int64(1), snapshot.Revision)
	require.Equal(t, service.OpenAISharedHealthStateCooldown, snapshot.State)
	require.InDelta(t, 1, snapshot.EWMAErrorRate, 0.0001)
	require.Equal(t, 1500*time.Millisecond, snapshot.EWMATTFT)

	duplicate := failure
	duplicate.Success = true
	duplicate.StatusCode = 200
	snapshot, err = store.RecordAttempt(ctx, duplicate)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.FailureStreak)
	require.Equal(t, int64(1), snapshot.Revision)
	require.Equal(t, service.OpenAISharedHealthStateCooldown, snapshot.State)

	domainKey := openAISharedHealthFailureDomainKey(domain, key.CanonicalModel)
	domainCount, err := rdb.Exists(ctx, domainKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), domainCount)
	require.NotContains(t, domainKey, "org-secret")

	success := service.OpenAISharedHealthEvent{
		ID:         "attempt-2",
		Key:        key,
		Domains:    []service.OpenAIFailureDomain{domain},
		Success:    true,
		StatusCode: 200,
		TTFT:       500 * time.Millisecond,
		ObservedAt: now.Add(time.Second),
	}
	snapshot, err = store.RecordAttempt(ctx, success)
	require.NoError(t, err)
	require.Equal(t, 0, snapshot.FailureStreak)
	require.Equal(t, int64(2), snapshot.Revision)
	require.Equal(t, service.OpenAISharedHealthStateHealthy, snapshot.State)
	require.Zero(t, snapshot.CooldownUntil)
	require.InDelta(t, 0.8, snapshot.EWMAErrorRate, 0.0001)
	require.InDelta(t, 1300, snapshot.EWMATTFT.Milliseconds(), 1)

	ttl, err := rdb.TTL(ctx, openAISharedHealthAccountModelKey(key)).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 20*time.Minute)
}

func TestOpenAISharedHealthGetRejectsUnknownSchema(t *testing.T) {
	store, rdb, _ := newOpenAISharedHealthUnitStore(t)
	ctx := context.Background()
	key := mustOpenAISharedHealthKey(t)
	require.NoError(t, rdb.HSet(ctx, openAISharedHealthAccountModelKey(key), "schema_version", 2, "state", "healthy").Err())

	snapshot, err := store.GetAccountModel(ctx, key)
	require.ErrorIs(t, err, service.ErrOpenAISharedHealthUnknownSchema)
	require.Equal(t, service.OpenAISharedHealthStateUnknown, snapshot.State)
}

func TestOpenAISharedHealthHalfOpenLeaseHasOneWinnerAndRejectsStaleFence(t *testing.T) {
	store, _, mr := newOpenAISharedHealthUnitStore(t)
	ctx := context.Background()
	key := mustOpenAISharedHealthKey(t)
	leaseTTL := 15 * time.Second

	type result struct {
		lease service.OpenAISharedHalfOpenLease
		ok    bool
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"instance-a", "instance-b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			<-start
			lease, ok, err := store.AcquireHalfOpen(ctx, key, owner, leaseTTL)
			results <- result{lease: lease, ok: ok, err: err}
		}(owner)
	}
	close(start)
	wg.Wait()
	close(results)

	var winner service.OpenAISharedHalfOpenLease
	winners := 0
	for got := range results {
		require.NoError(t, got.err)
		if got.ok {
			winner = got.lease
			winners++
		}
	}
	require.Equal(t, 1, winners)
	require.Positive(t, winner.FencingToken)

	mr.FastForward(leaseTTL + time.Second)
	newLease, ok, err := store.AcquireHalfOpen(ctx, key, "instance-c", leaseTTL)
	require.NoError(t, err)
	require.True(t, ok)
	require.Greater(t, newLease.FencingToken, winner.FencingToken)

	err = store.CompleteHalfOpen(ctx, winner, true, time.Now().UTC())
	require.ErrorIs(t, err, service.ErrOpenAISharedHealthLeaseLost)
	require.NoError(t, store.CompleteHalfOpen(ctx, newLease, true, time.Now().UTC()))
	snapshot, err := store.GetAccountModel(ctx, key)
	require.NoError(t, err)
	require.Equal(t, service.OpenAISharedHealthStateHalfOpen, snapshot.State, "one successful probe must not restore normal scheduling")

	secondLease, ok, err := store.AcquireHalfOpen(ctx, key, "instance-d", leaseTTL)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, store.CompleteHalfOpen(ctx, secondLease, true, time.Now().UTC()))
	snapshot, err = store.GetAccountModel(ctx, key)
	require.NoError(t, err)
	require.Equal(t, service.OpenAISharedHealthStateHealthy, snapshot.State, "two consecutive successful probes restore normal scheduling")
}

func TestOpenAISharedHealthPropagatesCanceledContext(t *testing.T) {
	store, _, _ := newOpenAISharedHealthUnitStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.GetAccountModel(ctx, mustOpenAISharedHealthKey(t))
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), context.Canceled.Error()))
}

type openAIAdmissionStore interface {
	AcquireAdmission(context.Context, service.OpenAISharedAdmissionRequest) (service.OpenAISharedAdmissionDecision, error)
	RenewAdmission(context.Context, service.OpenAISharedAdmissionRequest) error
	ReleaseAdmission(context.Context, service.OpenAISharedAdmissionRequest) error
	RecordSlowSessionGuard(context.Context, int64, time.Time) error
	HasSlowSessionGuard(context.Context, int64) (bool, error)
}

func requireOpenAIAdmissionStore(t *testing.T, store service.OpenAISharedHealthStore) openAIAdmissionStore {
	t.Helper()
	admissionStore, ok := store.(openAIAdmissionStore)
	require.True(t, ok, "OpenAI shared health store must support admission leases and request quality")
	return admissionStore
}

func TestOpenAISharedHealthStoreHasNoSharedQualityAPI(t *testing.T) {
	store, _, _ := newOpenAISharedHealthUnitStore(t)
	_, hasGetQuality := store.(interface {
		GetRequestQuality(context.Context, service.OpenAISharedHealthKey, service.OpenAIAdmissionRequestShape) (any, error)
	})
	_, hasRecordQuality := store.(interface {
		RecordRequestQuality(context.Context, service.OpenAISharedHealthKey, service.OpenAIAdmissionRequestShape, time.Duration, time.Time) (any, error)
	})
	require.False(t, hasGetQuality)
	require.False(t, hasRecordQuality)
}

func TestOpenAISharedHealthAdmissionLongBlocksAcrossModelsAndGroupsAndReleaseIsLeaseScoped(t *testing.T) {
	store, rdb, _ := newOpenAISharedHealthUnitStore(t)
	admissionStore := requireOpenAIAdmissionStore(t, store)
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)

	long := service.OpenAISharedAdmissionRequest{AccountID: 153, LeaseID: "safe-lease-a", Shape: service.OpenAIAdmissionShapeLong, ObservedAt: now}
	decision, err := admissionStore.AcquireAdmission(ctx, long)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, 1, decision.ActiveLong)

	// These represent different requested models and groups. Neither identifier
	// is accepted by the account-only admission contract, so the shared lease
	// must still reject the second request for account 153.
	normal := service.OpenAISharedAdmissionRequest{AccountID: 153, LeaseID: "safe-lease-b", Shape: service.OpenAIAdmissionShapeNormal, ObservedAt: now.Add(time.Second)}
	decision, err = admissionStore.AcquireAdmission(ctx, normal)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, "long_pre_first_output", decision.Reason)

	wrongLease := long
	wrongLease.LeaseID = "safe-lease-c"
	require.NoError(t, admissionStore.ReleaseAdmission(ctx, wrongLease))
	decision, err = admissionStore.AcquireAdmission(ctx, normal)
	require.NoError(t, err)
	require.False(t, decision.Allowed)

	require.NoError(t, admissionStore.ReleaseAdmission(ctx, long))
	decision, err = admissionStore.AcquireAdmission(ctx, normal)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, 1, decision.ActiveNormal)

	keys, err := rdb.Keys(ctx, openAISharedHealthKeyPrefix+"admission:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.NotContains(t, keys[0], "safe-lease")
	require.Equal(t, openAISharedHealthAdmissionKey(153), keys[0])
}

func TestOpenAISharedHealthSlowSessionGuardIsAccountOnly(t *testing.T) {
	store, _, _ := newOpenAISharedHealthUnitStore(t)
	admissionStore := requireOpenAIAdmissionStore(t, store)
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)

	require.NoError(t, admissionStore.RecordSlowSessionGuard(ctx, 153, now))
	guarded, err := admissionStore.HasSlowSessionGuard(ctx, 153)
	require.NoError(t, err)
	require.True(t, guarded)
	guarded, err = admissionStore.HasSlowSessionGuard(ctx, 154)
	require.NoError(t, err)
	require.False(t, guarded)

	decision, err := admissionStore.AcquireAdmission(ctx, service.OpenAISharedAdmissionRequest{
		AccountID: 153, LeaseID: "safe-lease-guarded", Shape: service.OpenAIAdmissionShapeNormal, ObservedAt: now,
	})
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, "slow_session_guard", decision.Reason)
}
