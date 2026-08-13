package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
