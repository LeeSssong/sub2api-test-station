package handler

import (
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

type monitorV2SnapshotterStub struct {
	snapshot *service.MonitorV2Snapshot
	userID   int64
	window   service.MonitorV2Window
	calls    int
}

func (s *monitorV2SnapshotterStub) Snapshot(
	_ context.Context,
	userID int64,
	window service.MonitorV2Window,
	_ time.Time,
) (*service.MonitorV2Snapshot, error) {
	s.calls++
	s.userID = userID
	s.window = window
	return s.snapshot, nil
}

func TestMonitorV2HandlerReturnsVersionedNoStoreContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ttft := 880.0
	averageLatency := 2400.0
	latestCheckedAt := time.Date(2026, 7, 29, 11, 59, 30, 0, time.UTC)
	stub := &monitorV2SnapshotterStub{
		snapshot: &service.MonitorV2Snapshot{
			ContractVersion:        service.MonitorV2ContractVersion,
			Window:                 service.MonitorV2Window7D,
			RefreshIntervalSeconds: 300,
			GeneratedAt:            time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			Groups: []service.MonitorV2Group{
				{
					ID:              7,
					Name:            "公开组",
					Platform:        service.PlatformOpenAI,
					RateMultiplier:  0.2,
					Status:          service.MonitorV2StatusOperational,
					SourceUpdatedAt: &latestCheckedAt,
					TTFT: service.MonitorV2Metric{
						State:       service.MonitorV2MetricAvailable,
						Value:       &ttft,
						SampleCount: 180,
					},
					AverageLatency: service.MonitorV2Metric{
						State:       service.MonitorV2MetricAvailable,
						Value:       &averageLatency,
						SampleCount: 199,
					},
					Timeline: []service.MonitorV2TimelinePoint{{
						BucketStart: latestCheckedAt.Add(-time.Hour),
						Status:      service.MonitorV2StatusUnavailable,
						HasResult:   true,
					}},
				},
			},
		},
	}
	handler := NewMonitorV2Handler(stub)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)
	context.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	handler.Snapshot(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, service.MonitorV2Window7D, stub.window)
	require.Equal(t, int64(42), stub.userID)

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, service.MonitorV2ContractVersion, envelope.Data["contract_version"])
	require.Equal(t, float64(300), envelope.Data["refresh_interval_seconds"])
	require.Equal(t, "7d", envelope.Data["window"])
	groups, ok := envelope.Data["groups"].([]any)
	require.True(t, ok)
	require.Len(t, groups, 1)
	group := groups[0].(map[string]any)
	require.Equal(t, latestCheckedAt.Format(time.RFC3339), group["source_updated_at"])
	_, hasFlagship := group["is_flagship"]
	require.False(t, hasFlagship)
	_, hasModels := group["models"]
	require.False(t, hasModels)
	_, hasAvailability := group["availability"]
	require.True(t, hasAvailability)
	_, hasCacheHit := group["cache_hit"]
	require.False(t, hasCacheHit)
	require.Equal(t, float64(880), group["ttft"].(map[string]any)["value"])
	require.Equal(t, float64(180), group["ttft"].(map[string]any)["sample_count"])
	require.Equal(t, float64(2400), group["average_latency"].(map[string]any)["value"])
	require.Equal(t, float64(199), group["average_latency"].(map[string]any)["sample_count"])
	timeline := group["timeline"].([]any)
	require.Equal(t, true, timeline[0].(map[string]any)["has_result"])

	serialized := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{
		"user_id",
		"api_key",
		"account_id",
		"account_name",
		"supplier",
		"credential",
		"balance",
		"request_id",
		"client_ip",
		"user_agent",
		"prompt",
		"endpoint",
		"raw_error",
	} {
		require.NotContains(t, serialized, forbidden)
	}
}

func TestMonitorV2HandlerUsesAuthenticatedUserIDRegardlessOfRole(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			stub := &monitorV2SnapshotterStub{snapshot: &service.MonitorV2Snapshot{
				ContractVersion: service.MonitorV2ContractVersion,
				Window:          service.MonitorV2Window7D,
				Groups:          []service.MonitorV2Group{},
			}}
			handler := NewMonitorV2Handler(stub)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)
			context.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			context.Set(string(middleware.ContextKeyUserRole), role)

			handler.Snapshot(context)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, int64(42), stub.userID)
		})
	}
}

func TestMonitorV2HandlerRejectsMissingAuthenticatedSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &monitorV2SnapshotterStub{snapshot: &service.MonitorV2Snapshot{}}
	handler := NewMonitorV2Handler(stub)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)

	handler.Snapshot(context)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Zero(t, stub.calls)
}

func TestMonitorV2HandlerRejectsUnsupportedWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMonitorV2Handler(&monitorV2SnapshotterStub{})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=15d", nil)

	handler.Snapshot(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Body.String(), "unsupported monitor window")
}
