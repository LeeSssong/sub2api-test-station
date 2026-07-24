package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/d04readiness"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
)

type NativePricingSource struct {
	Reader sub2api.Reader
	Clock  func() time.Time
}

func (s NativePricingSource) PublicPricing(ctx context.Context) ([]PublicGroup, error) {
	groups, err := s.Reader.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list public pricing groups: %w", err)
	}
	channels, err := s.Reader.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list public pricing channels: %w", err)
	}
	visible := make(map[int64]string)
	for _, group := range groups {
		if group.CustomerVisible() {
			visible[group.ID] = group.Name
		}
	}
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock().UTC()
	}
	updated := now.Format("2006-01-02 15:04 UTC")
	byGroup := make(map[int64]*PublicGroup)
	for _, channel := range channels {
		if channel.Status != "active" {
			continue
		}
		for _, groupID := range channel.GroupIDs {
			name, ok := visible[groupID]
			if !ok {
				continue
			}
			group := byGroup[groupID]
			if group == nil {
				group = &PublicGroup{Name: name, UpdatedAt: updated}
				byGroup[groupID] = group
			}
			for _, price := range channel.ModelPricing {
				for _, modelID := range price.Models {
					group.Models = append(group.Models, PublicModel{ModelID: modelID, Input: priceValue(price.InputPrice), Output: priceValue(price.OutputPrice), CacheRead: priceValue(price.CacheReadPrice), CacheWrite: priceValue(price.CacheWritePrice)})
					for _, interval := range price.Intervals {
						group.Models = append(group.Models, PublicModel{
							ModelID: modelID, Tier: intervalLabel(interval),
							Input: priceValue(interval.InputPrice), Output: priceValue(interval.OutputPrice),
							CacheRead: priceValue(interval.CacheReadPrice), CacheWrite: priceValue(interval.CacheWritePrice),
						})
					}
				}
			}
		}
	}
	result := make([]PublicGroup, 0, len(byGroup))
	for _, group := range byGroup {
		result = append(result, *group)
	}
	return result, nil
}

func priceValue(value *float64) string {
	if value == nil {
		return ""
	}
	formatted := strconv.FormatFloat(*value*1_000_000, 'f', 6, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if !strings.Contains(formatted, ".") {
		return formatted + ".00"
	}
	if len(formatted)-strings.IndexByte(formatted, '.')-1 == 1 {
		return formatted + "0"
	}
	return formatted
}

func intervalLabel(interval sub2api.ChannelModelPriceInterval) string {
	if label := strings.TrimSpace(interval.TierLabel); label != "" {
		return label
	}
	min := compactTokens(interval.MinTokens)
	if interval.MaxTokens == nil {
		return ">" + min
	}
	max := compactTokens(*interval.MaxTokens)
	if interval.MinTokens == 0 {
		return "<=" + max
	}
	return ">" + min + "-<=" + max
}

func compactTokens(value int64) string {
	if value%1_000_000 == 0 && value >= 1_000_000 {
		return strconv.FormatInt(value/1_000_000, 10) + "m"
	}
	if value%1_000 == 0 && value >= 1_000 {
		return strconv.FormatInt(value/1_000, 10) + "k"
	}
	return strconv.FormatInt(value, 10)
}

type OpsRepository interface {
	ListCandidates(context.Context) ([]candidates.Candidate, error)
	ListPublicGroupNames(context.Context) ([]string, error)
}

type ProductionRepository interface {
	ListProduction(context.Context) ([]upstreams.Source, error)
}

type OpsEvidenceRepository interface {
	ListIncidentSummaries(context.Context, int) ([]string, error)
	ListAgentSummaries(context.Context, int) ([]string, error)
}

type D04ReadinessSource interface {
	Read() (d04readiness.Result, error)
}

type AccountQualitySource interface {
	Read(time.Time) (accountquality.Result, error)
}

type NativeOpsReader interface {
	sub2api.AccountReader
	ListGroups(context.Context) ([]sub2api.Group, error)
	GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error)
}

type DatabaseOpsSource struct {
	Repository OpsRepository
	Production ProductionRepository
	Pricing    interface {
		LatestPricingSnapshot(context.Context, domain.UpstreamID) (store.PricingSnapshot, bool, error)
	}
	Evidence OpsEvidenceRepository
	Quality  interface {
		ListQualityReports(context.Context, int) ([]qualityreports.Report, error)
	}
	Native         NativeOpsReader
	Readiness      D04ReadinessSource
	AccountQuality AccountQualitySource
	Clock          func() time.Time
}

