package service

import "math"

const (
	// UpstreamBillingRateMultiplierPolicyExtraKey stores the explicit pricing
	// synchronization policy in accounts.extra. Missing values are managed for
	// backward compatibility with accounts created before this policy existed.
	UpstreamBillingRateMultiplierPolicyExtraKey = "upstream_billing_rate_multiplier_policy"

	UpstreamBillingRateMultiplierPolicyManaged        = "upstream_managed"
	UpstreamBillingRateMultiplierPolicyManualOverride = "manual_override"

	UpstreamBillingRateMultiplierDecisionReasonUpdated              = "updated"
	UpstreamBillingRateMultiplierDecisionReasonUnchanged            = "unchanged"
	UpstreamBillingRateMultiplierDecisionReasonManualOverride       = "manual_override"
	UpstreamBillingRateMultiplierDecisionReasonMissingEffectiveRate = "missing_effective_rate_multiplier"
	UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate = "invalid_effective_rate_multiplier"
	UpstreamBillingRateMultiplierDecisionReasonInvalidProbe         = "invalid_probe_snapshot"
	UpstreamBillingRateMultiplierDecisionReasonInvalidPolicy        = "invalid_policy"
	UpstreamBillingRateMultiplierDecisionReasonMissingAccount       = "missing_account"

	// Account.rate_multiplier is currently persisted as decimal(10,4).
	upstreamBillingRateMultiplierMax   = 999999.9999
	upstreamBillingRateMultiplierScale = 10000.0
)

// UpstreamBillingRateMultiplierDecision describes whether a validated probe
// value should be written to an account. A nil RateMultiplier always means no
// write should be performed.
type UpstreamBillingRateMultiplierDecision struct {
	RateMultiplier *float64
	Reason         string
}

// UpstreamBillingRateMultiplierPolicyFromExtra reads the explicit policy from
// accounts.extra. An absent or nil policy is a legacy manual override: older
// accounts may already carry an operator-configured multiplier, so a probe
// must never overwrite it until an administrator explicitly opts the account
// into upstream-managed pricing. Invalid values are rejected instead of
// silently falling back to managed mode, so a malformed setting cannot
// overwrite a manually configured multiplier.
func UpstreamBillingRateMultiplierPolicyFromExtra(extra map[string]any) (string, bool) {
	if len(extra) == 0 {
		return UpstreamBillingRateMultiplierPolicyManualOverride, true
	}
	value, ok := extra[UpstreamBillingRateMultiplierPolicyExtraKey]
	if !ok || value == nil {
		return UpstreamBillingRateMultiplierPolicyManualOverride, true
	}
	policy, ok := value.(string)
	if !ok {
		return "", false
	}
	switch policy {
	case UpstreamBillingRateMultiplierPolicyManaged, UpstreamBillingRateMultiplierPolicyManualOverride:
		return policy, true
	default:
		return "", false
	}
}

// DecideUpstreamBillingRateMultiplierSync validates a native billing probe
// value and returns an idempotent managed-account update decision. It is pure:
// callers remain responsible for persistence, auditing, cache invalidation,
// and lifecycle/scheduler wiring.
func DecideUpstreamBillingRateMultiplierSync(
	snapshot *UpstreamBillingProbeSnapshot,
	account *Account,
	policy string,
) UpstreamBillingRateMultiplierDecision {
	if account == nil {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonMissingAccount}
	}
	if policy == "" {
		policy = UpstreamBillingRateMultiplierPolicyManaged
	}
	switch policy {
	case UpstreamBillingRateMultiplierPolicyManualOverride:
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonManualOverride}
	case UpstreamBillingRateMultiplierPolicyManaged:
	default:
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonInvalidPolicy}
	}
	if snapshot == nil || snapshot.Status != UpstreamBillingProbeStatusOK {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonInvalidProbe}
	}
	if snapshot.Data == nil {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonMissingEffectiveRate}
	}
	if _, exists := snapshot.Data["effective_rate_multiplier"]; !exists {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonMissingEffectiveRate}
	}
	rate, ok := resolveAccountExtraNumber(snapshot.Data, "effective_rate_multiplier")
	if !ok {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate}
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate}
	}
	// Match PostgreSQL decimal(10,4) persistence before deciding whether a
	// write is necessary. Probe values are positive, so math.Round's half-away
	// behavior is the same as decimal half-up rounding at the storage boundary.
	rate = math.Round(rate*upstreamBillingRateMultiplierScale) / upstreamBillingRateMultiplierScale
	if rate <= 0 || rate > upstreamBillingRateMultiplierMax {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonInvalidEffectiveRate}
	}
	if equalBillingMultiplier(account.BillingRateMultiplier(), rate) {
		return UpstreamBillingRateMultiplierDecision{Reason: UpstreamBillingRateMultiplierDecisionReasonUnchanged}
	}
	return UpstreamBillingRateMultiplierDecision{
		RateMultiplier: &rate,
		Reason:         UpstreamBillingRateMultiplierDecisionReasonUpdated,
	}
}
