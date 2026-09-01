package service

import (
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
		"amount":      func(v *AdminRechargeInput) { v.Amount = decimal.Zero },
		"trade":       func(v *AdminRechargeInput) { v.PaymentTradeNo = "" },
		"note":        func(v *AdminRechargeInput) { v.Note = " " },
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

func TestValidateAdminRechargeInputDefaultsPaymentType(t *testing.T) {
	in := AdminRechargeInput{UserID: 7, OperatorUserID: 9, Amount: decimal.NewFromInt(1), PaymentTradeNo: "T", Note: "N"}
	if err := ValidateAdminRechargeInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.PaymentType != "admin_recharge" {
		t.Fatalf("payment type=%q, want admin_recharge", in.PaymentType)
	}
}
