package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/lab"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterLabAuthRoutes exposes a cookie-backed forward-auth probe only in the
// isolated lab process. The native admin middleware remains authoritative.
func RegisterLabAuthRoutes(v1 *gin.RouterGroup, adminAuth servermiddleware.AdminAuthMiddleware) {
	if !lab.Enabled() {
		return
	}
	v1.GET(
		"/auth/lab-session",
		lab.SessionCookieAuthorization(),
		gin.HandlerFunc(adminAuth),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
}
