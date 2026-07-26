package accounthealth

import (
	"math"
	"testing"
)

func TestComputeProfitWithMultiplier(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 100, UserCost: 40, Multiplier: float64Ptr(0.15)})
	if !got.Computable {
		t.Fatal("Computable = false, want true")
	}
	if math.Abs(got.UpstreamCost-15) > 1e-9 {
		t.Fatalf("UpstreamCost = %v, want 15", got.UpstreamCost)
	}
	if math.Abs(got.Gross-25) > 1e-9 {
		t.Fatalf("Gross = %v, want 25", got.Gross)
	}
	if got.Margin == nil || math.Abs(*got.Margin-0.625) > 1e-9 {
		t.Fatalf("Margin = %v, want 0.625", got.Margin)
	}
}

func TestComputeProfitWithoutMultiplierIsNotComputable(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 100, UserCost: 40, Multiplier: nil})
	if got.Computable {
		t.Fatal("倍率缺失时不得可核算")
	}
	if got.UpstreamCost != 0 || got.Gross != 0 {
		t.Fatalf("倍率缺失时不得以 1 兜底: %+v", got)
	}
	if got.Margin != nil {
		t.Fatalf("Margin = %v, want nil", got.Margin)
	}
}

func TestComputeProfitZeroRevenueHasNoMargin(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 0, UserCost: 0, Multiplier: float64Ptr(0.2)})
	if !got.Computable {
		t.Fatal("Computable = false, want true")
	}
	if got.Margin != nil {
		t.Fatalf("Margin = %v, want nil (不得除零)", got.Margin)
	}
}

func TestSumProfitExcludesIncomputable(t *testing.T) {
	total, excluded := SumProfit([]ProfitInput{
		{StandardCost: 100, UserCost: 40, Multiplier: float64Ptr(0.15)},
		{StandardCost: 200, UserCost: 60, Multiplier: float64Ptr(0.25)},
		{StandardCost: 500, UserCost: 999, Multiplier: nil},
	})
	if excluded != 1 {
		t.Fatalf("excluded = %d, want 1", excluded)
	}
	if math.Abs(total.Revenue-100) > 1e-9 {
		t.Fatalf("Revenue = %v, want 100 (排除项的收入不得计入)", total.Revenue)
	}
	if math.Abs(total.UpstreamCost-65) > 1e-9 {
		t.Fatalf("UpstreamCost = %v, want 65", total.UpstreamCost)
	}
	if math.Abs(total.Gross-35) > 1e-9 {
		t.Fatalf("Gross = %v, want 35", total.Gross)
	}
	if total.Margin == nil || math.Abs(*total.Margin-0.35) > 1e-9 {
		t.Fatalf("Margin = %v, want 0.35", total.Margin)
	}
}

func TestSumProfitAllIncomputable(t *testing.T) {
	total, excluded := SumProfit([]ProfitInput{{StandardCost: 10, UserCost: 5, Multiplier: nil}})
	if excluded != 1 || total.Computable {
		t.Fatalf("total = %+v, excluded = %d", total, excluded)
	}
}
