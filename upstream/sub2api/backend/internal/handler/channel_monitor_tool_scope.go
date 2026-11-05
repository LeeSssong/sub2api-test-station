package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type channelMonitorToolAccess interface {
	GetAvailableGroups(context.Context, int64) ([]service.Group, error)
	GetGroupToolMappings(context.Context, []int64) (map[int64]service.GroupToolMapping, error)
}

// nil means the original independent page. A non-nil empty set means no access.
// Resolve mappings only after authorizing the account's groups; never accept client group IDs.
func channelMonitorToolScope(c *gin.Context, access channelMonitorToolAccess) (map[int64]bool, bool) {
	raw := c.Query("tool_id")
	if _, supplied := c.Request.URL.Query()["tool_id"]; !supplied {
		return nil, true
	}
	if raw == "" {
		response.BadRequest(c, "invalid tool id")
		return nil, false
	}
	if _, err := service.NormalizeGroupToolIDs([]string{raw}); err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "user not found in context")
		return nil, false
	}
	if access == nil {
		response.Error(c, http.StatusInternalServerError, "channel monitor tool authorization unavailable")
		return nil, false
	}
	groups, err := access.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	scope := make(map[int64]bool)
	if len(groups) == 0 {
		return scope, true
	}
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	mappings, err := access.GetGroupToolMappings(c.Request.Context(), ids)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	for _, g := range groups {
		if g.Status != "" && g.Status != service.StatusActive {
			continue
		}
		tools, _ := service.ResolveGroupTools(g.Platform, mappings[g.ID].ToolIDs)
		for _, tool := range tools {
			if tool == raw {
				scope[g.ID] = true
				break
			}
		}
	}
	return scope, true
}
