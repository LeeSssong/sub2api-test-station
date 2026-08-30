package service

import (
	"context"
	"strings"
)

// AccountStatsCostResolution separates the pre-multiplier account-stat cost
// from the final account cost snapshot. Image tier prices are already final
// upstream costs and therefore do not receive the account rate multiplier.
type AccountStatsCostResolution struct {
	StatsCost        *float64
	ApplyAccountRate bool
	Matched          bool
}

// resolveAccountStatsCost 计算账号统计定价费用。
// 返回 nil 表示不覆盖，使用默认公式（total_cost × account_rate_multiplier）。
//
// 优先级（先命中为准）：
//  1. 自定义规则（始终尝试，不依赖 ApplyPricingToAccountStats 开关）
//  2. ApplyPricingToAccountStats 启用时，直接使用本次请求的客户计费（倍率前的 totalCost）
//  3. 模型定价文件（LiteLLM）中上游模型的默认价格
//  4. nil → 走默认公式（total_cost × account_rate_multiplier）
//
// upstreamModel 是最终发往上游的模型 ID。
// totalCost 是本次请求的客户计费（倍率前），用于优先级 2。
// serviceTier 是最终参与用户计费的 OpenAI 服务层级，用于优先级 3。
func resolveAccountStatsCost(
	ctx context.Context,
	channelService *ChannelService,
	billingService *BillingService,
	accountID int64,
	groupID int64,
	upstreamModel string,
	tokens UsageTokens,
	requestCount int,
	totalCost float64,
	serviceTier string,
) *float64 {
	resolution := resolveAccountStatsCostResolution(ctx, channelService, billingService,
		accountID, groupID, upstreamModel, BillingModeToken, "", 0, tokens, requestCount, totalCost, serviceTier)
	return resolution.StatsCost
}

func resolveAccountStatsCostResolution(
	ctx context.Context,
	channelService *ChannelService,
	billingService *BillingService,
	accountID int64,
	groupID int64,
	upstreamModel string,
	billingMode BillingMode,
	imageSize string,
	imageCount int,
	tokens UsageTokens,
	requestCount int,
	totalCost float64,
	serviceTier string,
) AccountStatsCostResolution {
	if channelService == nil || upstreamModel == "" {
		return AccountStatsCostResolution{ApplyAccountRate: true}
	}
	channel, err := channelService.GetChannelForGroup(ctx, groupID)
	if err != nil || channel == nil {
		return AccountStatsCostResolution{ApplyAccountRate: true}
	}

	platform := channelService.GetGroupPlatform(ctx, groupID)

	// 优先级 1：自定义规则（始终尝试）
	if resolution := tryCustomRulesResolution(channel, accountID, groupID, platform, upstreamModel,
		billingMode, imageSize, imageCount, tokens, requestCount); resolution.Matched {
		return resolution
	}

	// 优先级 2：渠道开启"应用模型定价到账号统计"时，直接使用客户计费（倍率前）
	if channel.ApplyPricingToAccountStats {
		cost := totalCost
		if cost <= 0 {
			return AccountStatsCostResolution{ApplyAccountRate: true}
		}
		return AccountStatsCostResolution{StatsCost: &cost, ApplyAccountRate: true, Matched: true}
	}

	// 优先级 3：模型定价文件（LiteLLM）默认价格
	if billingService != nil {
		cost := tryModelFilePricing(billingService, upstreamModel, tokens, serviceTier)
		if cost != nil {
			return AccountStatsCostResolution{StatsCost: cost, ApplyAccountRate: true, Matched: true}
		}
	}

	return AccountStatsCostResolution{ApplyAccountRate: true}
}

// tryModelFilePricing 使用模型定价文件（LiteLLM/fallback）中的价格计算费用。
func tryModelFilePricing(billingService *BillingService, model string, tokens UsageTokens, serviceTier string) *float64 {
	pricing, err := billingService.GetModelPricing(model)
	if err != nil || pricing == nil {
		return nil
	}
	normalizedTier := normalizeBillingServiceTier(serviceTier)
	if normalizedTier == "priority" || normalizedTier == "flex" ||
		billingService.shouldApplySessionLongContextPricing(tokens, pricing) {
		breakdown, err := billingService.CalculateCostWithServiceTier(model, tokens, 1, normalizedTier)
		if err != nil || breakdown == nil || breakdown.TotalCost <= 0 {
			return nil
		}
		return &breakdown.TotalCost
	}
	cost := float64(tokens.InputTokens)*pricing.InputPricePerToken +
		float64(tokens.OutputTokens)*pricing.OutputPricePerToken +
		float64(tokens.CacheCreationTokens)*pricing.CacheCreationPricePerToken +
		float64(tokens.CacheReadTokens)*pricing.CacheReadPricePerToken +
		float64(tokens.ImageOutputTokens)*pricing.ImageOutputPricePerToken
	if cost <= 0 {
		return nil
	}
	return &cost
}

