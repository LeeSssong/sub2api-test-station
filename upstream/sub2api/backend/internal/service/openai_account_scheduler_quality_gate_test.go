package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestOpenAISchedulerLegacyGroupPolicyKeepsQualityGateOverrideUnset(t *testing.T) {
	policies, err := parseOpenAISchedulerGroupPolicies(`{"7":{"mode":"weighted_override","top_k":2}}`)
	if err != nil {
		t.Fatal(err)
	}
	policy := normalizeOpenAISchedulerGroupPoliciesForRead(policies)[7]
	if policy.QualityGate != nil || policy.SessionEscape != nil {
		t.Fatalf("legacy policy unexpectedly enabled new controls: %#v", policy)
	}
}

func TestOpenAISchedulerUnconfiguredGroupUsesDefaultQualityGate(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey: "true",
	}}
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, &config.Config{})},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	policy, ok := scheduler.qualityGatePolicyForGroup(context.Background(), 77)
	if !ok || !policy.Enabled || policy.MinSamples != 5 || policy.ErrorRateThreshold != 0.5 || policy.TTFTThresholdMs != 15000 {
		t.Fatalf("unconfigured group must use enabled defaults: policy=%#v ok=%v", policy, ok)
	}
}

func TestOpenAISchedulerExplicitQualityGateDisableRemainsDisabled(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:               "true",
		SettingKeyOpenAIAdvancedSchedulerGroupOverrides: `{"77":{"quality_gate":{"enabled":false,"min_samples":5,"error_rate_threshold":0.5,"ttft_threshold_ms":15000,"enter_consecutive":2,"recover_consecutive":2,"cooldown_seconds":30}}}`,
	}}
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, &config.Config{})},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	if _, ok := scheduler.qualityGatePolicyForGroup(context.Background(), 77); ok {
		t.Fatal("explicit quality_gate.enabled=false must remain disabled")
	}
}

func TestOpenAIQualityGateRuntimeIsolatedByGroup(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	scheduler := &defaultOpenAIAccountScheduler{stats: stats}
	for i := 0; i < 4; i++ {
		scheduler.ReportResultForGroup(11, 42, false, nil)
	}
	scheduler.ReportResultForGroup(12, 42, true, nil)
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 3
	policy.EnterConsecutive = 1

	bad, _ := stats.qualityGateEvidence(11, 42)
	otherGroup, _ := stats.qualityGateEvidence(12, 42)
	if !evaluateOpenAIQualityGate(policy, bad).Bad {
		t.Fatal("group 11 evidence should be bad")
	}
	if otherGroup.SampleCount != 1 || otherGroup.ErrorRate != 0 {
		t.Fatalf("group 12 must retain its own report: %#v", otherGroup)
	}
	if evaluateOpenAIQualityGate(policy, otherGroup).Bad {
		t.Fatal("group 12 must not inherit group 11 bad evidence")
	}
}

func TestOpenAIGatewayServiceGroupResultReportingKeepsRuntimeEvidenceIsolated(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

	cfg := &config.Config{}
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{openAIAdvancedSchedulerSettingKey: "true"}}
	service := &OpenAIGatewayService{
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(settings, cfg)},
		openaiAccountStats: newOpenAIAccountRuntimeStats(),
	}
	for range 4 {
		service.ReportOpenAIAccountScheduleResultForGroup(11, 42, "gpt-5.5", false, nil)
		service.ReportOpenAIAccountScheduleResultForGroup(12, 42, "gpt-5.5", true, nil)
	}

	group11, ok := service.openaiAccountStats.qualityGateEvidence(11, 42)
	if !ok || group11.SampleCount != 4 || group11.ErrorRate <= 0.5 {
		t.Fatalf("group 11 evidence = %#v, ok=%v", group11, ok)
	}
	group12, ok := service.openaiAccountStats.qualityGateEvidence(12, 42)
	if !ok || group12.SampleCount != 4 || group12.ErrorRate >= 0.1 || group12.ErrorRate >= group11.ErrorRate {
		t.Fatalf("group 12 evidence = %#v, ok=%v", group12, ok)
	}
}

