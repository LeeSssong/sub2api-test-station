package httpserver

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestNativePricingFormatsPerMillionAndIncludesIntervals(t *testing.T) {
	input := 0.000005
	output := 0.000030
	cacheRead := 0.0000005
	cacheWrite := 0.00000625
	tierInput := 0.000010
	tierOutput := 0.000045
	reader := pricingReader{
		groups: []sub2api.Group{{ID: 3, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
		channels: []sub2api.Channel{{
			ID: 7, Name: "Neko", Status: "active", GroupIDs: []int64{3},
			ModelPricing: []sub2api.ChannelModelPrice{{
				Models: []string{"gpt-5.6-sol"}, InputPrice: &input, OutputPrice: &output,
				CacheReadPrice: &cacheRead, CacheWritePrice: &cacheWrite,
				Intervals: []sub2api.ChannelModelPriceInterval{{
					MinTokens: 272000, InputPrice: &tierInput, OutputPrice: &tierOutput,
				}},
			}},
		}},
	}
	source := NativePricingSource{Reader: reader, Clock: func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }}

	groups, err := source.PublicPricing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Models) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	base := groups[0].Models[0]
	if base.Input != "5.00" || base.Output != "30.00" || base.CacheRead != "0.50" || base.CacheWrite != "6.25" {
		t.Fatalf("base = %#v", base)
	}
	tier := groups[0].Models[1]
	if tier.Tier != ">272k" || tier.Input != "10.00" || tier.Output != "45.00" {
		t.Fatalf("tier = %#v", tier)
	}
}

type pricingReader struct {
	channels []sub2api.Channel
	groups   []sub2api.Group
}

func (r pricingReader) ListChannels(context.Context) ([]sub2api.Channel, error) {
	return r.channels, nil
}
func (r pricingReader) ListGroups(context.Context) ([]sub2api.Group, error) { return r.groups, nil }
func (pricingReader) ListChannelMonitors(context.Context) ([]sub2api.ChannelMonitor, error) {
	return nil, nil
}
func (pricingReader) GetChannelMonitorHistory(context.Context, int64, string, int) ([]sub2api.MonitorHistory, error) {
	return nil, nil
}
func (pricingReader) GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return sub2api.OpsSnapshot{}, nil
}
func (pricingReader) GetUsageStats(context.Context, sub2api.UsageQuery) (sub2api.UsageStats, error) {
	return sub2api.UsageStats{}, nil
}
