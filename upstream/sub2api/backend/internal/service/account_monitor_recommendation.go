package service

import (
	"strings"
	"time"
)

const (
	accountMonitorGroupRecommendationSource        = "monitor_probe"
	accountMonitorGroupRecommendationMinSamples    = 20
	accountMonitorGroupRecommendationEvidenceAge   = 7 * 24 * time.Hour
	accountMonitorGroupRecommendationMarginPro     = 0.05
	accountMonitorGroupRecommendationMarginPlus    = 0.05
	accountMonitorGroupRecommendationMarginSpecial = 0.02
)

type accountMonitorRecommendationTarget struct {
	key         string
	name        string
	margin      float64
	successRate float64
	group       AccountMonitorGroup
}

// EvaluateAccountMonitorGroupRecommendation derives a read-only group
// recommendation from supplied monitor evidence. It intentionally performs no
// repository or network access.
func EvaluateAccountMonitorGroupRecommendation(
	account Account,
	currentGroupNames []string,
	groups []AccountMonitorGroup,
	evidence AccountMonitorQualityEvidence,
	latest AccountMonitorLatest,
	now time.Time,
) *AccountMonitorGroupRecommendation {
	currentTarget, isTestGroup := accountMonitorRecommendationCurrentTarget(currentGroupNames)
	if currentTarget == "" && !isTestGroup {
		return nil
	}
	if account.Platform != PlatformOpenAI {
		return nil
	}
	if !isTestGroup && (!account.Schedulable || (account.Status != "" && account.Status != StatusActive)) {
		return nil
	}

	targets := accountMonitorRecommendationTargets(groups)
	if len(targets) == 0 {
		return nil
	}
	codexAuth := account.IsOpenAIAgentIdentity() || account.IsOpenAIPersonalAccessToken()
	if codexAuth {
		pro, ok := targets[AccountMonitorGroupRecommendationTargetPro]
		if !ok {
			return nil
		}
		if reason, status := accountMonitorRecommendationHardGate(account, evidence, latest, now, true); reason != "" {
			return accountMonitorRecommendationBlocked(isTestGroup, pro, evidence, reason, status, true)
		}
		if !isTestGroup && currentTarget == AccountMonitorGroupRecommendationTargetPro {
			return nil
		}
		return accountMonitorRecommendationResult(AccountMonitorGroupRecommendationStatusRecommended, pro, evidence, AccountMonitorGroupRecommendationActionMigrate, []string{"codex_auth_default_pro"})
	}

	if reason, status := accountMonitorRecommendationHardGate(account, evidence, latest, now, false); reason != "" {
		return accountMonitorRecommendationBlocked(isTestGroup, accountMonitorRecommendationTarget{}, evidence, reason, status, false)
	}
	if account.RateMultiplier != nil {
		costAllowed := false
		for _, target := range targets {
			if accountMonitorRecommendationCostAllowed(account, target) {
				costAllowed = true
				break
			}
		}
		if !costAllowed {
			return accountMonitorRecommendationBlocked(isTestGroup, accountMonitorRecommendationTarget{}, evidence, "profit_below_minimum", AccountMonitorGroupRecommendationStatusBlocked, false)
		}
	}

	var firstReason string
	for _, key := range []string{AccountMonitorGroupRecommendationTargetPro, AccountMonitorGroupRecommendationTargetPlus, AccountMonitorGroupRecommendationTargetSpecial} {
		target, ok := targets[key]
		if !ok || !accountMonitorRecommendationCostAllowed(account, target) {
			if firstReason == "" {
				firstReason = "profit_below_minimum"
			}
			continue
		}
		if reason := accountMonitorRecommendationQualityReason(evidence, target); reason != "" {
			if firstReason == "" {
				firstReason = reason
			}
			continue
		}
		if !isTestGroup && currentTarget == target.key {
			return nil
		}
		return accountMonitorRecommendationResult(AccountMonitorGroupRecommendationStatusRecommended, target, evidence, AccountMonitorGroupRecommendationActionMigrate, accountMonitorRecommendationReasons(firstReason))
	}

	if !isTestGroup {
		return nil
	}
	return accountMonitorRecommendationResult(AccountMonitorGroupRecommendationStatusNotRecommended, accountMonitorRecommendationTarget{}, evidence, AccountMonitorGroupRecommendationActionNone, accountMonitorRecommendationReasons(firstReason))
}

func accountMonitorRecommendationCurrentTarget(names []string) (string, bool) {
	for _, name := range names {
		switch accountMonitorRecommendationNormalizeGroupName(name) {
		case "test":
			return "", true
		case AccountMonitorGroupRecommendationTargetPro, AccountMonitorGroupRecommendationTargetPlus, AccountMonitorGroupRecommendationTargetSpecial:
			return accountMonitorRecommendationNormalizeGroupName(name), false
		}
	}
	return "", false
}

func accountMonitorRecommendationTargets(groups []AccountMonitorGroup) map[string]accountMonitorRecommendationTarget {
	targets := make(map[string]accountMonitorRecommendationTarget, 3)
	for _, group := range groups {
		if group.Status != "" && group.Status != StatusActive {
			continue
		}
		key := accountMonitorRecommendationNormalizeGroupName(group.Name)
		if key == "" || key == "test" {
			continue
		}
		if _, exists := targets[key]; exists {
			continue
		}
		target := accountMonitorRecommendationTarget{key: key, group: group}
		switch key {
		case AccountMonitorGroupRecommendationTargetPro:
			target.name = "GPT-Pro"
			target.margin = accountMonitorGroupRecommendationMarginPro
			target.successRate = .98
		case AccountMonitorGroupRecommendationTargetPlus:
			target.name = "GPT-Plus"
			target.margin = accountMonitorGroupRecommendationMarginPlus
			target.successRate = .95
		case AccountMonitorGroupRecommendationTargetSpecial:
			target.name = "GPT-特惠"
			target.margin = accountMonitorGroupRecommendationMarginSpecial
			target.successRate = .70
		default:
			continue
		}
		targets[key] = target
	}
	return targets
}

