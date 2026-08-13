package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
