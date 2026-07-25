package accountrecommendation

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"example.invalid/relay-ops-service/internal/sub2api"
)

const minimumScoreDelta = 0.05

type Result struct {
	EvidenceState string        `json:"evidence_state"`
	Groups        []GroupResult `json:"groups"`
}

type GroupResult struct {
	GroupID            int64       `json:"group_id"`
	GroupName          string      `json:"group_name"`
	CurrentAccountID   int64       `json:"current_account_id"`
	CandidateAccountID int64       `json:"candidate_account_id,omitempty"`
	Decision           string      `json:"decision"`
	ScoreDelta         float64     `json:"score_delta"`
	Reasons            []string    `json:"reasons"`
	EvidenceState      string      `json:"evidence_state"`
	Current            AccountView `json:"current"`
	Candidate          AccountView `json:"candidate,omitempty"`
}

type AccountView struct {
	AccountID    int64  `json:"account_id"`
	Name         string `json:"name"`
	ModelID      string `json:"model_id"`
	SuccessRate  string `json:"success_rate"`
	TTFT         string `json:"ttft"`
	Latency      string `json:"latency"`
	Multiplier   string `json:"multiplier"`
	UsageWindows string `json:"usage_windows"`
	Status       string `json:"status"`
}

func Analyze(projection sub2api.AccountMonitorProjection) Result {
	result := Result{EvidenceState: projectionEvidenceState(projection)}
	groups := make(map[int64][]sub2api.AccountMonitorAccount)
	names := make(map[int64]string)
	for _, account := range projection.Accounts {
		for index, groupID := range account.GroupIDs {
			groups[groupID] = append(groups[groupID], account)
			if index < len(account.GroupNames) && strings.TrimSpace(account.GroupNames[index]) != "" {
				names[groupID] = account.GroupNames[index]
			}
		}
	}
	groupIDs := make([]int64, 0, len(groups))
	for groupID := range groups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	result.Groups = make([]GroupResult, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		rows := append([]sub2api.AccountMonitorAccount(nil), groups[groupID]...)
		sort.Slice(rows, func(i, j int) bool { return rows[i].AccountID < rows[j].AccountID })
		group := GroupResult{GroupID: groupID, GroupName: names[groupID], Decision: "insufficient_evidence", EvidenceState: result.EvidenceState}
		currentRows := make([]sub2api.AccountMonitorAccount, 0, len(rows))
		for _, row := range rows {
			if row.Status == "active" && row.Schedulable {
				currentRows = append(currentRows, row)
			}
		}
		if len(currentRows) == 0 {
			group.EvidenceState = "no_current_account"
			group.Reasons = []string{"当前分组没有 active + schedulable 账号"}
			result.Groups = append(result.Groups, group)
			continue
		}
		current := currentRows[0]
		group.CurrentAccountID = current.AccountID
		group.Current = accountView(current)
		if result.EvidenceState != "fresh" {
			group.Reasons = []string{"监控证据" + evidenceLabel(result.EvidenceState) + "，暂不建议更换"}
			result.Groups = append(result.Groups, group)
			continue
		}
		if len(currentRows) == 1 {
			if len(rows) > 1 {
				if _, candidateState := bestCandidate(current, rows[1:]); candidateState != "" {
					group.EvidenceState = candidateState
					group.Reasons = []string{candidateReason(candidateState)}
					result.Groups = append(result.Groups, group)
					continue
				}
			}
			group.Decision = "current_ok"
			group.EvidenceState = "no_candidate"
			group.Reasons = []string{"当前分组没有满足条件的候选账号"}
			result.Groups = append(result.Groups, group)
			continue
		}
		candidate, candidateState := bestCandidate(current, currentRows[1:])
		if candidateState != "" {
			group.EvidenceState = candidateState
			group.Reasons = []string{candidateReason(candidateState)}
			result.Groups = append(result.Groups, group)
			continue
		}
		group.CandidateAccountID = candidate.AccountID
		group.Candidate = accountView(candidate)
		currentScore := score(current)
		candidateScore := score(candidate)
		group.ScoreDelta = round(candidateScore - currentScore)
		switch {
		case group.ScoreDelta >= minimumScoreDelta:
			group.Decision = "candidate_better"
			group.EvidenceState = "fresh"
			group.Reasons = recommendationReasons(current, candidate)
		default:
			group.Decision = "current_ok"
			group.EvidenceState = "margin_below_threshold"
			group.Reasons = []string{"综合评分差值 " + formatScore(group.ScoreDelta) + "，低于 0.05 更换阈值"}
		}
		result.Groups = append(result.Groups, group)
	}
	return result
}

