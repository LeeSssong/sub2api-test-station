package service

import (
	"context"
	"sort"
	"strings"
	"time"
)

// OpenAIAccountSchedulerProjectionRequest contains the monitor-owned snapshot
// inputs needed to explain the scheduler's current candidate order. Accounts
// and LoadMap are read only; Project never fetches a slot or mutates scheduler
// state.
type OpenAIAccountSchedulerProjectionRequest struct {
	GroupID                 int64
	Platform                string
	RequestedModel          string
	RequiredTransport       OpenAIUpstreamTransport
	RequiredCapability      OpenAIEndpointCapability
	RequiredImageCapability OpenAIImagesCapability
	RequireCompact          bool
	RequirePrivacySet       bool
	UseUpstreamTokenCost    bool
	ExcludedIDs             map[int64]struct{}
	SnapshotAt              time.Time
	Accounts                []*Account
	LoadMap                 map[int64]*AccountLoadInfo
}

// OpenAIAccountSchedulerProjection is the scheduler-owned, side-effect-free
// candidate view consumed by account monitoring.
type OpenAIAccountSchedulerProjection struct {
	SnapshotAt       time.Time                                   `json:"snapshot_at"`
	PolicyKey        string                                      `json:"policy_key"`
	PolicyLabel      string                                      `json:"policy_label"`
	EffectiveWeights map[string]float64                          `json:"effective_weights"`
	CandidateCount   int                                         `json:"candidate_count"`
	Candidates       []OpenAIAccountSchedulerProjectionCandidate `json:"candidates"`
}

type OpenAIAccountSchedulerProjectionCandidate struct {
	AccountID         int64                    `json:"account_id"`
	Rank              *int                     `json:"rank,omitempty"`
	Eligible          bool                     `json:"eligible"`
	PrimaryReasonCode AccountMonitorReasonCode `json:"primary_reason_code,omitempty"`
}

type OpenAIAccountSchedulerProjectionProvider interface {
	Project(context.Context, OpenAIAccountSchedulerProjectionRequest) (*OpenAIAccountSchedulerProjection, error)
}

type openAIAccountSchedulerPolicyResolution struct {
	weights    GatewayOpenAIWSSchedulerScoreWeightsView
	fairness   OpenAISchedulerFairnessSettings
	topK       int
	policy     OpenAISchedulerGroupPolicy
	configured bool
}

func (s *defaultOpenAIAccountScheduler) resolveOpenAIAccountSchedulerPolicy(ctx context.Context, groupID int64) openAIAccountSchedulerPolicyResolution {
	if s == nil || s.service == nil {
		return openAIAccountSchedulerPolicyResolution{
			weights:    (&OpenAIGatewayService{}).openAIWSSchedulerWeights(),
			fairness:   defaultOpenAISchedulerFairnessSettings(),
			topK:       7,
			configured: false,
		}
	}

	weights := s.service.openAIWSSchedulerWeightsForRequest(ctx)
	topK := s.service.openAIWSLBTopKForRequest(ctx)
	runtime := s.service.openAIAdvancedSchedulerRuntimeSettings(ctx)
	fairness := resolveOpenAISchedulerFairnessForGroup(runtime.fairness, groupID)
	policy, configured := runtime.groupPolicies[groupID]
	if configured {
		weights, fairness = applyOpenAISchedulerGroupPolicy(weights, fairness, policy, true)
		if policy.Values.TopK > 0 {
			topK = policy.Values.TopK
		}
	}
	return openAIAccountSchedulerPolicyResolution{
		weights:    weights,
		fairness:   fairness,
		topK:       topK,
		policy:     policy,
		configured: configured,
	}
}

func schedulerProjectionEffectiveWeights(weights GatewayOpenAIWSSchedulerScoreWeightsView) map[string]float64 {
	return map[string]float64{
		"priority":          weights.Priority,
		"load":              weights.Load,
		"queue":             weights.Queue,
		"error_rate":        weights.ErrorRate,
		"ttft":              weights.TTFT,
		"reset":             weights.Reset,
		"quota_headroom":    weights.QuotaHeadroom,
		"upstream_cost":     weights.UpstreamCost,
		"previous_response": weights.Previous,
		"session_sticky":    weights.SessionSticky,
	}
}

func schedulerProjectionPolicyKey(configured bool) string {
	if configured {
		return "group_policy"
	}
	return "global_policy"
}

func schedulerProjectionPolicyLabel(policy OpenAISchedulerGroupPolicy, configured bool) string {
	if !configured {
		return "默认调度策略"
	}
	if policy.Priority != (OpenAISchedulerBusinessPriority{}) {
		priority := policy.Priority
		switch {
		case priority.Profit <= priority.TTFT && priority.Profit <= priority.Latency:
			return "利润优先"
		case priority.TTFT <= priority.Latency:
			return "首字优先"
		default:
			return "完整耗时优先"
		}
	}
	switch policy.Preset {
	case OpenAISchedulerPresetSpecialOffer:
		return "体验优先"
	case OpenAISchedulerPresetPro:
		return "利润优先"
	case OpenAISchedulerPresetBalanced:
		return "体验均衡"
	default:
		return "当前分组调度策略"
	}
}

