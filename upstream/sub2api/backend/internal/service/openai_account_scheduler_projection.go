package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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
	QualityOrder            []int64
	SnapshotAt              time.Time
	Accounts                []*Account
	LoadMap                 map[int64]*AccountLoadInfo
}

// OpenAIAccountSchedulerProjection is the scheduler-owned, side-effect-free
// candidate view consumed by account monitoring.
type OpenAIAccountSchedulerProjection struct {
	SnapshotAt  time.Time `json:"snapshot_at"`
	PolicyKey   string    `json:"policy_key"`
	PolicyLabel string    `json:"policy_label"`
	// EffectiveWeights remains available to in-process callers but is not part
	// of the public projection JSON; EffectiveFacts is the readable contract.
	EffectiveWeights map[string]float64                          `json:"-"`
	EffectiveFacts   []AccountMonitorSchedulerFact               `json:"effective_facts"`
	ModelQuotaParity string                                      `json:"model_quota_parity,omitempty"`
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
	weights         GatewayOpenAIWSSchedulerScoreWeightsView
	fairness        OpenAISchedulerFairnessSettings
	topK            int
	policy          OpenAISchedulerGroupPolicy
	configured      bool
	qualityWeights  GatewayOpenAIWSSchedulerScoreWeightsView
	qualityFairness OpenAISchedulerFairnessSettings
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

func (s *defaultOpenAIAccountScheduler) resolveOpenAIAccountSchedulerPolicyForProjection(ctx context.Context, groupID int64) (openAIAccountSchedulerPolicyResolution, error) {
	if s == nil || s.service == nil {
		return openAIAccountSchedulerPolicyResolution{
			weights:         (&OpenAIGatewayService{}).openAIWSSchedulerWeights(),
			fairness:        defaultOpenAISchedulerFairnessSettings(),
			topK:            7,
			configured:      false,
			qualityWeights:  (&OpenAIGatewayService{}).openAIWSSchedulerWeights(),
			qualityFairness: defaultOpenAISchedulerFairnessSettings(),
		}, nil
	}

	weights := s.service.openAIWSSchedulerWeights()
	topK := s.service.openAIWSLBTopK()
	runtime, err := s.service.openAIAdvancedSchedulerRuntimeSettingsForProjection(ctx)
	if err != nil {
		return openAIAccountSchedulerPolicyResolution{}, err
	}
	if !runtime.enabled {
		return openAIAccountSchedulerPolicyResolution{
			weights:         weights,
			fairness:        defaultOpenAISchedulerFairnessSettings(),
			topK:            topK,
			configured:      false,
			qualityWeights:  weights,
			qualityFairness: defaultOpenAISchedulerFairnessSettings(),
		}, nil
	}
	overridden := applyOpenAIAdvancedSchedulerWeightOverrides(weights, runtime.weightOverrides)
	if overridden.configWeights().IsValid() {
		weights = overridden
	}
	if runtime.lbTopKOverride > 0 {
		topK = runtime.lbTopKOverride
	}
	qualityWeights := weights
	qualityFairness := resolveOpenAISchedulerFairnessForGroup(runtime.fairness, groupID)
	fairness := qualityFairness
	policy, configured := runtime.groupPolicies[groupID]
	if configured {
		weights, fairness = applyOpenAISchedulerGroupPolicy(weights, fairness, policy, true)
		if policy.Values.TopK > 0 {
			topK = policy.Values.TopK
		}
	}
	return openAIAccountSchedulerPolicyResolution{
		weights:         weights,
		fairness:        fairness,
		topK:            topK,
		policy:          policy,
		configured:      configured,
		qualityWeights:  qualityWeights,
		qualityFairness: qualityFairness,
	}, nil
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerRuntimeSettingsForProjection(ctx context.Context) (openAIAdvancedSchedulerRuntimeSettings, error) {
	settings := openAIAdvancedSchedulerRuntimeSettings{
		oauthSchedulingRateMultiplier: defaultOpenAIOAuthSchedulingRateMultiplier,
		weightOverrides:               map[string]float64{},
		fairness:                      defaultOpenAISchedulerFairnessSettings(),
		groupPolicies:                 map[int64]OpenAISchedulerGroupPolicy{},
	}
	if s == nil {
		return settings, nil
	}
	repo := s.openAIAdvancedSchedulerSettingRepo()
	if repo == nil {
		return settings, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdvancedSchedulerSettingDBTimeout)
	defer cancel()
	values, err := repo.GetMultiple(dbCtx, openAIAdvancedSchedulerRuntimeSettingKeys())
	if err != nil {
		return openAIAdvancedSchedulerRuntimeSettings{}, err
	}
	settings.lowUpstreamRatePriorityEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAILowUpstreamRatePriorityEnabled]), "true")
	settings.oauthSchedulingRateMultiplier = parseOpenAIOAuthSchedulingRateMultiplier(values[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
	settings.enabled = strings.EqualFold(strings.TrimSpace(values[openAIAdvancedSchedulerSettingKey]), "true")
	settings.stickyWeightedEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled]), "true")
	settings.subscriptionPriorityEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled]), "true")
	settings.lbTopKOverride = parsePositiveIntOverride(values[SettingKeyOpenAIAdvancedSchedulerLBTopK])
	settings.weightOverrides = parseOpenAIAdvancedSchedulerWeightOverrides(values)
	settings.fairness = parseOpenAISchedulerFairnessRuntimeSettings(values)
	settings.groupPolicies = normalizeOpenAISchedulerRuntimeGroupPolicies(settings.lbTopKOverride, settings.weightOverrides, settings.fairness, values[SettingKeyOpenAIAdvancedSchedulerGroupOverrides])
	return settings, nil
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

