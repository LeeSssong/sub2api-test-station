package service

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidateAdminRechargeInputRequiresAuditFields(t *testing.T) {
	base := AdminRechargeInput{UserID: 7, OperatorUserID: 9, Amount: decimal.NewFromInt(100), PaymentTradeNo: "TRX-1", Note: "manual top up"}
	if err := ValidateAdminRechargeInput(&base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*AdminRechargeInput){
		"user":        func(v *AdminRechargeInput) { v.UserID = 0 },
		"operator":    func(v *AdminRechargeInput) { v.OperatorUserID = 0 },
		"amounts":     func(v *AdminRechargeInput) { v.Amount = decimal.Zero; v.GiftQuota = decimal.Zero },
		"trade":       func(v *AdminRechargeInput) { v.PaymentTradeNo = "" },
		"gift":        func(v *AdminRechargeInput) { v.GiftQuota = decimal.NewFromInt(-1) },
		"paymentType": func(v *AdminRechargeInput) { v.PaymentType = "stripe" },
	} {
		in := base
		mutate(&in)
		if err := ValidateAdminRechargeInput(&in); err == nil {
			t.Fatalf("%s input should be rejected", name)
		}
	}
}

func TestValidateAdminRechargeInputAllowsGiftOnlyAndEmptyNote(t *testing.T) {
	in := AdminRechargeInput{UserID: 7, OperatorUserID: 9, GiftQuota: decimal.NewFromInt(5), PaymentTradeNo: "GIFT-1"}
	if err := ValidateAdminRechargeInput(&in); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAdminRechargeInputDefaultsPaymentType(t *testing.T) {
	in := AdminRechargeInput{UserID: 7, OperatorUserID: 9, Amount: decimal.NewFromInt(1), PaymentTradeNo: "T", Note: "N"}
	if err := ValidateAdminRechargeInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.PaymentType != "admin_recharge" {
		t.Fatalf("payment type=%q, want admin_recharge", in.PaymentType)
	}
}

func TestValidateAdminRechargeInputNormalizesAndValidatesTradeNumber(t *testing.T) {
	in := AdminRechargeInput{UserID: 7, OperatorUserID: 9, Amount: decimal.NewFromInt(1), PaymentTradeNo: "  WX:2026/09-01_ABC.7  ", Note: "N"}
	if err := ValidateAdminRechargeInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.PaymentTradeNo != "WX:2026/09-01_ABC.7" {
		t.Fatalf("trade number=%q, want trimmed value", in.PaymentTradeNo)
	}
	for _, tradeNo := range []string{"bad#trade", "订单-1", strings.Repeat("A", 129)} {
		in.PaymentTradeNo = tradeNo
		if err := ValidateAdminRechargeInput(&in); err == nil {
			t.Fatalf("trade number %q should be rejected", tradeNo)
		}
	}
}
