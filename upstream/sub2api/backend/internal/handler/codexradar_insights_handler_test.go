package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexRadarInsightsGetterStub struct {
	value service.CodexRadarInsights
	stale bool
	err   error
}

func (s codexRadarInsightsGetterStub) Get(context.Context) (service.CodexRadarInsights, bool, error) {
	return s.value, s.stale, s.err
}

func TestCodexRadarInsightsHandlerSuccessAndStale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stale := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2/codexradar-insights", nil)
		h := NewCodexRadarInsightsHandler(codexRadarInsightsGetterStub{value: service.CodexRadarInsights{
			GeneratedAt: "2026-08-19T01:59:16Z", SourceUpdatedAt: "2026-08-19T01:54:42Z",
			Recommendations: []service.CodexRadarRecommendation{},
		}, stale: stale})
		h.Get(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"stale":`)
		require.Contains(t, recorder.Body.String(), `"generated_at":"2026-08-19T01:59:16Z"`)
	}
}

func TestCodexRadarInsightsHandlerUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2/codexradar-insights", nil)
	NewCodexRadarInsightsHandler(codexRadarInsightsGetterStub{err: service.ErrCodexRadarUnavailable}).Get(c)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "站长推荐暂时不可用")
	require.NotContains(t, recorder.Body.String(), "codexradar")
}
