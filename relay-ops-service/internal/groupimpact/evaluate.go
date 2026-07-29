package groupimpact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	PrimaryNoAvailableAccounts   = "no_available_accounts"
	PrimaryAllRequestsFailed     = "all_requests_failed"
	PrimaryPartialRequestFailure = "partial_request_failures"
	PrimaryLostRedundancy        = "lost_redundancy"
	PrimaryTTFTDegraded          = "ttft_degraded"
)

func Evaluate(snapshot Snapshot) Impact {
	impact := Impact{ObservedAt: snapshot.ObservedAt}
	runtime := validRuntime(snapshot.Runtime)
	capacity := validCapacity(snapshot.Capacity)

	switch {
	case capacity != nil && capacity.Available == 0:
		impact.Severity = "P0"
		impact.Primary = PrimaryNoAvailableAccounts
	case runtime != nil && runtime.Requests > 0 && runtime.Successes == 0:
		impact.Severity = "P0"
		impact.Primary = PrimaryAllRequestsFailed
	case runtime != nil && runtime.Requests >= 20 && runtime.ErrorRate >= .05:
		impact.Severity = "P1"
		impact.Primary = PrimaryPartialRequestFailure
	case capacity != nil && capacity.Total >= 3 && capacity.Available <= 1:
		impact.Severity = "P1"
		impact.Primary = PrimaryLostRedundancy
	case runtime != nil && runtime.Requests >= 20 && runtime.Successes > 0 &&
		runtime.TTFTP95MS > 3000 && runtime.TTFTBaselineP95MS > 0 &&
		runtime.TTFTP95MS >= runtime.TTFTBaselineP95MS*1.3:
		impact.Severity = "P1"
		impact.Primary = PrimaryTTFTDegraded
	}

	impact.Failing = impact.Primary != ""
	impact.Headline, impact.UserImpact, impact.Current, impact.Action = describeImpact(
		impact.Primary, runtime, capacity,
	)
	impact.Summary = impactSummary(impact.Primary, runtime, capacity)
	impact.Clues = buildClues(snapshot.NativeMonitor, capacity)
	impact.EvidenceHash = evidenceHash(snapshot, runtime, capacity)
	impact.MaterialHash = materialHash(snapshot, impact, runtime, capacity)
	return impact
}

func impactSummary(
	primary string,
	runtime *RuntimeEvidence,
	capacity *CapacityEvidence,
) string {
	switch primary {
	case PrimaryNoAvailableAccounts:
		return "该分组当前没有可用账号。"
	case PrimaryAllRequestsFailed:
		return fmt.Sprintf("过去 15 分钟共 %d 次请求，全部失败。", runtime.Requests)
	case PrimaryPartialRequestFailure:
		return fmt.Sprintf(
			"过去 15 分钟共 %d 次请求，其中 %d 次失败，错误率 %.2f%%。",
			runtime.Requests, runtime.Requests-runtime.Successes, runtime.ErrorRate*100,
		)
	case PrimaryLostRedundancy:
		return fmt.Sprintf(
			"可用账号已从 %d 个降至 %d 个，当前没有冗余。",
			capacity.Total, capacity.Available,
		)
	case PrimaryTTFTDegraded:
		slower := (runtime.TTFTP95MS/runtime.TTFTBaselineP95MS - 1) * 100
		return fmt.Sprintf(
			"过去 15 分钟首字响应 P95 为 %.1f 秒，24 小时基线为 %.1f 秒，慢约 %.0f%%。",
			runtime.TTFTP95MS/1000, runtime.TTFTBaselineP95MS/1000, slower,
		)
	default:
		return "当前未发现持续用户影响。"
	}
}

func validRuntime(value *RuntimeEvidence) *RuntimeEvidence {
	if value == nil ||
		value.Requests < 0 ||
		value.Successes < 0 ||
		value.Successes > value.Requests ||
		!finite(value.ErrorRate) ||
		value.ErrorRate < 0 ||
		value.ErrorRate > 1 ||
		!finite(value.TTFTP95MS) ||
		value.TTFTP95MS < 0 ||
		!finite(value.TTFTBaselineP95MS) ||
		value.TTFTBaselineP95MS < 0 {
		return nil
	}
	return value
}

