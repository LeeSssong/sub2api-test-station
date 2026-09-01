package service

import (
	"context"
	"fmt"
	"math"
	"sort"
)

const openAIAccountScheduleLayerUnifiedQuality = "unified_quality"

type openAIUnifiedQualityRecheckKey struct{}

// openAIUnifiedQualityCandidate contains only the values that are allowed to
// affect ordinary text ordering. Capacity, priority, fairness, sticky bonus,
// and total duration deliberately do not appear in this comparator contract.
type openAIUnifiedQualityCandidate struct {
	account             *Account
	successRate         *float64
	ttftMS              *float64
	effectiveU          *float64
	effectiveCostStatus string
}

type openAIProfitPartition struct {
	candidates   []openAIUnifiedQualityCandidate
	bypass       bool
	bypassReason string
}

func partitionOpenAIUnifiedQualityCandidates(ctx context.Context, candidates []openAIUnifiedQualityCandidate) openAIProfitPartition {
	orderedAll := sortOpenAIUnifiedQualityCandidates(candidates)
	gate, _ := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	if gate == nil {
		return openAIProfitPartition{candidates: orderedAll}
	}
	preferred := make([]openAIUnifiedQualityCandidate, 0, len(candidates))
	marginBelow, unknownU, invalidU := 0, 0, 0
	for _, candidate := range candidates {
		if !finiteQualityValue(candidate.effectiveU) {
			if candidate.effectiveCostStatus == "" || candidate.effectiveCostStatus == EffectiveCostStatusUnknown {
				unknownU++
			} else {
				invalidU++
			}
			continue
		}
		if candidate.effectiveCostStatus != "" && candidate.effectiveCostStatus != EffectiveCostStatusReady {
			invalidU++
			continue
		}
		if profitControlOverThreshold(*candidate.effectiveU, gate.threshold) {
			marginBelow++
			continue
		}
		preferred = append(preferred, candidate)
	}
	if len(preferred) > 0 {
		return openAIProfitPartition{candidates: sortOpenAIUnifiedQualityCandidates(preferred)}
	}
	reason := "margin_below"
	if marginBelow == 0 {
		switch {
		case unknownU > 0 && invalidU == 0:
			reason = "unknown_u"
		case invalidU > 0 && unknownU == 0:
			reason = "invalid_u"
		case unknownU > 0:
			reason = "unknown_u"
		}
	}
	return openAIProfitPartition{candidates: orderedAll, bypass: true, bypassReason: reason}
}

// sortOpenAIUnifiedQualityCandidates returns a copy in the stable, nullable
// lexicographic order approved for T96:
// success rate DESC, trimmed TTFT ASC, live U ASC, account ID ASC.
func sortOpenAIUnifiedQualityCandidates(candidates []openAIUnifiedQualityCandidate) []openAIUnifiedQualityCandidate {
	ordered := append([]openAIUnifiedQualityCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return isOpenAIUnifiedQualityCandidateBetter(ordered[i], ordered[j])
	})
	return ordered
}

func isOpenAIUnifiedQualityCandidateBetter(left, right openAIUnifiedQualityCandidate) bool {
	if left.account == nil || right.account == nil {
		return left.account != nil
	}
	if cmp := compareOpenAIUnifiedQualityNullableDesc(left.successRate, right.successRate); cmp != 0 {
		return cmp < 0
	}
	if cmp := compareOpenAIUnifiedQualityNullableAsc(left.ttftMS, right.ttftMS); cmp != 0 {
		return cmp < 0
	}
	if cmp := compareOpenAIUnifiedQualityNullableAsc(left.effectiveU, right.effectiveU); cmp != 0 {
		return cmp < 0
	}
	return left.account.ID < right.account.ID
}

func compareOpenAIUnifiedQualityNullableDesc(left, right *float64) int {
	leftValid, rightValid := finiteQualityValue(left), finiteQualityValue(right)
	if leftValid != rightValid {
		if leftValid {
			return -1
		}
		return 1
	}
	if !leftValid {
		return 0
	}
	if *left > *right {
		return -1
	}
	if *left < *right {
		return 1
	}
	return 0
}

