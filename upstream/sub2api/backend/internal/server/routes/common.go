package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

const readinessProbeTimeout = 2 * time.Second

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine, readiness ReadinessChecker) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", readinessHandler(readiness, readinessProbeTimeout))

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}

func RegisterFeishuUpstreamBalanceRoutes(r *gin.Engine, h *handler.Handlers) {
	if h == nil || h.FeishuUpstreamBalance == nil {
		return
	}
	r.POST("/api/v1/notifications/feishu/upstream-balance/callback", h.FeishuUpstreamBalance.Handle)
}

func readinessHandler(readiness ReadinessChecker, timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if readiness == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- readiness.Check(ctx) }()

		select {
		case err := <-result:
			if err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
		case <-ctx.Done():
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		}
	}
}
