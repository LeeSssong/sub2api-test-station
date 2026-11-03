package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type lineSelectionHandlerStub struct {
	monitorV4SnapshotterStub
	value *service.MonitorV4LineSelection
	err   error
	calls int
}

func (s *lineSelectionHandlerStub) LineSelection(_ context.Context, userID int64, _ time.Time) (*service.MonitorV4LineSelection, error) {
	s.calls++
	s.userID = userID
	return s.value, s.err
}
func TestMonitorV4LineSelectionHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	end := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	zero := 0.0
	for _, tc := range []struct {
		name, query string
		auth        bool
		fail        bool
		status      int
	}{
		{"ok", "", true, false, 200}, {"24h", "?window=24h", true, false, 200},
		{"unsupported", "?window=7d", true, false, 400}, {"unauthenticated", "", false, false, 401}, {"failure", "", true, true, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &lineSelectionHandlerStub{value: &service.MonitorV4LineSelection{ContractVersion: "1", Window: service.MonitorV4Window24H, WindowStart: end.Add(-24 * time.Hour), WindowEnd: end, GeneratedAt: end.Add(7 * time.Minute), Groups: []service.MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 1, TTFTP50MS: &zero, TTFTSampleCount: 1}, {ID: 8}}}}
			if tc.fail {
				stub.err = errors.New("query failed")
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/monitor-v4/line-selection"+tc.query, nil)
			if tc.auth {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			}
			NewMonitorV4Handler(stub).LineSelection(c)
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			if tc.status != 200 {
				return
			}
			require.Equal(t, int64(42), stub.userID)
			var body struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Data, 6)
			require.Equal(t, "1", body.Data["contract_version"])
			require.Equal(t, "24h", body.Data["window"])
			require.Equal(t, "2026-11-03T04:07:00Z", body.Data["generated_at"])
			require.Equal(t, "2026-11-02T04:00:00Z", body.Data["window_start"])
			require.Equal(t, "2026-11-03T04:00:00Z", body.Data["window_end"])
			groups := body.Data["groups"].([]any)
			first := groups[0].(map[string]any)
			require.Len(t, first, 5)
			require.Equal(t, 0.0, first["ttft_p50_ms"])
			require.Equal(t, 1.0, first["sla_request_count"])
			require.Nil(t, groups[1].(map[string]any)["ttft_p50_ms"])
		})
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/monitor-v4/line-selection", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	NewMonitorV4Handler(&monitorV4SnapshotterStub{}).LineSelection(c)
	require.Equal(t, 500, rec.Code)
}
