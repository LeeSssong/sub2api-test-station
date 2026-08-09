package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

var openAIHalfOpenRound4Now = time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

type openAIHalfOpenRound4Repo struct {
	schedulerTestOpenAIAccountRepo
}

func (r openAIHalfOpenRound4Repo) SetError(context.Context, int64, string) error { return nil }

type openAIHalfOpenRound4GroupRepo struct {
	GroupRepository
	group *Group
}

func (r openAIHalfOpenRound4GroupRepo) GetByID(context.Context, int64) (*Group, error) {
	return r.group, nil
}

func newOpenAIHalfOpenRound4Service(accounts []Account) (*OpenAIGatewayService, *defaultOpenAIAccountScheduler) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:          openAIHalfOpenRound4Repo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cfg:                  cfg,
		concurrencyService:   NewConcurrencyService(schedulerTestConcurrencyCache{}),
		openaiModelTransient: newOpenAIAccountModelTransientState(32),
	}
	return svc, &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats(), now: func() time.Time {
		return openAIHalfOpenRound4Now
	}}
}

func openAIHalfOpenRound4Account(id int64, groupID int64) Account {
	return Account{
		ID: id, Name: "half-open", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
		Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff},
	}
}

func expireOpenAIHalfOpenRound4Cooldown(t *testing.T, svc *OpenAIGatewayService, account Account, model string) {
	t.Helper()
	first := openAIHalfOpenRound4Now.Add(-12 * time.Second)
	canonicalModel := account.GetMappedModel(model)
	svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
		AccountID: account.ID, CanonicalModel: canonicalModel, StatusCode: 502, Now: first,
	})
	svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
		AccountID: account.ID, CanonicalModel: canonicalModel, StatusCode: 502, Now: first.Add(time.Second),
	})
	require.True(t, svc.getOpenAIAccountModelTransientState().isBlocked(account.ID, canonicalModel, openAIHalfOpenRound4Now))
}

func TestOpenAIHalfOpenScheduler_PrivacyIneligibleExpiredCandidateIsSkipped(t *testing.T) {
	groupID := int64(48001)
	account := openAIHalfOpenRound4Account(48011, groupID)
	account.Extra = nil
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	svc.schedulerSnapshot = &SchedulerSnapshotService{groupRepo: openAIHalfOpenRound4GroupRepo{group: &Group{
		ID: groupID, Name: "private", Platform: PlatformOpenAI, Status: StatusActive, RequirePrivacySet: true,
	}}}
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})

	require.Error(t, err)
	require.Nil(t, selection)
	snapshot := svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)
	require.Len(t, snapshot, 1)
	require.False(t, snapshot[0].HalfOpenInFlight, "an ineligible account must never acquire the probe lease")
}

func TestOpenAIHalfOpenScheduler_ModelAndCapabilityIneligibleCandidatesAreSkipped(t *testing.T) {
	groupID := int64(48002)
	tests := []struct {
		name string
		edit func(*Account)
		req  OpenAIAccountScheduleRequest
	}{
		{
			name: "model",
			edit: func(account *Account) {
				account.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-other": "gpt-other"}}
			},
			req: OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5"},
		},
		{
			name: "capability",
			edit: func(account *Account) {
				account.Extra["openai_responses_supported"] = false
			},
			req: OpenAIAccountScheduleRequest{RequestedModel: "gpt-image-2", RequiredCapability: OpenAIEndpointCapabilityResponses},
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account := openAIHalfOpenRound4Account(int64(48021+i), groupID)
			test.edit(&account)
			svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
			expireOpenAIHalfOpenRound4Cooldown(t, svc, account, test.req.RequestedModel)
			test.req.GroupID, test.req.Platform = &groupID, PlatformOpenAI

			selection, _, err := scheduler.Select(context.Background(), test.req)

			require.Error(t, err)
			require.Nil(t, selection)
			require.False(t, svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)[0].HalfOpenInFlight)
		})
	}
}

