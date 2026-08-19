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

type codexRadarCommunityGetterStub struct {
	value service.CodexRadarCommunity
	stale bool
	err   error
}

func (s codexRadarCommunityGetterStub) Get(context.Context) (service.CodexRadarCommunity, bool, error) {
	return s.value, s.stale, s.err
}

func TestCodexRadarCommunityHandlerSuccessAndStale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stale := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2/codexradar-community", nil)
		NewCodexRadarCommunityHandler(codexRadarCommunityGetterStub{value: service.CodexRadarCommunity{
			GeneratedAt: "2026-08-19T05:00:00Z",
			Tabs:        []service.CodexRadarCommunityTab{},
		}, stale: stale}).Get(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"stale":`)
		require.Contains(t, recorder.Body.String(), `"generated_at":"2026-08-19T05:00:00Z"`)
	}
}

func TestCodexRadarCommunityHandlerUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2/codexradar-community", nil)
	NewCodexRadarCommunityHandler(codexRadarCommunityGetterStub{err: service.ErrCodexRadarCommunityUnavailable}).Get(c)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "社区测试数据暂时不可用")
	require.NotContains(t, recorder.Body.String(), "codexradar")
}