// Project builds the deterministic order before Top-K random choice. It
// deliberately accepts the caller's account/load snapshot so monitor reads do
// not acquire slots, create sticky bindings, or refresh runtime state.
func (s *defaultOpenAIAccountScheduler) Project(ctx context.Context, req OpenAIAccountSchedulerProjectionRequest) (*OpenAIAccountSchedulerProjection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := req.SnapshotAt
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	platform := normalizeOpenAICompatiblePlatform(req.Platform)
	policy := s.resolveOpenAIAccountSchedulerPolicy(ctx, req.GroupID)
	projection := &OpenAIAccountSchedulerProjection{
		SnapshotAt:       now,
		PolicyKey:        schedulerProjectionPolicyKey(policy.configured),
		PolicyLabel:      schedulerProjectionPolicyLabel(policy.policy, policy.configured),
		EffectiveWeights: schedulerProjectionEffectiveWeights(policy.weights),
	}

	groupID := req.GroupID
	var groupIDPtr *int64
	if groupID > 0 {
		groupIDPtr = &groupID
	}
	scheduleReq := OpenAIAccountScheduleRequest{
		GroupID:                 groupIDPtr,
		Platform:                platform,
		RequestedModel:          req.RequestedModel,
		RequiredTransport:       req.RequiredTransport,
		RequiredCapability:      req.RequiredCapability,
		RequiredImageCapability: req.RequiredImageCapability,
		RequireCompact:          req.RequireCompact,
		UseUpstreamTokenCost:    req.UseUpstreamTokenCost,
		ExcludedIDs:             req.ExcludedIDs,
	}

	eligible := make([]*Account, 0, len(req.Accounts))
	ineligible := make([]OpenAIAccountSchedulerProjectionCandidate, 0, len(req.Accounts))
	for _, account := range req.Accounts {
		candidate := OpenAIAccountSchedulerProjectionCandidate{}
		if account == nil {
			candidate.PrimaryReasonCode = AccountMonitorReasonNotEligible
			ineligible = append(ineligible, candidate)
			continue
		}
		candidate.AccountID = account.ID
		reason := ""
		switch {
		case req.ExcludedIDs != nil && hasOpenAIAccountID(req.ExcludedIDs, account.ID):
			reason = "excluded"
		case !account.IsSchedulableAt(now):
			reason = "not_schedulable"
		case account.Platform != platform || !account.IsOpenAICompatible():
			reason = "platform_mismatch"
		case req.RequirePrivacySet && !account.IsPrivacySet():
			reason = "privacy_not_set"
		default:
			if compatible, compatibilityReason := s.isAccountRequestCompatibleReason(ctx, account, scheduleReq); !compatible {
				reason = compatibilityReason
			} else if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
				reason = "transport_incompatible"
			} else if req.RequireCompact && openAICompactSupportTier(account) == 0 {
				reason = "compact_unsupported"
			}
		}
		if reason != "" {
			candidate.PrimaryReasonCode = schedulerProjectionReasonCode(account, reason, now)
			ineligible = append(ineligible, candidate)
			continue
		}
		candidate.Eligible = true
		eligible = append(eligible, account)
	}

	if len(eligible) > 0 {
		plan := s.buildOpenAIAccountLoadPlanAt(ctx, scheduleReq, eligible, req.LoadMap, now)
		ranked := append([]openAIAccountCandidateScore(nil), plan.preTopKCandidates...)
		sort.SliceStable(ranked, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
		})
		for index, rankedCandidate := range ranked {
			rank := index + 1
			candidate := OpenAIAccountSchedulerProjectionCandidate{
				AccountID: rankedCandidate.account.ID,
				Rank:      &rank,
				Eligible:  true,
			}
			if index == 0 && schedulerProjectionUsesStrategy(policy.policy, policy.configured) {
				candidate.PrimaryReasonCode = AccountMonitorReasonStrategy
			} else if index > 0 && rankedCandidate.score == ranked[index-1].score {
				candidate.PrimaryReasonCode = AccountMonitorReasonTieBreak
			}
			projection.Candidates = append(projection.Candidates, candidate)
		}
	}
	sort.SliceStable(ineligible, func(i, j int) bool {
		return ineligible[i].AccountID < ineligible[j].AccountID
	})
	projection.Candidates = append(projection.Candidates, ineligible...)
	projection.CandidateCount = len(projection.Candidates)
	return projection, nil
}

func (s *OpenAIGatewayService) Project(ctx context.Context, req OpenAIAccountSchedulerProjectionRequest) (*OpenAIAccountSchedulerProjection, error) {
	if s == nil {
		return (&defaultOpenAIAccountScheduler{}).Project(ctx, req)
	}
	return (&defaultOpenAIAccountScheduler{service: s, stats: s.openaiAccountStats}).Project(ctx, req)
}

func hasOpenAIAccountID(ids map[int64]struct{}, accountID int64) bool {
	_, ok := ids[accountID]
	return ok
}

func schedulerProjectionUsesStrategy(policy OpenAISchedulerGroupPolicy, configured bool) bool {
	if !configured || policy.Priority == (OpenAISchedulerBusinessPriority{}) {
		return false
	}
	return true
}

func schedulerProjectionReasonCode(account *Account, reason string, now time.Time) AccountMonitorReasonCode {
	if account != nil && openAIAccountSchedulerCooldownActiveAt(account, now) {
		return AccountMonitorReasonCooldown
	}
	if strings.HasPrefix(reason, "runtime_") || strings.Contains(reason, "cooldown") || strings.Contains(reason, "rate_limit") || strings.Contains(reason, "overload") || strings.Contains(reason, "quarantine") {
		return AccountMonitorReasonCooldown
	}
	return AccountMonitorReasonNotEligible
}

func openAIAccountSchedulerCooldownActiveAt(account *Account, now time.Time) bool {
	if account == nil {
		return false
	}
	for _, until := range []*time.Time{
		account.RateLimitResetAt,
		account.OverloadUntil,
		account.TempUnschedulableUntil,
	} {
		if until != nil && now.Before(*until) {
			return true
		}
	}
	return false
}
