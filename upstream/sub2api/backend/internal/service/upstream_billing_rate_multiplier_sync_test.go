package service

import (
	"math"
	"testing"
)

func TestDecideUpstreamBillingRateMultiplierSync(t *testing.T) {
	managedRate := 1.0
	currentRate := 0.07

	tests := []struct {
		name       string
		account    *Account
		snapshot   *UpstreamBillingProbeSnapshot
		policy     string
		wantRate   *float64
		wantReason string
	}{
		{
			name:    "valid managed snapshot updates multiplier",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.07},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantRate:   testFloat64Ptr(0.07),
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUpdated,
		},
		{
			name:    "zero multiplier is rejected",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.0},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate,
		},
		{
			name:    "non-finite multiplier is rejected",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": math.NaN()},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate,
		},
		{
			name:    "unchanged managed snapshot is a no-op",
			account: &Account{RateMultiplier: &currentRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.07},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUnchanged,
		},
		{
			name:    "manual override keeps configured multiplier",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.07},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManualOverride,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonManualOverride,
		},
		{
			name:    "missing effective multiplier does not fall back to related fields",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data: map[string]any{
					"resolved_rate_multiplier": 0.07,
					"group_rate_multiplier":    0.06,
				},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonMissingEffectiveRate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideUpstreamBillingRateMultiplierSync(tt.snapshot, tt.account, tt.policy)
			if decision.Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", decision.Reason, tt.wantReason)
			}
			if tt.wantRate == nil {
				if decision.RateMultiplier != nil {
					t.Fatalf("rate multiplier = %v, want nil", *decision.RateMultiplier)
				}
				return
			}
			if decision.RateMultiplier == nil {
				t.Fatal("rate multiplier = nil, want update")
			}
			if *decision.RateMultiplier != *tt.wantRate {
				t.Fatalf("rate multiplier = %v, want %v", *decision.RateMultiplier, *tt.wantRate)
			}
		})
	}
}

func TestUpstreamBillingRateMultiplierPolicyFromExtra(t *testing.T) {
	tests := []struct {
		name       string
		extra      map[string]any
		wantPolicy string
		wantValid  bool
	}{
		{
			name:       "missing policy defaults to upstream managed",
			extra:      map[string]any{},
			wantPolicy: UpstreamBillingRateMultiplierPolicyManaged,
			wantValid:  true,
		},
		{
			name: "explicit manual override is retained",
			extra: map[string]any{
				UpstreamBillingRateMultiplierPolicyExtraKey: UpstreamBillingRateMultiplierPolicyManualOverride,
			},
			wantPolicy: UpstreamBillingRateMultiplierPolicyManualOverride,
			wantValid:  true,
		},
		{
			name: "malformed policy is rejected",
			extra: map[string]any{
				UpstreamBillingRateMultiplierPolicyExtraKey: true,
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPolicy, gotValid := UpstreamBillingRateMultiplierPolicyFromExtra(tt.extra)
			if gotPolicy != tt.wantPolicy || gotValid != tt.wantValid {
				t.Fatalf("policy, valid = %q, %t; want %q, %t", gotPolicy, gotValid, tt.wantPolicy, tt.wantValid)
			}
		})
	}
}

func testFloat64Ptr(value float64) *float64 {
	return &value
}
