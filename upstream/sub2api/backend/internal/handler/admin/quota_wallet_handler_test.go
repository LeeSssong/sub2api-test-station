package admin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type quotaWalletHandlerFake struct {
	summary service.QuotaSummary
	called  string
}

type quotaWalletBalanceCacheFake struct {
	service.BillingCache
	invalidatedUserIDs []int64
}

func withTestAdmin(r *gin.Engine) {
	r.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 99})
		c.Next()
	})
}

func (f *quotaWalletBalanceCacheFake) InvalidateUserBalance(_ context.Context, userID int64) error {
	f.invalidatedUserIDs = append(f.invalidatedUserIDs, userID)
	return nil
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
	withTestAdmin(r)
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
	withTestAdmin(r)
	r.POST("/admin/users/:id/quota-ledger", h.CreateQuotaLedgerEntry)
	req := httptest.NewRequest("POST", "/admin/users/7/quota-ledger", strings.NewReader(`{"record_type":"recharge","amount_cny":1,"payment_trade_no":"T-1","note":"manual"}`))
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
	withTestAdmin(r)
	r.POST("/admin/users/:id/quota-ledger", h.CreateQuotaLedgerEntry)
	req := httptest.NewRequest("POST", "/admin/users/7/quota-ledger", strings.NewReader(`{"record_type":"recharge","amount_cny":5,"gift_quota_usd":2,"payment_trade_no":"T-2","note":"manual"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "admin-test-1")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, 200, resp.Code)
	require.Equal(t, service.QuotaRecordRecharge, fake.called)
	require.Contains(t, resp.Body.String(), `"ledger_entry_id":9`)
}

func TestQuotaWalletHandlerInvalidatesBalanceCacheAfterLedgerMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name       string
		recordType string
		wantCall   string
	}{
		{name: "recharge", recordType: service.QuotaRecordRecharge, wantCall: service.QuotaRecordRecharge},
		{name: "refund", recordType: service.QuotaRecordRefund, wantCall: service.QuotaRecordRefund},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &quotaWalletHandlerFake{}
			cache := &quotaWalletBalanceCacheFake{}
			h := NewUserHandler(nil, nil, nil, cache, nil, nil, nil)
			h.SetQuotaWalletService(fake)
			r := gin.New()
			withTestAdmin(r)
			r.POST("/admin/users/:id/quota-ledger", h.CreateQuotaLedgerEntry)

			body := `{"record_type":"` + tc.recordType + `","amount_cny":5}`
			if tc.recordType == service.QuotaRecordRecharge {
				body = `{"record_type":"recharge","amount_cny":5,"payment_trade_no":"T-3","note":"manual"}`
			}
			req := httptest.NewRequest("POST", "/admin/users/7/quota-ledger", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "admin-test-"+tc.recordType)
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			require.Equal(t, 200, resp.Code)
			require.Equal(t, tc.wantCall, fake.called)
			require.Equal(t, []int64{7}, cache.invalidatedUserIDs)
		})
	}
}
