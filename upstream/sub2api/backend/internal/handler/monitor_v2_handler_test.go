package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monitorV2SnapshotterStub struct {
	snapshot *service.MonitorV2Snapshot
	window   service.MonitorV2Window
}

func (s *monitorV2SnapshotterStub) Snapshot(
	_ context.Context,
	window service.MonitorV2Window,
	_ time.Time,
) (*service.MonitorV2Snapshot, error) {
	s.window = window
	return s.snapshot, nil
}

func TestMonitorV2HandlerReturnsVersionedNoStoreContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	value := 99.5
	ttftP95 := 880.0
	latencyP95 := 2400.0
	stub := &monitorV2SnapshotterStub{
		snapshot: &service.MonitorV2Snapshot{
			ContractVersion: service.MonitorV2ContractVersion,
			Window:          service.MonitorV2Window7D,
			GeneratedAt:     time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			Groups: []service.MonitorV2Group{
				{
					ID:             7,
					Name:           "公开组",
					Platform:       service.PlatformOpenAI,
					RateMultiplier: 0.2,
					Status:         service.MonitorV2StatusOperational,
					Availability: service.MonitorV2Availability{
						MonitorV2Metric: service.MonitorV2Metric{
							State:       service.MonitorV2MetricAvailable,
							Value:       &value,
							SampleCount: 200,
						},
						SuccessCount:  199,
						EligibleCount: 200,
					},
					TTFTP95: service.MonitorV2Metric{
						State:       service.MonitorV2MetricAvailable,
						Value:       &ttftP95,
						SampleCount: 180,
					},
					LatencyP95: service.MonitorV2Metric{
						State:       service.MonitorV2MetricAvailable,
						Value:       &latencyP95,
						SampleCount: 199,
					},
					Models: []service.MonitorV2Model{
						{Name: "gpt-5.4", Status: service.MonitorStatusOperational},
					},
				},
			},
		},
	}
	handler := NewMonitorV2Handler(stub)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)

	handler.Snapshot(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, service.MonitorV2Window7D, stub.window)

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, "2", envelope.Data["contract_version"])
	require.Equal(t, "7d", envelope.Data["window"])
	groups, ok := envelope.Data["groups"].([]any)
	require.True(t, ok)
	require.Len(t, groups, 1)
	group := groups[0].(map[string]any)
	require.Equal(t, float64(880), group["ttft_p95"].(map[string]any)["value"])
	require.Equal(t, float64(180), group["ttft_p95"].(map[string]any)["sample_count"])
	require.Equal(t, float64(2400), group["latency_p95"].(map[string]any)["value"])
	require.Equal(t, float64(199), group["latency_p95"].(map[string]any)["sample_count"])

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
