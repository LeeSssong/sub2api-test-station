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
			name:    "managed snapshot is quantized to persisted four decimal precision",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.249975},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantRate:   testFloat64Ptr(0.25),
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUpdated,
		},
		{
			name:    "quantized managed snapshot matching persisted value is unchanged",
			account: &Account{RateMultiplier: testFloat64Ptr(0.25)},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.249975},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUnchanged,
		},
		{
			name:    "smallest half boundary rounds to minimum persisted positive value",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.00005},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantRate:   testFloat64Ptr(0.0001),
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUpdated,
		},
		{
			name:    "value rounding to zero is rejected",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.000049},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate,
		},
		{
			name:    "value rounding to decimal maximum is accepted",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 999999.99994},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantRate:   testFloat64Ptr(999999.9999),
			wantReason: UpstreamBillingRateMultiplierDecisionReasonUpdated,
		},
		{
			name:    "value overflowing decimal maximum after rounding is rejected",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 999999.99995},
			},
			policy:     UpstreamBillingRateMultiplierPolicyManaged,
			wantReason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate,
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
			name:    "malformed multiplier is rejected",
			account: &Account{RateMultiplier: &managedRate},
			snapshot: &UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": "not-a-number"},
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

func TestValidateUpstreamBillingRateMultiplierPolicyIntentRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		rate   *float64
	}{
		{name: "unknown policy", policy: "automatic"},
		{name: "manual override without multiplier", policy: UpstreamBillingRateMultiplierPolicyManualOverride},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateUpstreamBillingRateMultiplierPolicyIntent(&tt.policy, tt.rate)
			if err == nil {
				t.Fatal("expected invalid policy intent to be rejected")
			}
		})
	}
}

func TestLegacyAccountWithoutPolicyUsesManagedProbeMultiplier(t *testing.T) {
	configuredRate := 0.8
	policy, valid := UpstreamBillingRateMultiplierPolicyFromExtra(map[string]any{})
	decision := DecideUpstreamBillingRateMultiplierSync(&UpstreamBillingProbeSnapshot{
		Status: UpstreamBillingProbeStatusOK,
		Data:   map[string]any{"effective_rate_multiplier": 0.07},
	}, &Account{RateMultiplier: &configuredRate}, policy)

	if !valid {
		t.Fatal("legacy account policy must remain valid")
	}
	if decision.Reason != UpstreamBillingRateMultiplierDecisionReasonUpdated {
		t.Fatalf("decision reason = %q, want %q", decision.Reason, UpstreamBillingRateMultiplierDecisionReasonUpdated)
	}
	if decision.RateMultiplier == nil || *decision.RateMultiplier != 0.07 {
		t.Fatalf("legacy account multiplier decision = %v, want 0.07", decision.RateMultiplier)
	}
}

func testFloat64Ptr(value float64) *float64 {
	return &value
}