func validCapacity(value *CapacityEvidence) *CapacityEvidence {
	if value == nil || value.Available < 0 || value.Total < 0 || value.Available > value.Total {
		return nil
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func describeImpact(
	primary string,
	runtime *RuntimeEvidence,
	capacity *CapacityEvidence,
) (string, string, []Fact, string) {
	switch primary {
	case PrimaryNoAvailableAccounts:
		return "已无可用账号",
			"该分组用户当前无法正常完成请求。",
			appendCapacityFact(nil, capacity),
			"检查账号余额和调度状态，尽快恢复至少一个可用账号。"
	case PrimaryAllRequestsFailed:
		current := []Fact{{
			Label: "过去 15 分钟",
			Value: fmt.Sprintf("共 %d 次请求，全部失败。", runtime.Requests),
		}}
		current = appendCapacityFact(current, capacity)
		return "当前已不可用",
			"该分组用户当前无法正常完成请求。",
			current,
			"检查该分组最近错误和账号状态，优先确认失败是否集中在单个账号。"
	case PrimaryPartialRequestFailure:
		failures := runtime.Requests - runtime.Successes
		current := []Fact{{
			Label: "过去 15 分钟",
			Value: fmt.Sprintf(
				"共 %d 次请求，其中 %d 次失败，错误率 %.2f%%。",
				runtime.Requests, failures, runtime.ErrorRate*100,
			),
		}}
		current = appendCapacityFact(current, capacity)
		return "部分请求持续失败",
			"部分用户可能遇到请求失败或需要重试；当前不是完全不可用。",
			current,
			"检查该分组最近错误和账号状态，优先确认失败是否集中在单个账号。"
	case PrimaryLostRedundancy:
		return fmt.Sprintf("只剩 %d 个可用账号", capacity.Available),
			"请求目前仍可完成，但剩余账号一旦异常，整个分组将不可用。",
			appendCapacityFact(nil, capacity),
			"确认账号余额和调度状态，恢复至少一个备用账号。"
	case PrimaryTTFTDegraded:
		slower := (runtime.TTFTP95MS/runtime.TTFTBaselineP95MS - 1) * 100
		current := []Fact{{
			Label: "过去 15 分钟",
			Value: fmt.Sprintf(
				"首字响应 P95 为 %.1f 秒，24 小时基线为 %.1f 秒，慢约 %.0f%%。",
				runtime.TTFTP95MS/1000, runtime.TTFTBaselineP95MS/1000, slower,
			),
		}}
		current = appendCapacityFact(current, capacity)
		return "首字响应明显变慢",
			"请求仍可完成，但用户开始等待首个响应的时间明显变长。",
			current,
			"查看分账号首字延迟，确认是否需要暂停明显偏慢的账号。"
	default:
		current := appendCapacityFact(nil, capacity)
		return "运行正常", "当前未发现持续用户影响。", current, "无需立即处理，继续观察。"
	}
}

func appendCapacityFact(facts []Fact, capacity *CapacityEvidence) []Fact {
	if capacity == nil {
		return facts
	}
	value := fmt.Sprintf("可用账号 %d / %d。", capacity.Available, capacity.Total)
	if capacity.Available > 0 {
		value = fmt.Sprintf("可用账号 %d / %d，仍有服务能力。", capacity.Available, capacity.Total)
	}
	return append(facts, Fact{Label: "当前容量", Value: value})
}

func buildClues(
	monitors []NativeMonitorEvidence,
	capacity *CapacityEvidence,
) []Fact {
	clues := make([]Fact, 0)
	if capacity != nil {
		unavailable := append([]UnavailableAccount(nil), capacity.Unavailable...)
		sort.SliceStable(unavailable, func(i, j int) bool {
			if unavailable[i].Name == unavailable[j].Name {
				return unavailable[i].Reason < unavailable[j].Reason
			}
			return unavailable[i].Name < unavailable[j].Name
		})
		for _, account := range unavailable {
			name := strings.TrimSpace(account.Name)
			reason := humanCause(account.Reason)
			if name == "" || reason == "" {
				continue
			}
			clues = append(clues, Fact{
				Label: name, Value: reason, Confirmed: true,
			})
			if len(clues) == 5 {
				break
			}
		}
	}
	sortedMonitors := append([]NativeMonitorEvidence(nil), monitors...)
	sort.SliceStable(sortedMonitors, func(i, j int) bool {
		if sortedMonitors[i].Model == sortedMonitors[j].Model {
			return sortedMonitors[i].Status < sortedMonitors[j].Status
		}
		return sortedMonitors[i].Model < sortedMonitors[j].Model
	})
	for _, monitor := range sortedMonitors {
		switch strings.ToLower(strings.TrimSpace(monitor.Status)) {
		case "abnormal", "error", "failed", "unavailable":
			clues = append(clues, Fact{
				Label: "原生监控", Value: "当前异常，可作为排查线索。",
			})
		case "normal", "healthy", "success", "available":
			clues = append(clues, Fact{
				Label: "原生监控", Value: "当前正常。",
			})
		}
	}
	if len(clues) == 0 {
		clues = append(clues, Fact{
			Label: "原因", Value: "尚未确认具体故障账号或上游链路。",
		})
	}
	return clues
}

func humanCause(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "balance_exhausted", "balance exhausted":
		return "余额已耗尽"
	case "paused", "active but paused", "unschedulable":
		return "当前未参与调度"
	default:
		return strings.TrimSpace(value)
	}
}

func evidenceHash(
	snapshot Snapshot,
	runtime *RuntimeEvidence,
	capacity *CapacityEvidence,
) string {
	value := struct {
		GroupID  int64
		Runtime  *RuntimeEvidence
		Capacity *CapacityEvidence
		Monitors []NativeMonitorEvidence
	}{
		GroupID: snapshot.GroupID, Runtime: runtime, Capacity: capacity,
		Monitors: append([]NativeMonitorEvidence(nil), snapshot.NativeMonitor...),
	}
	sort.SliceStable(value.Monitors, func(i, j int) bool {
		if value.Monitors[i].Model == value.Monitors[j].Model {
			return value.Monitors[i].Status < value.Monitors[j].Status
		}
		return value.Monitors[i].Model < value.Monitors[j].Model
	})
	return canonicalHash(value)
}

func materialHash(
	snapshot Snapshot,
	impact Impact,
	runtime *RuntimeEvidence,
	capacity *CapacityEvidence,
) string {
	causes := make([]string, 0)
	affected := make([]string, 0)
	if capacity != nil {
		for _, account := range capacity.Unavailable {
			if name := strings.TrimSpace(account.Name); name != "" {
				affected = append(affected, name)
			}
			if cause := humanCause(account.Reason); cause != "" {
				causes = append(causes, cause)
			}
		}
	}
	if runtime != nil {
		affected = append(affected, runtime.AffectedScope...)
	}
	sort.Strings(affected)
	sort.Strings(causes)
	return canonicalHash(struct {
		GroupID  int64
		Severity string
		Primary  string
		Affected []string
		Causes   []string
		Action   string
	}{
		GroupID: snapshot.GroupID, Severity: impact.Severity, Primary: impact.Primary,
		Affected: affected, Causes: causes, Action: impact.Action,
	})
}

func canonicalHash(value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
