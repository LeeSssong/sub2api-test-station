package service

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingOpenAIQualityProvider struct {
	snapshot OpenAIAccountQualitySnapshot
	calls    int
}

func (p *recordingOpenAIQualityProvider) Snapshot(context.Context) OpenAIAccountQualitySnapshot {
	p.calls++
	return p.snapshot
}

func TestOpenAIUnifiedQualityComparatorUsesDeterministicLexicographicOrder(t *testing.T) {
	candidates := []openAIUnifiedQualityCandidate{
		{account: &Account{ID: 40}, quality: OpenAIQualityBreakdown{QualityScore: 60, SuccessScore: 90, P50TTFTMS: floatPtr(100)}},
		{account: &Account{ID: 30}, quality: OpenAIQualityBreakdown{QualityScore: 80, SuccessScore: 90, P50TTFTMS: floatPtr(500)}},
		{account: &Account{ID: 20}, quality: OpenAIQualityBreakdown{QualityScore: 80, SuccessScore: 90, P50TTFTMS: floatPtr(100)}},
		{account: &Account{ID: 10}, quality: OpenAIQualityBreakdown{QualityScore: 80, SuccessScore: 95, P50TTFTMS: floatPtr(900)}},
	}

	for seed := int64(0); seed < 100; seed++ {
		shuffled := append([]openAIUnifiedQualityCandidate(nil), candidates...)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		ordered := sortOpenAIUnifiedQualityCandidates(shuffled)
		require.Equal(t, []int64{10, 20, 30, 40}, unifiedCandidateIDs(ordered), "seed=%d", seed)
	}
}

func TestOpenAIUnifiedQualityRequiresExplicitRequestOptIn(t *testing.T) {
	ctx := context.Background()
	require.False(t, OpenAIUnifiedQualitySchedulingRequested(ctx))
	require.True(t, OpenAIUnifiedQualitySchedulingRequested(WithOpenAIUnifiedQualityScheduling(ctx)))
}

func TestOpenAIUnifiedQualityComparatorKeepsNullsLastAtEachPosition(t *testing.T) {
	candidates := []openAIUnifiedQualityCandidate{
		{account: &Account{ID: 1}, quality: OpenAIQualityBreakdown{QualityScore: 70, SuccessScore: 90}},
		{account: &Account{ID: 2}, quality: OpenAIQualityBreakdown{QualityScore: 70, SuccessScore: 90, P50TTFTMS: floatPtr(2)}},
		{account: &Account{ID: 3}, quality: OpenAIQualityBreakdown{QualityScore: 70, SuccessScore: 95, P50TTFTMS: floatPtr(5)}},
	}
	ordered := sortOpenAIUnifiedQualityCandidates(candidates)
	require.Equal(t, []int64{3, 2, 1}, unifiedCandidateIDs(ordered))
}

func TestOpenAIUnifiedQualityComparatorIgnoresLegacySchedulingSignals(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	base := openAIUnifiedQualityCandidate{
		account:    &Account{ID: 10, Priority: 99},
		quality:    OpenAIQualityBreakdown{QualityScore: 80, SuccessScore: 90, P50TTFTMS: value(100)},
		effectiveU: value(0.2),
	}
	other := base
	other.account = &Account{ID: 20, Priority: 1}
	ordered := sortOpenAIUnifiedQualityCandidates([]openAIUnifiedQualityCandidate{other, base})
	require.Equal(t, []int64{10, 20}, unifiedCandidateIDs(ordered))
}

func TestOpenAIUnifiedQualityCompositePrefersFasterNearPerfectAccount(t *testing.T) {
	slowPerfect := OpenAIAccountQuality{AccountID: 1, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
		OpenAIQualityWindow1H: {AttemptCount: 20, SuccessCount: 20, SuccessRate: floatPtr(1), TTFTSampleCount: 20, TTFTP50MS: floatPtr(18000), TTFTP90MS: floatPtr(18000)},
	}}
	fastNearPerfect := OpenAIAccountQuality{AccountID: 2, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
		OpenAIQualityWindow1H: {AttemptCount: 2000, SuccessCount: 1993, SuccessRate: floatPtr(.9965), TTFTSampleCount: 20, TTFTP50MS: floatPtr(4700), TTFTP90MS: floatPtr(4700)},
	}}
	breakdowns := buildOpenAIQualityBreakdowns([]*Account{{ID: 1, Concurrency: 1}, {ID: 2, Concurrency: 1}}, map[int64]OpenAIAccountQuality{1: slowPerfect, 2: fastNearPerfect}, nil, nil)
	ordered := sortOpenAIUnifiedQualityCandidates([]openAIUnifiedQualityCandidate{
		{account: &Account{ID: 1}, quality: breakdowns[1]},
		{account: &Account{ID: 2}, quality: breakdowns[2]},
	})
	require.Equal(t, []int64{2, 1}, unifiedCandidateIDs(ordered))
}

