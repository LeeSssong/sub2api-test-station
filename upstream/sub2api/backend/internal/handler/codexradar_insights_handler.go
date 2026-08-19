package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type codexRadarInsightsGetter interface {
	Get(context.Context) (service.CodexRadarInsights, bool, error)
}

type CodexRadarInsightsHandler struct {
	service codexRadarInsightsGetter
}

func NewCodexRadarInsightsHandler(service codexRadarInsightsGetter) *CodexRadarInsightsHandler {
	return &CodexRadarInsightsHandler{service: service}
}

func (h *CodexRadarInsightsHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "站长推荐暂时不可用"}})
		return
	}
	value, stale, err := h.service.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "站长推荐暂时不可用"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"generated_at": value.GeneratedAt, "source_updated_at": value.SourceUpdatedAt,
		"recommendations": value.Recommendations, "stale": stale,
	})
}