func compareOpenAIUnifiedQualityNullableAsc(left, right *float64) int {
	leftValid, rightValid := finiteQualityValue(left), finiteQualityValue(right)
	if leftValid != rightValid {
		if leftValid {
			return -1
		}
		return 1
	}
	if !leftValid {
		return 0
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func finiteQualityValue(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func (s *defaultOpenAIAccountScheduler) selectByUnifiedQuality(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	if req.RequiredImageCapability != "" || NormalizeOpenAICompatiblePlatform(req.Platform) != PlatformOpenAI {
		selection, candidateCount, topK, loadSkew, err := s.selectByLoadBalance(ctx, req)
		decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerLoadBalance, CandidateCount: candidateCount, TopK: topK, LoadSkew: loadSkew}
		if selection != nil && selection.Account != nil {
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
		}
		return selection, decision, err
	}
	return s.selectByUnifiedQualityInternal(ctx, req)
}

func (s *defaultOpenAIAccountScheduler) selectByUnifiedQualityInternal(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	extraRetryCount := 0
	if s != nil && s.service != nil {
		extraRetryCount = s.service.OpenAIUnifiedExtraRetryCount(ctx, req.GroupID)
	}
	decision := OpenAIAccountScheduleDecision{
		Layer: openAIAccountScheduleLayerUnifiedQuality, SelectionLayer: openAIAccountScheduleLayerUnifiedQuality,
		UnifiedQuality: true, ExtraRetryCount: extraRetryCount,
	}
	if s == nil || s.service == nil {
		return nil, decision, ErrNoAvailableAccounts
	}
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, PlatformOpenAI)
	if err != nil {
		return nil, decision, err
	}
	decision.CandidateCount = len(accounts)
	if len(accounts) == 0 {
		return nil, decision, noAvailableOpenAISelectionError(req.RequestedModel, false, "pool=0")
	}

	snapshot := s.service.OpenAIAccountQualitySnapshot(ctx)
	decision.QualityWindowEnd = snapshot.WindowEnd
	decision.QualitySnapshotStale = snapshot.Stale
	qualityCandidates := make([]openAIUnifiedQualityCandidate, 0, len(accounts))
	excluded := make([]int64, 0)
	excludeReasons := make(map[string]int)
	for i := range accounts {
		account := &accounts[i]
		if account == nil || account.ID <= 0 {
			continue
		}
		if _, ok := req.ExcludedIDs[account.ID]; ok {
			excluded = append(excluded, account.ID)
			excludeReasons["excluded"]++
			continue
		}
		if req.GroupID != nil && !s.service.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
			excluded = append(excluded, account.ID)
			excludeReasons["group_mismatch"]++
			continue
		}
		if req.RequirePrivacySet && !account.IsPrivacySet() {
			excluded = append(excluded, account.ID)
			excludeReasons["privacy_not_set"]++
			continue
		}
		if reason := openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, account, PlatformOpenAI, req.RequestedModel, req.RequireCompact, req.RequiredCapability); reason != "" {
			excluded = append(excluded, account.ID)
			excludeReasons[reason]++
			continue
		}
		if !parentHealthyForShadow(account, s.service.parentAccountLookup(ctx)) {
			excluded = append(excluded, account.ID)
			excludeReasons["shadow_parent_unhealthy"]++
			continue
		}
		if s.service.isOpenAIAccountRequestRuntimeBlocked(account, req.RequestedModel) {
			excluded = append(excluded, account.ID)
			excludeReasons["runtime_blocked"]++
			continue
		}
		if s.service.isOpenAIProxyStreamQuarantined(ctx, account) {
			excluded = append(excluded, account.ID)
			excludeReasons["proxy_stream_quarantined"]++
			continue
		}
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			excluded = append(excluded, account.ID)
			excludeReasons["transport_incompatible"]++
			continue
		}

		var successRate, ttftMS *float64
		if quality, ok := snapshot.Accounts[account.ID]; ok {
			successRate = finiteQualityPointer(quality.SuccessRate)
			ttftMS = finiteQualityPointer(quality.TTFTTrimmedMeanMS)
		}
		var effectiveU *float64
		cost := EffectiveCostForAccount(account)
		if cost.Status == EffectiveCostStatusReady {
			effectiveU = finiteQualityPointer(cost.U)
		}
		qualityCandidates = append(qualityCandidates, openAIUnifiedQualityCandidate{
			account: account, successRate: successRate, ttftMS: ttftMS, effectiveU: effectiveU, effectiveCostStatus: cost.Status,
		})
	}
	partition := partitionOpenAIUnifiedQualityCandidates(ctx, qualityCandidates)
	ordered := partition.candidates
	decision.EligibleCount = len(ordered)
	decision.CandidateAccountIDs = unifiedCandidateIDs(ordered)
	if gatewayProfitControlGateActive(ctx) {
		decision.ProfitMode = "native"
		decision.ProfitBypass = partition.bypass
		decision.ProfitBypassReason = partition.bypassReason
	}
	decision.ExcludedAccountIDs = append([]int64(nil), excluded...)
	sort.Slice(decision.ExcludedAccountIDs, func(i, j int) bool { return decision.ExcludedAccountIDs[i] < decision.ExcludedAccountIDs[j] })
	decision.ExcludeReasons = cloneStringIntMap(excludeReasons)
	if len(ordered) == 0 {
		return nil, decision, noAvailableOpenAISelectionError(req.RequestedModel, false, fmt.Sprintf("pool=%d", len(accounts)))
	}

	var lastAcquireErr error
	var waitCandidate *openAIUnifiedQualityCandidate
	waitRank := 0
	for rank, candidate := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, decision, err
		}
		account := candidate.account
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if acquireErr != nil {
			lastAcquireErr = acquireErr
			continue
		}
		if result == nil || !result.Acquired {
			if waitCandidate == nil {
				candidateCopy := candidate
				waitCandidate = &candidateCopy
				waitRank = rank + 1
			}
			continue
		}
		release := result.ReleaseFunc
		if release == nil {
			release = func() {}
		}
		fresh := s.service.resolveFreshSchedulableOpenAIAccountBeforeProfit(ctx, account, PlatformOpenAI, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
		if fresh == nil || (req.GroupID != nil && !s.service.openAIAccountMatchesSchedulingGroup(fresh, req.GroupID)) {
			release()
			continue
		}
		// When no scheduler snapshot is configured, refresh directly from the
		// native account repository before evaluating the live cost contract.
		if s.service.schedulerSnapshot == nil && s.service.accountRepo != nil {
			if latest, refreshErr := s.service.accountRepo.GetByID(ctx, fresh.ID); refreshErr == nil && latest != nil {
				fresh = latest
			}
		}
		freshCost := EffectiveCostForAccount(fresh)
		freshU := (*float64)(nil)
		if freshCost.Status == EffectiveCostStatusReady {
			freshU = finiteQualityPointer(freshCost.U)
		}
		if !sameOpenAIUnifiedQualityCost(candidate.effectiveCostStatus, candidate.effectiveU, freshCost.Status, freshU) {
			release()
			if _, alreadyRechecked := ctx.Value(openAIUnifiedQualityRecheckKey{}).(bool); !alreadyRechecked {
				recheckCtx := context.WithValue(ctx, openAIUnifiedQualityRecheckKey{}, true)
				return s.selectByUnifiedQualityInternal(recheckCtx, req)
			}
			continue
		}
		if req.SessionHash != "" && !req.PreserveStickyBinding {
			_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, fresh.ID)
		}
		decision.SelectedAccountID = fresh.ID
		decision.SelectedAccountType = fresh.Type
		decision.SelectedRank = rank + 1
		selection := &AccountSelectionResult{Account: fresh, Acquired: true, ReleaseFunc: release, profitBypass: partition.bypass, unifiedQuality: true}
		return attachSelectionProfitGate(ctx, selection), decision, nil
	}
	if waitCandidate != nil && waitCandidate.account != nil {
		cfg := s.service.schedulingConfig()
		decision.SelectedAccountID = waitCandidate.account.ID
		decision.SelectedAccountType = waitCandidate.account.Type
		decision.SelectedRank = waitRank
		selection := &AccountSelectionResult{
			Account: waitCandidate.account,
			WaitPlan: &AccountWaitPlan{
				AccountID: waitCandidate.account.ID, MaxConcurrency: waitCandidate.account.Concurrency,
				Timeout: cfg.FallbackWaitTimeout, MaxWaiting: cfg.FallbackMaxWaiting,
			},
			profitBypass: partition.bypass, unifiedQuality: true,
		}
		return attachSelectionProfitGate(ctx, selection), decision, nil
	}
	if lastAcquireErr != nil && ctx.Err() != nil {
		return nil, decision, ctx.Err()
	}
	return nil, decision, noAvailableOpenAISelectionError(req.RequestedModel, false, fmt.Sprintf("pool=%d, selection_order_exhausted", len(accounts)))
}

func sameOpenAIUnifiedQualityCost(leftStatus string, leftU *float64, rightStatus string, rightU *float64) bool {
	if leftStatus != rightStatus {
		return false
	}
	if !finiteQualityValue(leftU) || !finiteQualityValue(rightU) {
		return leftU == nil && rightU == nil
	}
	return *leftU == *rightU
}

func finiteQualityPointer(value *float64) *float64 {
	if !finiteQualityValue(value) {
		return nil
	}
	copy := *value
	return &copy
}

func unifiedCandidateIDs(candidates []openAIUnifiedQualityCandidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.account != nil {
			ids = append(ids, candidate.account.ID)
		}
	}
	return ids
}
