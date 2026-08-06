package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
	"github.com/shopspring/decimal"
)

const RequiredCostSamples int64 = 6

type CostGuardQuery struct {
	AccountID       int64
	GroupID         int64
	GroupMultiplier decimal.Decimal
}

type CostGuardEvidence struct {
	Model                    string
	EquivalentSiteMultiplier *decimal.Decimal
	SampleCount              int64
	CostSource               string
	ObservedAt               time.Time
}

type CostGuard struct {
	UpstreamMultiplier       *decimal.Decimal `json:"upstream_multiplier"`
	UpstreamMultiplierSource string           `json:"upstream_multiplier_source"`
	EquivalentSiteMultiplier *decimal.Decimal `json:"equivalent_site_multiplier"`
	CostSource               string           `json:"cost_source"`
	Model                    string           `json:"model"`
	SampleCount              int64            `json:"sample_count"`
	RequiredSampleCount      int64            `json:"required_sample_count"`
	GroupMultiplier          decimal.Decimal  `json:"group_multiplier"`
	Gap                      *decimal.Decimal `json:"gap,omitempty"`
	Status                   string           `json:"status"`
	ObservedAt               time.Time        `json:"observed_at"`
}

type CostGuardRepository interface {
	ReadCostGuardEvidence(context.Context, CostGuardQuery) (CostGuardEvidence, error)
}

// CostGuardService computes the current group verdict from immutable request
// evidence. Native multiplier lookup is deliberately injected so New API's
// generic account→upstream-group resolver remains the single source of truth.
type CostGuardService struct {
	Repository    CostGuardRepository
	Accounts      sub2api.AccountReader
	Monitors      sub2api.AccountMonitorReader
	NativeResolve func(context.Context, string) (float64, string, bool)
}

func (s CostGuardService) ReadCostGuard(ctx context.Context, query CostGuardQuery) (CostGuard, error) {
	if s.Repository == nil {
		return CostGuard{}, fmt.Errorf("cost guard repository is required")
	}
	if query.AccountID <= 0 || query.GroupID <= 0 || query.GroupMultiplier.IsNegative() {
		return CostGuard{}, fmt.Errorf("cost guard query is invalid")
	}
	evidence, err := s.Repository.ReadCostGuardEvidence(ctx, query)
	if err != nil {
		return CostGuard{}, err
	}
	result := CostGuard{
		RequiredSampleCount:      RequiredCostSamples,
		GroupMultiplier:          query.GroupMultiplier,
		Model:                    evidence.Model,
		SampleCount:              evidence.SampleCount,
		CostSource:               evidence.CostSource,
		EquivalentSiteMultiplier: evidence.EquivalentSiteMultiplier,
		ObservedAt:               evidence.ObservedAt,
		Status:                   "unknown",
	}
	if result.EquivalentSiteMultiplier != nil {
		gap := result.EquivalentSiteMultiplier.Sub(query.GroupMultiplier)
		result.Gap = &gap
		higher := gap.GreaterThan(decimal.Zero)
		if higher && evidence.SampleCount >= RequiredCostSamples {
			result.Status = "loss_confirmed"
		} else if higher && evidence.SampleCount > 0 {
			result.Status = "loss_observing"
		} else if evidence.SampleCount >= RequiredCostSamples && gap.Abs().LessThanOrEqual(decimal.RequireFromString("0.000001")) {
			result.Status = "zero_margin"
		} else if evidence.SampleCount >= RequiredCostSamples {
			result.Status = "cost_covered"
		} else if evidence.SampleCount > 0 {
			result.Status = "insufficient_samples"
		}
		if evidence.CostSource != "reconciled_bill" && higher {
			result.Status = "pricing_risk"
		}
	}
	// Resolve the generic upstream pricing mapping first. This gives New API's
	// native /api/pricing group_ratio priority over any measured fallback.
	if s.Accounts != nil {
		accounts, accountErr := s.Accounts.ListAccounts(ctx)
		if accountErr == nil {
			for _, account := range accounts {
				if account.ID != query.AccountID {
					continue
				}
				if s.NativeResolve != nil {
					if value, source, ok := s.NativeResolve(ctx, account.Name); ok {
						v := decimal.NewFromFloat(value)
						result.UpstreamMultiplier = &v
						result.UpstreamMultiplierSource = source
					} else {
						result.UpstreamMultiplierSource = "unknown"
					}
				}
				break
			}
		}
	}
	if result.UpstreamMultiplier == nil && s.Monitors != nil {
		if projection, monitorErr := s.Monitors.ListAccountMonitors(ctx); monitorErr == nil {
			for _, account := range projection.Accounts {
				if account.AccountID != query.AccountID || account.Multiplier.Status != "ok" || account.Multiplier.Value == nil {
					continue
				}
				value := decimal.NewFromFloat(*account.Multiplier.Value)
				result.UpstreamMultiplier = &value
				if account.Multiplier.Source == "declared" {
					result.UpstreamMultiplierSource = "upstream_declared"
				} else if account.Multiplier.Source == "measured" {
					result.UpstreamMultiplierSource = "quota_measurement"
				} else {
					result.UpstreamMultiplierSource = "upstream_pricing"
				}
				break
			}
		}
	}
	if result.UpstreamMultiplierSource == "" {
		result.UpstreamMultiplierSource = "unknown"
	}
	return result, nil
}

func CostSourceLabel(source string) string {
	switch strings.TrimSpace(source) {
	case "reconciled_bill":
		return "账单实测"
	case "upstream_pricing":
		return "上游定价推算"
	case "quota_measurement":
		return "额度测得"
	default:
		return "待确认"
	}
}
