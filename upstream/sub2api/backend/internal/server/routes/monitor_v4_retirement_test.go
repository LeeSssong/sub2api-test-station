package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMonitorV4RetiresManualCheckAndKeepsReadRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	RegisterUserRoutes(router.Group("/api/v1"), &handler.Handlers{}, middleware.JWTAuthMiddleware(pass), middleware.AuditLogMiddleware(pass), nil, middleware.NewPanelRateLimiter(nil, nil))
	paths := map[string]bool{}
	for _, route := range router.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	require.True(t, paths["GET /api/v1/monitor-v4"])
	require.True(t, paths["GET /api/v1/monitor-v4/timeline"])
	require.True(t, paths["GET /api/v1/keys"])
	require.False(t, paths["POST /api/v1/monitor-v4/check"])
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/monitor-v4/check", nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
