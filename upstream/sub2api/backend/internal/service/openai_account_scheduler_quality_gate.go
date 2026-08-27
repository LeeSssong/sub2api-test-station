package service

import (
	"context"
	"time"
)

const (
	openAIQualityGateDefaultMinSamples         = 5
	openAIQualityGateDefaultErrorRate          = 0.5
	openAIQualityGateDefaultTTFTMs             = 15000
	openAIQualityGateDefaultEnterConsecutive   = 2
	openAIQualityGateDefaultRecoverConsecutive = 2
	openAIQualityGateDefaultCooldownSeconds    = 30
	openAISessionEscapeDefaultTTLSeconds       = 60
)

type openAIQualityGateEvidence struct {
	SampleCount int
	ErrorRate   float64
	TTFTMs      float64
	HasTTFT     bool
	ReadError   bool
	Fused       *AccountMonitorQualityEvidence
}

type openAIQualityGateEvaluation struct {
	Known bool
	Bad   bool
}

type openAIQualityGateState struct {
	BlockedUntil time.Time
	BadStreak    int
	GoodStreak   int
	Blocked      bool
}

func defaultOpenAIQualityGatePolicy() OpenAISchedulerQualityGatePolicy {
	return OpenAISchedulerQualityGatePolicy{
		MinSamples:         openAIQualityGateDefaultMinSamples,
		ErrorRateThreshold: openAIQualityGateDefaultErrorRate,
		TTFTThresholdMs:    openAIQualityGateDefaultTTFTMs,
		EnterConsecutive:   openAIQualityGateDefaultEnterConsecutive,
		RecoverConsecutive: openAIQualityGateDefaultRecoverConsecutive,
		CooldownSeconds:    openAIQualityGateDefaultCooldownSeconds,
	}
}

func defaultOpenAISessionEscapePolicy() OpenAISchedulerSessionEscapePolicy {
	gate := defaultOpenAIQualityGatePolicy()
	return OpenAISchedulerSessionEscapePolicy{
		MinSamples:         gate.MinSamples,
		ErrorRateThreshold: gate.ErrorRateThreshold,
		TTFTThresholdMs:    gate.TTFTThresholdMs,
		EnterConsecutive:   gate.EnterConsecutive,
		RecoverConsecutive: gate.RecoverConsecutive,
		TTLSeconds:         openAISessionEscapeDefaultTTLSeconds,
	}
}

func (p OpenAISchedulerSessionEscapePolicy) asQualityGatePolicy() OpenAISchedulerQualityGatePolicy {
	return OpenAISchedulerQualityGatePolicy{
		Enabled:            p.Enabled,
		MinSamples:         p.MinSamples,
		ErrorRateThreshold: p.ErrorRateThreshold,
		TTFTThresholdMs:    p.TTFTThresholdMs,
		EnterConsecutive:   p.EnterConsecutive,
		RecoverConsecutive: p.RecoverConsecutive,
		CooldownSeconds:    p.TTLSeconds,
	}
}

func evaluateOpenAIQualityGate(policy OpenAISchedulerQualityGatePolicy, evidence openAIQualityGateEvidence) openAIQualityGateEvaluation {
	if evidence.Fused != nil {
		evidence = openAIQualityGateEvidenceFromAccountMonitorEvidence(*evidence.Fused)
	}
	if !policy.Enabled || evidence.ReadError || evidence.SampleCount < policy.MinSamples {
		return openAIQualityGateEvaluation{}
	}
	bad := evidence.ErrorRate >= policy.ErrorRateThreshold
	if evidence.HasTTFT && evidence.TTFTMs >= float64(policy.TTFTThresholdMs) {
		bad = true
	}
	return openAIQualityGateEvaluation{Known: true, Bad: bad}
}

func openAIQualityGateEvidenceFromAccountMonitorEvidence(evidence AccountMonitorQualityEvidence) openAIQualityGateEvidence {
	if !evidence.Known {
		return openAIQualityGateEvidence{ReadError: evidence.UnknownReason == "read_error", Fused: &evidence}
	}
	result := openAIQualityGateEvidence{
		SampleCount: evidence.SampleCount,
		ErrorRate:   1 - evidence.SuccessRate,
		Fused:       &evidence,
	}
	if evidence.TTFTP50MS != nil {
		result.TTFTMs = *evidence.TTFTP50MS
		result.HasTTFT = true
	}
	return result
}

