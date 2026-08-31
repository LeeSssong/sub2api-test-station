package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAISharedHealthStoreStub struct {
	mu                sync.Mutex
	snapshots         map[string]OpenAISharedHealthSnapshot
	getErr            error
	recordErr         error
	lease             OpenAISharedHalfOpenLease
	leaseHeld         bool
	nextFence         int64
	recordCalls       int
	completeCalls     int
	lastEvent         OpenAISharedHealthEvent
	admissions        map[string]OpenAISharedAdmissionRequest
	admissionCalls    int
	releaseCalls      int
	slowGuard         map[int64]time.Time
	recordContextErr  error
	renewCalls        int
	admissionDecision *OpenAISharedAdmissionDecision
}

func newOpenAISharedHealthStoreStub() *openAISharedHealthStoreStub {
	return &openAISharedHealthStoreStub{
		snapshots:  make(map[string]OpenAISharedHealthSnapshot),
		admissions: make(map[string]OpenAISharedAdmissionRequest),
		slowGuard:  make(map[int64]time.Time),
	}
}

func TestClassifyOpenAIAdmissionRequestShape(t *testing.T) {
	require.Equal(t, OpenAIAdmissionShapeUnknown, ClassifyOpenAIAdmissionRequestShape(-1, 65536))
	require.Equal(t, OpenAIAdmissionShapeUnknown, ClassifyOpenAIAdmissionRequestShape(0, 0))
	require.Equal(t, OpenAIAdmissionShapeNormal, ClassifyOpenAIAdmissionRequestShape(65536, 65536))
	require.Equal(t, OpenAIAdmissionShapeLong, ClassifyOpenAIAdmissionRequestShape(65537, 65536))
}

func sharedHealthStubKey(key OpenAISharedHealthKey) string {
	return fmt.Sprintf("%d:%s", key.AccountID, key.HashedSuffix())
}

func (s *openAISharedHealthStoreStub) GetAccountModel(_ context.Context, key OpenAISharedHealthKey) (OpenAISharedHealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return OpenAISharedHealthSnapshot{Key: key, State: OpenAISharedHealthStateUnknown}, s.getErr
	}
	snapshot, ok := s.snapshots[sharedHealthStubKey(key)]
	if !ok {
		return OpenAISharedHealthSnapshot{SchemaVersion: 1, Key: key, State: OpenAISharedHealthStateUnknown}, nil
	}
	return snapshot, nil
}

func (s *openAISharedHealthStoreStub) RecordAttempt(ctx context.Context, event OpenAISharedHealthEvent) (OpenAISharedHealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordCalls++
	s.lastEvent = event
	s.recordContextErr = ctx.Err()
	if s.recordErr != nil {
		return OpenAISharedHealthSnapshot{Key: event.Key, State: OpenAISharedHealthStateUnknown}, s.recordErr
	}
	key := sharedHealthStubKey(event.Key)
	snapshot := s.snapshots[key]
	snapshot.SchemaVersion = 1
	snapshot.Key = event.Key
	snapshot.Revision++
	snapshot.ObservedAt = event.ObservedAt
	if event.Success {
		snapshot.HalfOpenSuccesses = 0
		snapshot.State = OpenAISharedHealthStateHealthy
		snapshot.FailureStreak = 0
		snapshot.CooldownUntil = time.Time{}
	} else {
		snapshot.FailureStreak++
		snapshot.State = OpenAISharedHealthStateSoftFailed
		if event.CooldownUntil.After(event.ObservedAt) {
			snapshot.State = OpenAISharedHealthStateCooldown
			snapshot.CooldownUntil = event.CooldownUntil
		}
	}
	s.snapshots[key] = snapshot
	return snapshot, nil
}

func (s *openAISharedHealthStoreStub) AcquireHalfOpen(_ context.Context, key OpenAISharedHealthKey, owner string, ttl time.Duration) (OpenAISharedHalfOpenLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return OpenAISharedHalfOpenLease{}, false, s.getErr
	}
	if s.leaseHeld {
		return OpenAISharedHalfOpenLease{}, false, nil
	}
	s.nextFence++
	s.leaseHeld = true
	s.lease = OpenAISharedHalfOpenLease{Key: key, Owner: owner, FencingToken: s.nextFence, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(ttl)}
	return s.lease, true, nil
}

