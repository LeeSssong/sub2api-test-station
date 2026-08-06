package reconciliation

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
	"github.com/shopspring/decimal"
)

type costGuardEvidenceRepo struct{ evidence CostGuardEvidence }

func (r costGuardEvidenceRepo) ReadCostGuardEvidence(context.Context, CostGuardQuery) (CostGuardEvidence, error) {
	return r.evidence, nil
}

type costGuardAccounts struct{}

func (costGuardAccounts) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return []sub2api.Account{{ID: 42, Name: "mapped-account"}}, nil
}

func TestCostGuardRequiresSixReconciledSamplesForConfirmedLoss(t *testing.T) {
	ratio := decimal.RequireFromString("0.70")
	service := CostGuardService{
		Repository: costGuardEvidenceRepo{evidence: CostGuardEvidence{
			Model: "model-a", EquivalentSiteMultiplier: &ratio, SampleCount: 5,
			CostSource: "reconciled_bill", ObservedAt: time.Now().UTC(),
		}},
	}
	observing, err := service.ReadCostGuard(context.Background(), CostGuardQuery{
		AccountID: 42, GroupID: 7, GroupMultiplier: decimal.RequireFromString("0.50"),
	})
	if err != nil || observing.Status != "loss_observing" || observing.SampleCount != 5 {
		t.Fatalf("observing = %#v err=%v", observing, err)
	}
	service.Repository = costGuardEvidenceRepo{evidence: CostGuardEvidence{
		Model: "model-a", EquivalentSiteMultiplier: &ratio, SampleCount: 6,
		CostSource: "reconciled_bill", ObservedAt: time.Now().UTC(),
	}}
	confirmed, err := service.ReadCostGuard(context.Background(), CostGuardQuery{
		AccountID: 42, GroupID: 7, GroupMultiplier: decimal.RequireFromString("0.50"),
	})
	if err != nil || confirmed.Status != "loss_confirmed" {
		t.Fatalf("confirmed = %#v err=%v", confirmed, err)
	}
}

func TestCostGuardUsesNativeResolverWithoutChangingBillingVerdict(t *testing.T) {
	ratio := decimal.RequireFromString("0.20")
	service := CostGuardService{
		Repository: costGuardEvidenceRepo{evidence: CostGuardEvidence{
			Model: "model-a", EquivalentSiteMultiplier: &ratio, SampleCount: 6,
			CostSource: "reconciled_bill", ObservedAt: time.Now().UTC(),
		}},
		Accounts: costGuardAccounts{},
		NativeResolve: func(context.Context, string) (float64, string, bool) {
			return 0.17, "upstream_pricing", true
		},
	}
	result, err := service.ReadCostGuard(context.Background(), CostGuardQuery{
		AccountID: 42, GroupID: 7, GroupMultiplier: decimal.RequireFromString("0.25"),
	})
	if err != nil || result.UpstreamMultiplier == nil || !result.UpstreamMultiplier.Equal(decimal.RequireFromString("0.17")) ||
		result.UpstreamMultiplierSource != "upstream_pricing" || result.Status != "cost_covered" {
		t.Fatalf("result = %#v err=%v", result, err)
	}
}