func accountMonitorRecommendationNormalizeGroupName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "【专属】", "")
	switch normalized {
	case "gpt-pro":
		return AccountMonitorGroupRecommendationTargetPro
	case "gpt-plus":
		return AccountMonitorGroupRecommendationTargetPlus
	case "gpt-特惠", "gpt-特惠分组":
		return AccountMonitorGroupRecommendationTargetSpecial
	case "gpt-测试分组":
		return "test"
	default:
		return ""
	}
}

func accountMonitorRecommendationHardGate(account Account, evidence AccountMonitorQualityEvidence, latest AccountMonitorLatest, now time.Time, codexAuth bool) (string, string) {
	if evidence.Source == "stale" || evidence.ObservedAt.IsZero() || (!now.IsZero() && now.Sub(evidence.ObservedAt) > accountMonitorGroupRecommendationEvidenceAge) {
		return "sample_insufficient", AccountMonitorGroupRecommendationStatusObserve
	}
	if evidence.SampleCount < accountMonitorGroupRecommendationMinSamples {
		return "sample_insufficient", AccountMonitorGroupRecommendationStatusObserve
	}
	if accountMonitorFatalProbeError(latest) {
		return accountMonitorRecommendationFatalReason(latest), AccountMonitorGroupRecommendationStatusBlocked
	}
	if evidence.LatencySampleCount == 0 || evidence.LatencyP95MS == nil {
		return "latency_exceeds_limit", AccountMonitorGroupRecommendationStatusBlocked
	}
	if evidence.TTFTSampleCount == 0 || evidence.TTFTP50MS == nil {
		return "ttft_exceeds_target", AccountMonitorGroupRecommendationStatusBlocked
	}
	if !codexAuth && account.RateMultiplier == nil {
		return "profit_below_minimum", AccountMonitorGroupRecommendationStatusBlocked
	}
	return "", ""
}

func accountMonitorRecommendationFatalReason(latest AccountMonitorLatest) string {
	code := strings.ToLower(strings.TrimSpace(latest.ErrorCode))
	switch {
	case strings.Contains(code, "auth") || strings.Contains(code, "unauthorized") || strings.Contains(code, "forbidden") || (latest.HTTPStatus != nil && (*latest.HTTPStatus == 401 || *latest.HTTPStatus == 403)):
		return "auth_failed"
	case strings.Contains(code, "balance") || strings.Contains(code, "billing") || (latest.HTTPStatus != nil && *latest.HTTPStatus == 402):
		return "balance_unavailable"
	case strings.Contains(code, "quota"):
		return "quota_unavailable"
	default:
		return "model_unavailable"
	}
}

func accountMonitorRecommendationCostAllowed(account Account, target accountMonitorRecommendationTarget) bool {
	if account.RateMultiplier == nil {
		return false
	}
	return *account.RateMultiplier <= target.group.RateMultiplier-target.margin
}

func accountMonitorRecommendationQualityReason(evidence AccountMonitorQualityEvidence, target accountMonitorRecommendationTarget) string {
	if evidence.SuccessRate < target.successRate {
		switch target.key {
		case AccountMonitorGroupRecommendationTargetPro:
			return "success_rate_below_pro"
		case AccountMonitorGroupRecommendationTargetPlus:
			return "success_rate_below_plus"
		default:
			return "success_rate_below_special"
		}
	}
	weights := normalizeAccountMonitorScoreWeights(target.group.ScoreWeights)
	if *evidence.TTFTP50MS > float64(weights.TTFTLimitMS) {
		return "ttft_exceeds_target"
	}
	if *evidence.LatencyP95MS > float64(weights.LatencyLimitMS) {
		return "latency_exceeds_limit"
	}
	return ""
}

func accountMonitorRecommendationBlocked(isTestGroup bool, target accountMonitorRecommendationTarget, evidence AccountMonitorQualityEvidence, reason, status string, codexAuth bool) *AccountMonitorGroupRecommendation {
	if !isTestGroup {
		return nil
	}
	if codexAuth {
		return accountMonitorRecommendationResult(AccountMonitorGroupRecommendationStatusBlocked, target, evidence, AccountMonitorGroupRecommendationActionHold, []string{reason, "codex_auth_default_pro"})
	}
	return accountMonitorRecommendationResult(status, accountMonitorRecommendationTarget{}, evidence, AccountMonitorGroupRecommendationActionNone, []string{reason})
}

func accountMonitorRecommendationResult(status string, target accountMonitorRecommendationTarget, evidence AccountMonitorQualityEvidence, action string, reasons []string) *AccountMonitorGroupRecommendation {
	return &AccountMonitorGroupRecommendation{
		Status: status, Target: target.key, TargetName: target.name, Action: action,
		ReasonCodes: reasons, SampleCount: evidence.SampleCount, ObservedAt: evidence.ObservedAt.UTC(), Source: accountMonitorGroupRecommendationSource,
	}
}

func accountMonitorRecommendationReasons(reason string) []string {
	if reason == "" {
		return nil
	}
	return []string{reason}
}
