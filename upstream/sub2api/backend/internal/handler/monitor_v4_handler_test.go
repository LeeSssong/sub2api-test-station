package handler

import (
	"context"
	"encoding/json"
	"errors"
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

func TestMonitorV4ResponseOmitsLegacyOperationalFlag(t *testing.T) {
	body, err := json.Marshal(monitorV4GroupResponse{RealRequestCount: 10, RealSuccessCount: 9})
	require.NoError(t, err)
	require.NotContains(t, string(body), "current_operational")
	require.Contains(t, string(body), `"real_request_count":10`)
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

type monitorTimelineStub struct {
	monitorV4SnapshotterStub
	user   int64
	window service.MonitorV4Window
}

func (s *monitorTimelineStub) TimelineWithGranularity(_ context.Context, id int64, w service.MonitorV4Window, _ string, _ time.Time) (*service.MonitorV4Timeline, error) {
	s.user = id
	s.window = w
	return &service.MonitorV4Timeline{Window: w, SuccessRateBasis: "ops_sla", Points: []service.MonitorV4TimelinePoint{}}, nil
}
func TestMonitorV4TimelineHandlerAuthAndWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		user   int64
		window string
		code   int
	}{{0, "1h", 401}, {42, "invalid", 400}, {42, "24h&granularity=invalid", 400}, {42, "24h&granularity=day", 200}, {42, "24h", 200}} {
		stub := &monitorTimelineStub{}
		h := NewMonitorV4Handler(stub)
		rr := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rr)
		c.Request = httptest.NewRequest("GET", "/monitor-v4/timeline?window="+tc.window, nil)
		if tc.user > 0 {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: tc.user})
		}
		h.Timeline(c)
		require.Equal(t, tc.code, rr.Code)
		if tc.code == 200 {
			require.Equal(t, tc.user, stub.user)
			require.Equal(t, service.MonitorV4Window24H, stub.window)
			require.Contains(t, rr.Body.String(), `"success_rate_basis":"ops_sla"`)
		} else {
			require.Zero(t, stub.user)
		}
	}
}

type monitorV4SLASnapshotterStub struct{ monitorV4SnapshotterStub }

func (s *monitorV4SLASnapshotterStub) SLACounts(_ context.Context, snapshot *service.MonitorV4Snapshot) (map[int64]service.MonitorV4SLACounts, error) {
	return map[int64]service.MonitorV4SLACounts{6: {SuccessCount: 660, RequestCount: 667}, 7: {}}, nil
}
func TestMonitorV4SnapshotExposesSLAWithoutReplacingLegacyCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &monitorV4SLASnapshotterStub{monitorV4SnapshotterStub{snapshot: &service.MonitorV4Snapshot{Groups: []service.MonitorV4Group{{ID: 6, RealSuccessCount: 660, RealRequestCount: 841}, {ID: 7}}}}}
	c, r := func() (*gin.Context, *httptest.ResponseRecorder) {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest("GET", "/api/v1/monitor-v4?window=1h", nil)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		return c, r
	}()
	NewMonitorV4Handler(stub).Snapshot(c)
	require.Equal(t, 200, r.Code)
	var envelope struct {
		Data struct {
			Groups []map[string]any `json:"groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &envelope))
	require.Equal(t, float64(667), envelope.Data.Groups[0]["sla_request_count"])
	require.Equal(t, float64(660), envelope.Data.Groups[0]["sla_success_count"])
	require.Equal(t, float64(841), envelope.Data.Groups[0]["real_request_count"])
	require.Equal(t, float64(0), envelope.Data.Groups[1]["sla_request_count"])
}

type monitorV4SLAErrorStub struct{ monitorV4SnapshotterStub }

func (s *monitorV4SLAErrorStub) SLACounts(context.Context, *service.MonitorV4Snapshot) (map[int64]service.MonitorV4SLACounts, error) {
	return nil, errors.New("SLA unavailable")
}
func TestMonitorV4SnapshotFailsClosedWhenSLACountsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest("GET", "/api/v1/monitor-v4?window=1h", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	NewMonitorV4Handler(&monitorV4SLAErrorStub{monitorV4SnapshotterStub{snapshot: &service.MonitorV4Snapshot{}}}).Snapshot(c)
	require.Equal(t, 500, r.Code)
	require.NotContains(t, r.Body.String(), "real_request_count")
}
