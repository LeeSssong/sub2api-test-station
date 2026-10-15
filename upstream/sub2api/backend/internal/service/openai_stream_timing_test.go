package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A missing hook in either Responses path must lose the event evidence, even
// though the client still receives a valid response.
func TestOpenAIResponsesStreamTimingEvidence(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeVisible, expiresAt: time.Now().Add(time.Minute).UnixNano()})
			t.Cleanup(func() {
				gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeSemantic, expiresAt: time.Now().Add(time.Minute).UnixNano()})
			})
			wire := "data: {\"type\":\"response.created\"}\n\n" +
				"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"reasoning\",\"summary\":[],\"encrypted_content\":\"private-encrypted-data\"}}\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"private-answer\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"
			collector := requesttiming.New(time.Now(), 0)
			ctx := requesttiming.With(context.Background(), collector)
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			req, outer := requesttiming.StartAttempt(req, 451, 0)
			req, physical := requesttiming.StartTransport(req)
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire)), Request: req}
			physical.Response(resp, nil)
			outer.Response(resp, nil)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = req
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
			account := &Account{ID: 451, Platform: PlatformOpenAI}
			var err error
			if passthrough {
				_, err = svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "test-model", "test-model")
			} else {
				_, err = svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "test-model", "test-model")
			}
			require.NoError(t, err)
			require.Contains(t, recorder.Body.String(), "private-answer")
			collector.Finish(200, false)
			collector.WhenFinished(func(snapshot requesttiming.Snapshot) {
				raw, marshalErr := json.Marshal(snapshot)
				require.NoError(t, marshalErr)
				require.NotContains(t, string(raw), "private-answer")
				require.NotContains(t, string(raw), "private-encrypted-data")
				var detail struct {
					Streams []struct {
						Attempt    int `json:"attempt"`
						ReadBytes  int `json:"read_bytes"`
						EventCount int `json:"event_count"`
						Events     []struct {
							Type           string `json:"type"`
							ContentBytes   int    `json:"content_bytes"`
							EncryptedBytes int    `json:"encrypted_bytes"`
							TTFTCount      int    `json:"ttft_count"`
						} `json:"events"`
					} `json:"streams"`
				}
				require.NoError(t, json.Unmarshal(raw, &detail))
				require.Len(t, detail.Streams, 1, "missing stream diagnostics")
				stream := detail.Streams[0]
				require.Equal(t, 2, stream.Attempt, "must identify the physical HTTP attempt")
				require.Equal(t, len(wire), stream.ReadBytes, "must not count nested body wrappers twice")
				require.Equal(t, 4, stream.EventCount)
				require.Len(t, stream.Events, 4)
				require.Equal(t, "response.output_item.added", stream.Events[1].Type)
				require.Equal(t, 22, stream.Events[1].EncryptedBytes)
				require.Zero(t, stream.Events[1].TTFTCount)
				require.Equal(t, "response.output_text.delta", stream.Events[2].Type)
				require.Equal(t, 14, stream.Events[2].ContentBytes)
				require.Equal(t, 1, stream.Events[2].TTFTCount)
			})
		})
	}
}

func TestOpenAIChatFallbackStreamTimingEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector := requesttiming.New(time.Now(), 0)
	ctx := requesttiming.With(context.Background(), collector)
	wire := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private-thought\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"private-answer\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	chunks := 0
	st := svc.scanCCStream(c, resp, "fixture", "fixture", time.Now(), func(chunk *apicompat.ChatCompletionsChunk) { chunks++ })
	require.NoError(t, st.Err)
	require.True(t, st.SawDone)
	require.Equal(t, 3, chunks)
	collector.Finish(200, false)
	collector.WhenFinished(func(s requesttiming.Snapshot) {
		raw, err := json.Marshal(s)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private-thought")
		require.NotContains(t, string(raw), "private-answer")
		require.Len(t, s.Streams, 1, "CC fallback also needs stream evidence")
		d := s.Streams[0]
		require.Equal(t, "chat_completions", d.Format)
		require.Equal(t, 4, d.EventCount)
		require.Equal(t, "chat.chunk", d.Events[0].Type)
		require.Equal(t, 1, d.Events[0].ReasoningCount)
		require.EqualValues(t, 29, d.Events[0].ContentBytes)
		require.Equal(t, "chat.usage", d.Events[1].Type)
		require.Zero(t, d.Events[1].TTFTCount)
		require.Equal(t, "[DONE]", d.Events[2].Type)
	})
}

type stagedTimingBody struct {
	steps   []string
	delays  []time.Duration
	index   int
	current *strings.Reader
}

