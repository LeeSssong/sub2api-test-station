package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIExplicitContinueErrorProvidesRetryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	writeOpenAIExplicitContinueError(c, false)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "10", recorder.Header().Get("Retry-After"))
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, "upstream_temporarily_unavailable", payload.Get("error.type").String())
	require.True(t, payload.Get("error.retryable").Bool())
	require.False(t, payload.Get("error.resume_supported").Bool())
	require.Equal(t, int64(10), payload.Get("error.retry_after_seconds").Int())
	require.False(t, payload.Get("error.response_id").Exists())
}
