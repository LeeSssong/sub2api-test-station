package service

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOpenAISchedulerLegacyGroupPolicyKeepsQualityGateDisabled(t *testing.T) {
	policies, err := parseOpenAISchedulerGroupPolicies(`{"7":{"mode":"weighted_override","top_k":2}}`)
	if err != nil {
		t.Fatal(err)
	}
	policy := normalizeOpenAISchedulerGroupPoliciesForRead(policies)[7]
	if policy.QualityGate != nil || policy.SessionEscape != nil {
		t.Fatalf("legacy policy unexpectedly enabled new controls: %#v", policy)
	}
}

func TestOpenAIQualityGateRuntimeIsolatedByGroup(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	for i := 0; i < 4; i++ {
		stats.reportForGroup(11, 42, false, nil)
	}
	policy := defaultOpenAIQualityGatePolicy()
	policy.Enabled = true
	policy.MinSamples = 3
	policy.EnterConsecutive = 1

	bad, _ := stats.qualityGateEvidence(11, 42)
	otherGroup, _ := stats.qualityGateEvidence(12, 42)
	if !evaluateOpenAIQualityGate(policy, bad).Bad {
		t.Fatal("group 11 evidence should be bad")
	}
	if evaluateOpenAIQualityGate(policy, otherGroup).Known {
		t.Fatal("group 12 must not inherit group 11 runtime evidence")
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