func TestOpenAIUnifiedQualitySelectorUsesQualityOrderForOrdinaryText(t *testing.T) {
	groupID := int64(11)
	accounts := []Account{
		unifiedQualityTestAccount(10, groupID),
		unifiedQualityTestAccount(20, groupID),
		unifiedQualityTestAccount(30, groupID),
	}
	quality := &recordingOpenAIQualityProvider{snapshot: OpenAIAccountQualitySnapshot{Accounts: map[int64]OpenAIAccountQuality{
		10: {AccountID: 10, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{OpenAIQualityWindow1H: {AttemptCount: 20, SuccessCount: 18, SuccessRate: floatPtr(.9), TTFTSampleCount: 20, TTFTP50MS: floatPtr(900), TTFTP90MS: floatPtr(900)}}},
		20: {AccountID: 20, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{OpenAIQualityWindow1H: {AttemptCount: 20, SuccessCount: 18, SuccessRate: floatPtr(.9), TTFTSampleCount: 20, TTFTP50MS: floatPtr(100), TTFTP90MS: floatPtr(100)}}},
		30: {AccountID: 30, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{OpenAIQualityWindow1H: {AttemptCount: 20, SuccessCount: 16, SuccessRate: floatPtr(.8), TTFTSampleCount: 20, TTFTP50MS: floatPtr(1), TTFTP90MS: floatPtr(1)}}},
	}}}
	repo := &schedulerTestOpenAIAccountRepo{accounts: accounts}
	service := &OpenAIGatewayService{accountRepo: repo, openaiQuality: quality}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}

	selection, decision, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.4",
		RequiredTransport: OpenAIUpstreamTransportAny,
		unifiedQuality:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(20), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerUnifiedQuality, decision.Layer)
	require.Equal(t, []int64{20, 10, 30}, decision.CandidateAccountIDs)
	selection.ReleaseFunc()
}

func TestOpenAIUnifiedQualitySelectorBypassesQualityProviderForImages(t *testing.T) {
	groupID := int64(11)
	quality := &recordingOpenAIQualityProvider{}
	repo := &schedulerTestOpenAIAccountRepo{accounts: []Account{unifiedQualityTestAccount(10, groupID)}}
	service := &OpenAIGatewayService{accountRepo: repo, openaiQuality: quality}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		GroupID:                 &groupID,
		Platform:                PlatformOpenAI,
		RequestedModel:          "gpt-image-2",
		RequiredTransport:       OpenAIUpstreamTransportAny,
		RequiredImageCapability: OpenAIImagesCapabilityBasic,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Zero(t, quality.calls)
	selection.ReleaseFunc()
}

func TestOpenAIUnifiedQualityProfitPartitionPrefersQualifiedCandidates(t *testing.T) {
	threshold := 0.5
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{threshold: threshold})
	qualified := openAIUnifiedQualityCandidate{account: &Account{ID: 1}, effectiveU: floatPtr(0.2), effectiveCostStatus: EffectiveCostStatusReady}
	over := openAIUnifiedQualityCandidate{account: &Account{ID: 2}, effectiveU: floatPtr(0.8), effectiveCostStatus: EffectiveCostStatusReady}
	unknown := openAIUnifiedQualityCandidate{account: &Account{ID: 3}, effectiveCostStatus: EffectiveCostStatusUnknown}

	partition := partitionOpenAIUnifiedQualityCandidates(ctx, []openAIUnifiedQualityCandidate{over, unknown, qualified})
	require.False(t, partition.bypass)
	require.Equal(t, []int64{1}, unifiedCandidateIDs(partition.candidates))
}

