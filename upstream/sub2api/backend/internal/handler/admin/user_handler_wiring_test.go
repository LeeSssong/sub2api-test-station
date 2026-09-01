package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProvideUserHandlerInjectsQuotaWalletService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &quotaWalletHandlerFake{}
	h := ProvideUserHandler(nil, nil, nil, nil, nil, nil, nil, fake)
	r := gin.New()
	r.GET("/admin/users/:id/quota-summary", h.GetQuotaSummary)

	req := httptest.NewRequest("GET", "/admin/users/7/quota-summary", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	require.Equal(t, 200, resp.Code)
	require.NotContains(t, resp.Body.String(), "quota wallet service not available")
}

var _ service.QuotaWalletService = (*quotaWalletHandlerFake)(nil)
