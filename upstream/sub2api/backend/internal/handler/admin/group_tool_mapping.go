package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

type groupToolMappingAdmin interface {
	GetGroupToolMapping(context.Context, int64) (service.GroupToolMapping, error)
	SetGroupToolMapping(context.Context, int64, []string, int64) (service.GroupToolMapping, error)
}

func (h *GroupHandler) GetToolMappings(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid group id")
		return
	}
	svc, ok := h.adminService.(groupToolMappingAdmin)
	if !ok {
		response.InternalError(c, "tool mapping unavailable")
		return
	}
	m, err := svc.GetGroupToolMapping(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, m)
}
func (h *GroupHandler) SetToolMappings(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid group id")
		return
	}
	svc, ok := h.adminService.(groupToolMappingAdmin)
	if !ok {
		response.InternalError(c, "tool mapping unavailable")
		return
	}
	actor, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || actor.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req struct {
		ToolIDs *[]string `json:"tool_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "tool_ids is required")
		return
	}
	m, err := svc.SetGroupToolMapping(c.Request.Context(), id, *req.ToolIDs, actor.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, m)
}
