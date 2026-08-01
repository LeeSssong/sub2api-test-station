package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountMonitorHandler struct {
	monitorService *service.AccountMonitorService
	runner         *service.AccountMonitorRunner
}

func NewAccountMonitorHandler(
	monitorService *service.AccountMonitorService,
	runner *service.AccountMonitorRunner,
) *AccountMonitorHandler {
	return &AccountMonitorHandler{
		monitorService: monitorService,
		runner:         runner,
	}
}

type accountMonitorSettingsRequest struct {
	IntervalSeconds int `json:"interval_seconds" binding:"required"`
}

type accountMonitorHistoryResponse struct {
	Items []dto.AccountMonitorHistoryItem `json:"items"`
}

type accountMonitorScoreWeightsRequest struct {
	Cost    int `json:"cost"`
	Success int `json:"success"`
	TTFT    int `json:"ttft"`
	Latency int `json:"latency"`
}

func (h *AccountMonitorHandler) List(c *gin.Context) {
	page, err := h.monitorService.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, page)
}

func (h *AccountMonitorHandler) UpdateSettings(c *gin.Context) {
	var req accountMonitorSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	settings, err := h.monitorService.UpdateSettings(
		c.Request.Context(),
		subject.UserID,
		req.IntervalSeconds,
	)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_INTERVAL", err.Error()))
		return
	}
	h.runner.ReloadSettings(settings)
	response.Success(c, settings)
}

func (h *AccountMonitorHandler) Run(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	completed, err := h.monitorService.RunAll(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, gin.H{"completed": completed})
}

func (h *AccountMonitorHandler) RunOne(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "invalid account id"))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	result, err := h.monitorService.RunOne(c.Request.Context(), subject.UserID, accountID)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MONITOR_RUN_FAILED", err.Error()))
		return
	}
	response.Success(c, result)
}

func (h *AccountMonitorHandler) History(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "invalid account id"))
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		limit, _ = strconv.Atoi(raw)
	}
	history, err := h.monitorService.History(c.Request.Context(), accountID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]dto.AccountMonitorHistoryItem, 0, len(history))
	for _, row := range history {
		items = append(items, dto.AccountMonitorHistoryItem{
			AccountID:  row.AccountID,
			ModelID:    row.ModelID,
			Status:     row.Status,
			ErrorCode:  row.ErrorCode,
			HTTPStatus: row.HTTPStatus,
			TTFTMS:     row.TTFTMS,
			LatencyMS:  row.LatencyMS,
			CheckedAt:  row.CheckedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	response.Success(c, accountMonitorHistoryResponse{Items: items})
}

func (h *AccountMonitorHandler) GetGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	weights, err := h.monitorService.GetGroupScoreWeights(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) UpdateGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	var req accountMonitorScoreWeightsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	weights, err := h.monitorService.UpdateGroupScoreWeights(c.Request.Context(), groupID, subject.UserID, service.AccountMonitorScoreWeights{
		Cost: req.Cost, Success: req.Success, TTFT: req.TTFT, Latency: req.Latency,
	})
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_SCORE_WEIGHTS", err.Error()))
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) ResetGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	weights, err := h.monitorService.ResetGroupScoreWeights(c.Request.Context(), groupID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}