func TestOpenAIUnifiedQualityProfitPartitionFallsBackWhenNoCandidateIsQualified(t *testing.T) {
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{threshold: 0.5})
	over := openAIUnifiedQualityCandidate{account: &Account{ID: 2}, effectiveU: floatPtr(0.8), effectiveCostStatus: EffectiveCostStatusReady}
	unknown := openAIUnifiedQualityCandidate{account: &Account{ID: 3}, effectiveCostStatus: EffectiveCostStatusUnknown}

	margin := partitionOpenAIUnifiedQualityCandidates(ctx, []openAIUnifiedQualityCandidate{over})
	require.True(t, margin.bypass)
	require.Equal(t, "margin_below", margin.bypassReason)
	allUnknown := partitionOpenAIUnifiedQualityCandidates(ctx, []openAIUnifiedQualityCandidate{unknown})
	require.True(t, allUnknown.bypass)
	require.Equal(t, "unknown_u", allUnknown.bypassReason)
}

func TestOpenAIUnifiedQualitySelectorUsesNativeProfitPartitionAndFallback(t *testing.T) {
	groupID := int64(11)
	qualified := unifiedQualityTestAccount(1, groupID)
	qualified.RateMultiplier = floatPtr(0.2)
	over := unifiedQualityTestAccount(2, groupID)
	over.RateMultiplier = floatPtr(0.8)
	repo := &schedulerTestOpenAIAccountRepo{accounts: []Account{over, qualified}}
	quality := &recordingOpenAIQualityProvider{snapshot: OpenAIAccountQualitySnapshot{Accounts: map[int64]OpenAIAccountQuality{
		1: {AccountID: 1, SuccessRate: floatPtr(0.8)},
		2: {AccountID: 2, SuccessRate: floatPtr(0.9)},
	}}}
	service := &OpenAIGatewayService{accountRepo: repo, openaiQuality: quality}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{threshold: 0.5})

	selection, decision, err := scheduler.selectByUnifiedQuality(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.4", RequiredTransport: OpenAIUpstreamTransportAny,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), selection.Account.ID)
	require.Equal(t, "native", decision.ProfitMode)
	require.False(t, decision.ProfitBypass)
	selection.ReleaseFunc()

	// When every known U is over the native threshold, availability wins and
	// the selected result carries a bypass marker for the handler post-slot check.
	qualified.RateMultiplier = floatPtr(0.7)
	repo.accounts = []Account{qualified}
	selection, decision, err = scheduler.selectByUnifiedQuality(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.4", RequiredTransport: OpenAIUpstreamTransportAny,
	})
	require.NoError(t, err)
	require.True(t, decision.ProfitBypass)
	require.Equal(t, "margin_below", decision.ProfitBypassReason)
	postSlotCtx := ContextWithSelectionProfitGate(ctx, selection)
	vetoed, _ := OpenAIProfitControlVeto(postSlotCtx, selection.Account)
	require.False(t, vetoed)
	selection.ReleaseFunc()
}

type changingUnifiedQualityAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *changingUnifiedQualityAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform && openAIStickyAccountMatchesGroup(&account, &groupID) {
			result = append(result, account)
		}
	}
	return result, nil
}

func (r *changingUnifiedQualityAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for _, account := range r.accounts {
		if account.ID != id {
			continue
		}
		if id == 1 {
			account.RateMultiplier = floatPtr(0.8)
		}
		return &account, nil
	}
	return nil, ErrAccountNotFound
}

func TestOpenAIUnifiedQualitySelectorRechecksLiveCostAfterSlot(t *testing.T) {
	groupID := int64(11)
	first := unifiedQualityTestAccount(1, groupID)
	first.RateMultiplier = floatPtr(0.2)
	second := unifiedQualityTestAccount(2, groupID)
	second.RateMultiplier = floatPtr(0.3)
	repo := &changingUnifiedQualityAccountRepo{accounts: []Account{first, second}}
	quality := &recordingOpenAIQualityProvider{snapshot: OpenAIAccountQualitySnapshot{Accounts: map[int64]OpenAIAccountQuality{
		1: {AccountID: 1, SuccessRate: floatPtr(0.9)},
		2: {AccountID: 2, SuccessRate: floatPtr(0.8)},
	}}}
	service := &OpenAIGatewayService{accountRepo: repo, openaiQuality: quality}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{threshold: 0.5})

	selection, _, err := scheduler.selectByUnifiedQuality(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5.4", RequiredTransport: OpenAIUpstreamTransportAny,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(2), selection.Account.ID)
	selection.ReleaseFunc()
}

func unifiedQualityTestAccount(id, groupID int64) Account {
	rate := 0.1
	return Account{ID: id, Name: "quality", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, RateMultiplier: &rate, GroupIDs: []int64{groupID}, CreatedAt: time.Now()}
}
