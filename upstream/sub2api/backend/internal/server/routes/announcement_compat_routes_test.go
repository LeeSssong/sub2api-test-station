package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnnouncementCompatibilityRoutesKeepAuthenticationAndHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterUserRoutes(router.Group("/api/v1"), &handler.Handlers{},
		func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) },
		func(c *gin.Context) { c.Next() }, nil, nil)

	for _, endpoint := range []struct{ method, suffix string }{
		{http.MethodGet, ""},
		{http.MethodPost, "/7/read"},
	} {
		for _, prefix := range []string{"/announcements", "/notices"} {
			path := "/api/v1" + prefix + endpoint.suffix
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(endpoint.method, path, nil))
			require.Equal(t, http.StatusUnauthorized, response.Code, "%s %s", endpoint.method, path)
		}
	}

	handlers := make(map[string]string)
	for _, route := range router.Routes() {
		handlers[route.Method+" "+route.Path] = route.Handler
	}
	require.NotEmpty(t, handlers["GET /api/v1/notices"])
	require.Equal(t, handlers["GET /api/v1/announcements"], handlers["GET /api/v1/notices"])
	require.Equal(t, handlers["POST /api/v1/announcements/:id/read"], handlers["POST /api/v1/notices/:id/read"])
}
