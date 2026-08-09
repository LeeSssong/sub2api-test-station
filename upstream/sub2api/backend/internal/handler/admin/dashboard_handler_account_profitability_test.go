package admin

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDashboardHandlerAccountProfitabilityParsesInclusiveDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("GET", "/api/v1/admin/operations/account-profitability?start_date=2026-08-08&end_date=2026-08-08&timezone=Asia/Shanghai", nil)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req

	start, end, err := parseAccountProfitabilityRange(ctx)
	require.NoError(t, err)
	require.Equal(t, "2026-08-08", start.In(end.Location()).Format("2006-01-02"))
	require.Equal(t, "2026-08-09", end.In(start.Location()).Format("2006-01-02"))
	require.Equal(t, "Asia/Shanghai", start.Location().String())
}

func TestDashboardHandlerAccountProfitabilityReturnsContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta("SUM(COALESCE(ul.account_cost, COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)))")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "platform", "account_type", "status", "extra",
			"procurement_cost_cny", "procurement_cost_effective_at", "expires_at",
			"revenue", "relay_expense", "request_count", "tokens",
		}).AddRow(int64(11), "Relay", service.PlatformOpenAI, service.AccountTypeAPIKey, service.StatusActive,
			`{"account_monitor_balance":{"version":1,"source":"sub2api","status":"ok"}}`, nil, nil, nil,
			10.0, 4.0, int64(2), int64(100)))

	recorder := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/admin/operations/account-profitability?start_date=2026-08-08&end_date=2026-08-08&timezone=Asia/Shanghai", nil)
	h := NewDashboardHandler(nil, nil)
	h.SetAccountProfitabilityService(service.NewAccountProfitabilityService(db))
	router.GET("/api/v1/admin/operations/account-profitability", h.GetAccountProfitability)
	router.ServeHTTP(recorder, ctx.Request)

	require.Equal(t, 200, recorder.Code)
	var envelope struct {
		Code int                                `json:"code"`
		Data service.AccountProfitabilityReport `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, "2026-08-08", envelope.Data.StartDate)
	require.Equal(t, "2026-08-08", envelope.Data.EndDate)
	require.Equal(t, 1, envelope.Data.Summary.AccountCount)
	require.Equal(t, 10.0, envelope.Data.Summary.Revenue)
	require.NoError(t, mock.ExpectationsWereMet())
}
