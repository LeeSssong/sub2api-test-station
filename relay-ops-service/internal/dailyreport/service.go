package dailyreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/cachepolicy"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
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

type AccountQualitySource interface {
	Read(time.Time) (accountquality.Result, error)
}

type Result struct {
	ReportDate   string `json:"report_date"`
	Groups       int    `json:"groups"`
	Notification string `json:"notification"`
	AgentStatus  string `json:"agent_status"`
}

type Service struct {
	Reader         opsmetrics.Reader
	Candidates     CandidateReader
	Incidents      IncidentReader
	AccountQuality AccountQualitySource
	Agent          AnalysisRunner
	Notifier       MessageSender
	Timezone       *time.Location
	Now            func() time.Time
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
	runtime, err := opsmetrics.Collect(ctx, s.Reader, now)
	if err != nil {
		return Result{}, fmt.Errorf("collect site runtime: %w", err)
	}
	evidenceRefs := make([]string, 0, len(runtime.Groups)+3)
	for _, group := range runtime.Groups {
		evidenceRefs = append(evidenceRefs, "group:"+strconv.FormatInt(group.ID, 10)+":ops15m")
	}
	publicGroups := len(runtime.Groups)
	footer := make([]string, 0, 3)
	if s.Candidates != nil {
		items, err := s.Candidates.ListCandidates(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("list candidates: %w", err)
		}
		footer = append(footer, "候选站 "+strconv.Itoa(len(items)))
		evidenceRefs = append(evidenceRefs, "candidates:summary")
	}
	if s.Incidents != nil {
		items, err := s.Incidents.ListIncidentSummaries(ctx, 20)
		if err != nil {
			return Result{}, fmt.Errorf("list incidents: %w", err)
		}
		footer = append(footer, "活动事件 "+strconv.Itoa(len(items)))
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
	footer = append(footer, "只读分析："+analysis.Summary)
	quality := accountquality.View{}
	if s.AccountQuality != nil {
		if evidence, readErr := s.AccountQuality.Read(now); readErr == nil {
			quality = evidence.View(now)
		} else {
			quality = accountquality.View{Available: true, Stale: true}
		}
	}
	message := notify.RenderOperationsDigest(notify.OperationsDigestView{
		Date: date, Runtime: runtime, AccountQuality: quality, Footer: footer,
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
