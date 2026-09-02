package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SchedulerLogHandler struct {
	repo   service.OpenAISchedulerLogRepository
	health func() service.OpenAISchedulerLogSinkHealth
}

func NewSchedulerLogHandler(repo service.OpenAISchedulerLogRepository, health func() service.OpenAISchedulerLogSinkHealth) *SchedulerLogHandler {
	return &SchedulerLogHandler{repo: repo, health: health}
}

func (h *SchedulerLogHandler) List(c *gin.Context) {
	if h == nil || h.repo == nil {
		response.Error(c, http.StatusServiceUnavailable, "Scheduler log service not available")
		return
	}
	from, to, err := parseSchedulerLogTimeRange(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
		}
	}
	if limit > 200 {
		limit = 200
	}
	filter := &service.OpenAISchedulerLogListFilter{From: from, To: to, Limit: limit,
		Outcome: strings.TrimSpace(c.Query("outcome")), Mechanism: strings.TrimSpace(c.Query("mechanism")), Query: strings.TrimSpace(c.Query("q"))}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		filter.AccountID = &id
	}
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		cursor, parseErr := repository.DecodeOpenAISchedulerLogCursor(raw)
		if parseErr != nil {
			response.BadRequest(c, parseErr.Error())
			return
		}
		filter.Cursor = cursor
	}
	result, err := h.repo.ListOpenAISchedulerLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	incomplete := false
	if h.health != nil {
		incomplete = h.health().DroppedCount > 0 || h.health().WriteFailed > 0
	}
	response.Success(c, gin.H{"items": result.Logs, "next_cursor": result.NextCursor, "incomplete": incomplete, "from": from, "to": to})
}

func (h *SchedulerLogHandler) GetTimeline(c *gin.Context) {
	if h == nil || h.repo == nil {
		response.Error(c, http.StatusServiceUnavailable, "Scheduler log service not available")
		return
	}
	id := strings.TrimSpace(c.Param("logical_request_id"))
	if id == "" {
		response.BadRequest(c, "logical_request_id is required")
		return
	}
	result, err := h.repo.GetOpenAISchedulerLogTimeline(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseSchedulerLogTimeRange(c *gin.Context) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.Add(-time.Hour)
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid from, expect RFC3339")
		}
		from = parsed.UTC()
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid to, expect RFC3339")
		}
		to = parsed.UTC()
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("from must be before to")
	}
	return from, to, nil
}
