package lab

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireLabAdminAllowsOnlyBootstrapEndpointsWithoutAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/auth/login/2fa",
		"/api/v1/auth/refresh",
		"/api/v1/auth/logout",
		"/api/v1/auth/lab-session",
		"/api/v1/settings/public",
		"/health",
	} {
		router := gin.New()
		router.Use(RequireLabAdmin(true, rejectingAdminAuth()))
		router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNoContent, response.Code, path)
	}
}

func TestRequireLabAdminRejectsAnonymousProductionAndNonAdminTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireLabAdmin(true, testAdminAuth()))
	router.GET("/api/v1/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for name, authorization := range map[string]string{
		"anonymous":        "",
		"production token": "Bearer production-admin",
		"lab normal user":  "Bearer lab-user",
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
			request.Header.Set("Authorization", authorization)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, response.Code)
		})
	}
}

func TestRequireLabAdminAcceptsLabAdminAndIsDisabledForProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{true, false} {
		router := gin.New()
		router.Use(RequireLabAdmin(enabled, testAdminAuth()))
		router.GET("/api/v1/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		if enabled {
			request.Header.Set("Authorization", "Bearer lab-admin")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code)
	}
}

func TestSessionCookieAuthorizationIgnoresForgedAuthorizationAndUsesLabCookie(t *testing.T) {
	t.Setenv("AUTH_COOKIE_NAME", "sub2api_lab_session")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SessionCookieAuthorization())
	router.Use(testAdminAuth())
	router.GET("/check", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for name, testCase := range map[string]struct {
		configure func(*http.Request)
		want      int
	}{
		"authorization only": {func(r *http.Request) { r.Header.Set("Authorization", "Bearer lab-admin") }, http.StatusUnauthorized},
		"production cookie":  {func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sub2api_session", Value: "lab-admin"}) }, http.StatusUnauthorized},
		"lab user cookie":    {func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sub2api_lab_session", Value: "lab-user"}) }, http.StatusForbidden},
		"lab admin cookie":   {func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sub2api_lab_session", Value: "lab-admin"}) }, http.StatusNoContent},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/check", nil)
			testCase.configure(request)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, testCase.want, response.Code)
		})
	}
}

func TestSetAndClearAdminSessionCookieUseIsolatedSecureScope(t *testing.T) {
	t.Setenv("LAB_ONLY", "1")
	t.Setenv("AUTH_COOKIE_NAME", "sub2api_lab_session")
	gin.SetMode(gin.TestMode)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	require.NoError(t, SetAdminSessionCookie(context, "signed-lab-admin-jwt"))

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "sub2api_lab_session", cookies[0].Name)
	require.Equal(t, "/admin/lab", cookies[0].Path)
	require.True(t, cookies[0].HttpOnly)
	require.True(t, cookies[0].Secure)
	require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)

	clearResponse := httptest.NewRecorder()
	clearContext, _ := gin.CreateTestContext(clearResponse)
	clearContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	require.NoError(t, ClearAdminSessionCookie(clearContext))
	cleared := clearResponse.Result().Cookies()
	require.Len(t, cleared, 1)
	require.Equal(t, -1, cleared[0].MaxAge)
}

func testAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.GetHeader("Authorization") {
		case "Bearer lab-admin":
			c.Next()
		case "Bearer lab-user":
			c.AbortWithStatus(http.StatusForbidden)
		default:
			c.AbortWithStatus(http.StatusUnauthorized)
		}
	}
}

func rejectingAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) { c.AbortWithStatus(http.StatusTeapot) }
}