// tryCustomRules 遍历自定义规则，按数组顺序先命中为准。
func tryCustomRules(
	channel *Channel, accountID, groupID int64,
	platform, model string, tokens UsageTokens, requestCount int,
) *float64 {
	return tryCustomRulesResolution(channel, accountID, groupID, platform, model,
		BillingModeToken, "", 0, tokens, requestCount).StatsCost
}

func tryCustomRulesResolution(
	channel *Channel,
	accountID, groupID int64,
	platform, model string,
	billingMode BillingMode,
	imageSize string,
	imageCount int,
	tokens UsageTokens,
	requestCount int,
) AccountStatsCostResolution {
	modelLower := strings.ToLower(model)
	for _, rule := range channel.AccountStatsPricingRules {
		if !matchAccountStatsRule(&rule, accountID, groupID) {
			continue
		}
		pricing := findPricingForModel(rule.Pricing, platform, modelLower)
		if pricing == nil {
			continue // 规则匹配但模型不在规则定价中，继续下一条
		}
		if billingMode == BillingModeImage {
			if pricing.BillingMode != BillingModeImage || imageCount <= 0 {
				return AccountStatsCostResolution{ApplyAccountRate: true}
			}
			tier, ok := ClassifyImageBillingTier(imageSize)
			if !ok {
				return AccountStatsCostResolution{ApplyAccountRate: true}
			}
			for _, interval := range pricing.Intervals {
				if strings.EqualFold(strings.TrimSpace(interval.TierLabel), tier) && interval.PerRequestPrice != nil && *interval.PerRequestPrice >= 0 {
					cost := *interval.PerRequestPrice * float64(imageCount)
					return AccountStatsCostResolution{StatsCost: &cost, Matched: true}
				}
			}
			return AccountStatsCostResolution{ApplyAccountRate: true}
		}
		if pricing.BillingMode == BillingModeImage {
			return AccountStatsCostResolution{ApplyAccountRate: true}
		}
		cost := calculateStatsCost(pricing, tokens, requestCount)
		if cost == nil {
			return AccountStatsCostResolution{ApplyAccountRate: true}
		}
		return AccountStatsCostResolution{StatsCost: cost, ApplyAccountRate: true, Matched: true}
	}
	return AccountStatsCostResolution{ApplyAccountRate: true}
}

// matchAccountStatsRule 检查规则是否匹配指定的 accountID 和 groupID。
// 匹配条件：accountID ∈ rule.AccountIDs 或 groupID ∈ rule.GroupIDs。
// 如果规则的 AccountIDs 和 GroupIDs 都为空，视为不匹配。
func matchAccountStatsRule(rule *AccountStatsPricingRule, accountID, groupID int64) bool {
	if len(rule.AccountIDs) == 0 && len(rule.GroupIDs) == 0 {
		return false
	}
	for _, id := range rule.AccountIDs {
		if id == accountID {
			return true
		}
	}
	for _, id := range rule.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

// findPricingForModel 在定价列表中查找匹配的模型定价。
// 先精确匹配，再通配符匹配（按配置顺序，先匹配先使用）。
func findPricingForModel(pricingList []ChannelModelPricing, platform, modelLower string) *ChannelModelPricing {
	// 精确匹配优先
	for i := range pricingList {
		p := &pricingList[i]
		if !isPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			if strings.ToLower(m) == modelLower {
				return p
			}
		}
	}
	// 通配符匹配：按配置顺序，先匹配先使用
	for i := range pricingList {
		p := &pricingList[i]
		if !isPlatformMatch(platform, p.Platform) {
			continue
		}
		for _, m := range p.Models {
			ml := strings.ToLower(m)
			if !strings.HasSuffix(ml, "*") {
				continue
			}
			prefix := strings.TrimSuffix(ml, "*")
			if strings.HasPrefix(modelLower, prefix) {
				return p
			}
		}
	}
	return nil
}