func schedulerProjectionEffectiveFacts(weights GatewayOpenAIWSSchedulerScoreWeightsView) []AccountMonitorSchedulerFact {
	facts := []AccountMonitorSchedulerFact{
		{Label: "账号优先级权重", Value: strconv.FormatFloat(weights.Priority, 'g', -1, 64)},
		{Label: "当前负载权重", Value: strconv.FormatFloat(weights.Load, 'g', -1, 64)},
		{Label: "排队量权重", Value: strconv.FormatFloat(weights.Queue, 'g', -1, 64)},
		{Label: "错误率权重", Value: strconv.FormatFloat(weights.ErrorRate, 'g', -1, 64)},
		{Label: "首 Token 权重", Value: strconv.FormatFloat(weights.TTFT, 'g', -1, 64)},
		{Label: "重置时间权重", Value: strconv.FormatFloat(weights.Reset, 'g', -1, 64)},
		{Label: "额度余量权重", Value: strconv.FormatFloat(weights.QuotaHeadroom, 'g', -1, 64)},
		{Label: "上游成本权重", Value: strconv.FormatFloat(weights.UpstreamCost, 'g', -1, 64)},
		{Label: "上次响应权重", Value: strconv.FormatFloat(weights.Previous, 'g', -1, 64)},
		{Label: "会话粘性权重", Value: strconv.FormatFloat(weights.SessionSticky, 'g', -1, 64)},
	}
	return facts
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
	policy, err := s.resolveOpenAIAccountSchedulerPolicyForProjection(ctx, req.GroupID)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAI scheduler projection policy: %w", err)
	}
	projection := &OpenAIAccountSchedulerProjection{
		SnapshotAt:       now,
		PolicyKey:        schedulerProjectionPolicyKey(policy.configured),
		PolicyLabel:      schedulerProjectionPolicyLabel(policy.policy, policy.configured),
		EffectiveWeights: schedulerProjectionEffectiveWeights(policy.weights),
		EffectiveFacts:   schedulerProjectionEffectiveFacts(policy.weights),
	}
	if platform == PlatformGrok && strings.TrimSpace(req.RequestedModel) == "" {
		projection.ModelQuotaParity = "unknown"
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
			if compatible, compatibilityReason := s.isAccountRequestProjectionCompatibleReason(ctx, account, scheduleReq, now); !compatible {
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
		if platform == PlatformGrok {
			quotaAccounts := make([]Account, 0, len(eligible))
			for _, account := range eligible {
				if account != nil {
					quotaAccounts = append(quotaAccounts, *account)
				}
			}
			quotaFiltered := s.filterGrokFreeQuotaAccounts(ctx, quotaAccounts)
			quotaAllowed := make(map[int64]struct{}, len(quotaFiltered))
			for i := range quotaFiltered {
				quotaAllowed[quotaFiltered[i].ID] = struct{}{}
			}
			filteredEligible := make([]*Account, 0, len(eligible))
			for _, account := range eligible {
				if _, allowed := quotaAllowed[account.ID]; allowed {
					filteredEligible = append(filteredEligible, account)
					continue
				}
				ineligible = append(ineligible, OpenAIAccountSchedulerProjectionCandidate{
					AccountID: account.ID, PrimaryReasonCode: AccountMonitorReasonNotEligible,
				})
			}
			eligible = filteredEligible
		}
	}

	if len(eligible) > 0 {
		plan := s.buildOpenAIAccountLoadPlanAtWithPolicy(ctx, scheduleReq, eligible, req.LoadMap, now, &policy, false)
		ranked := append([]openAIAccountCandidateScore(nil), plan.preTopKCandidates...)
		sort.SliceStable(ranked, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
		})
		strategyOrderDiffers := schedulerProjectionOrderDiffersFromQualityOrder(ranked, req.QualityOrder)
		for index, rankedCandidate := range ranked {
			rank := index + 1
			candidate := OpenAIAccountSchedulerProjectionCandidate{
				AccountID: rankedCandidate.account.ID,
				Rank:      &rank,
				Eligible:  true,
			}
			if index == 0 && strategyOrderDiffers {
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

func schedulerProjectionOrderDiffersFromQualityOrder(ranked []openAIAccountCandidateScore, qualityOrder []int64) bool {
	if len(qualityOrder) == 0 {
		return false
	}
	if len(ranked) != len(qualityOrder) {
		return true
	}
	for i := range ranked {
		if ranked[i].account == nil || ranked[i].account.ID != qualityOrder[i] {
			return true
		}
	}
	return false
}

func (s *defaultOpenAIAccountScheduler) isAccountRequestProjectionCompatibleReason(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest, now time.Time) (bool, string) {
	if account == nil {
		return false, "account_nil"
	}
	if s != nil && s.service != nil && s.service.isOpenAIAccountRequestRuntimeBlockedAtReadOnly(account, req.RequestedModel, now) {
		return false, "runtime_blocked"
	}
	if s != nil && s.service != nil && s.service.isOpenAIProxyStreamQuarantinedAtReadOnly(ctx, account, now) {
		return false, "proxy_stream_quarantined"
	}
	if paused, decision := shouldAutoPauseOpenAIAccountByQuotaAt(ctx, account, now); paused {
		reason := "quota_auto_pause"
		if decision.window != "" {
			reason += "_" + decision.window
		}
		return false, reason
	}
	if !parentHealthyForShadow(account, func(id int64) *Account {
		return s.lookupShadowParentAccount(ctx, id)
	}) {
		return false, "shadow_parent_unhealthy"
	}
	if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
		return false, "model_not_supported"
	}
	if req.GroupID != nil && s != nil && s.service != nil &&
		s.service.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID) &&
		s.service.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact) {
		return false, "channel_upstream_restricted"
	}
	if !accountSupportsOpenAICapabilities(account, req.RequiredCapability, req.RequiredImageCapability) {
		return false, "capability_mismatch"
	}
	if vetoed, reason := openAIProfitControlVetoReasonReadOnly(ctx, account); vetoed {
		return false, reason
	}
	return true, ""
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
