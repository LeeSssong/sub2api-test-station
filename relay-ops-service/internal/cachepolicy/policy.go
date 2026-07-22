package cachepolicy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"example.invalid/relay-ops-service/internal/sub2api"
)

type Result struct {
	Ready            bool
	EligibleModels   int
	DiscountedModels int
	Blockers         []string
}

type UsageSummary struct {
	Confirmed           bool
	CacheReadTokens     int64
	CacheCreationTokens int64
	HitRatePercent      float64
}

type modelKey struct {
	groupID int64
	model   string
}

type priceKey struct {
	modelKey
	tier string
}

type priceSignature struct {
	inputSet bool
	input    float64
	readSet  bool
	read     float64
	writeSet bool
	write    float64
}

func Evaluate(groups []sub2api.Group, channels []sub2api.Channel) Result {
	publicGroups := make(map[int64]struct{})
	for _, group := range groups {
		if group.CustomerVisible() && strings.EqualFold(strings.TrimSpace(group.Platform), "openai") {
			publicGroups[group.ID] = struct{}{}
		}
	}

	eligible := make(map[modelKey]struct{})
	invalid := make(map[modelKey]struct{})
	seen := make(map[priceKey]priceSignature)
	blockers := make(map[string]struct{})
	for _, channel := range channels {
		if channel.Status != "active" {
			continue
		}
		for _, groupID := range channel.GroupIDs {
			if _, ok := publicGroups[groupID]; !ok {
				continue
			}
			for _, pricing := range channel.ModelPricing {
				for _, rawModel := range pricing.Models {
					model := strings.ToLower(strings.TrimSpace(rawModel))
					if !strings.HasPrefix(model, "gpt-") {
						continue
					}
					key := modelKey{groupID: groupID, model: model}
					eligible[key] = struct{}{}
					base := signature(pricing.InputPrice, pricing.CacheReadPrice, pricing.CacheWritePrice)
					validatePrice(priceKey{modelKey: key, tier: "base"}, base, seen, invalid, blockers)
					for _, interval := range pricing.Intervals {
						tier := intervalKey(interval)
						entry := signature(interval.InputPrice, interval.CacheReadPrice, interval.CacheWritePrice)
						validatePrice(priceKey{modelKey: key, tier: tier}, entry, seen, invalid, blockers)
					}
				}
			}
		}
	}

	blockerList := make([]string, 0, len(blockers))
	for blocker := range blockers {
		blockerList = append(blockerList, blocker)
	}
	sort.Strings(blockerList)
	result := Result{Ready: len(blockerList) == 0, EligibleModels: len(eligible), Blockers: blockerList}
	for key := range eligible {
		if _, blocked := invalid[key]; !blocked {
			result.DiscountedModels++
		}
	}
	return result
}

func Summarize(usage sub2api.UsageStats) UsageSummary {
	summary := UsageSummary{
		Confirmed:           usage.CacheMetricsPresent,
		CacheReadTokens:     usage.TotalCacheReadTokens,
		CacheCreationTokens: usage.TotalCacheCreationTokens,
	}
	denominator := usage.TotalInputTokens + usage.TotalCacheReadTokens
	if denominator > 0 {
		summary.HitRatePercent = float64(usage.TotalCacheReadTokens) * 100 / float64(denominator)
	}
	return summary
}

func validatePrice(key priceKey, current priceSignature, seen map[priceKey]priceSignature, invalid map[modelKey]struct{}, blockers map[string]struct{}) {
	if previous, ok := seen[key]; ok && previous != current {
		addBlocker("conflicting_cache_pricing", key, invalid, blockers)
	} else if !ok {
		seen[key] = current
	}
	if !current.inputSet || current.input <= 0 {
		addBlocker("input_price_invalid", key, invalid, blockers)
	}
	if !current.readSet {
		addBlocker("cache_read_price_missing", key, invalid, blockers)
	} else if current.read < 0 || !current.inputSet || current.read >= current.input {
		addBlocker("cache_read_not_discounted", key, invalid, blockers)
	}
	if strings.HasPrefix(key.model, "gpt-5.6-") {
		if !current.writeSet {
			addBlocker("cache_write_price_missing", key, invalid, blockers)
		} else if current.write < 0 {
			addBlocker("cache_write_price_invalid", key, invalid, blockers)
		}
	}
}

func addBlocker(code string, key priceKey, invalid map[modelKey]struct{}, blockers map[string]struct{}) {
	invalid[key.modelKey] = struct{}{}
	blockers[fmt.Sprintf("%s:group=%d:model=%s:tier=%s", code, key.groupID, key.model, key.tier)] = struct{}{}
}

func signature(input, read, write *float64) priceSignature {
	result := priceSignature{}
	if input != nil {
		result.inputSet, result.input = true, *input
	}
	if read != nil {
		result.readSet, result.read = true, *read
	}
	if write != nil {
		result.writeSet, result.write = true, *write
	}
	return result
}

func intervalKey(interval sub2api.ChannelModelPriceInterval) string {
	maximum := "max"
	if interval.MaxTokens != nil {
		maximum = strconv.FormatInt(*interval.MaxTokens, 10)
	}
	label := strings.TrimSpace(interval.TierLabel)
	if label == "" {
		label = "range"
	}
	return fmt.Sprintf("%s:%d-%s", label, interval.MinTokens, maximum)
}
