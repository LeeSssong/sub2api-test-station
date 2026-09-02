package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