func (s *openAISharedHealthStoreStub) CompleteHalfOpen(_ context.Context, lease OpenAISharedHalfOpenLease, success bool, observedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.leaseHeld || lease.FencingToken != s.lease.FencingToken || lease.Owner != s.lease.Owner {
		return ErrOpenAISharedHealthLeaseLost
	}
	s.completeCalls++
	s.leaseHeld = false
	key := sharedHealthStubKey(lease.Key)
	snapshot := s.snapshots[key]
	snapshot.SchemaVersion = 1
	snapshot.Key = lease.Key
	snapshot.ObservedAt = observedAt
	snapshot.Revision++
	if success {
		snapshot.HalfOpenSuccesses++
		if snapshot.HalfOpenSuccesses >= 2 {
			snapshot.HalfOpenSuccesses = 0
			snapshot.State = OpenAISharedHealthStateHealthy
			snapshot.CooldownUntil = time.Time{}
		} else {
			snapshot.State = OpenAISharedHealthStateHalfOpen
			snapshot.CooldownUntil = observedAt
		}
		snapshot.FailureStreak = 0
	} else {
		snapshot.HalfOpenSuccesses = 0
		snapshot.State = OpenAISharedHealthStateCooldown
		snapshot.FailureStreak++
		snapshot.CooldownUntil = observedAt.Add(openAIModelTransientShortCooldown)
	}
	s.snapshots[key] = snapshot
	return nil
}

func openAIAdmissionStubKey(request OpenAISharedAdmissionRequest) string {
	return fmt.Sprintf("%d:%s:%s", request.AccountID, request.Shape, request.LeaseID)
}

func (s *openAISharedHealthStoreStub) AcquireAdmission(_ context.Context, request OpenAISharedAdmissionRequest) (OpenAISharedAdmissionDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.admissionCalls++
	if s.getErr != nil {
		return OpenAISharedAdmissionDecision{}, s.getErr
	}
	if s.admissionDecision != nil {
		return *s.admissionDecision, nil
	}
	s.admissions[openAIAdmissionStubKey(request)] = request
	return OpenAISharedAdmissionDecision{Allowed: true, Reason: "acquired", LeaseExpiresAt: request.ObservedAt.Add(90 * time.Second)}, nil
}

func (s *openAISharedHealthStoreStub) RenewAdmission(_ context.Context, request OpenAISharedAdmissionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewCalls++
	if s.getErr != nil {
		return s.getErr
	}
	if _, ok := s.admissions[openAIAdmissionStubKey(request)]; !ok {
		return ErrOpenAISharedHealthLeaseLost
	}
	return nil
}

func (s *openAISharedHealthStoreStub) ReleaseAdmission(_ context.Context, request OpenAISharedAdmissionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseCalls++
	if s.getErr != nil {
		return s.getErr
	}
	delete(s.admissions, openAIAdmissionStubKey(request))
	return nil
}

func TestOpenAIAdmissionIsPermanentlyDisabled(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.admissionDecision = &OpenAISharedAdmissionDecision{Allowed: false, Reason: "slow_session_guard"}
	cfg := &config.Config{}
	cfg.Gateway.OpenAISharedHealth = DefaultOpenAISharedHealthConfig()
	cfg.Gateway.OpenAISharedHealth.Enabled = true
	cfg.Gateway.OpenAISharedHealth.AdmissionEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	svc.SetOpenAISharedHealthStore(store)

	release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeLong)
	release()

	require.True(t, decision.Allowed)
	require.Equal(t, "disabled", decision.Reason)
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Zero(t, store.admissionCalls)
	require.Zero(t, store.releaseCalls)
	require.Empty(t, store.admissions)
}

func (s *openAISharedHealthStoreStub) RecordSlowSessionGuard(_ context.Context, accountID int64, observedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return s.getErr
	}
	s.slowGuard[accountID] = observedAt.Add(10 * time.Minute)
	return nil
}

func (s *openAISharedHealthStoreStub) HasSlowSessionGuard(_ context.Context, accountID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return false, s.getErr
	}
	return s.slowGuard[accountID].After(time.Now()), nil
}