func (s DatabaseOpsSource) Snapshot(ctx context.Context) (OpsView, error) {
	if s.Native == nil {
		return OpsView{}, fmt.Errorf("native Sub2API reader is required")
	}
	accounts, err := s.Native.ListAccounts(ctx)
	if err != nil {
		return OpsView{}, fmt.Errorf("list active Sub2API accounts: %w", err)
	}
	groups, err := s.Native.ListGroups(ctx)
	if err != nil {
		return OpsView{}, fmt.Errorf("list Sub2API groups: %w", err)
	}
	now := s.now()
	siteRuntime, err := opsmetrics.Collect(ctx, collectedOpsReader{groups: groups, accounts: accounts, native: s.Native}, now)
	if err != nil {
		return OpsView{}, fmt.Errorf("project native site runtime: %w", err)
	}
	liveSnapshot, err := (d04readiness.Collector{
		Accounts: accountSliceReader(accounts), Clock: func() time.Time { return now },
	}).Collect(ctx, d04readiness.Inputs{SnapshotID: "ops-live"})
	if err != nil {
		return OpsView{}, fmt.Errorf("project active Sub2API accounts: %w", err)
	}
	groupNames := make(map[int64]string, len(groups))
	publicGroups := make([]string, 0, len(groups))
	for _, group := range groups {
		groupNames[group.ID] = group.Name
		if group.CustomerVisible() {
			publicGroups = append(publicGroups, group.Name)
		}
	}
	readiness := liveReadiness(liveSnapshot, groupNames, "readiness_result_unavailable", now)
	if s.Readiness != nil {
		if result, readErr := s.Readiness.Read(); readErr == nil {
			if result.AccountSetSHA256 == liveSnapshot.UpstreamDiscovery.AccountSetSHA256 {
				readiness = projectD04Readiness(result, now)
				overlayLiveAccountMetadata(&readiness, liveSnapshot.ActiveUpstreams, groupNames)
			} else {
				readiness = liveReadiness(liveSnapshot, groupNames, "upstream_account_set_changed", now)
				readiness.SnapshotID = result.SnapshotID
				readiness.EvaluatedAt = result.EvaluatedAt.UTC().Format("2006-01-02 15:04 UTC")
				readiness.Stale = true
			}
		}
	}
	accountQualityView := accountquality.View{}
	if s.AccountQuality != nil {
		if result, readErr := s.AccountQuality.Read(now); readErr == nil {
			accountQualityView = result.ViewForAccountSet(now, siteRuntime.AccountSetSHA256)
		} else {
			accountQualityView = accountquality.View{Available: true, Stale: true}
		}
	}
	incidents := []string{}
	agentReports := []string{}
	qualityViews := []QualityReportView{}
	if s.Evidence != nil {
		var err error
		incidents, err = s.Evidence.ListIncidentSummaries(ctx, 10)
		if err != nil {
			return OpsView{}, err
		}
		agentReports, err = s.Evidence.ListAgentSummaries(ctx, 10)
		if err != nil {
			return OpsView{}, err
		}
	}
	if s.Quality != nil {
		reports, err := s.Quality.ListQualityReports(ctx, 20)
		if err != nil {
			return OpsView{}, err
		}
		for _, report := range reports {
			qualityViews = append(qualityViews, QualityReportView{
				ReportID: report.ReportID, ReportHash: report.ReportHash, Upstream: report.UpstreamName,
				Status: report.Status, QualityScore: report.QualityScore, TotalScore: report.TotalScore,
				Direct: report.Direct, Gateway: report.Gateway, Models: report.Models, Pricing: report.Pricing, Capacity: report.Capacity,
			})
		}
	}
	return OpsView{
		PublicGroups: publicGroups, NativeMonitorURL: "/monitor", QualityReports: qualityViews,
		Incidents: incidents, AgentReports: agentReports, D04LaunchReadiness: readiness, AccountQuality: accountQualityView,
		SiteRuntime: siteRuntime,
		RefreshedAt: now.Format("2006-01-02 15:04 UTC"),
	}, nil
}

type collectedOpsReader struct {
	groups   []sub2api.Group
	accounts []sub2api.Account
	native   interface {
		GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error)
	}
}

func (r collectedOpsReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return r.groups, nil
}

func (r collectedOpsReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return r.accounts, nil
}

func (r collectedOpsReader) GetOpsSnapshot(ctx context.Context, query sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return r.native.GetOpsSnapshot(ctx, query)
}

type accountSliceReader []sub2api.Account

func (a accountSliceReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return []sub2api.Account(a), nil
}

