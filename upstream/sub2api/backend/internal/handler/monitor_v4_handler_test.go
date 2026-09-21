package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monitorV4SnapshotterStub struct {
	snapshot *service.MonitorV4Snapshot
	userID   int64
	window   service.MonitorV4Window
}

func (s *monitorV4SnapshotterStub) Snapshot(
	_ context.Context,
	userID int64,
	window service.MonitorV4Window,
	_ time.Time,
) (*service.MonitorV4Snapshot, error) {
	s.userID = userID
	s.window = window
	return s.snapshot, nil
}

func TestMonitorV4HandlerReturnsCacheHitRateContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cacheHitRate := 0.4
	stub := &monitorV4SnapshotterStub{snapshot: &service.MonitorV4Snapshot{
		ContractVersion:        service.MonitorV4ContractVersion,
		Window:                 service.MonitorV4Window1H,
		RefreshIntervalSeconds: 300,
		GeneratedAt:            time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		Groups: []service.MonitorV4Group{
			{ID: 7, Name: "Cached", CacheHitRate: &cacheHitRate},
			{ID: 8, Name: "No successful real request"},
		},
	}}
	handler := NewMonitorV4Handler(stub)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v4?window=1h", nil)
	ginContext.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	handler.Snapshot(ginContext)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, int64(42), stub.userID)
	require.Equal(t, service.MonitorV4Window1H, stub.window)

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, "2026-08-31T12:00:00Z", envelope.Data["generated_at"])
	require.Equal(t, map[string]any{
		"contract_version":         "2",
		"window":                   "1h",
		"refresh_interval_seconds": float64(300),
		"generated_at":             "2026-08-31T12:00:00Z",
		"groups":                   envelope.Data["groups"],
	}, envelope.Data)
	groups, ok := envelope.Data["groups"].([]any)
	require.True(t, ok)
	require.Len(t, groups, 2)

	withSamples := groups[0].(map[string]any)
	require.Equal(t, cacheHitRate, withSamples["cache_hit_rate"])
	require.Nil(t, withSamples["cache_read_tokens_p95"])
	require.Equal(t, float64(0), withSamples["cache_read_tokens_sample_count"])

	withoutSamples := groups[1].(map[string]any)
	require.Nil(t, withoutSamples["cache_hit_rate"])
	require.Nil(t, withoutSamples["cache_read_tokens_p95"])
	require.Equal(t, float64(0), withoutSamples["cache_read_tokens_sample_count"])
}

type monitorV4CheckerStub struct {
	monitorV4SnapshotterStub
	ids  []int64
	user int64
}

func (s *monitorV4CheckerStub) Check(_ context.Context, user int64, ids []int64) ([]service.MonitorV4CheckResult, error) {
	s.ids = ids
	s.user = user
	return []service.MonitorV4CheckResult{{GroupID: 7, Status: "success"}}, nil
}
func TestMonitorV4ManualCheckUsesAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &monitorV4CheckerStub{}
	h := NewMonitorV4Handler(stub)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/monitor-v4/check", strings.NewReader(`{"group_ids":[7],"user_id":99}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.Check(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.user)
	require.Equal(t, []int64{7}, stub.ids)
	require.NotContains(t, recorder.Body.String(), "account_id")
}
func TestMonitorV4ManualCheckRejectsOversizedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &monitorV4CheckerStub{}
	h := NewMonitorV4Handler(stub)
	ids := make([]int64, 101)
	data, _ := json.Marshal(map[string]any{"group_ids": ids})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/monitor-v4/check", bytes.NewReader(data))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.Check(c)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Nil(t, stub.ids)
}