func TestOpenAISlowSessionGuardIsPermanentlyDisabled(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{}
	svc.SetOpenAISharedHealthStore(store)

	firstToken := 30001
	tests := []struct {
		name          string
		result        *OpenAIForwardResult
		halfOpenProbe bool
		wantGuard     bool
	}{
		{
			name:   "trusted slow completed stream",
			result: &OpenAIForwardResult{Stream: true, FirstTokenMs: &firstToken},
		},
		{
			name:          "half open probe",
			result:        &OpenAIForwardResult{Stream: true, FirstTokenMs: &firstToken},
			halfOpenProbe: true,
		},
		{
			name:   "failed stream",
			result: &OpenAIForwardResult{Stream: true, FirstTokenMs: &firstToken, OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"},
		},
		{
			name:   "unknown stream without first output",
			result: &OpenAIForwardResult{Stream: true},
		},
		{
			name:   "client cancelled stream",
			result: &OpenAIForwardResult{Stream: true, FirstTokenMs: &firstToken, ClientDisconnect: true},
		},
		{
			name:   "non-stream result",
			result: &OpenAIForwardResult{FirstTokenMs: &firstToken},
		},
		{
			name: "below slow threshold",
			result: func() *OpenAIForwardResult {
				value := 29999
				return &OpenAIForwardResult{Stream: true, FirstTokenMs: &value}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.mu.Lock()
			store.slowGuard = make(map[int64]time.Time)
			store.mu.Unlock()

			svc.RecordOpenAISlowSessionGuard(153, tt.result, tt.halfOpenProbe)

			store.mu.Lock()
			_, got := store.slowGuard[153]
			store.mu.Unlock()
			require.Equal(t, tt.wantGuard, got)
		})
	}
}

func TestOpenAIAdmissionRejectDecisionIsIgnored(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.admissionDecision = &OpenAISharedAdmissionDecision{Allowed: false, Reason: "slow_session_guard"}
	svc := &OpenAIGatewayService{}
	svc.SetOpenAISharedHealthStore(store)

	release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeNormal)
	require.True(t, decision.Allowed)
	require.Equal(t, "disabled", decision.Reason)
	release()

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Zero(t, store.admissionCalls)
	require.Zero(t, store.releaseCalls)
	require.Empty(t, store.admissions)
}

func TestOpenAIAdmissionIgnoresGroupPolicy(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.admissionDecision = &OpenAISharedAdmissionDecision{Allowed: false, Reason: "slow_session_guard"}
	svc := &OpenAIGatewayService{}
	svc.SetOpenAISharedHealthStore(store)

	policies := []OpenAISchedulerGroupPolicy{
		{Mode: OpenAISchedulerGroupPolicyModePreset, Preset: OpenAISchedulerPresetPro, Values: openAISchedulerPresetValues(OpenAISchedulerPresetPro)},
		{Mode: OpenAISchedulerGroupPolicyModePreset, Preset: OpenAISchedulerPresetSpecialOffer, Values: openAISchedulerPresetValues(OpenAISchedulerPresetSpecialOffer)},
	}

	for _, policy := range policies {
		t.Run(string(policy.Preset), func(t *testing.T) {
			// Group policy must not affect the provider-native-only concurrency path.
			release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeNormal)
			require.True(t, decision.Allowed)
			require.Equal(t, "disabled", decision.Reason)
			release()
		})
	}
}

func TestOpenAIAdmissionDoesNotReadStore(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.getErr = errors.New("redis unavailable")
	svc := &OpenAIGatewayService{}
	svc.SetOpenAISharedHealthStore(store)

	release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeNormal)
	require.True(t, decision.Allowed)
	require.Equal(t, "disabled", decision.Reason)
	release()
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Zero(t, store.admissionCalls)
	require.Zero(t, store.releaseCalls)
}

func TestOpenAIAdmissionFirstSemanticOutputReleasesIdempotently(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{}
	svc.SetOpenAISharedHealthStore(store)

	release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeNormal)
	require.True(t, decision.Allowed)
	ctx := WithOpenAIFirstSemanticOutputCallback(context.Background(), release)
	notifyOpenAIFirstSemanticOutput(ctx)
	notifyOpenAIFirstSemanticOutput(ctx)
	release()

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Empty(t, store.admissions)
}

