package service

import (
	"math"
	"strings"
)

// Effective cost model keys are kept in accounts.extra so this additive slice
// remains compatible with the existing Ent account projection and JSONB merge
// semantics. They are intentionally generic and do not name a provider.
const (
	EffectiveCostModelExtraKey           = "effective_cost_model"
	UpstreamActualCostExtraKey           = "upstream_actual_cost"
	UpstreamObtainedQuotaExtraKey        = "upstream_obtained_quota"
	EffectiveCostModelDirectMultiplier   = "direct_multiplier"
	EffectiveCostModelRatioBasedUpstream = "ratio_based_upstream"
	EffectiveCostModelSelfOwned          = "self_owned"
	EffectiveCostStatusReady             = "ready"
	EffectiveCostStatusUnknown           = "unknown"
)

type EffectiveCost struct {
	Model  string   `json:"model"`
	Status string   `json:"status"`
	A      *float64 `json:"a,omitempty"`
	R      *float64 `json:"r,omitempty"`
	U      *float64 `json:"u,omitempty"`
}

func EffectiveCostForAccount(account *Account) EffectiveCost {
	if account == nil {
		return EffectiveCost{Status: EffectiveCostStatusUnknown}
	}
	// OAuth is a locked self-owned model. Legacy rate_multiplier and any stale
	// ratio keys must never become an upstream return multiplier for OAuth.
	if account.Type == AccountTypeOAuth {
		cost, quota := account.ProcurementCostCNY, account.EstimatedUsableQuotaUSD
		if !validNonNegative(cost) || !validPositive(quota) {
			return EffectiveCost{Model: EffectiveCostModelSelfOwned, Status: EffectiveCostStatusUnknown}
		}
		a := *cost / *quota
		if !finiteNonNegative(a) {
			return EffectiveCost{Model: EffectiveCostModelSelfOwned, Status: EffectiveCostStatusUnknown}
		}
		return EffectiveCost{Model: EffectiveCostModelSelfOwned, Status: EffectiveCostStatusReady, A: &a, U: &a}
	}

	model := strings.TrimSpace(account.EffectiveCostModel)
	if model == "" {
		model = EffectiveCostModelDirectMultiplier
	}
	r := account.RateMultiplier
	if !validNonNegative(r) {
		return EffectiveCost{Model: model, Status: EffectiveCostStatusUnknown}
	}
	if model == EffectiveCostModelRatioBasedUpstream {
		actual, quota := account.UpstreamActualCost, account.UpstreamObtainedQuota
		if !validNonNegative(actual) || !validPositive(quota) {
			return EffectiveCost{Model: model, Status: EffectiveCostStatusUnknown}
		}
		a := *actual / *quota
		u := a * *r
		if !finiteNonNegative(a) || !finiteNonNegative(u) {
			return EffectiveCost{Model: model, Status: EffectiveCostStatusUnknown}
		}
		return EffectiveCost{Model: model, Status: EffectiveCostStatusReady, A: &a, R: r, U: &u}
	}
	if model != EffectiveCostModelDirectMultiplier {
		return EffectiveCost{Model: model, Status: EffectiveCostStatusUnknown}
	}
	a := float64(1)
	u := *r
	return EffectiveCost{Model: model, Status: EffectiveCostStatusReady, A: &a, R: r, U: &u}
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
func validNonNegative(value *float64) bool { return value != nil && finiteNonNegative(*value) }
func validPositive(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value > 0
}

// NormalizeEffectiveCostModel enforces the account-type invariants before an
// admin mutation is persisted. OAuth always becomes self_owned; API keys use
// direct_multiplier when the model is omitted.
func NormalizeEffectiveCostModel(accountType, requested string, actual, quota, rate *float64) (string, error) {
	model := strings.TrimSpace(requested)
	if accountType == AccountTypeOAuth {
		if model != "" && model != EffectiveCostModelSelfOwned {
			return "", ErrInvalidEffectiveCostModel
		}
		return EffectiveCostModelSelfOwned, nil
	}
	if model == "" {
		model = EffectiveCostModelDirectMultiplier
	}
	if model != EffectiveCostModelDirectMultiplier && model != EffectiveCostModelRatioBasedUpstream {
		return "", ErrInvalidEffectiveCostModel
	}
	if model == EffectiveCostModelRatioBasedUpstream {
		if !validNonNegative(actual) || !validPositive(quota) {
			return "", ErrInvalidEffectiveCostRatio
		}
	}
	return model, nil
}

func applyEffectiveCostConfiguration(account *Account, requested string, actual, quota *float64) error {
	if account == nil {
		return nil
	}
	model, err := NormalizeEffectiveCostModel(account.Type, requested, actual, quota, account.RateMultiplier)
	if err != nil {
		return err
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.EffectiveCostModel = model
	account.Extra[EffectiveCostModelExtraKey] = model
	if model == EffectiveCostModelRatioBasedUpstream {
		account.UpstreamActualCost = actual
		account.UpstreamObtainedQuota = quota
		account.Extra[UpstreamActualCostExtraKey] = *actual
		account.Extra[UpstreamObtainedQuotaExtraKey] = *quota
		return nil
	}
	account.UpstreamActualCost = nil
	account.UpstreamObtainedQuota = nil
	delete(account.Extra, UpstreamActualCostExtraKey)
	delete(account.Extra, UpstreamObtainedQuotaExtraKey)
	return nil
}