func liveReadiness(snapshot d04readiness.Snapshot, groupNames map[int64]string, reason string, now time.Time) D04LaunchReadinessView {
	if len(snapshot.ActiveUpstreams) == 0 {
		reason = "active_upstreams_empty"
	}
	view := D04LaunchReadinessView{
		Available: true, Decision: "NO-GO", AccountSetSHA256: snapshot.UpstreamDiscovery.AccountSetSHA256,
		EvaluatedAt: now.UTC().Format("2006-01-02 15:04 UTC"), Error: d04BlockerLabel(reason),
		Blockers: d04BlockerLabel(reason), BlockerCodes: reason,
	}
	for _, account := range snapshot.ActiveUpstreams {
		runtime := "可用"
		if !account.RuntimeAvailable {
			runtime = "暂不可用"
		}
		view.Upstreams = append(view.Upstreams, D04LaunchReadinessUpstreamView{
			AccountID: strconv.FormatInt(account.AccountID, 10), DisplayName: account.DisplayName,
			Groups: groupLabels(account.GroupIDs, groupNames), Runtime: runtime, Balance: "未知",
			FinancialAge: "未知", Quality: "等待新门禁检查", Samples: 0,
			Blockers: d04BlockerLabel(reason), BlockerCodes: reason,
		})
	}
	return view
}

func overlayLiveAccountMetadata(view *D04LaunchReadinessView, live []d04readiness.ActiveUpstream, groupNames map[int64]string) {
	byID := make(map[int64]d04readiness.ActiveUpstream, len(live))
	for _, account := range live {
		byID[account.AccountID] = account
	}
	for index := range view.Upstreams {
		id, err := strconv.ParseInt(view.Upstreams[index].AccountID, 10, 64)
		if err != nil {
			continue
		}
		account, ok := byID[id]
		if !ok {
			continue
		}
		view.Upstreams[index].DisplayName = account.DisplayName
		view.Upstreams[index].Groups = groupLabels(account.GroupIDs, groupNames)
		if account.RuntimeAvailable {
			view.Upstreams[index].Runtime = "可用"
		} else {
			view.Upstreams[index].Runtime = "暂不可用"
		}
	}
}

func groupLabels(ids []int64, names map[int64]string) string {
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if name := strings.TrimSpace(names[id]); name != "" {
			labels = append(labels, name)
		} else {
			labels = append(labels, strconv.FormatInt(id, 10))
		}
	}
	return strings.Join(labels, ", ")
}

func (s DatabaseOpsSource) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func projectD04Readiness(result d04readiness.Result, now time.Time) D04LaunchReadinessView {
	view := D04LaunchReadinessView{
		Available: true, Decision: strings.ToUpper(strings.ReplaceAll(result.Decision, "_", "-")),
		SnapshotID: result.SnapshotID, AccountSetSHA256: result.AccountSetSHA256,
		EvaluatedAt: result.EvaluatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		Blockers:    localizedD04Blockers(result.BlockingReasons), BlockerCodes: strings.Join(result.BlockingReasons, ", "),
	}
	if now.Sub(result.EvaluatedAt) > 20*time.Minute {
		view.Stale = true
		view.Decision = "NO-GO"
		view.Error = "证据已过期"
		view.Blockers = joinD04BlockerLabel(view.Blockers, d04BlockerLabel("readiness_result_stale"))
		view.BlockerCodes = joinBlocker(view.BlockerCodes, "readiness_result_stale")
	}
	for _, upstream := range result.Upstreams {
		balance := "未知"
		if upstream.BalanceUSD != nil {
			balance = fmt.Sprintf("%.2f USD", *upstream.BalanceUSD)
		}
		financialAge := "未知"
		if upstream.FinancialRecordedAt != nil {
			financialAge = upstream.FinancialRecordedAt.UTC().Format("2006-01-02 15:04 UTC")
		}
		quality := "缺少账户归属证据"
		if upstream.QualityRecordedAt != nil && upstream.SuccessRate != nil && upstream.ErrorRate != nil && upstream.TTFTP95MS != nil && upstream.TotalLatencyP95MS != nil {
			quality = fmt.Sprintf("成功 %.1f%% · 错误 %.1f%% · TTFT P95 %.0fms · 总耗时 P95 %.0fms", *upstream.SuccessRate*100, *upstream.ErrorRate*100, *upstream.TTFTP95MS, *upstream.TotalLatencyP95MS)
		}
		runtime := "可用"
		if !upstream.RuntimeAvailable {
			runtime = "暂不可用"
		}
		var groups []string
		for _, groupID := range upstream.GroupIDs {
			groups = append(groups, strconv.FormatInt(groupID, 10))
		}
		samples := int64(0)
		if upstream.SampleCount != nil {
			samples = *upstream.SampleCount
		}
		view.Upstreams = append(view.Upstreams, D04LaunchReadinessUpstreamView{
			AccountID: strconv.FormatInt(upstream.AccountID, 10), DisplayName: upstream.DisplayName,
			Groups: strings.Join(groups, ", "), Runtime: runtime, Balance: balance,
			FinancialAge: financialAge, Quality: quality, Samples: samples,
			Blockers:     localizedD04Blockers(upstream.BlockingReasons),
			BlockerCodes: strings.Join(upstream.BlockingReasons, ", "),
		})
	}
	return view
}

