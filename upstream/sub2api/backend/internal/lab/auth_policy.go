package lab

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const adminSessionCookiePath = "/admin/lab"

var unauthenticatedLabPaths = map[string]struct{}{
	"/api/v1/auth/login":       {},
	"/api/v1/auth/login/2fa":   {},
	"/api/v1/auth/refresh":     {},
	"/api/v1/auth/logout":      {},
	"/api/v1/settings/public":  {},
	"/api/v1/auth/lab-session": {},
}

// Enabled reports whether this process is the isolated lab service.
func Enabled() bool {
	return strings.TrimSpace(os.Getenv("LAB_ONLY")) == "1"
}

// RequireLabAdmin applies the native admin authenticator to every non-bootstrap
// panel API in the lab process. Production processes bypass this policy.
func RequireLabAdmin(enabled bool, adminAuth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled || !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.Next()
			return
		}
		if _, allowed := unauthenticatedLabPaths[c.Request.URL.Path]; allowed {
			c.Next()
			return
		}
		adminAuth(c)
	}
}

// SessionCookieAuthorization converts only the isolated HttpOnly lab cookie to
// the native Bearer form consumed by AdminAuthMiddleware. Incoming Authorization
// headers are discarded so production or forged Bearer tokens cannot bypass it.
func SessionCookieAuthorization() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Header.Del("Authorization")
		cookieName, err := sessionCookieName()
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		token, err := c.Cookie(cookieName)
		if err != nil || strings.TrimSpace(token) == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Request.Header.Set("Authorization", "Bearer "+token)
		c.Next()
	}
}

// SetAdminSessionCookie stores the already-issued native lab JWT in an
// HttpOnly cookie so Caddy can authorize top-level page requests.
func SetAdminSessionCookie(c *gin.Context, token string) error {
	if !Enabled() {
		return nil
	}
	name, err := sessionCookieName()
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("lab admin session token is empty")
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     adminSessionCookiePath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// ClearAdminSessionCookie expires the page authorization cookie on logout.
func ClearAdminSessionCookie(c *gin.Context) error {
	if !Enabled() {
		return nil
	}
	name, err := sessionCookieName()
	if err != nil {
		return err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     adminSessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func sessionCookieName() (string, error) {
	name := strings.TrimSpace(os.Getenv("AUTH_COOKIE_NAME"))
	if name == "" || !strings.HasPrefix(name, "sub2api_lab_") {
		return "", fmt.Errorf("AUTH_COOKIE_NAME is not lab-scoped")
	}
	return name, nil
}
