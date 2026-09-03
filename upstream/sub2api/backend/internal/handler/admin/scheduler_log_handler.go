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
	health := service.OpenAISchedulerLogSinkHealth{}
	if h.health != nil {
		health = h.health()
	}
	response.Success(c, gin.H{
		"items": summarizeOpenAISchedulerLogs(result.Logs), "next_cursor": result.NextCursor,
		"incomplete":    health.DroppedCount > 0 || health.WriteFailed > 0,
		"dropped_count": health.DroppedCount, "write_failed_count": health.WriteFailed,
		"from": from, "to": to,
	})
}

func summarizeOpenAISchedulerLogs(logs []service.OpenAISchedulerLog) []service.OpenAISchedulerLogSummary {
	byRequest := make(map[string]*service.OpenAISchedulerLogSummary)
	order := make([]string, 0)
	for _, event := range logs {
		id := strings.TrimSpace(event.LogicalRequestID)
		if id == "" {
			continue
		}
		summary := byRequest[id]
		if summary == nil {
			summary = &service.OpenAISchedulerLogSummary{LogicalRequestID: id, StartedAt: event.EventAt, FinalOutcome: "unknown"}
			byRequest[id] = summary
			order = append(order, id)
		}
		if summary.StartedAt.IsZero() || (!event.EventAt.IsZero() && event.EventAt.Before(summary.StartedAt)) {
			summary.StartedAt = event.EventAt
		}
		if event.CanonicalModel != "" {
			summary.CanonicalModel = event.CanonicalModel
		}
		if event.GroupID != nil {
			summary.GroupID = event.GroupID
		}
		if event.EventName == service.OpenAIEventSchedulerSelection && event.AccountID != nil && summary.SelectedAccountID == nil {
			summary.SelectedAccountID = event.AccountID
		}
		if event.AlgorithmVersion != "" {
			summary.AlgorithmVersion = event.AlgorithmVersion
		}
		if budget := schedulerDecisionInt(event.Decision, "runtime_retry_budget"); budget > summary.RuntimeRetryBudget {
			summary.RuntimeRetryBudget = budget
		}
		if switches := schedulerDecisionInt(event.Decision, "switch_count"); switches > summary.SwitchCount {
			summary.SwitchCount = switches
		}
		if event.FinalOutcome != "" {
			summary.FinalOutcome = event.FinalOutcome
		}
	}
	summaries := make([]service.OpenAISchedulerLogSummary, 0, len(order))
	for _, id := range order {
		summaries = append(summaries, *byRequest[id])
	}
	return summaries
}

func schedulerDecisionInt(decision map[string]any, key string) int {
	switch value := decision[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
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
	duration := time.Hour
	switch strings.TrimSpace(c.Query("time_range")) {
	case "", "1h":
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid time_range, expect 1h, 24h, or 7d")
	}
	from := to.Add(-duration)
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