func TestOpenAIHalfOpenScheduler_ChannelIneligibleExpiredCandidateIsSkipped(t *testing.T) {
	groupID := int64(48007)
	account := openAIHalfOpenRound4Account(48071, groupID)
	account.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-disallowed"}}
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	channel := &Channel{
		ID: 4807, Status: StatusActive, GroupIDs: []int64{groupID}, RestrictModels: true,
		BillingModelSource: BillingModelSourceUpstream,
	}
	channelCache := newEmptyChannelCache()
	channelCache.loadedAt = time.Now()
	channelCache.channelByGroupID[groupID] = channel
	channelCache.groupPlatform[groupID] = PlatformOpenAI
	channelCache.pricingByGroupModel[channelModelKey{groupID: groupID, platform: PlatformOpenAI, model: "gpt-allowed"}] = &ChannelModelPricing{
		Platform: PlatformOpenAI, Models: []string{"gpt-allowed"},
	}
	svc.channelService = &ChannelService{}
	svc.channelService.cache.Store(channelCache)
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})

	require.Error(t, err)
	require.Nil(t, selection)
	require.False(t, svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)[0].HalfOpenInFlight)
}

func TestOpenAIHalfOpenScheduler_DBRecheckRejectionReleasesLease(t *testing.T) {
	groupID, otherGroupID := int64(48003), int64(48004)
	stale := openAIHalfOpenRound4Account(48031, groupID)
	latest := stale
	latest.GroupIDs = []int64{otherGroupID}
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{latest})
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&stale},
		accountsByID:     map[int64]*Account{stale.ID: &stale},
	}
	svc.cfg.RunMode = config.RunModeStandard
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: snapshotCache}
	expireOpenAIHalfOpenRound4Cooldown(t, svc, stale, "gpt-5.5")

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts))
	require.Nil(t, selection)
	snapshot := svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)
	require.Len(t, snapshot, 1)
	require.False(t, snapshot[0].HalfOpenInFlight)
}

func TestOpenAIHalfOpenScheduler_SlotFailureReleasesLease(t *testing.T) {
	groupID := int64(48008)
	account := openAIHalfOpenRound4Account(48081, groupID)
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{
		acquireResults: map[int64]bool{account.ID: false},
	})
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})

	require.Error(t, err)
	require.Nil(t, selection)
	snapshot := svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)
	require.Len(t, snapshot, 1)
	require.False(t, snapshot[0].HalfOpenInFlight)
}

func TestOpenAIHalfOpenScheduler_SelectionReleaseClearsLeaseAndIsIdempotent(t *testing.T) {
	groupID := int64(48005)
	account := openAIHalfOpenRound4Account(48051, groupID)
	rate := 0.2
	account.RateMultiplier = &rate
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID: groupID, platform: PlatformOpenAI, threshold: 0.5, pricingAt: openAIHalfOpenRound4Now,
	})

	selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.True(t, selection.HalfOpenProbe)
	require.True(t, selection.ProfitGateActive())
	require.True(t, svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)[0].HalfOpenInFlight)

	selection.ReleaseFunc()
	selection.ReleaseFunc()

	snapshot := svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)
	require.Len(t, snapshot, 1)
	require.False(t, snapshot[0].HalfOpenInFlight)
}

func TestOpenAIHalfOpenScheduler_ValidProbeOwnsSingleLeaseUntilCompleted(t *testing.T) {
	groupID := int64(48006)
	account := openAIHalfOpenRound4Account(48061, groupID)
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")
	req := OpenAIAccountScheduleRequest{GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5"}

	selection, _, err := scheduler.Select(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.True(t, selection.HalfOpenProbe)

	second, _, secondErr := scheduler.Select(context.Background(), req)
	require.Error(t, secondErr)
	require.Nil(t, second)

	selection.CompleteHalfOpenProbe(true)
	selection.ReleaseFunc()
	require.Empty(t, svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now))
}

func TestOpenAIHalfOpenHandler_FailedForwardCompletesLeaseWithoutDoubleStreak(t *testing.T) {
	groupID := int64(48009)
	account := openAIHalfOpenRound4Account(48091, groupID)
	svc, scheduler := newOpenAIHalfOpenRound4Service([]Account{account})
	expireOpenAIHalfOpenRound4Cooldown(t, svc, account, "gpt-5.5")

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.5",
	})
	require.NoError(t, err)
	require.True(t, selection.HalfOpenProbe)

	// Responses and Messages complete the probe from the actual Forward result,
	// then their normal failure recorder adds the one observed failure event.
	selection.CompleteHalfOpenProbe(false)
	selection.ReleaseFunc()
	svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
		AccountID: account.ID, CanonicalModel: "gpt-5.5", StatusCode: 502, Now: openAIHalfOpenRound4Now,
	})

	snapshot := svc.SnapshotOpenAIAccountModelRuntime(openAIHalfOpenRound4Now)
	require.Len(t, snapshot, 1)
	require.Equal(t, 3, snapshot[0].FailureStreak)
	require.False(t, snapshot[0].HalfOpenInFlight)
}
