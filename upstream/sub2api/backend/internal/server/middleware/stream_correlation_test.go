package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientRequestIDCapturesStreamCorrelationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientRequestID())
	router.GET("/responses", func(c *gin.Context) {
		require.Equal(t, "thread-1", c.Request.Context().Value(ctxkey.ThreadID))
		require.Equal(t, "window-1", c.Request.Context().Value(ctxkey.WindowID))
		require.Equal(t, "session-1", c.Request.Context().Value(ctxkey.SessionID))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/responses", nil)
	req.Header.Set("X-Codex-Thread-Id", "thread-1")
	req.Header.Set("X-Codex-Window-Id", "window-1")
	req.Header.Set("X-Session-Id", "session-1")

	router.ServeHTTP(httptest.NewRecorder(), req)
}

func TestClientRequestIDOmitsInvalidOptionalCorrelationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientRequestID())
	router.GET("/responses", func(c *gin.Context) {
		_, hasThread := c.Request.Context().Value(ctxkey.ThreadID).(string)
		require.False(t, hasThread)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/responses", nil)
	req.Header.Set("X-Codex-Thread-Id", "   ")
	req.Header.Set("X-Codex-Window-Id", string(make([]byte, maxPersistentRequestIDBytes+1)))

	router.ServeHTTP(httptest.NewRecorder(), req)
}