func projectionEvidenceState(projection sub2api.AccountMonitorProjection) string {
	if projection.SchemaVersion != 1 || projection.Stale {
		return "stale"
	}
	return "fresh"
}

func bestCandidate(current sub2api.AccountMonitorAccount, rows []sub2api.AccountMonitorAccount) (sub2api.AccountMonitorAccount, string) {
	eligible := make([]sub2api.AccountMonitorAccount, 0, len(rows))
	for _, candidate := range rows {
		if candidate.Status != "active" || !candidate.Schedulable {
			continue
		}
		if candidate.Stale {
			continue
		}
		if candidate.SampleCount < 3 || current.SampleCount < 3 {
			continue
		}
		if strings.TrimSpace(current.ModelID) == "" || candidate.ModelID != current.ModelID {
			continue
		}
		if !usableLatestStatus(current) || !usableLatestStatus(candidate) {
			continue
		}
		if candidate.TTFTP95MS == nil || candidate.LatencyP95MS == nil || current.TTFTP95MS == nil || current.LatencyP95MS == nil {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		if current.SampleCount < 3 {
			return sub2api.AccountMonitorAccount{}, "insufficient_samples"
		}
		for _, candidate := range rows {
			switch {
			case candidate.Status != "active" || !candidate.Schedulable:
				return sub2api.AccountMonitorAccount{}, "candidate_inactive_or_unschedulable"
			case candidate.Stale:
				return sub2api.AccountMonitorAccount{}, "stale"
			case candidate.SampleCount < 3:
				return sub2api.AccountMonitorAccount{}, "insufficient_samples"
			case candidate.ModelID != current.ModelID:
				return sub2api.AccountMonitorAccount{}, "incompatible_model"
			case !usableLatestStatus(candidate):
				return sub2api.AccountMonitorAccount{}, "recent_failure"
			}
		}
		return sub2api.AccountMonitorAccount{}, "no_eligible_candidate"
	}
	sort.Slice(eligible, func(i, j int) bool {
		left, right := score(eligible[i]), score(eligible[j])
		if left != right {
			return left > right
		}
		return eligible[i].AccountID < eligible[j].AccountID
	})
	return eligible[0], ""
}

func usableLatestStatus(account sub2api.AccountMonitorAccount) bool {
	switch strings.ToLower(strings.TrimSpace(account.LatestStatus)) {
	case "", "failed", "error", "http_error", "timeout", "balance_exhausted", "account_test_error":
		return false
	default:
		return account.ErrorCode == ""
	}
}

func score(account sub2api.AccountMonitorAccount) float64 {
	stability := clamp(account.SuccessRate)
	performance := 0.0
	if account.TTFTP95MS != nil && account.LatencyP95MS != nil {
		performance = clamp(1 - ((*account.TTFTP95MS + *account.LatencyP95MS) / 2 / 2000))
	}
	multiplier := 1 / (1 + math.Max(account.Multiplier, 0))
	headroom := 1 - maxUtilization(account)
	recentLoad := 1 - math.Min(float64(account.RequestCount)/1000, 1)
	headroom = (headroom + recentLoad) / 2
	return 0.40*stability + 0.25*performance + 0.20*multiplier + 0.15*headroom
}

func maxUtilization(account sub2api.AccountMonitorAccount) float64 {
	maximum := 0.0
	for _, window := range account.UsageWindows {
		if window.Utilization > maximum {
			maximum = window.Utilization
		}
	}
	return clamp(maximum)
}

func recommendationReasons(current, candidate sub2api.AccountMonitorAccount) []string {
	reasons := make([]string, 0, 4)
	if candidate.SuccessRate > current.SuccessRate {
		reasons = append(reasons, "稳定性更高："+formatPercent(candidate.SuccessRate)+" vs "+formatPercent(current.SuccessRate))
	}
	if candidate.TTFTP95MS != nil && current.TTFTP95MS != nil && *candidate.TTFTP95MS < *current.TTFTP95MS {
		reasons = append(reasons, "TTFT 更低："+formatMS(*candidate.TTFTP95MS)+" vs "+formatMS(*current.TTFTP95MS))
	}
	if candidate.Multiplier < current.Multiplier {
		reasons = append(reasons, "倍率更低："+formatMultiplier(candidate.Multiplier)+" vs "+formatMultiplier(current.Multiplier))
	}
	if maxUtilization(candidate) < maxUtilization(current) {
		reasons = append(reasons, "用量窗口余量更高")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "综合评分更高")
	}
	return reasons
}