func advanceOpenAIQualityGateState(policy OpenAISchedulerQualityGatePolicy, state openAIQualityGateState, evaluation openAIQualityGateEvaluation, now time.Time) openAIQualityGateState {
	if !policy.Enabled || !evaluation.Known {
		return state
	}
	if state.Blocked && !state.BlockedUntil.IsZero() && !now.Before(state.BlockedUntil) {
		state.Blocked = false
		state.BlockedUntil = time.Time{}
		state.BadStreak = 0
		state.GoodStreak = 0
	}
	if evaluation.Bad {
		state.BadStreak++
		state.GoodStreak = 0
		if state.BadStreak >= policy.EnterConsecutive {
			state.Blocked = true
			state.BlockedUntil = now.Add(time.Duration(policy.CooldownSeconds) * time.Second)
		}
		return state
	}
	state.BadStreak = 0
	if !state.Blocked {
		state.GoodStreak = 0
		return state
	}
	state.GoodStreak++
	if state.GoodStreak >= policy.RecoverConsecutive {
		state.Blocked = false
		state.BlockedUntil = time.Time{}
		state.GoodStreak = 0
	}
	return state
}

func shouldEscapeOpenAISession(policy OpenAISchedulerSessionEscapePolicy, state openAIQualityGateState, now time.Time) bool {
	return policy.Enabled && state.Blocked && (state.BlockedUntil.IsZero() || now.Before(state.BlockedUntil))
}

func selectOpenAIQualityGateFallback(accounts []*Account, blocked map[int64]struct{}) (*Account, bool) {
	var selected *Account
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if _, ok := blocked[account.ID]; !ok {
			continue
		}
		if selected == nil || account.ID < selected.ID {
			selected = account
		}
	}
	return selected, selected != nil
}

func normalizeOpenAISchedulerQualityGateForRead(policy *OpenAISchedulerQualityGatePolicy) *OpenAISchedulerQualityGatePolicy {
	if policy == nil {
		return nil
	}
	normalized := *policy
	defaults := defaultOpenAIQualityGatePolicy()
	if normalized.MinSamples <= 0 {
		normalized.MinSamples = defaults.MinSamples
	} else if normalized.MinSamples > 10000 {
		normalized.MinSamples = 10000
	}
	if normalized.ErrorRateThreshold <= 0 || normalized.ErrorRateThreshold > 1 {
		normalized.ErrorRateThreshold = defaults.ErrorRateThreshold
	}
	if normalized.TTFTThresholdMs <= 0 || normalized.TTFTThresholdMs > 120000 {
		normalized.TTFTThresholdMs = defaults.TTFTThresholdMs
	}
	if normalized.EnterConsecutive <= 0 || normalized.EnterConsecutive > 10 {
		normalized.EnterConsecutive = defaults.EnterConsecutive
	}
	if normalized.RecoverConsecutive <= 0 || normalized.RecoverConsecutive > 10 {
		normalized.RecoverConsecutive = defaults.RecoverConsecutive
	}
	if normalized.CooldownSeconds <= 0 || normalized.CooldownSeconds > 3600 {
		normalized.CooldownSeconds = defaults.CooldownSeconds
	}
	return &normalized
}

func normalizeOpenAISchedulerSessionEscapeForRead(policy *OpenAISchedulerSessionEscapePolicy) *OpenAISchedulerSessionEscapePolicy {
	if policy == nil {
		return nil
	}
	normalized := *policy
	defaults := defaultOpenAISessionEscapePolicy()
	if normalized.MinSamples <= 0 {
		normalized.MinSamples = defaults.MinSamples
	} else if normalized.MinSamples > 10000 {
		normalized.MinSamples = 10000
	}
	if normalized.ErrorRateThreshold <= 0 || normalized.ErrorRateThreshold > 1 {
		normalized.ErrorRateThreshold = defaults.ErrorRateThreshold
	}
	if normalized.TTFTThresholdMs <= 0 || normalized.TTFTThresholdMs > 120000 {
		normalized.TTFTThresholdMs = defaults.TTFTThresholdMs
	}
	if normalized.EnterConsecutive <= 0 || normalized.EnterConsecutive > 10 {
		normalized.EnterConsecutive = defaults.EnterConsecutive
	}
	if normalized.RecoverConsecutive <= 0 || normalized.RecoverConsecutive > 10 {
		normalized.RecoverConsecutive = defaults.RecoverConsecutive
	}
	if normalized.TTLSeconds <= 0 || normalized.TTLSeconds > 3600 {
		normalized.TTLSeconds = defaults.TTLSeconds
	}
	return &normalized
}