func TestOpenAIAdmissionDoesNotStartRenewal(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	cfg := &config.Config{}
	cfg.Gateway.OpenAISharedHealth = DefaultOpenAISharedHealthConfig()
	cfg.Gateway.OpenAISharedHealth.AdmissionRenewSeconds = 5
	cfg.Gateway.OpenAISharedHealth.AdmissionLeaseTTLSeconds = 30
	svc := &OpenAIGatewayService{cfg: cfg}
	svc.SetOpenAISharedHealthStore(store)

	release, decision := svc.AcquireOpenAIAdmission(153, OpenAIAdmissionShapeNormal)
	require.True(t, decision.Allowed)
	release()
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Zero(t, store.renewCalls)
	require.Zero(t, store.admissionCalls)
	require.Zero(t, store.releaseCalls)
	require.Empty(t, store.admissions)
}

func TestOpenAIAccountModelTransientSharedCooldownBlocksAnotherService(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	serviceA := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	serviceB := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	serviceA.SetOpenAISharedHealthStore(store)
	serviceB.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	for _, at := range []time.Time{now, now.Add(time.Second)} {
		serviceA.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
			EventID: "attempt-" + at.Format(time.RFC3339Nano), AccountID: 153, CanonicalModel: "gpt-5.6-sol",
			StatusCode: 503, ErrorType: "transient_upstream", SafeToReplay: true, Platform: PlatformOpenAI, Now: at,
		})
	}

	account := &Account{ID: 153, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	require.True(t, serviceB.isOpenAIAccountModelRuntimeBlockedAt(account, "gpt-5.6-sol", now.Add(2*time.Second)))
	require.False(t, serviceB.openaiModelTransient.isBlocked(account.ID, "gpt-5.6-sol", now.Add(2*time.Second)), "the second instance must be blocked by shared state, not an accidental local write")
}

func TestOpenAIAccountModelTransientSharedSnapshotFallbackExpires(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	key, err := NewOpenAISharedHealthKey(153, "gpt-5.6-sol")
	require.NoError(t, err)
	store.snapshots[sharedHealthStubKey(key)] = OpenAISharedHealthSnapshot{
		SchemaVersion: 1, Key: key, State: OpenAISharedHealthStateCooldown,
		FailureStreak: 2, CooldownUntil: now.Add(time.Minute), ObservedAt: now,
	}
	account := &Account{ID: 153, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	require.True(t, svc.isOpenAIAccountModelRuntimeBlockedAt(account, key.CanonicalModel, now))

	store.mu.Lock()
	store.getErr = errors.New("redis unavailable")
	store.mu.Unlock()
	require.True(t, svc.isOpenAIAccountModelRuntimeBlockedAt(account, key.CanonicalModel, now.Add(10*time.Second)), "a 10-second trusted snapshot must remain conservative")
	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedAt(account, key.CanonicalModel, now.Add(31*time.Second)), "a 31-second snapshot becomes unknown instead of permanently blocking")
}

func TestOpenAIAccountModelTransientRedisFailureDoesNotClearS1Veto(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.getErr = errors.New("redis unavailable")
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	account := &Account{ID: 153, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusError, Schedulable: false}

	selected := svc.recheckSelectedOpenAIAccountFromDBBeforeProfit(context.Background(), account, nil, PlatformOpenAI, "gpt-5.6-sol", false, "")
	require.Nil(t, selected)
}

func TestOpenAIAccountModelTransientSharedHalfOpenHasOneWinner(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	key, err := NewOpenAISharedHealthKey(153, "gpt-5.6-sol")
	require.NoError(t, err)
	store.snapshots[sharedHealthStubKey(key)] = OpenAISharedHealthSnapshot{
		SchemaVersion: 1, Key: key, State: OpenAISharedHealthStateCooldown,
		FailureStreak: 2, CooldownUntil: now.Add(-time.Second), ObservedAt: now.Add(-time.Second),
	}
	services := []*OpenAIGatewayService{
		{openaiModelTransient: newOpenAIAccountModelTransientState(16)},
		{openaiModelTransient: newOpenAIAccountModelTransientState(16)},
	}
	for _, svc := range services {
		svc.SetOpenAISharedHealthStore(store)
	}

	var winners int
	for _, svc := range services {
		if svc.AcquireOpenAIAccountModelHalfOpenProbe(key.AccountID, key.CanonicalModel, now) {
			winners++
		}
	}
	require.Equal(t, 1, winners)
	services[0].ReleaseOpenAIAccountModelHalfOpenProbe(key.AccountID, key.CanonicalModel, true, now)
	services[1].ReleaseOpenAIAccountModelHalfOpenProbe(key.AccountID, key.CanonicalModel, true, now)
	require.Equal(t, 1, store.completeCalls)
}