func TestOpenAIQualityGateStateAdvancesOncePerReportedObservation(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 3
	policy.ErrorRateThreshold = 0.5
	policy.EnterConsecutive = 2
	policy.RecoverConsecutive = 2
	for range 6 {
		stats.reportForGroup(11, 42, false, nil)
	}
	now := time.Now().UTC()
	state, evaluation := stats.qualityGateState(11, 42, policy, now, true)
	if !evaluation.Known || !evaluation.Bad || state.BadStreak != 1 || state.Blocked {
		t.Fatalf("first confirmed bad observation = state %#v, evaluation %#v", state, evaluation)
	}

	state, _ = stats.qualityGateState(11, 42, policy, now.Add(time.Second), true)
	if state.BadStreak != 1 || state.Blocked {
		t.Fatalf("repeated evaluation advanced unchanged evidence: %#v", state)
	}

	stats.reportForGroup(11, 42, false, nil)
	state, _ = stats.qualityGateState(11, 42, policy, now.Add(2*time.Second), true)
	if !state.Blocked || state.BadStreak != 2 {
		t.Fatalf("second bad observation did not enter gate: %#v", state)
	}
	state, _ = stats.qualityGateState(11, 42, policy, now.Add(3*time.Second), true)
	if state.BadStreak != 2 {
		t.Fatalf("repeated blocked evaluation changed streak: %#v", state)
	}

	for range 4 {
		stats.reportForGroup(11, 42, true, nil)
	}
	state, _ = stats.qualityGateState(11, 42, policy, now.Add(4*time.Second), true)
	if !state.Blocked || state.GoodStreak != 1 {
		t.Fatalf("first recovery observation did not advance once: %#v", state)
	}

	state, _ = stats.qualityGateState(11, 42, policy, now.Add(5*time.Second), true)
	if !state.Blocked || state.GoodStreak != 1 {
		t.Fatalf("repeated recovery evaluation changed streak: %#v", state)
	}

	stats.reportForGroup(11, 42, true, nil)
	state, _ = stats.qualityGateState(11, 42, policy, now.Add(6*time.Second), true)
	if state.Blocked || state.GoodStreak != 0 {
		t.Fatalf("second good recovery observation did not clear gate: %#v", state)
	}
	state, _ = stats.qualityGateState(11, 42, policy, now.Add(10*time.Second), true)
	if state.Blocked || state.GoodStreak != 0 {
		t.Fatalf("repeated cleared-state evaluation changed state: %#v", state)
	}

}

func TestOpenAIQualityGateUsesTheSharedFusedEvidenceProjection(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	evidence := fuseAccountMonitorQualityEvidence(
		AccountMonitorWindowAggregate{RequestCount: 5, SuccessCount: 1, LastObservedAt: &observedAt},
		AccountMonitorAggregate{},
		AccountMonitorLatest{},
		AccountMonitorSettings{IntervalSeconds: 300},
		now,
	)
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 5
	projected := openAIQualityGateEvidenceFromAccountMonitorEvidence(evidence)
	result := evaluateOpenAIQualityGate(policy, projected)
	if !result.Known || !result.Bad || projected.SampleCount != evidence.SampleCount || projected.ErrorRate != 0.8 {
		t.Fatalf("shared fused gate projection = %#v, evaluation = %#v, evidence = %#v", projected, result, evidence)
	}
}

func TestOpenAIQualityGateLegacyGroupNeutralReportRemainsIsolatedCompatibilityPath(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	stats.report(42, true, nil)
	if _, ok := stats.qualityGateEvidence(0, 42); !ok {
		t.Fatal("legacy group-neutral runtime report should remain readable for group-neutral callers")
	}
	if _, ok := stats.qualityGateEvidence(11, 42); ok {
		t.Fatal("concrete group must not inherit group-neutral runtime evidence")
	}
}

func TestOpenAIQualityGateRuntimeEvidenceExpiresFromLastObservation(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	stats.reportForGroup(11, 42, false, nil)
	stat := stats.loadOrCreateForGroup(11, 42)
	stat.lastReportAt.Store(time.Now().Add(-20 * time.Minute).UnixNano())
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 1
	policy.EnterConsecutive = 1

	evidence, ok := stats.qualityGateEvidence(11, 42)
	if !ok {
		t.Fatal("runtime evidence should be readable")
	}
	if evaluation := evaluateOpenAIQualityGate(policy, evidence); evaluation.Known || evaluation.Bad {
		t.Fatalf("stale runtime evidence must be unknown and fail open: evidence=%#v evaluation=%#v", evidence, evaluation)
	}
}

func TestOpenAIQualityGateInsufficientSamplesFailsOpen(t *testing.T) {
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 5
	result := evaluateOpenAIQualityGate(policy, openAIQualityGateEvidence{
		SampleCount: 4,
		ErrorRate:   1,
	})
	if result.Bad || result.Known {
		t.Fatalf("insufficient evidence must fail open: %#v", result)
	}
}

func TestOpenAIQualityGateBlocksConfirmedBadEvidence(t *testing.T) {
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 3
	policy.EnterConsecutive = 1
	result := evaluateOpenAIQualityGate(policy, openAIQualityGateEvidence{
		SampleCount: 3,
		ErrorRate:   0.8,
	})
	if !result.Bad || !result.Known {
		t.Fatalf("bad evidence must be confirmed: %#v", result)
	}
	state := openAIQualityGateState{}
	state = advanceOpenAIQualityGateState(policy, state, result, time.Unix(100, 0))
	if !state.Blocked {
		t.Fatalf("bad evidence did not enter blocked state: %#v", state)
	}
}