func (b *stagedTimingBody) Read(p []byte) (int, error) {
	if b.current == nil || b.current.Len() == 0 {
		if b.index == len(b.steps) {
			return 0, io.EOF
		}
		time.Sleep(b.delays[b.index])
		b.current = strings.NewReader(b.steps[b.index])
		b.index++
	}
	return b.current.Read(p)
}
func (b *stagedTimingBody) Close() error { return nil }

func TestOpenAIStreamTimingDistinguishesLateBodyFromLateContent(t *testing.T) {
	preamble := "data: {\"type\":\"response.created\"}\n\n"
	reasoning := "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"reasoning\",\"summary\":[],\"encrypted_content\":\"opaque\"}}\n\n"
	answer := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n"
	terminal := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"
	for _, passthrough := range []bool{false, true} {
		for _, buffered := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/buffered=%v", passthrough, buffered), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeVisible, expiresAt: time.Now().Add(time.Minute).UnixNano()})
				t.Cleanup(func() {
					gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeSemantic, expiresAt: time.Now().Add(time.Minute).UnixNano()})
				})
				body := &stagedTimingBody{steps: []string{preamble, reasoning, answer, terminal}, delays: []time.Duration{0, 0, 25 * time.Millisecond, 0}}
				if buffered {
					body.steps = []string{preamble + reasoning + answer + terminal}
					body.delays = []time.Duration{25 * time.Millisecond}
				}
				collector := requesttiming.New(time.Now(), 0)
				ctx := requesttiming.With(context.Background(), collector)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				account := &Account{ID: 1, Platform: PlatformOpenAI}
				var err error
				if passthrough {
					_, err = svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "test-model", "test-model")
				} else {
					_, err = svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "test-model", "test-model")
				}
				require.NoError(t, err)
				collector.Finish(200, false)
				collector.WhenFinished(func(s requesttiming.Snapshot) {
					require.Len(t, s.Streams, 1)
					d := s.Streams[0]
					require.GreaterOrEqual(t, d.MaxReadWaitMS, 20.0)
					require.NotNil(t, d.FirstReadMS)
					require.Equal(t, 4, d.EventCount)
					if buffered {
						require.GreaterOrEqual(t, *d.FirstReadMS, 20.0)
						require.Equal(t, 1, d.ReadWindows[0].Calls)
					} else {
						require.GreaterOrEqual(t, d.Events[2].FirstMS-d.Events[0].FirstMS, 20.0)
						require.GreaterOrEqual(t, d.Events[2].FirstMS-*d.FirstReadMS, 20.0)
						require.Equal(t, 1, d.Events[1].ReasoningCount)
						require.Zero(t, d.Events[1].VisibleCount)
					}
				})
			})
		}
	}
}

func TestOpenAIRawChatStreamTimingEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector := requesttiming.New(time.Now(), 0)
	ctx := requesttiming.With(context.Background(), collector)
	wire := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"private-answer\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	result, err := svc.streamRawChatCompletions(ctx, c, resp, &Account{ID: 1, Platform: PlatformOpenAI}, "test-model", "test-model", "test-model", nil, nil, time.Now(), 0)
	require.NoError(t, err)
	require.NotNil(t, result.FirstTokenMs)
	collector.Finish(200, false)
	collector.WhenFinished(func(s requesttiming.Snapshot) {
		raw, marshalErr := json.Marshal(s)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(raw), "private-answer")
		require.Len(t, s.Streams, 1)
		d := s.Streams[0]
		require.Equal(t, 4, d.EventCount)
		require.Equal(t, 2, d.Events[0].TTFTCount, "record existing raw CC TTFT semantics, including role-only chunks")
		require.Equal(t, 1, d.Events[0].VisibleCount)
		require.Zero(t, d.Events[1].TTFTCount, "usage-only chunks must be excluded")
	})
}

func TestOpenAIStreamTimingRetainsUnsupportedContentEvidenceWithoutPayload(t *testing.T) {
	collector := requesttiming.New(time.Now(), 0)
	ctx := requesttiming.TrackStream(requesttiming.With(context.Background(), collector), &http.Response{Body: io.NopCloser(strings.NewReader(""))}, "responses")
	observeOpenAIStreamTiming(ctx, `{"type":"private-event-name","text":"private-content"}`, "private-event-name", OpenAITTFTModeVisible)
	collector.Finish(200, false)
	collector.WhenFinished(func(s requesttiming.Snapshot) {
		raw, err := json.Marshal(s)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private-event-name")
		require.NotContains(t, string(raw), "private-content")
		require.Equal(t, "unknown", s.Streams[0].Events[0].Type)
		require.EqualValues(t, 15, s.Streams[0].Events[0].ContentBytes)
		require.Zero(t, s.Streams[0].Events[0].VisibleCount)
		require.Zero(t, s.Streams[0].Events[0].TTFTCount)
	})
}

