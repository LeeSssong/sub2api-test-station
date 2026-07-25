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
	publicGroups := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.CustomerVisible() {
			publicGroups = append(publicGroups, group.Name)
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
		Incidents: incidents, AgentReports: agentReports, AccountQuality: accountQualityView,
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

func (s DatabaseOpsSource) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
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
