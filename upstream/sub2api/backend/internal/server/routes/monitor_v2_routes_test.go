package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monitorV2RouteSnapshotter struct{}

func (monitorV2RouteSnapshotter) Snapshot(
	context.Context,
	service.MonitorV2Window,
	time.Time,
) (*service.MonitorV2Snapshot, error) {
	return &service.MonitorV2Snapshot{
		ContractVersion: service.MonitorV2ContractVersion,
		Window:          service.MonitorV2Window7D,
		GeneratedAt:     time.Now().UTC(),
		Groups:          []service.MonitorV2Group{},
	}, nil
}

func TestMonitorV2RouteUsesAuthenticatedUserBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	authCalled := false
	jwt := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		authCalled = true
		c.Next()
	})
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	RegisterUserRoutes(v1, &handler.Handlers{
		MonitorV2: handler.NewMonitorV2Handler(monitorV2RouteSnapshotter{}),
	}, jwt, audit, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)
	engine.ServeHTTP(recorder, request)

	require.True(t, authCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"contract_version":"2"`)
}
