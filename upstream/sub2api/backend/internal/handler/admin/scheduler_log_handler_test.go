package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type schedulerLogRepoStub struct {
	listFilter *service.OpenAISchedulerLogListFilter
	logs       []service.OpenAISchedulerLog
}

func (s *schedulerLogRepoStub) BatchInsertOpenAISchedulerLogs(context.Context, []service.OpenAISchedulerLogInsert) (int, error) {
	return 0, nil
}
func (s *schedulerLogRepoStub) DeleteOpenAISchedulerLogsBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (s *schedulerLogRepoStub) ListOpenAISchedulerLogs(_ context.Context, filter *service.OpenAISchedulerLogListFilter) (*service.OpenAISchedulerLogList, error) {
	s.listFilter = filter
	return &service.OpenAISchedulerLogList{Logs: s.logs}, nil
}
func (s *schedulerLogRepoStub) GetOpenAISchedulerLogTimeline(_ context.Context, logicalRequestID string) (*service.OpenAISchedulerLogTimeline, error) {
	return &service.OpenAISchedulerLogTimeline{LogicalRequestID: logicalRequestID, Attempts: s.logs}, nil
}

func TestSchedulerLogHandlerDefaultsToOneHourAndReturnsCursorList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &schedulerLogRepoStub{logs: []service.OpenAISchedulerLog{{ID: 7, LogicalRequestID: "req-1", AlgorithmVersion: service.OpenAISchedulerAlgorithmVersion, Outcome: "selected"}}}
	h := NewSchedulerLogHandler(repo, func() service.OpenAISchedulerLogSinkHealth {
		return service.OpenAISchedulerLogSinkHealth{DroppedCount: 2}
	})
	r := gin.New()
	r.GET("/admin/scheduler/logs", h.List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/scheduler/logs?limit=999", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.listFilter)
	require.Equal(t, 200, repo.listFilter.Limit)
	require.WithinDuration(t, time.Now().UTC().Add(-time.Hour), repo.listFilter.From, 3*time.Second)
	var body struct {
		Data struct {
			Items       []service.OpenAISchedulerLog `json:"items"`
			Incomplete  bool                         `json:"incomplete"`
			Dropped     uint64                       `json:"dropped_count"`
			WriteFailed uint64                       `json:"write_failed_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	require.True(t, body.Data.Incomplete)
	require.EqualValues(t, 2, body.Data.Dropped)
}

func TestSchedulerLogHandlerAcceptsSevenDayRangeAndReportsWriteFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &schedulerLogRepoStub{}
	h := NewSchedulerLogHandler(repo, func() service.OpenAISchedulerLogSinkHealth {
		return service.OpenAISchedulerLogSinkHealth{WriteFailed: 3}
	})
	r := gin.New()
	r.GET("/admin/scheduler/logs", h.List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/scheduler/logs?time_range=7d", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.WithinDuration(t, time.Now().UTC().Add(-7*24*time.Hour), repo.listFilter.From, 3*time.Second)
	var body struct {
		Data struct {
			Incomplete  bool   `json:"incomplete"`
			WriteFailed uint64 `json:"write_failed_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Data.Incomplete)
	require.EqualValues(t, 3, body.Data.WriteFailed)
}

func TestSchedulerLogHandlerRejectsInvalidFilterAndReturnsTimeline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &schedulerLogRepoStub{}
	h := NewSchedulerLogHandler(repo, nil)
	r := gin.New()
	r.GET("/admin/scheduler/logs", h.List)
	r.GET("/admin/scheduler/logs/:logical_request_id", h.GetTimeline)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/scheduler/logs?account_id=bad", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/scheduler/logs/req-1", nil))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSchedulerLogHandlerAggregatesEventsIntoOneLogicalRequestSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	startedAt := time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)
	accountID := int64(131)
	repo := &schedulerLogRepoStub{logs: []service.OpenAISchedulerLog{
		{ID: 1, EventAt: startedAt, LogicalRequestID: "req-1", EventName: service.OpenAIEventSchedulerSelection,
			AccountID: &accountID, CanonicalModel: "gpt-5.6", AlgorithmVersion: service.OpenAISchedulerAlgorithmVersion,
			Decision: map[string]any{"runtime_retry_budget": float64(4), "switch_count": float64(0)}},
		{ID: 2, EventAt: startedAt.Add(time.Second), LogicalRequestID: "req-1", EventName: service.OpenAIEventSchedulerRequestOutcome,
			FinalOutcome: "success", AlgorithmVersion: service.OpenAISchedulerAlgorithmVersion,
			Decision: map[string]any{"switch_count": float64(1)}},
	}}
	h := NewSchedulerLogHandler(repo, nil)
	r := gin.New()
	r.GET("/admin/scheduler/logs", h.List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/scheduler/logs", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items []struct {
				LogicalRequestID   string    `json:"logical_request_id"`
				StartedAt          time.Time `json:"started_at"`
				SelectedAccountID  *int64    `json:"selected_account_id"`
				RuntimeRetryBudget int       `json:"runtime_retry_budget"`
				SwitchCount        int       `json:"switch_count"`
				FinalOutcome       string    `json:"final_outcome"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, "req-1", body.Data.Items[0].LogicalRequestID)
	require.Equal(t, startedAt, body.Data.Items[0].StartedAt)
	require.Equal(t, accountID, *body.Data.Items[0].SelectedAccountID)
	require.Equal(t, 4, body.Data.Items[0].RuntimeRetryBudget)
	require.Equal(t, 1, body.Data.Items[0].SwitchCount)
	require.Equal(t, "success", body.Data.Items[0].FinalOutcome)
}
