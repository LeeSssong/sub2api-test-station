package cachepolicy

import (
	"math"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestEvaluateRequiresDiscountedCacheReadForPublicGPTModels(t *testing.T) {
	input, read, write := 5e-6, 0.5e-6, 6.25e-6
	result := Evaluate(
		[]sub2api.Group{{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
		[]sub2api.Channel{{ID: 7, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{
			Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &read, CacheWritePrice: &write,
		}}}},
	)
	if !result.Ready || result.EligibleModels != 1 || result.DiscountedModels != 1 || len(result.Blockers) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestEvaluateRejectsUnsafeCachePricing(t *testing.T) {
	input, read, same, write := 5e-6, 0.5e-6, 5e-6, 6.25e-6
	tests := []struct {
		name    string
		pricing sub2api.ChannelModelPrice
		blocker string
	}{
		{name: "missing read", pricing: sub2api.ChannelModelPrice{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheWritePrice: &write}, blocker: "cache_read_price_missing"},
		{name: "read not discounted", pricing: sub2api.ChannelModelPrice{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &same, CacheWritePrice: &write}, blocker: "cache_read_not_discounted"},
		{name: "missing 5.6 write", pricing: sub2api.ChannelModelPrice{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &read}, blocker: "cache_write_price_missing"},
		{name: "interval incomplete", pricing: sub2api.ChannelModelPrice{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &read, CacheWritePrice: &write, Intervals: []sub2api.ChannelModelPriceInterval{{MinTokens: 272001, InputPrice: &input}}}, blocker: "cache_read_price_missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Evaluate(
				[]sub2api.Group{{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
				[]sub2api.Channel{{ID: 7, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{tt.pricing}}},
			)
			if result.Ready || !hasBlocker(result.Blockers, tt.blocker) {
				t.Fatalf("result = %#v, want blocker %q", result, tt.blocker)
			}
		})
	}
}

func TestEvaluateIgnoresPrivateInactiveAndNonGPTPricing(t *testing.T) {
	input, read := 5e-6, 0.5e-6
	result := Evaluate(
		[]sub2api.Group{
			{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active"},
			{ID: 3, Name: "private", Platform: "openai", Status: "active", IsExclusive: true},
		},
		[]sub2api.Channel{
			{ID: 7, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{Models: []string{"claude-sonnet"}, InputPrice: &input}}},
			{ID: 8, Status: "disabled", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{Models: []string{"gpt-disabled"}, InputPrice: &input}}},
			{ID: 9, Status: "active", GroupIDs: []int64{3}, ModelPricing: []sub2api.ChannelModelPrice{{Models: []string{"gpt-private"}, InputPrice: &input, CacheReadPrice: &read}}},
		},
	)
	if !result.Ready || result.EligibleModels != 0 || len(result.Blockers) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestEvaluateRejectsContradictoryDuplicatePricing(t *testing.T) {
	input, readA, readB, write := 5e-6, 0.5e-6, 0.4e-6, 6.25e-6
	result := Evaluate(
		[]sub2api.Group{{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
		[]sub2api.Channel{
			{ID: 7, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &readA, CacheWritePrice: &write}}},
			{ID: 8, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &readB, CacheWritePrice: &write}}},
		},
	)
	if result.Ready || !hasBlocker(result.Blockers, "conflicting_cache_pricing") {
		t.Fatalf("result = %#v", result)
	}
}

func TestSummarizeCalculatesCacheHitRate(t *testing.T) {
	summary := Summarize(sub2api.UsageStats{
		TotalInputTokens: 1000, TotalCacheReadTokens: 3000, TotalCacheCreationTokens: 500,
		CacheMetricsPresent: true,
	})
	if !summary.Confirmed || summary.CacheReadTokens != 3000 || summary.CacheCreationTokens != 500 || summary.HitRatePercent != 75 {
		t.Fatalf("summary = %#v", summary)
	}

	unconfirmed := Summarize(sub2api.UsageStats{TotalInputTokens: 1000, TotalCacheReadTokens: 3000})
	if unconfirmed.Confirmed {
		t.Fatalf("unconfirmed summary = %#v", unconfirmed)
	}

	empty := Summarize(sub2api.UsageStats{CacheMetricsPresent: true})
	if math.IsNaN(empty.HitRatePercent) || math.IsInf(empty.HitRatePercent, 0) || empty.HitRatePercent != 0 {
		t.Fatalf("empty summary = %#v", empty)
	}
}

func hasBlocker(blockers []string, prefix string) bool {
	for _, blocker := range blockers {
		if strings.HasPrefix(blocker, prefix) {
			return true
		}
	}
	return false
}
