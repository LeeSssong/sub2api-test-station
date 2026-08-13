package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type financialMutationRepo struct{}

func (financialMutationRepo) ReadSnapshot(context.Context, service.AccountFinancialSnapshotQuery) (*service.AccountFinancialSnapshot, error) {
	return &service.AccountFinancialSnapshot{GeneratedAt: time.Now()}, nil
}
func (financialMutationRepo) CreateReview(_ context.Context, in service.UsageCostReviewInput) (*service.UsageCostReviewResult, error) {
	cost := float64(2)
	if in.ManualCostCNY != nil {
		cost = *in.ManualCostCNY
	}
	return &service.UsageCostReviewResult{Created: true, UsageLogID: in.UsageLogID, AccountID: 7, BusinessDate: "2026-08-13", ManualCostCNY: cost}, nil
}
func (financialMutationRepo) FreezeReviewFilter(context.Context, service.ReviewFilter) (int64, error) {
	return 9, nil
}
func (financialMutationRepo) ReviewFiltered(_ context.Context, in service.ReviewFilteredInput) (*service.ReviewFilteredResult, error) {
	return &service.ReviewFilteredResult{
		Cutoff: 9, MaxUsageLogID: in.MaxUsageLogID, Matched: 1, Updated: 1,
		Reviews: []service.UsageCostReviewResult{{Created: true, UsageLogID: 1, AccountID: 7, BusinessDate: "2026-08-13", ManualCostCNY: 1}},
	}, nil
}
func (financialMutationRepo) SetOAuthDailyCost(_ context.Context, in service.OAuthDailyCostInput) (*service.FinancialMutationResult, error) {
	return &service.FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, NewValue: in.CostCNY}, nil
}
func (financialMutationRepo) SetTodayOverride(_ context.Context, in service.TodayOverrideInput) (*service.FinancialMutationResult, error) {
	value := in.RevenueCNY
	kind := "revenue"
	if value == nil {
		value, kind = in.CostCNY, "cost"
	}
	return &service.FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, NewValue: value, MutationKind: kind}, nil
}
func (financialMutationRepo) GetUsageEvidence(context.Context, int64) (*service.UsageFinancialEvidence, error) {
	return &service.UsageFinancialEvidence{EvidenceStatus: "unavailable"}, nil
}

type financialAuditRecorder struct{ entries []*service.AuditLog }

func (r *financialAuditRecorder) Record(entry *service.AuditLog) {
	r.entries = append(r.entries, entry)
}

func TestFinancialMutationHandlersPersistCorrelationThroughService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &financialAuditRecorder{}
	svc := service.NewAccountFinancialServiceWithAudit(financialMutationRepo{}, time.Now, service.NewAccountFinancialAudit(recorder))
	h := NewAccountFinancialHandler(svc)
	const requestID = "handler-correlation-123"

	tests := []struct {
		name, method, route, path, body string
		handler                         gin.HandlerFunc
	}{
		{"one", http.MethodPost, "/one/:usageLogID/review", "/one/1/review", `{"manual_cost_cny":1}`, h.ReviewOne},
		{"selected", http.MethodPost, "/selected", "/selected", `{"usage_log_ids":[1],"manual_cost_cny":1}`, h.ReviewSelected},
		{"filtered", http.MethodPost, "/filtered", "/filtered", `{"max_usage_log_id":9,"manual_cost_cny":1}`, h.ReviewFiltered},
		{"oauth", http.MethodPut, "/oauth/:id", "/oauth/4", `{"business_date":"2026-08-13","cost_cny":1}`, h.SetOAuthCost},
		{"override", http.MethodPut, "/override/:id", "/override/5", `{"business_date":"2026-08-13","cost_cny":1}`, h.SetTodayOverride},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(recorder.entries)
			r := gin.New()
			r.Handle(tt.method, tt.route, func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), ctxkey.RequestID, requestID)
				c.Request = c.Request.WithContext(ctx)
				tt.handler(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			require.Equal(t, http.StatusOK, w.Code)
			require.Len(t, recorder.entries, before+1)
			require.Equal(t, requestID, recorder.entries[before].RequestID)
		})
	}
}

func TestFinancialRequestIDFallbackUsesLoggerNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.RequestID, strings.Repeat("x", 70))
		ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "客户"+string([]byte{0xff})+"-id")
		c.Request = c.Request.WithContext(ctx)
		require.Equal(t, "客户-id", financialRequestID(c))
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestFinancialRequestIDPrefersServerCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, "client-id")
		ctx = context.WithValue(ctx, ctxkey.RequestID, "server-id")
		c.Request = c.Request.WithContext(ctx)
		require.Equal(t, "server-id", financialRequestID(c))
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestFinancialRequestIDFallsBackToClientIDThenHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/client", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, "client-id")
		c.Request = c.Request.WithContext(ctx)
		require.Equal(t, "client-id", financialRequestID(c))
	})
	r.GET("/header", func(c *gin.Context) {
		require.Equal(t, "header-id", financialRequestID(c))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/client", nil))
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/header", nil)
	req.Header.Set("X-Request-ID", "header-id")
	r.ServeHTTP(w, req)
}

func TestAccountFinancialReportRejectsUnknownRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAccountFinancialHandler(nil)
	r.GET("/api/v1/admin/operations/account-financial", h.GetReport)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/operations/account-financial?range=year", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountFinancialReviewRejectsInvalidAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAccountFinancialHandler(nil)
	r.POST("/api/v1/admin/usage/cost-exceptions/:usageLogID/review", h.ReviewOne)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/usage/cost-exceptions/1/review", strings.NewReader(`{"manual_cost_cny":-1}`)))
	require.Equal(t, http.StatusBadRequest, w.Code)
}
