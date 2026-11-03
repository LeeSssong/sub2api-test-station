package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMonitorV4LineSelectionRouteRequiresSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	RegisterUserRoutes(router.Group("/api/v1"), &handler.Handlers{MonitorV4: handler.NewMonitorV4Handler(nil)}, middleware.JWTAuthMiddleware(pass), middleware.AuditLogMiddleware(pass), nil, middleware.NewPanelRateLimiter(nil, nil))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v4/line-selection", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
