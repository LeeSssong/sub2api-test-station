package accounting

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

func TestBuildSnapshotExcludesInternalRevenueButIncludesInternalCost(t *testing.T) {
	usage := UsageTotals{
		ExternalRevenueCNY: decimal.RequireFromString("12.50"),
		CustomerCostCNY:    decimal.RequireFromString("7.25"),
		InternalCostCNY:    decimal.RequireFromString("3.00"),
	}
	cash := CashEventTotals{OutflowCNY: decimal.RequireFromString("5.00")}

	got := BuildSnapshot(time.Date(2026, 8, 2, 0, 0, 0, 0, shanghai), usage, cash)

	if got.ExternalRevenueCNY.StringFixed(2) != "12.50" {
		t.Fatalf("external revenue = %s", got.ExternalRevenueCNY.StringFixed(2))
	}
	if got.ResourceCostCNY.StringFixed(2) != "10.25" {
		t.Fatalf("resource cost = %s", got.ResourceCostCNY.StringFixed(2))
	}
	if got.OperatingGrossProfitCNY.StringFixed(2) != "2.25" {
		t.Fatalf("operating gross profit = %s", got.OperatingGrossProfitCNY.StringFixed(2))
	}
	if got.CashNetResultCNY.StringFixed(2) != "7.50" {
		t.Fatalf("cash net result = %s", got.CashNetResultCNY.StringFixed(2))
	}
}

func TestCashEventValidationRejectsSecretsAndInvalidAmounts(t *testing.T) {
	_, err := ValidateCashEvent(CashEventInput{
		EventType: "account_purchase",
		PaidAt:    time.Date(2026, 8, 2, 1, 0, 0, 0, shanghai),
		AmountCNY: decimal.NewFromInt(-1),
		Notes:     "token=super-secret-value",
	})
	if err == nil {
		t.Fatal("invalid cash event was accepted")
	}
}

func TestValidateCashEventRules(t *testing.T) {
	valid := CashEventInput{
		EventType:  "account_purchase",
		PaidAt:     time.Date(2026, 8, 2, 1, 0, 0, 0, shanghai),
		AmountCNY:  decimal.RequireFromString("12.50"),
		SourceKind: SourceKindOwnedOAuth,
		Notes:      "monthly account",
	}
	if _, err := ValidateCashEvent(valid); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	for _, eventType := range []string{"account_purchase", "upstream_topup", "fee"} {
		input := valid
		input.EventType = eventType
		if _, err := ValidateCashEvent(input); err != nil {
			t.Fatalf("%s rejected: %v", eventType, err)
		}
	}
	refund := valid
	refund.EventType = "refund"
	refund.AmountCNY = decimal.RequireFromString("-1.25")
	if _, err := ValidateCashEvent(refund); err != nil {
		t.Fatalf("refund rejected: %v", err)
	}

	tests := []CashEventInput{
		{EventType: "unknown", PaidAt: valid.PaidAt, AmountCNY: valid.AmountCNY, SourceKind: valid.SourceKind},
		{EventType: "account_purchase", PaidAt: valid.PaidAt, AmountCNY: decimal.Zero, SourceKind: valid.SourceKind},
		{EventType: "account_purchase", PaidAt: valid.PaidAt, AmountCNY: decimal.RequireFromString("-1"), SourceKind: valid.SourceKind},
		{EventType: "refund", PaidAt: valid.PaidAt, AmountCNY: decimal.RequireFromString("1"), SourceKind: valid.SourceKind},
		{EventType: "account_purchase", PaidAt: valid.PaidAt, AmountCNY: valid.AmountCNY, SourceKind: "invalid"},
		{EventType: "account_purchase", PaidAt: valid.PaidAt, AmountCNY: valid.AmountCNY, SourceKind: valid.SourceKind, AccountID: ptrToInt64(0)},
		{EventType: "account_purchase", PaidAt: time.Time{}, AmountCNY: valid.AmountCNY, SourceKind: valid.SourceKind},
	}
	for _, input := range tests {
		_, err := ValidateCashEvent(input)
		if err == nil {
			t.Errorf("invalid input accepted: %#v", input)
		}
	}
}

func TestValidateCashEventRejectsCredentialLikeNotesAndLongNotes(t *testing.T) {
	base := CashEventInput{
		EventType:  "account_purchase",
		PaidAt:     time.Date(2026, 8, 2, 1, 0, 0, 0, shanghai),
		AmountCNY:  decimal.RequireFromString("1"),
		SourceKind: SourceKindOwnedOAuth,
	}
	for _, notes := range []string{
		"api_key=secret",
		"password: foo",
		"access_token abc",
		"cookie=foo",
		"oauth_token=foo",
	} {
		input := base
		input.Notes = notes
		_, err := ValidateCashEvent(input)
		if err == nil {
			t.Errorf("credential-like note accepted: %q", notes)
		}
	}
	input := base
	input.Notes = string(make([]byte, 501))
	_, err := ValidateCashEvent(input)
	if err == nil {
		t.Fatal("overlong note accepted")
	}
}

func TestClassifySourceKind(t *testing.T) {
	if got, err := ClassifySourceKind("oauth"); err != nil || got != SourceKindOwnedOAuth {
		t.Fatalf("oauth classification = %q, %v", got, err)
	}
	if got, err := ClassifySourceKind("apikey"); err != nil || got != SourceKindUpstreamAPIKey {
		t.Fatalf("apikey classification = %q, %v", got, err)
	}
	_, err := ClassifySourceKind("custom")
	if err == nil {
		t.Fatal("unsupported source type accepted")
	}
}

func TestLocalDayAndDayWindow(t *testing.T) {
	instant := time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC)
	got := LocalDay(instant)
	if got.Year() != 2026 || got.Month() != time.August || got.Day() != 3 || got.Hour() != 0 {
		t.Fatalf("local day = %s", got)
	}
	if got.Location().String() != "Asia/Shanghai" {
		t.Fatalf("local day location = %s", got.Location())
	}

	window := NewDayWindow(instant)
	if !window.Start.Equal(got) {
		t.Fatalf("window start = %s, want %s", window.Start, got)
	}
	if !window.End.Equal(got.AddDate(0, 0, 1)) {
		t.Fatalf("window end = %s", window.End)
	}
}

func ptrToInt64(value int64) *int64 {
	return &value
}
