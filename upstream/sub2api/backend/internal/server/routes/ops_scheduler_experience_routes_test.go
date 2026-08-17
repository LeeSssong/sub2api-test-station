package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAISchedulerExperienceRouteRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Ops: adminhandler.NewOpsHandler(nil)}}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
		}
		c.Next()
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	path := "/api/v1/admin/ops/openai-scheduler-experience"
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer admin")
	router.ServeHTTP(authorized, request)
	require.Equal(t, http.StatusServiceUnavailable, authorized.Code)
}
