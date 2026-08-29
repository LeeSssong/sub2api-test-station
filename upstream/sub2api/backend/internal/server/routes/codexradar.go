package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterCodexRadarRoutes exposes CodexRadar's public, read-only proxy.
// The handlers only fetch fixed external CodexRadar sources and do not require
// a JWT, user context, audit entry, or application data lookup.
func RegisterCodexRadarRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	public := v1.Group("/public/codexradar")
	if panelRateLimiter != nil {
		public.Use(panelRateLimiter.PublicIP())
	}
	public.GET("/insights", h.CodexRadar.Get)
	public.GET("/community", h.CodexRadarCommunity.Get)
}
