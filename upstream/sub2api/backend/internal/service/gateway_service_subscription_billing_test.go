//go:build unit

package service

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier locks in the fix
// that subscription-mode billing honours the group (and any user-specific) rate
// multiplier — i.e. cmd.SubscriptionCost tracks ActualCost (= TotalCost *
// RateMultiplier), not raw TotalCost.
func TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	subID := int64(42)

	tests := []struct {
		name           string
		totalCost      float64
		actualCost     float64
		isSubscription bool
		wantSub        float64
		wantBalance    float64
	}{
		{
			name:           "subscription with 2x multiplier consumes 2x quota",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: true,
			wantSub:        2.0,
			wantBalance:    0,
		},
		{
			name:           "subscription with 0.5x multiplier consumes 0.5x quota",
			totalCost:      1.0,
			actualCost:     0.5,
			isSubscription: true,
			wantSub:        0.5,
			wantBalance:    0,
		},
		{
			name:           "free subscription (multiplier 0) consumes no quota",
			totalCost:      1.0,
			actualCost:     0,
			isSubscription: true,
			wantSub:        0,
			wantBalance:    0,
		},
		{
			name:           "balance billing keeps using ActualCost (regression)",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: false,
			wantSub:        0,
			wantBalance:    2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &postUsageBillingParams{
				Cost:               &CostBreakdown{TotalCost: tt.totalCost, ActualCost: tt.actualCost},
				User:               &User{ID: 1},
				APIKey:             &APIKey{ID: 2, GroupID: &groupID},
				Account:            &Account{ID: 3},
				UsageCompleteness:  UsageCompletenessComplete,
				Subscription:       &UserSubscription{ID: subID},
				IsSubscriptionBill: tt.isSubscription,
			}

			cmd := buildUsageBillingCommand("req-1", nil, p)
			if cmd == nil {
				t.Fatal("buildUsageBillingCommand returned nil")
			}
			if cmd.SubscriptionCost != tt.wantSub {
				t.Errorf("SubscriptionCost = %v, want %v", cmd.SubscriptionCost, tt.wantSub)
			}
			if cmd.BalanceCost != tt.wantBalance {
				t.Errorf("BalanceCost = %v, want %v", cmd.BalanceCost, tt.wantBalance)
			}
		})
	}
}

func TestBuildUsageBillingCommand_UsesResolvedAccountCostForAccountQuota(t *testing.T) {
	groupID := int64(7)
	account := &Account{
		ID:   3,
		Type: AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 10.0,
		},
	}
	p := &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 0.90, ActualCost: 0.45},
		User:                  &User{ID: 1},
		APIKey:                &APIKey{ID: 2, GroupID: &groupID},
		Account:               account,
		UsageCompleteness:     UsageCompletenessComplete,
		AccountRateMultiplier: 0.5,
		AccountCost:           0.24,
		AccountCostSet:        true,
	}

	cmd := buildUsageBillingCommand("req-fixed-image", nil, p)
	if cmd == nil {
		t.Fatal("buildUsageBillingCommand returned nil")
	}
	if cmd.AccountQuotaCost != 0.24 {
		t.Errorf("AccountQuotaCost = %v, want 0.24", cmd.AccountQuotaCost)
	}
	if cmd.BalanceCost != 0.45 {
		t.Errorf("BalanceCost = %v, want 0.45", cmd.BalanceCost)
	}

	p.AccountCostSet = false
	p.AccountCost = 0
	legacy := buildUsageBillingCommand("req-legacy", nil, p)
	if legacy.AccountQuotaCost != 0.45 {
		t.Errorf("legacy AccountQuotaCost = %v, want 0.45", legacy.AccountQuotaCost)
	}
}

func TestNotifyAccountQuota_UsesResolvedAccountCost(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	p := &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 0.90, ActualCost: 0.45},
		Account:               &Account{ID: 3, Type: AccountTypeAPIKey},
		AccountRateMultiplier: 0.5,
		AccountCost:           0.24,
		AccountCostSet:        true,
	}
	notifyAccountQuota(p, &billingDeps{balanceNotifyService: &BalanceNotifyService{}}, nil)

	if !strings.Contains(logs.String(), "account_cost=0.24") {
		t.Fatalf("notifyAccountQuota log = %q, want resolved account_cost=0.24", logs.String())
	}
}