func TestOpenAIQualityGateUsesHysteresisForRecovery(t *testing.T) {
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 1
	policy.EnterConsecutive = 2
	policy.RecoverConsecutive = 2
	bad := evaluateOpenAIQualityGate(policy, openAIQualityGateEvidence{SampleCount: 1, ErrorRate: 1})
	good := evaluateOpenAIQualityGate(policy, openAIQualityGateEvidence{SampleCount: 1, ErrorRate: 0})
	state := openAIQualityGateState{}
	now := time.Unix(100, 0)
	state = advanceOpenAIQualityGateState(policy, state, bad, now)
	if state.Blocked {
		t.Fatal("gate entered before enter hysteresis")
	}
	state = advanceOpenAIQualityGateState(policy, state, bad, now.Add(time.Second))
	if !state.Blocked {
		t.Fatal("gate did not enter after enter hysteresis")
	}
	state = advanceOpenAIQualityGateState(policy, state, good, now.Add(2*time.Second))
	if !state.Blocked {
		t.Fatal("gate recovered before recover hysteresis")
	}
	state = advanceOpenAIQualityGateState(policy, state, good, now.Add(3*time.Second))
	if state.Blocked {
		t.Fatal("gate did not recover after recover hysteresis")
	}
}

func TestOpenAIQualityGateSoftFallbackIsDeterministicWhenAllBlocked(t *testing.T) {
	accounts := []*Account{{ID: 9}, {ID: 3}, {ID: 7}}
	blocked := map[int64]struct{}{9: {}, 3: {}, 7: {}}
	selected, fallback := selectOpenAIQualityGateFallback(accounts, blocked)
	if selected == nil || selected.ID != 3 || !fallback {
		t.Fatalf("fallback = %#v, %v; want lowest account ID and explicit fallback", selected, fallback)
	}
}

func TestOpenAISessionEscapeIsGroupIsolatedAndExpires(t *testing.T) {
	policy := defaultOpenAISessionEscapePolicy()
	policy.Enabled = true
	policy.MinSamples = 1
	policy.EnterConsecutive = 1
	policy.RecoverConsecutive = 1
	policy.TTLSeconds = 10
	bad := openAIQualityGateEvidence{SampleCount: 1, ErrorRate: 1}
	good := openAIQualityGateEvidence{SampleCount: 1, ErrorRate: 0}
	state := openAIQualityGateState{}
	now := time.Unix(100, 0)
	state = advanceOpenAIQualityGateState(policy.asQualityGatePolicy(), state, evaluateOpenAIQualityGate(policy.asQualityGatePolicy(), bad), now)
	if !shouldEscapeOpenAISession(policy, state, now) {
		t.Fatal("bad sticky choice should escape")
	}
	if shouldEscapeOpenAISession(policy, openAIQualityGateState{}, now) {
		t.Fatal("another group's state must not escape")
	}
	state = advanceOpenAIQualityGateState(policy.asQualityGatePolicy(), state, evaluateOpenAIQualityGate(policy.asQualityGatePolicy(), good), now.Add(1*time.Second))
	if shouldEscapeOpenAISession(policy, state, now.Add(1*time.Second)) {
		t.Fatal("recovered evidence should restore sticky behavior")
	}
	state.Blocked = true
	state.BlockedUntil = now.Add(10 * time.Second)
	if shouldEscapeOpenAISession(policy, state, now.Add(11*time.Second)) {
		t.Fatal("expired escape TTL should restore sticky behavior")
	}
}

func TestOpenAISchedulerQualityGatePolicyJSONRoundTrip(t *testing.T) {
	policy := OpenAISchedulerGroupPolicy{
		QualityGate:   &OpenAISchedulerQualityGatePolicy{Enabled: true, MinSamples: 4, ErrorRateThreshold: .4},
		SessionEscape: &OpenAISchedulerSessionEscapePolicy{Enabled: true, TTLSeconds: 12},
	}
	blob, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var decoded OpenAISchedulerGroupPolicy
	if err := json.Unmarshal(blob, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.QualityGate == nil || decoded.SessionEscape == nil || decoded.QualityGate.MinSamples != 4 || decoded.SessionEscape.TTLSeconds != 12 {
		t.Fatalf("policy JSON round trip lost fields: %s / %#v", blob, decoded)
	}
}

func TestOpenAIAccountCandidateComparatorPreservesExplicitZeroGroupPriority(t *testing.T) {
	better := openAIAccountCandidateScore{
		account:     &Account{ID: 1, Priority: 9},
		loadInfo:    &AccountLoadInfo{},
		priority:    0,
		prioritySet: true,
	}
	worse := openAIAccountCandidateScore{
		account:     &Account{ID: 2, Priority: 1},
		loadInfo:    &AccountLoadInfo{},
		priority:    5,
		prioritySet: true,
	}
	if !isOpenAIAccountCandidateBetter(better, worse) {
		t.Fatal("explicit group priority 0 must remain higher than priority 5")
	}
}
