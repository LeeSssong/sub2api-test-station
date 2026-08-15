package admin

import (
	"context"
	"encoding/json"
	"errors"
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

type financialFilterCaptureRepo struct {
	financialMutationRepo
	query    service.AccountFinancialSnapshotQuery
	snapshot *service.AccountFinancialSnapshot
}

type financialUsageReader struct {
	snapshot *service.AccountFinancialUsageSnapshot
	err      error
}

func (r *financialUsageReader) ReadAccountFinancialUsage(context.Context, time.Time, time.Time) (*service.AccountFinancialUsageSnapshot, error) {
	return r.snapshot, r.err
}

func (r *financialFilterCaptureRepo) ReadSnapshot(_ context.Context, q service.AccountFinancialSnapshotQuery) (*service.AccountFinancialSnapshot, error) {
	r.query = q
	if r.snapshot != nil {
		return r.snapshot, nil
	}
	return &service.AccountFinancialSnapshot{GeneratedAt: q.GeneratedAt}, nil
}

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
	svc := service.NewAccountFinancialServiceWithAudit(financialMutationRepo{}, nil, time.Now, service.NewAccountFinancialAudit(recorder))
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

func TestAccountFinancialReportReturnsNativeJSONContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 13, 4, 0, 0, 0, time.UTC)
	reader := &financialUsageReader{snapshot: &service.AccountFinancialUsageSnapshot{
		UserBalanceCNY: 90,
		Accounts:       []service.AccountFinancialUsageAccount{{ID: 7, Name: "native", Type: "api_key", Platform: "sub", Active: true}},
		Rows:           []service.AccountFinancialUsageRow{{AccountID: 7, Requests: 2, Tokens: 10, Cost: 1.25, UserCost: 2}},
	}}
	h := NewAccountFinancialHandler(service.NewAccountFinancialService(financialMutationRepo{}, reader, func() time.Time { return now }))
	r := gin.New()
	r.GET("/api/v1/admin/operations/account-financial", h.GetReport)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/operations/account-financial?range=24h", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data struct {
			Currency       string                   `json:"currency"`
			UserBalanceCNY float64                  `json:"user_unconsumed_balance_cny"`
			Summary        service.FinancialAmounts `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "USD", body.Data.Currency)
	require.Equal(t, float64(90), body.Data.UserBalanceCNY)
	require.Equal(t, service.FinancialAmounts{Requests: 2, Tokens: 10, Cost: 1.25, UserCost: 2, Profit: .75, Margin: body.Data.Summary.Margin, Revenue: 2, Expense: 1.25}, body.Data.Summary)
	require.NotNil(t, body.Data.Summary.Margin)
	require.Equal(t, .375, *body.Data.Summary.Margin)
	require.NotContains(t, w.Body.String(), "exception_count")
	require.NotContains(t, w.Body.String(), "complete")
}

func TestAccountFinancialReportUnavailableServiceAndReaderErrorsAreNonSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		handler *AccountFinancialHandler
	}{
		{name: "nil service", handler: NewAccountFinancialHandler(nil)},
		{name: "nil reader", handler: NewAccountFinancialHandler(service.NewAccountFinancialService(financialMutationRepo{}, nil, time.Now))},
		{name: "reader error", handler: NewAccountFinancialHandler(service.NewAccountFinancialService(financialMutationRepo{}, &financialUsageReader{err: errors.New("reader unavailable")}, time.Now))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/report", tt.handler.GetReport)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/report?range=today", nil))
			require.GreaterOrEqual(t, w.Code, http.StatusBadRequest, w.Body.String())
		})
	}
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

func TestAccountFinancialListExceptionsPassesRFC3339HalfOpenRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &financialFilterCaptureRepo{}
	h := NewAccountFinancialHandler(service.NewAccountFinancialService(repo, nil, time.Now))
	r := gin.New()
	r.GET("/exceptions", h.ListExceptions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exceptions?start_time=2026-08-13T00%3A00%3A00%2B08%3A00&end_time=2026-08-14T00%3A00%3A00%2B08%3A00", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "2026-08-13T00:00:00+08:00", repo.query.From.Format(time.RFC3339))
	require.Equal(t, "2026-08-14T00:00:00+08:00", repo.query.To.Format(time.RFC3339))
}

func TestAccountFinancialListExceptionsRejectsMalformedOrInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{
		"start_time=2026-08-13",
		"end_time=not-a-time",
		"start_time=2026-08-14T00%3A00%3A00Z&end_time=2026-08-13T00%3A00%3A00Z",
		"start_time=2026-08-13T00%3A00%3A00Z&end_time=2026-08-13T00%3A00%3A00Z",
	} {
		t.Run(query, func(t *testing.T) {
			repo := &financialFilterCaptureRepo{}
			h := NewAccountFinancialHandler(service.NewAccountFinancialService(repo, nil, time.Now))
			r := gin.New()
			r.GET("/exceptions", h.ListExceptions)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exceptions?"+query, nil))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.True(t, repo.query.From.IsZero())
			require.True(t, repo.query.To.IsZero())
		})
	}
}

func TestAccountFinancialListExceptionsReturnsScopedCostTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 13, 4, 0, 0, 0, time.UTC)
	upstreamRequestID := "upstream-request-42"
	upstreamModel := "gpt-upstream"
	billingTime := now.Add(-time.Minute)
	quota := 2500.0
	perUnit := 500000.0
	repo := &financialFilterCaptureRepo{}
	repo.snapshot = &service.AccountFinancialSnapshot{
		GeneratedAt: now,
		Accounts:    []service.AccountFinancialSnapshotAccount{{ID: 7, Name: "newapi-ledger", Type: "api_key"}},
		Entries:     []service.AccountFinancialSnapshotEntry{{UsageLogID: 42, AccountID: 7, RequestID: "local-request", CreatedAt: now, EvidenceStatus: "unavailable", ReasonCode: "record_not_found", Source: "newapi", UpstreamRequestID: &upstreamRequestID, UpstreamBillingTime: &billingTime, UpstreamModel: &upstreamModel, NewAPIQuota: &quota, NewAPIQuotaPerUnit: &perUnit}},
	}
	h := NewAccountFinancialHandler(service.NewAccountFinancialService(repo, nil, func() time.Time { return now }))
	r := gin.New()
	r.GET("/exceptions", h.ListExceptions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exceptions", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data struct {
			Items []service.AccountFinancialException
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	item := body.Data.Items[0]
	require.Equal(t, int64(7), item.AccountID)
	require.Equal(t, "newapi-ledger", item.AccountName)
	require.Equal(t, "api_key", item.AccountType)
	require.Equal(t, "newapi", item.Source)
	require.Equal(t, &upstreamRequestID, item.UpstreamRequestID)
	require.Equal(t, &upstreamModel, item.UpstreamModel)
	require.NotNil(t, item.UpstreamBillingTime)
	require.Equal(t, &quota, item.CostTrace.NewAPIQuota)
	require.Equal(t, &perUnit, item.CostTrace.NewAPIQuotaPerUnit)
	require.NotContains(t, w.Body.String(), "credentials")
	require.NotContains(t, w.Body.String(), "raw_response")
}
