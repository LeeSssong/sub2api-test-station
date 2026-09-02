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
			Items      []service.OpenAISchedulerLog `json:"items"`
			Incomplete bool                         `json:"incomplete"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	require.True(t, body.Data.Incomplete)
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