func TestOpenAIAccountModelTransientSharedSuccessResetsRemoteState(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	writer := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	reader := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	writer.SetOpenAISharedHealthStore(store)
	reader.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	for index := 0; index < 2; index++ {
		writer.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
			EventID: "failure-" + fmt.Sprint(index), AccountID: 153, CanonicalModel: "gpt-5.6-sol",
			StatusCode: 503, ErrorType: "transient_upstream", SafeToReplay: true, Platform: PlatformOpenAI, Now: now.Add(time.Duration(index) * time.Second),
		})
	}
	account := &Account{ID: 153, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	require.True(t, reader.isOpenAIAccountModelRuntimeBlockedAt(account, "gpt-5.6-sol", now.Add(2*time.Second)))

	writer.RecordOpenAIAccountModelSuccess(context.Background(), OpenAIAccountModelSuccessEvent{
		EventID: "success-1", AccountID: 153, CanonicalModel: "gpt-5.6-sol", Platform: PlatformOpenAI, Now: now.Add(3 * time.Second),
	})
	require.True(t, store.lastEvent.Success)
	require.False(t, reader.isOpenAIAccountModelRuntimeBlockedAt(account, "gpt-5.6-sol", now.Add(3*time.Second)))
}

func TestOpenAIAccountModelTransientSharedSuccessWriteFailureStillClearsLocalState(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	store.recordErr = errors.New("redis unavailable")
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	svc.openaiModelTransient.recordFailure(153, "gpt-5.6-sol", now)

	svc.RecordOpenAIAccountModelSuccess(context.Background(), OpenAIAccountModelSuccessEvent{
		EventID: "success-write-fails", AccountID: 153, CanonicalModel: "gpt-5.6-sol", Now: now.Add(time.Second),
	})

	require.False(t, svc.openaiModelTransient.isBlocked(153, "gpt-5.6-sol", now.Add(time.Second)))
	require.Equal(t, 1, store.recordCalls)
}

func TestOpenAISharedHealthRecordWritesIgnoreCanceledRequestContext(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.RecordOpenAIAccountModelSuccess(ctx, OpenAIAccountModelSuccessEvent{
		EventID: "cancelled-request-success", AccountID: 153, CanonicalModel: "gpt-5.6-sol", Now: time.Now().UTC(),
	})

	require.Equal(t, 1, store.recordCalls)
	require.NoError(t, store.recordContextErr)
}

func TestOpenAISharedHealthStaleFallbackMarksRequestDegraded(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	key, err := NewOpenAISharedHealthKey(153, "gpt-5.6-sol")
	require.NoError(t, err)
	svc.sharedHealthSnapshots[openAISharedHealthCacheKey(key)] = OpenAISharedHealthSnapshot{
		SchemaVersion: 1, Key: key, State: OpenAISharedHealthStateCooldown,
		FailureStreak: 2, CooldownUntil: now.Add(time.Minute), ObservedAt: now.Add(-31 * time.Second),
	}
	store.getErr = errors.New("redis unavailable")
	ctx, tracker := WithOpenAISharedHealthReadTracking(context.Background())
	account := &Account{ID: 153, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}

	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedAtContext(ctx, account, key.CanonicalModel, now, true))
	require.True(t, tracker.Degraded())
}

func TestOpenAISharedHealthMissingProjectionDoesNotMarkRequestDegraded(t *testing.T) {
	store := newOpenAISharedHealthStoreStub()
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	svc.SetOpenAISharedHealthStore(store)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	ctx, tracker := WithOpenAISharedHealthReadTracking(context.Background())
	account := &Account{ID: 154, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}

	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedAtContext(ctx, account, "gpt-5.6-sol", now, true))
	require.False(t, tracker.Degraded(), "a successful Redis miss is a known empty projection, not an outage")
}

func TestPreferOpenAIAccountsOutsideFailedDomainsPreservesOrderWithinBestBucket(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": "shared"}},
		{ID: 2, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": "fresh"}},
		{ID: 3, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": "fresh"}},
	}
	failed := DeriveOpenAIFailureDomains(accounts[0], 9)

	preferred := preferOpenAIAccountsOutsideFailureDomains(accounts, 9, failed)
	require.Equal(t, []int64{2, 3}, []int64{preferred[0].ID, preferred[1].ID})
}
