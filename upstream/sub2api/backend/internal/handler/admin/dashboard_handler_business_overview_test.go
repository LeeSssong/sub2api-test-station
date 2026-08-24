package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseBusinessOverviewQueryCustomBeijingRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/operations/business-overview?range=custom&start_date=2026-08-01&end_date=2026-08-01&timezone=Asia/Shanghai", nil)
	query, err := parseBusinessOverviewQuery(c)
	require.NoError(t, err)
	require.Equal(t, service.BusinessOverviewRangeCustom, query.Range)
	require.Equal(t, "2026-08-01", query.Start.Format("2006-01-02"))
	require.Equal(t, "2026-08-02", query.End.Format("2006-01-02"))
	require.Equal(t, "Asia/Shanghai", query.Start.Location().String())
}

func TestParseBusinessOverviewQueryRejectsInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/operations/business-overview?range=nope", nil)
	_, err := parseBusinessOverviewQuery(c)
	require.EqualError(t, err, "invalid range")
}
