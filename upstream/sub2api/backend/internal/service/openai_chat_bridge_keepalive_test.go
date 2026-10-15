//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bridgeHeartbeatWriter struct {
	gin.ResponseWriter
	once      sync.Once
	heartbeat chan struct{}
}

func (w *bridgeHeartbeatWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.HasPrefix(string(p), ":") {
		w.once.Do(func() { close(w.heartbeat) })
	}
	return n, err
}

func TestChatBridgeKeepaliveBeforeLargeRequestOutput(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "answer", true: "empty_failure"}[empty], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			collector := requesttiming.New(time.Now(), 0)
			ctx := requesttiming.With(context.Background(), collector)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			writer := &bridgeHeartbeatWriter{ResponseWriter: c.Writer, heartbeat: make(chan struct{})}
			c.Writer = writer
			reader, upstream := io.Pipe()
			defer reader.Close()
			defer upstream.Close()
			done := make(chan struct{})
			heartbeatBeforeOutput := make(chan bool, 1)
			go func() {
				defer close(done)
				select {
				case <-writer.heartbeat:
					heartbeatBeforeOutput <- true
				case <-time.After(2500 * time.Millisecond):
					heartbeatBeforeOutput <- false
				}
				wire := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
				if !empty {
					wire = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"
				}
				_, _ = io.WriteString(upstream, wire)
				_ = upstream.Close()
			}()
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
			result, err := svc.handleChatStreamingResponse(ctx, &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, c, &Account{ID: 541, Platform: PlatformOpenAI}, "model", "model", "model", time.Now(), openAISilentRefusalMinRequestBodyBytes)
			<-done
			require.True(t, <-heartbeatBeforeOutput, "large request must send heartbeat before upstream data")
			require.True(t, strings.HasPrefix(rec.Body.String(), ":\n\n"))
			if empty {
				var failover *UpstreamFailoverError
				require.True(t, errors.As(err, &failover), "heartbeat must not turn empty upstream into success")
				require.NotContains(t, rec.Body.String(), "data: [DONE]")
			} else {
				require.NoError(t, err)
				require.Equal(t, 2, result.Usage.OutputTokens)
				require.GreaterOrEqual(t, *result.FirstTokenMs, 900, "heartbeat must not set TTFT")
				require.Contains(t, rec.Body.String(), "answer")
				require.Contains(t, rec.Body.String(), "data: [DONE]")
			}
			collector.Finish(200, false)
			collector.WhenFinished(func(s requesttiming.Snapshot) {
				require.Contains(t, s.Events, "first_keepalive")
				require.Len(t, s.Streams, 1)
				require.LessOrEqual(t, s.Events["first_keepalive"], *s.Streams[0].FirstReadMS)
			})
		})
	}
}
