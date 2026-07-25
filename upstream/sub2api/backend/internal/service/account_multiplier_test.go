package service

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestAccountMultiplierResolveDeclaredUsesFreshEffectiveRate(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{
		RateMultiplier: float64Pointer(1),
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data: map[string]any{
					"billing_scope":             "token",
					"resolved_rate_multiplier":  0.17,
					"peak_rate_enabled":         false,
					"effective_rate_multiplier": 0.17,
				},
				ReceivedAt: probeTimePtr(now.Add(-time.Minute)),
				FreshUntil: probeTimePtr(now.Add(time.Hour)),
			},
		},
	}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceDeclared {
		t.Fatalf("Resolve() = %#v", got)
	}
	if got.Value == nil || math.Abs(*got.Value-0.17) > 1e-9 {
		t.Fatalf("Resolve().Value = %#v, want 0.17", got.Value)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("Resolve().ObservedAt = %#v", got.ObservedAt)
	}
}

func TestAccountMultiplierResolveDoesNotUseLocalBillingMultiplier(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{RateMultiplier: float64Pointer(0.08)}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)

	if got.Value != nil || got.Status != AccountMonitorMultiplierStatusUnavailable {
		t.Fatalf("Resolve() = %#v, want unavailable without value", got)
	}
	encoded, err := json.Marshal(AccountMonitorAccount{Multiplier: got})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == `{"multiplier":0.08}` {
		t.Fatalf("projection leaked local billing multiplier: %s", encoded)
	}
}

func TestAccountMultiplierResolveRejectsExpiredAndNonFiniteDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		value  float64
		expiry time.Time
		status string
	}{
		{name: "expired", value: 0.1, expiry: now.Add(-time.Second), status: AccountMonitorMultiplierStatusStale},
		{name: "nan", value: math.NaN(), expiry: now.Add(time.Hour), status: AccountMonitorMultiplierStatusFailed},
		{name: "infinite", value: math.Inf(1), expiry: now.Add(time.Hour), status: AccountMonitorMultiplierStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Extra: map[string]any{
				UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
					Status: UpstreamBillingProbeStatusOK,
					Data: map[string]any{
						"billing_scope":             "token",
						"resolved_rate_multiplier":  tt.value,
						"peak_rate_enabled":         false,
						"effective_rate_multiplier": tt.value,
					},
					ReceivedAt: probeTimePtr(now.Add(-time.Minute)),
					FreshUntil: probeTimePtr(tt.expiry),
				},
			}}

			got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)
			if got.Value != nil || got.Status != tt.status {
				t.Fatalf("Resolve() = %#v, want status %q without value", got, tt.status)
			}
		})
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
