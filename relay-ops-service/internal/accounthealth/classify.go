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
	AccountID     int64
	Name          string
	GroupNames    []string
	SuccessRate   float64
	SampleCount   int
	TTFTP95MS     *float64
	LatestStatus  string
	ErrorCode     string
	Unschedulable bool
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
	if sample.Unschedulable {
		return TierUnavailable
	}
	// Rolling capacity alerts set LatestStatus so the newest probe is the
	// current availability truth. Daily-report samples leave it empty and keep
	// using their aggregate success-rate tiers.
	if sample.LatestStatus != "" {
		if sample.LatestStatus == statusSuccess {
			return TierHealthy
		}
		return TierUnavailable
	}
	// balance_exhausted 短路必须先于零样本判定：窗口口径下「0 样本」是常态
	// （新增账号、跨零点后的第一小时）。若先判 Unknown，余额耗尽账号会被
	// GroupAvailabilities 剔出 Total，3 账号组缩成 2 账号组，告警阈值从
	// 「<=1」悄悄放宽到「==0」。
	if sample.ErrorCode == ErrorCodeBalanceExhausted {
		return TierUnavailable
	}
	if sample.SampleCount <= 0 {
		return TierUnknown
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