func localizedD04Blockers(reasons []string) string {
	labels := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		labels = append(labels, d04BlockerLabel(reason))
	}
	return strings.Join(labels, "；")
}

func d04BlockerLabel(reason string) string {
	labels := map[string]string{
		"active_upstreams_empty":               "没有已启用调度的上游",
		"upstream_discovery_failed":            "活动上游发现失败",
		"upstream_discovery_stale":             "活动上游清单已过期",
		"upstream_account_set_changed":         "活动上游已变化，等待新门禁检查",
		"upstream_temporarily_unavailable":     "上游暂不可用",
		"upstream_balance_unknown":             "余额证据缺失",
		"upstream_balance_below_minimum":       "余额低于最低门槛",
		"upstream_financial_evidence_stale":    "余额证据已过期",
		"upstream_quality_attribution_missing": "缺少账户归属质量证据",
		"upstream_quality_source_invalid":      "质量证据来源无效",
		"upstream_quality_metrics_stale":       "质量指标已过期",
		"upstream_samples_insufficient":        "自然样本不足",
		"upstream_success_rate_low":            "成功率低于门槛",
		"upstream_error_rate_high":             "错误率高于门槛",
		"upstream_ttft_p95_high":               "TTFT P95 高于门槛",
		"upstream_total_latency_p95_high":      "总耗时 P95 高于门槛",
		"launch_not_approved":                  "尚未批准首发开放",
		"account_backup_stale":                 "账户备份已过期",
		"account_backup_hash_unverified":       "账户备份校验未通过",
		"account_backup_scope_incomplete":      "账户备份范围不完整",
		"d04_not_read_only":                    "D04 未处于只读模式",
		"registration_not_closed":              "注册入口未关闭",
		"relay_ops_not_read_only":              "运维服务未处于只读模式",
		"feishu_not_dry_run":                   "飞书命令未处于预演模式",
		"service_unhealthy":                    "依赖服务不健康",
		"container_restarted":                  "容器出现未解释重启",
		"container_oom":                        "容器发生内存溢出",
		"disk_pressure":                        "磁盘使用率高于门槛",
		"d04_configuration_mismatch":           "D04 首发配置不一致",
		"d04_user_limit_exceeded":              "首发用户数超过上限",
		"d04_balance_drift":                    "D04 余额对账存在偏差",
		"d04_read_only_reason_present":         "D04 仍有只读降级原因",
		"primary_owner_missing":                "首发负责人未配置",
		"support_channel_missing":              "支持渠道未配置",
		"rollback_unverified":                  "注册回滚尚未验证",
		"readiness_result_stale":               "门禁结果已过期",
		"readiness_result_unavailable":         "等待门禁检查",
	}
	if label, ok := labels[reason]; ok {
		return label
	}
	return reason
}

func joinBlocker(left, right string) string {
	if left == "" {
		return right
	}
	return left + ", " + right
}

func joinD04BlockerLabel(left, right string) string {
	if left == "" {
		return right
	}
	return left + "；" + right
}

func pricingDiffLabel(raw []byte) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return "已记录"
	}
	var diff pricing.SemanticDiff
	if err := json.Unmarshal(raw, &diff); err != nil || !diff.SemanticChange() {
		return "页面已检查，无语义变化"
	}
	parts := make([]string, 0, 4)
	if diff.Multiplier != nil {
		parts = append(parts, "倍率变化")
	}
	if len(diff.PriceChanges) > 0 {
		parts = append(parts, fmt.Sprintf("价格 %d 项", len(diff.PriceChanges)))
	}
	if len(diff.AddedModels) > 0 {
		parts = append(parts, fmt.Sprintf("新增模型 %d 个", len(diff.AddedModels)))
	}
	if len(diff.RemovedModels) > 0 {
		parts = append(parts, fmt.Sprintf("移除模型 %d 个", len(diff.RemovedModels)))
	}
	return strings.Join(parts, "；")
}
