package admin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type quotaWalletHandlerFake struct {
	summary service.QuotaSummary
	called  string
}

func (f *quotaWalletHandlerFake) GetSummary(context.Context, int64) (service.QuotaSummary, error) {
	return f.summary, nil
}
func (f *quotaWalletHandlerFake) ListLedger(context.Context, int64, int, int, string) ([]service.QuotaLedgerEntry, int, error) {
	return nil, 0, nil
}
func (f *quotaWalletHandlerFake) Recharge(_ context.Context, in service.RechargeInput) (service.QuotaMutationResult, error) {
	f.called = service.QuotaRecordRecharge
	return service.QuotaMutationResult{LedgerEntryID: 9, Summary: f.summary}, nil
}
func (f *quotaWalletHandlerFake) Refund(_ context.Context, in service.RefundInput) (service.QuotaMutationResult, error) {
	f.called = service.QuotaRecordRefund
	return service.QuotaMutationResult{LedgerEntryID: 10, Summary: f.summary}, nil
}
func (f *quotaWalletHandlerFake) ConsumeUsage(context.Context, service.UsageConsumptionInput) (service.QuotaMutationResult, error) {
	return service.QuotaMutationResult{}, nil
}
func (f *quotaWalletHandlerFake) LegacyAdjust(context.Context, service.LegacyBalanceAdjustmentInput) (service.QuotaMutationResult, error) {
	return service.QuotaMutationResult{}, nil
}

func TestQuotaWalletHandlerSummaryUsesStringPrecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &quotaWalletHandlerFake{summary: service.QuotaSummary{UserID: 7, CashBalanceCNY: decimal.RequireFromString("10.125"), PaidQuotaBalanceUSD: decimal.RequireFromString("8.25"), GiftQuotaBalanceUSD: decimal.RequireFromString("1.75"), TotalQuotaBalanceUSD: decimal.RequireFromString("10"), WalletVersion: 3, UpdatedAt: time.Date(2026, 8, 23, 1, 2, 3, 0, time.UTC)}}
	h := NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetQuotaWalletService(fake)
	r := gin.New()
	r.GET("/admin/users/:id/quota-summary", h.GetQuotaSummary)
	req := httptest.NewRequest("GET", "/admin/users/7/quota-summary", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, 200, resp.Code)
	require.Contains(t, resp.Body.String(), `"cash_balance_cny":"10.125"`)
	require.Contains(t, resp.Body.String(), `"wallet_version":3`)
}

func TestQuotaWalletHandlerRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &quotaWalletHandlerFake{}
	h := NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetQuotaWalletService(fake)
	r := gin.New()
	r.POST("/admin/users/:id/quota-ledger", h.CreateQuotaLedgerEntry)
	req := httptest.NewRequest("POST", "/admin/users/7/quota-ledger", strings.NewReader(`{"record_type":"recharge","amount_cny":1}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, 400, resp.Code)
	require.Contains(t, resp.Body.String(), "Idempotency-Key")
	require.Empty(t, fake.called)
}

func TestQuotaWalletHandlerCreateRechargeDelegatesToCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &quotaWalletHandlerFake{}
	h := NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetQuotaWalletService(fake)
	r := gin.New()
	r.POST("/admin/users/:id/quota-ledger", h.CreateQuotaLedgerEntry)
	req := httptest.NewRequest("POST", "/admin/users/7/quota-ledger", strings.NewReader(`{"record_type":"recharge","amount_cny":5,"gift_quota_usd":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "admin-test-1")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, 200, resp.Code)
	require.Equal(t, service.QuotaRecordRecharge, fake.called)
	require.Contains(t, resp.Body.String(), `"ledger_entry_id":9`)
}