func candidateReason(state string) string {
	switch state {
	case "stale":
		return "候选账号证据已过期，暂不建议"
	case "insufficient_samples":
		return "样本不足 3 次，暂不建议"
	case "incompatible_model":
		return "模型不兼容，暂不建议"
	case "candidate_inactive_or_unschedulable":
		return "候选账号当前不可调度，暂不建议"
	case "recent_failure":
		return "最近探测失败，暂不建议"
	default:
		return "没有满足证据条件的候选账号"
	}
}

func accountView(account sub2api.AccountMonitorAccount) AccountView {
	return AccountView{
		AccountID: account.AccountID, Name: account.Name, ModelID: emptyAs(account.ModelID, "未选择模型"),
		SuccessRate: formatPercent(account.SuccessRate), TTFT: formatMetric(account.TTFTP95MS),
		Latency: formatMetric(account.LatencyP95MS), Multiplier: formatMultiplier(account.Multiplier),
		UsageWindows: usageWindows(account), Status: accountStatus(account),
	}
}

func usageWindows(account sub2api.AccountMonitorAccount) string {
	if len(account.UsageWindows) == 0 {
		return "无"
	}
	windows := append([]sub2api.AccountMonitorUsageWindow(nil), account.UsageWindows...)
	sort.Slice(windows, func(i, j int) bool { return windows[i].Name < windows[j].Name })
	parts := make([]string, 0, len(windows))
	for _, window := range windows {
		parts = append(parts, window.Name+" "+formatPercent(window.Utilization))
	}
	return strings.Join(parts, "、")
}

func accountStatus(account sub2api.AccountMonitorAccount) string {
	if account.Stale {
		return "证据已过期"
	}
	if account.Status != "active" || !account.Schedulable {
		return "当前不可调度"
	}
	if !usableLatestStatus(account) {
		return "最近失败"
	}
	return "正常"
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func formatScore(value float64) string      { return strconv.FormatFloat(value, 'f', 4, 64) }
func formatPercent(value float64) string    { return strconv.FormatFloat(value*100, 'f', 1, 64) + "%" }
func formatMS(value float64) string         { return strconv.FormatFloat(value, 'f', 0, 64) + "ms" }
func formatMultiplier(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) + "x" }
func formatMetric(value *float64) string {
	if value == nil {
		return "未知"
	}
	return formatMS(*value)
}
func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func evidenceLabel(state string) string {
	switch state {
	case "stale":
		return "已过期"
	default:
		return "不可用"
	}
}
