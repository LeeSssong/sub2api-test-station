package service

import (
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

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
// accounts.extra. An absent or nil policy is the managed default for accounts
// created before this policy existed. Explicit manual overrides are persisted
// by the account admin write paths. Invalid values are rejected instead of
// silently falling back to managed mode, so a malformed setting cannot
// overwrite a manually configured multiplier.
func UpstreamBillingRateMultiplierPolicyFromExtra(extra map[string]any) (string, bool) {
	if len(extra) == 0 {
		return UpstreamBillingRateMultiplierPolicyManaged, true
	}
	value, ok := extra[UpstreamBillingRateMultiplierPolicyExtraKey]
	if !ok || value == nil {
		return UpstreamBillingRateMultiplierPolicyManaged, true
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

func validateUpstreamBillingRateMultiplierPolicyIntent(policy *string, rateMultiplier *float64) (string, error) {
	if rateMultiplier != nil && (*rateMultiplier < 0 || math.IsNaN(*rateMultiplier) || math.IsInf(*rateMultiplier, 0)) {
		return "", infraerrors.BadRequest(
			"INVALID_UPSTREAM_BILLING_RATE_MULTIPLIER",
			"rate_multiplier must be a finite value >= 0",
		)
	}
	if policy == nil {
		return "", nil
	}
	switch *policy {
	case UpstreamBillingRateMultiplierPolicyManaged:
		return *policy, nil
	case UpstreamBillingRateMultiplierPolicyManualOverride:
		if rateMultiplier == nil {
			return "", infraerrors.BadRequest(
				"INVALID_UPSTREAM_BILLING_RATE_MULTIPLIER_POLICY",
				"rate_multiplier is required when rate_multiplier_policy is manual_override",
			)
		}
		return *policy, nil
	default:
		return "", infraerrors.BadRequest(
			"INVALID_UPSTREAM_BILLING_RATE_MULTIPLIER_POLICY",
			"rate_multiplier_policy must be upstream_managed or manual_override",
		)
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
