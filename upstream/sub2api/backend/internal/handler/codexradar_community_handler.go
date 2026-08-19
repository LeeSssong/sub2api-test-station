package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type codexRadarCommunityGetter interface {
	Get(context.Context) (service.CodexRadarCommunity, bool, error)
}

type CodexRadarCommunityHandler struct {
	service codexRadarCommunityGetter
}

func NewCodexRadarCommunityHandler(service codexRadarCommunityGetter) *CodexRadarCommunityHandler {
	return &CodexRadarCommunityHandler{service: service}
}

func (h *CodexRadarCommunityHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "社区测试数据暂时不可用"}})
		return
	}
	value, stale, err := h.service.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "社区测试数据暂时不可用"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"generated_at": value.GeneratedAt, "tabs": value.Tabs, "stale": stale})
}