func TestOpenAIChatBridgeStreamTimingEvidence(t *testing.T) {
	for _, async := range []bool{false, true} {
		for _, disconnected := range []bool{false, true} {
			t.Run(fmt.Sprintf("async=%v/disconnected=%v", async, disconnected), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				collector := requesttiming.New(time.Now(), 0)
				ctx := requesttiming.With(context.Background(), collector)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
				req, outer := requesttiming.StartAttempt(req, 541, 0)
				req, physical := requesttiming.StartTransport(req)
				wire := "data: {\"type\": \"response.created\"}\n\ndata: {\"type\": \"response.output_text.delta\", \"delta\": \"private-answer\"}\n\ndata: {\"type\": \"response.completed\", \"response\": {\"usage\": {\"input_tokens\": 10, \"output_tokens\": 20}}}\n\n"
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire)), Request: req}
				physical.Response(resp, nil)
				outer.Response(resp, nil)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = req
				if disconnected {
					c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
				}
				cfg := &config.Config{}
				if async {
					cfg.Gateway.StreamKeepaliveInterval = 1
				}
				svc := &OpenAIGatewayService{cfg: cfg}
				result, err := svc.handleChatStreamingResponse(ctx, resp, c, &Account{ID: 541, Platform: PlatformOpenAI}, "model", "model", "model", time.Now(), 0)
				require.NoError(t, err)
				require.Equal(t, 20, result.Usage.OutputTokens)
				require.NotNil(t, result.FirstTokenMs)
				collector.Finish(200, false)
				collector.WhenFinished(func(s requesttiming.Snapshot) {
					require.Len(t, s.Streams, 1, "Chat to Responses bridge needs read and event evidence")
					require.Equal(t, 2, s.Streams[0].Attempt)
					require.EqualValues(t, len(wire), s.Streams[0].ReadBytes)
					require.NotNil(t, s.Streams[0].FirstReadMS)
					require.Equal(t, 3, s.Streams[0].EventCount)
					require.Equal(t, "first_data", s.TTFTMode)
					require.Equal(t, 1, s.Streams[0].Events[0].TTFTCount)
					require.Equal(t, "completed", s.Terminal)
					require.Equal(t, disconnected, s.ClientDisconnect)
					if disconnected {
						require.Equal(t, "client_disconnected", s.Outcome)
						require.Contains(t, s.Events, "client_write_error")
						require.NotContains(t, s.Events, "first_output_flush")
					} else {
						require.Equal(t, "success", s.Outcome)
						require.Contains(t, s.Events, "first_output_flush")
					}
					raw, err := json.Marshal(s)
					require.NoError(t, err)
					require.NotContains(t, string(raw), "private-answer")
				})
			})
		}
	}
}

func TestOpenAIChatBridgeDistinguishesReadFromFrameDelay(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprintf("fragmented=%v", fragmented), func(t *testing.T) {
			collector := requesttiming.New(time.Now(), 0)
			ctx := requesttiming.With(context.Background(), collector)
			first := "data: {\"type\":\"response.created\"}\n\n"
			tail := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"
			body := &stagedTimingBody{steps: []string{first, tail}, delays: []time.Duration{35 * time.Millisecond, 0}}
			if fragmented {
				body.steps = []string{first[:8], first[8:] + tail}
				body.delays = []time.Duration{0, 35 * time.Millisecond}
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			_, err := svc.handleChatStreamingResponse(ctx, resp, c, &Account{ID: 541, Platform: PlatformOpenAI}, "model", "model", "model", time.Now(), 0)
			require.NoError(t, err)
			collector.Finish(200, false)
			collector.WhenFinished(func(s requesttiming.Snapshot) {
				require.Len(t, s.Streams, 1)
				stream := s.Streams[0]
				require.NotNil(t, stream.FirstReadMS)
				require.GreaterOrEqual(t, stream.MaxReadWaitMS, float64(25))
				if fragmented {
					require.GreaterOrEqual(t, stream.Samples[0].AtMS-*stream.FirstReadMS, float64(25))
				} else {
					require.GreaterOrEqual(t, *stream.FirstReadMS, float64(25))
				}
			})
		})
	}
}