func validateOpenAISchedulerQualityGatePolicy(policy OpenAISchedulerQualityGatePolicy) bool {
	return policy.MinSamples >= 1 && policy.MinSamples <= 10000 &&
		policy.ErrorRateThreshold >= 0 && policy.ErrorRateThreshold <= 1 &&
		policy.TTFTThresholdMs >= 1 && policy.TTFTThresholdMs <= 120000 &&
		policy.EnterConsecutive >= 1 && policy.EnterConsecutive <= 10 &&
		policy.RecoverConsecutive >= 1 && policy.RecoverConsecutive <= 10 &&
		policy.CooldownSeconds >= 1 && policy.CooldownSeconds <= 3600
}

func validateOpenAISchedulerSessionEscapePolicy(policy OpenAISchedulerSessionEscapePolicy) bool {
	return policy.MinSamples >= 1 && policy.MinSamples <= 10000 &&
		policy.ErrorRateThreshold >= 0 && policy.ErrorRateThreshold <= 1 &&
		policy.TTFTThresholdMs >= 1 && policy.TTFTThresholdMs <= 120000 &&
		policy.EnterConsecutive >= 1 && policy.EnterConsecutive <= 10 &&
		policy.RecoverConsecutive >= 1 && policy.RecoverConsecutive <= 10 &&
		policy.TTLSeconds >= 1 && policy.TTLSeconds <= 3600
}

func (s *defaultOpenAIAccountScheduler) qualityGatePolicyForGroup(ctx context.Context, groupID int64) (OpenAISchedulerQualityGatePolicy, bool) {
	if s == nil || s.service == nil {
		return OpenAISchedulerQualityGatePolicy{}, false
	}
	runtime := s.service.openAIAdvancedSchedulerRuntimeSettings(ctx)
	policy, ok := runtime.groupPolicies[groupID]
	if !ok || policy.QualityGate == nil || !policy.QualityGate.Enabled {
		return OpenAISchedulerQualityGatePolicy{}, false
	}
	return *policy.QualityGate, true
}

func (s *defaultOpenAIAccountScheduler) qualityGateBlockedForGroup(ctx context.Context, groupID, accountID int64, policy OpenAISchedulerQualityGatePolicy, now time.Time, advance bool) bool {
	if s == nil || s.stats == nil || !policy.Enabled {
		return false
	}
	state, _ := s.stats.qualityGateState(groupID, accountID, policy, now, advance)
	return state.Blocked && (state.BlockedUntil.IsZero() || now.Before(state.BlockedUntil))
}

func (s *defaultOpenAIAccountScheduler) sessionEscapePolicyForGroup(ctx context.Context, groupID int64) (OpenAISchedulerSessionEscapePolicy, bool) {
	if s == nil || s.service == nil {
		return OpenAISchedulerSessionEscapePolicy{}, false
	}
	runtime := s.service.openAIAdvancedSchedulerRuntimeSettings(ctx)
	policy, ok := runtime.groupPolicies[groupID]
	if !ok || policy.SessionEscape == nil || !policy.SessionEscape.Enabled {
		return OpenAISchedulerSessionEscapePolicy{}, false
	}
	return *policy.SessionEscape, true
}

func (s *defaultOpenAIAccountScheduler) shouldEscapeStickyAccountForGroup(ctx context.Context, groupID, accountID int64, now time.Time) (string, float64, float64, bool) {
	if s == nil || s.stats == nil || accountID <= 0 {
		return "", 0, 0, false
	}
	if gate, ok := s.qualityGatePolicyForGroup(ctx, groupID); ok {
		evidence, _ := s.stats.qualityGateEvidence(groupID, accountID)
		state, evaluation := s.stats.qualityGateState(groupID, accountID, gate, now, true)
		if state.Blocked && (state.BlockedUntil.IsZero() || now.Before(state.BlockedUntil)) {
			return "quality_gate", evidence.ErrorRate, evidence.TTFTMs, true
		}
		if evaluation.Known {
			return "", evidence.ErrorRate, evidence.TTFTMs, false
		}
	}
	if policy, ok := s.sessionEscapePolicyForGroup(ctx, groupID); ok {
		evidence, _ := s.stats.qualityGateEvidence(groupID, accountID)
		evaluation := evaluateOpenAIQualityGate(policy.asQualityGatePolicy(), evidence)
		state, _ := s.stats.qualityGateState(groupID, accountID, policy.asQualityGatePolicy(), now, true)
		if shouldEscapeOpenAISession(policy, state, now) {
			reason := "quality_gate"
			if evaluation.Bad {
				reason = "quality_gate_session_escape"
			}
			return reason, evidence.ErrorRate, evidence.TTFTMs, true
		}
		return "", evidence.ErrorRate, evidence.TTFTMs, false
	}
	return s.shouldEscapeStickyAccountAtGroup(groupID, accountID, s.service.openAIStickyEscapeConfig(), now)
}
