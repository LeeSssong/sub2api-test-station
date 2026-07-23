package domain

import (
	"testing"
	"time"
)

func TestShanghaiDateAcrossUTC(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if got := ShanghaiDate(time.Date(2026, 7, 19, 15, 59, 0, 0, time.UTC), loc); got != "2026-07-19" {
		t.Fatalf("got %s", got)
	}
	if got := ShanghaiDate(time.Date(2026, 7, 19, 16, 1, 0, 0, time.UTC), loc); got != "2026-07-20" {
		t.Fatalf("got %s", got)
	}
}

func TestParseMicroUSD(t *testing.T) {
	for input, want := range map[string]MicroUSD{"20": CheckinGrant, "5.000000": ReferralGrant, "0.07": 70000} {
		got, err := ParseMicroUSD(input)
		if err != nil || got != want {
			t.Fatalf("%s => %d, %v", input, got, err)
		}
	}
	if _, err := ParseMicroUSD("1.0000001"); err == nil {
		t.Fatal("expected precision error")
	}
}

func TestDailyLoginPolicyConstants(t *testing.T) {
	if GrantDailyLogin != "daily_login_credit" {
		t.Fatalf("kind=%s", GrantDailyLogin)
	}
	if DailyLoginCredit != 20_000_000 {
		t.Fatalf("credit=%d", DailyLoginCredit)
	}
}