// isPlatformMatch 判断平台是否匹配（空平台视为不限平台）。
func isPlatformMatch(queryPlatform, pricingPlatform string) bool {
	if queryPlatform == "" || pricingPlatform == "" {
		return true
	}
	return queryPlatform == pricingPlatform
}

// calculateStatsCost 使用给定的定价计算费用（不含任何倍率，原始费用）。
func calculateStatsCost(pricing *ChannelModelPricing, tokens UsageTokens, requestCount int) *float64 {
	if pricing == nil {
		return nil
	}
	switch pricing.BillingMode {
	case BillingModePerRequest, BillingModeImage:
		return calculatePerRequestStatsCost(pricing, requestCount)
	default:
		return calculateTokenStatsCost(pricing, tokens)
	}
}

// calculatePerRequestStatsCost 按次/图片计费。
func calculatePerRequestStatsCost(pricing *ChannelModelPricing, requestCount int) *float64 {
	if pricing.PerRequestPrice == nil || *pricing.PerRequestPrice <= 0 {
		return nil
	}
	cost := *pricing.PerRequestPrice * float64(requestCount)
	return &cost
}

// calculateTokenStatsCost Token 计费。
// If the pricing has intervals, find the matching interval by total token count
// and use its prices instead of the flat pricing fields.
func calculateTokenStatsCost(pricing *ChannelModelPricing, tokens UsageTokens) *float64 {
	p := pricing
	if len(pricing.Intervals) > 0 {
		totalTokens := tokens.InputTokens + tokens.OutputTokens + tokens.CacheCreationTokens + tokens.CacheReadTokens
		if iv := FindMatchingInterval(pricing.Intervals, totalTokens); iv != nil {
			p = &ChannelModelPricing{
				InputPrice:      iv.InputPrice,
				OutputPrice:     iv.OutputPrice,
				CacheWritePrice: iv.CacheWritePrice,
				CacheReadPrice:  iv.CacheReadPrice,
				PerRequestPrice: iv.PerRequestPrice,
			}
		}
	}
	deref := func(ptr *float64) float64 {
		if ptr == nil {
			return 0
		}
		return *ptr
	}
	cost := float64(tokens.InputTokens)*deref(p.InputPrice) +
		float64(tokens.OutputTokens)*deref(p.OutputPrice) +
		float64(tokens.CacheCreationTokens)*deref(p.CacheWritePrice) +
		float64(tokens.CacheReadTokens)*deref(p.CacheReadPrice) +
		float64(tokens.ImageOutputTokens)*deref(p.ImageOutputPrice)
	if cost <= 0 {
		return nil
	}
	return &cost
}

// applyAccountStatsCost resolves the account stats cost for a usage log entry.
// It resolves the upstream model (falling back to the requested model) and calls
// the 4-level priority chain via resolveAccountStatsCost.
func applyAccountStatsCost(
	ctx context.Context,
	usageLog *UsageLog,
	cs *ChannelService, bs *BillingService,
	accountID int64, groupID int64,
	upstreamModel, requestedModel string,
	tokens UsageTokens,
	totalCost float64,
	accountRateMultiplier float64,
) {
	model := upstreamModel
	if model == "" {
		model = requestedModel
	}
	requestCount := 1
	if usageLog != nil && usageLog.ImageCount > 0 {
		requestCount = usageLog.ImageCount
	}
	billingMode := BillingModeToken
	if usageLog != nil && usageLog.BillingMode != nil && *usageLog.BillingMode != "" {
		billingMode = BillingMode(*usageLog.BillingMode)
	}
	imageSize := ""
	if usageLog != nil && usageLog.ImageSize != nil {
		imageSize = *usageLog.ImageSize
	}
	imageCount := 0
	if usageLog != nil {
		imageCount = usageLog.ImageCount
	}
	serviceTier := ""
	if usageLog != nil && usageLog.ServiceTier != nil {
		serviceTier = *usageLog.ServiceTier
	}
	resolution := resolveAccountStatsCostResolution(ctx, cs, bs, accountID, groupID, model,
		billingMode, imageSize, imageCount, tokens, requestCount, totalCost, serviceTier)
	usageLog.AccountStatsCost = resolution.StatsCost
	finalCost := totalCost * accountRateMultiplier
	if resolution.Matched && resolution.StatsCost != nil {
		finalCost = *resolution.StatsCost
		if resolution.ApplyAccountRate {
			finalCost *= accountRateMultiplier
		}
	}
	usageLog.AccountCost = &finalCost
}
