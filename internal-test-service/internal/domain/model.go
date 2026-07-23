package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// MicroUSD is an integer number of one-millionth USD. Ledger arithmetic never
// uses floating point values.
type MicroUSD int64

const (
	GrantDailyLogin = "daily_login_credit"
	GrantCheckin    = "daily_checkin"
	GrantReferral   = "referral_reward"
	TaskPending     = "pending"
	TaskSucceeded   = "succeeded"
	TaskUncertain   = "uncertain"
	ModeReadOnly    = "read_only"
)

const (
	DailyLoginCredit MicroUSD = 20_000_000
	CheckinGrant     MicroUSD = DailyLoginCredit
	ReferralGrant    MicroUSD = 5_000_000
)

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func ShanghaiDate(now time.Time, loc *time.Location) string {
	return now.In(loc).Format("2006-01-02")
}

func ParseMicroUSD(value string) (MicroUSD, error) {
	whole, frac, hasDot := strings.Cut(value, ".")
	if !hasDot {
		frac = ""
	}
	if whole == "" || len(frac) > 6 {
		return 0, fmt.Errorf("invalid USD amount")
	}
	if whole[0] == '-' {
		return 0, fmt.Errorf("negative USD amount")
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid USD amount")
		}
	}
	for len(frac) < 6 {
		frac += "0"
	}
	var n int64
	for _, r := range whole {
		if n > math.MaxInt64/10 {
			return 0, fmt.Errorf("USD amount overflow")
		}
		n = n*10 + int64(r-'0')
	}
	if n > math.MaxInt64/1_000_000 {
		return 0, fmt.Errorf("USD amount overflow")
	}
	result := n * 1_000_000
	var fractional int64
	for _, r := range frac {
		fractional = fractional*10 + int64(r-'0')
	}
	result += fractional
	return MicroUSD(result), nil
}

func int64Pow10(n int) int64 {
	p := int64(1)
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}

func (m MicroUSD) String() string {
	whole := int64(m) / 1_000_000
	frac := int64(m) % 1_000_000
	return fmt.Sprintf("%d.%06d", whole, frac)
}

func ValidDate(value string) bool { return datePattern.MatchString(value) }
