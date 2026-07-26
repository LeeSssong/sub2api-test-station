// Package accounthealth turns raw Sub2API account monitor evidence into
// operator-facing health verdicts. Every function here is pure: no IO, no
// clock reads beyond explicitly passed values.
package accounthealth

type Tier string

const (
	TierHealthy     Tier = "healthy"
	TierDegraded    Tier = "degraded"
	TierUnavailable Tier = "unavailable"
	TierUnknown     Tier = "unknown"
)

const (
	HealthyMinSuccessRate     = 0.95
	DegradedMinSuccessRate    = 0.50
	SlowTTFTP95MS             = 3000.0
	ErrorCodeBalanceExhausted = "balance_exhausted"
)

type AccountSample struct {
	AccountID   int64
	Name        string
	GroupNames  []string
	SuccessRate float64
	SampleCount int
	TTFTP95MS   *float64
	ErrorCode   string
}

type AccountVerdict struct {
	AccountID  int64
	Name       string
	GroupNames []string
	Tier       Tier
	Slow       bool
}

func ClassifyAccount(sample AccountSample) AccountVerdict {
	verdict := AccountVerdict{
		AccountID:  sample.AccountID,
		Name:       sample.Name,
		GroupNames: sample.GroupNames,
		Tier:       tierFor(sample),
	}
	verdict.Slow = sample.TTFTP95MS != nil && *sample.TTFTP95MS > SlowTTFTP95MS
	return verdict
}

func tierFor(sample AccountSample) Tier {
	if sample.SampleCount <= 0 {
		return TierUnknown
	}
	if sample.ErrorCode == ErrorCodeBalanceExhausted {
		return TierUnavailable
	}
	switch {
	case sample.SuccessRate >= HealthyMinSuccessRate:
		return TierHealthy
	case sample.SuccessRate >= DegradedMinSuccessRate:
		return TierDegraded
	default:
		return TierUnavailable
	}
}
