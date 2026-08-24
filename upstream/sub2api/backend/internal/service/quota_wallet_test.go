package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

type quotaRepoFake struct{ wallet QuotaWallet }

func (f *quotaRepoFake) WithLockedWallet(ctx context.Context, id int64, fn func(context.Context, *QuotaWallet) error) error {
	return fn(ctx, &f.wallet)
}
func (f *quotaRepoFake) GetSummary(context.Context, int64) (QuotaSummary, error) {
	return mutationResult(&f.wallet, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero).Summary, nil
}
func (f *quotaRepoFake) ApplyMutation(_ context.Context, w *QuotaWallet, r QuotaMutationResult, _ string, _ string, _ string, _ string, _ string, _ *int64) (QuotaMutationResult, error) {
	f.wallet.CashBalanceCNY = r.Summary.CashBalanceCNY
	f.wallet.PaidQuotaBalanceUSD = r.Summary.PaidQuotaBalanceUSD
	f.wallet.GiftQuotaBalanceUSD = r.Summary.GiftQuotaBalanceUSD
	f.wallet.Version++
	r.Summary.WalletVersion = f.wallet.Version
	return r, nil
}
func (f *quotaRepoFake) ListLedger(context.Context, int64, int, int, string) ([]QuotaLedgerEntry, int, error) {
	return nil, 0, nil
}
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestQuotaRequestFingerprintUsesOperatorValue(t *testing.T) {
	operatorA := int64(7)
	operatorB := int64(7)
	if quotaRequestFingerprint(QuotaRecordRecharge, dec("10"), &operatorA) != quotaRequestFingerprint(QuotaRecordRecharge, dec("10"), &operatorB) {
		t.Fatal("same operator value must produce the same idempotency fingerprint")
	}
}

func TestQuotaWalletConsumptionPaidFirst(t *testing.T) {
	f := &quotaRepoFake{wallet: QuotaWallet{UserID: 1, PaidQuotaBalanceUSD: dec("5"), GiftQuotaBalanceUSD: dec("8"), Version: 1}}
	r, err := NewQuotaWalletService(f).ConsumeUsage(context.Background(), UsageConsumptionInput{UserID: 1, AmountUSD: dec("7")})
	if err != nil {
		t.Fatal(err)
	}
	if !r.PaidConsumedUSD.Equal(dec("5")) || !r.GiftConsumedUSD.Equal(dec("2")) {
		t.Fatalf("unexpected split: %+v", r)
	}
}

func TestQuotaWalletInsufficientDoesNotMutate(t *testing.T) {
	f := &quotaRepoFake{wallet: QuotaWallet{UserID: 1, PaidQuotaBalanceUSD: dec("1"), GiftQuotaBalanceUSD: dec("1")}}
	_, err := NewQuotaWalletService(f).ConsumeUsage(context.Background(), UsageConsumptionInput{UserID: 1, AmountUSD: dec("3")})
	if err != ErrQuotaInsufficient {
		t.Fatalf("want ErrQuotaInsufficient, got %v", err)
	}
	if !f.wallet.PaidQuotaBalanceUSD.Equal(dec("1")) {
		t.Fatal("wallet mutated")
	}
}

func TestQuotaWalletRechargeRefundRules(t *testing.T) {
	f := &quotaRepoFake{wallet: QuotaWallet{UserID: 1, CashBalanceCNY: dec("10"), PaidQuotaBalanceUSD: dec("10"), GiftQuotaBalanceUSD: dec("4")}}
	s := NewQuotaWalletService(f)
	r, err := s.Recharge(context.Background(), RechargeInput{UserID: 1, AmountCNY: dec("3"), GiftQuotaUSD: dec("2")})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Summary.PaidQuotaBalanceUSD.Equal(dec("13")) {
		t.Fatal("paid recharge mismatch")
	}
	r, err = s.Refund(context.Background(), RefundInput{UserID: 1, AmountCNY: dec("5")})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Summary.GiftQuotaBalanceUSD.IsZero() {
		t.Fatal("gift must clear on refund")
	}
	_, err = s.Refund(context.Background(), RefundInput{UserID: 1, AmountCNY: dec("99")})
	if err != ErrQuotaRefundExceedsCash {
		t.Fatalf("want cash limit, got %v", err)
	}
}

func TestQuotaWalletLegacySetCannotRemoveGift(t *testing.T) {
	f := &quotaRepoFake{wallet: QuotaWallet{UserID: 1, PaidQuotaBalanceUSD: dec("5"), GiftQuotaBalanceUSD: dec("3")}}
	_, err := NewQuotaWalletService(f).LegacyAdjust(context.Background(), LegacyBalanceAdjustmentInput{UserID: 1, Mode: "set", TargetUSD: dec("2")})
	if err != ErrQuotaLegacySetBelowGift {
		t.Fatalf("got %v", err)
	}
}
