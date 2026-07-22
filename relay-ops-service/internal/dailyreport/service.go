package dailyreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/cachepolicy"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type CandidateReader interface {
	ListCandidates(context.Context) ([]candidates.Candidate, error)
}

type IncidentReader interface {
	ListIncidentSummaries(context.Context, int) ([]string, error)
}

type IncidentStateRunner interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type Result struct {
	ReportDate   string `json:"report_date"`
	Groups       int    `json:"groups"`
	Notification string `json:"notification"`
	AgentStatus  string `json:"agent_status"`
}

type Service struct {
	Reader     sub2api.Reader
	Candidates CandidateReader
	Incidents  IncidentReader
	Agent      AnalysisRunner
	Notifier   MessageSender
	Timezone   *time.Location
	Now        func() time.Time
}

func (s Service) Run(ctx context.Context) (Result, error) {
	if s.Reader == nil {
		return Result{}, fmt.Errorf("daily report reader is required")
	}
	location := s.Timezone
	if location == nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	date := now.In(location).Format("2006-01-02")
	groups, err := s.Reader.ListGroups(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list public groups: %w", err)
	}
	channels, err := s.Reader.ListChannels(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list public channels: %w", err)
	}
	lines := make([]string, 0, len(groups))
	evidenceRefs := make([]string, 0, len(groups))
	publicGroups := 0
	for _, group := range groups {
		if !group.CustomerVisible() {
			continue
		}
		ops, err := s.Reader.GetOpsSnapshot(ctx, sub2api.OpsQuery{TimeRange: "24h", GroupID: group.ID})
		if err != nil {
			return Result{}, fmt.Errorf("get Ops for group %d: %w", group.ID, err)
		}
		usage, err := s.Reader.GetUsageStats(ctx, sub2api.UsageQuery{GroupID: group.ID, Period: "24h", Timezone: "Asia/Shanghai"})
		if err != nil {
			return Result{}, fmt.Errorf("get usage for group %d: %w", group.ID, err)
		}
		publicGroups++
		line := fmt.Sprintf("%s：SLA %.2f%%，错误 %d/%d，TTFT P95 %.0fms，总延迟 P95 %.0fms，本站费用 $%.6f，上游成本 $%.6f",
			group.Name, ops.Overview.SLA, ops.Overview.ErrorCountTotal, ops.Overview.RequestCountTotal,
			ops.Overview.TTFT.P95MS, ops.Overview.Duration.P95MS, usage.TotalCost, usage.TotalAccountCost)
		if strings.EqualFold(strings.TrimSpace(group.Platform), "openai") {
			policy := cachepolicy.Evaluate([]sub2api.Group{group}, channels)
			line += "，" + cacheReportLine(usage, policy.Ready, policy.EligibleModels, policy.DiscountedModels, policy.Blockers)
			evidenceRefs = append(evidenceRefs, "group:"+strconv.FormatInt(group.ID, 10)+":cache24h")
		}
		lines = append(lines, line)
		evidenceRefs = append(evidenceRefs, "group:"+strconv.FormatInt(group.ID, 10)+":ops24h")
	}
	if s.Candidates != nil {
		items, err := s.Candidates.ListCandidates(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("list candidates: %w", err)
		}
		lines = append(lines, "候选站 "+strconv.Itoa(len(items)))
		evidenceRefs = append(evidenceRefs, "candidates:summary")
	}
	if s.Incidents != nil {
		items, err := s.Incidents.ListIncidentSummaries(ctx, 20)
		if err != nil {
			return Result{}, fmt.Errorf("list incidents: %w", err)
		}
		lines = append(lines, "活动事件 "+strconv.Itoa(len(items)))
		evidenceRefs = append(evidenceRefs, "incidents:recent")
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1",
		IncidentID:      "daily-report:" + date,
		Severity:        "P2",
		Upstream:        "relay-ops",
		MetricName:      "daily_operations_report",
		CurrentValue:    "公开分组 " + strconv.Itoa(publicGroups),
		Samples:         int64(publicGroups),
		EvidenceRefs:    evidenceRefs,
		AllowedActions:  []string{"observe", "request_human_review"},
	}
	analysis := agent.Fallback(contract)
	result := Result{ReportDate: date, Groups: publicGroups, AgentStatus: "fallback", Notification: "not_configured"}
	if s.Agent != nil {
		if generated, analyzeErr := s.Agent.AnalyzeOnce(ctx, contract); analyzeErr == nil {
			analysis = generated
			result.AgentStatus = "completed"
		} else {
			result.AgentStatus = "fallback_after_error"
		}
	}
	message := notify.RenderFeishu(notify.IncidentView{
		Title:       "relay-ops 每日运营摘要 " + date,
		WhatWasDone: []string{"读取所有客户公开分组的 24 小时 Ops/Usage 聚合", "汇总候选站和近期事件", "执行只读 Agent 分析（如已配置）"},
		Results:     append(lines, "分析："+analysis.Summary),
		Change:      analysis.Change,
		Focus:       analysis.Focus,
		Links:       []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
	if s.Notifier == nil {
		return result, nil
	}
	reporter, ok := s.Incidents.(IncidentStateRunner)
	if !ok {
		return Result{}, fmt.Errorf("daily report incident state is required")
	}
	evidence := summaryHash(date)
	if _, err := reporter.Observe(ctx, incidents.Observation{
		Key:                 contract.IncidentID,
		Severity:            "P2",
		Failing:             true,
		EvidenceHash:        evidence,
		CurrentValue:        "daily operations report generated",
		ConfirmationWindows: 1,
	}); err != nil {
		return Result{}, fmt.Errorf("record daily report incident: %w", err)
	}
	if err := s.Notifier.SendIncident(ctx, contract.IncidentID, evidence, message); err != nil {
		result.Notification = "failed"
		return result, err
	}
	result.Notification = "delivered"
	return result, nil
}

func cacheReportLine(usage sub2api.UsageStats, ready bool, eligible, discounted int, blockers []string) string {
	summary := cachepolicy.Summarize(usage)
	parts := make([]string, 0, 2)
	if summary.Confirmed {
		parts = append(parts, fmt.Sprintf("缓存读取 %s，写入 %s，命中率 %.2f%%",
			formatTokens(summary.CacheReadTokens), formatTokens(summary.CacheCreationTokens), summary.HitRatePercent))
	} else {
		parts = append(parts, "缓存统计不可确认")
	}
	if ready {
		if eligible == 0 {
			parts = append(parts, "缓存优惠无适用模型")
		} else {
			parts = append(parts, fmt.Sprintf("缓存优惠 %d/%d 模型", discounted, eligible))
		}
	} else {
		codes := make([]string, 0, len(blockers))
		seen := make(map[string]struct{})
		for _, blocker := range blockers {
			code := strings.SplitN(blocker, ":", 2)[0]
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
		parts = append(parts, "缓存优惠未就绪（"+strings.Join(codes, "、")+"）")
	}
	return strings.Join(parts, "，")
}

func formatTokens(value int64) string {
	switch {
	case value >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(value)/1_000_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.2fK", float64(value)/1_000)
	default:
		return strconv.FormatInt(value, 10)
	}
}

func summaryHash(date string) string {
	sum := sha256.Sum256([]byte("daily-report-v1\x00" + date))
	return hex.EncodeToString(sum[:])
}
