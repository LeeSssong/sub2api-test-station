package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClassifyStreamError(t *testing.T) {
	tests := []struct {
		name      string
		stage     StreamFailureStage
		err       error
		clientEnd bool
		want      StreamErrorClass
	}{
		{name: "unexpected eof", stage: StreamFailureStageUpstreamBodyRead, err: io.ErrUnexpectedEOF, want: StreamErrorClassUpstreamEOF},
		{name: "client cancel", stage: StreamFailureStageClientWrite, err: context.Canceled, clientEnd: true, want: StreamErrorClassClientDisconnected},
		{name: "connection reset", stage: StreamFailureStageUpstreamBodyRead, err: syscall.ECONNRESET, want: StreamErrorClassUpstreamConnectionReset},
		{name: "timeout", stage: StreamFailureStageUpstreamBodyRead, err: context.DeadlineExceeded, want: StreamErrorClassUpstreamTimeout},
		{name: "proxy dns", stage: StreamFailureStageUpstreamConnect, err: &net.DNSError{Err: "no such host"}, want: StreamErrorClassProxyOrDNSFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClassifyStreamError(tt.stage, tt.err, tt.clientEnd))
		})
	}
}

func TestSanitizeStreamErrorChainPreservesTransportEvidence(t *testing.T) {
	input := errors.New("Bearer secret-token request_id=req-1 https://provider.invalid/path?api_key=hidden: unexpected EOF")

	got := SanitizeStreamErrorChain(input)

	require.NotContains(t, got, "secret-token")
	require.NotContains(t, got, "api_key=hidden")
	require.Contains(t, got, "unexpected EOF")
}

func TestStreamObservationSnapshotTracksLifecycleWithoutEventBody(t *testing.T) {
	obs := NewStreamObservation(StreamObservationInput{
		RequestID:        "req-1",
		LogicalRequestID: "logical-1",
		AttemptID:        "attempt-1",
		ClientRequestID:  "client-1",
		ThreadID:         "thread-1",
		WindowID:         "window-1",
		SessionID:        "session-1",
		Environment:      "production",
		DeploymentCommit: "commit-1",
		ContainerSlot:    "blue",
		ContainerID:      "container-1",
		AccountID:        7,
		AccountName:      "account-7",
		Model:            "gpt-5.6-sol",
	})

	obs.RecordHeaders(StreamHeaders{HTTPStatus: 200, ContentType: "text/event-stream", ContentEncoding: "gzip", Protocol: "HTTP/2"})
	obs.RecordEvent("response.output_text.delta", 9, 128)
	obs.RecordVisibleOutput(128)
	obs.RecordTerminal("response.completed", "resp-1", 4, 2, 6, 256)

	snapshot := obs.Snapshot()
	require.Equal(t, StreamLifecycleStageTerminalEvent, snapshot.Stage)
	require.Equal(t, "response.output_text.delta", snapshot.LastEventType)
	require.Equal(t, "resp-1", snapshot.ResponseID)
	require.True(t, snapshot.SemanticOutputSeen)
	require.True(t, snapshot.TerminalEventValid)
	require.Equal(t, int64(256), snapshot.BytesRead)
	require.Empty(t, snapshot.EventBody)
	require.False(t, snapshot.CorrelationDegraded)
}

func TestStreamObservationDoesNotPersistEventBodyOrEmitPerDeltaContract(t *testing.T) {
	obs := NewStreamObservation(StreamObservationInput{RequestID: "req-1", LogicalRequestID: "logical-1", AttemptID: "attempt-1"})
	obs.RecordEvent("response.output_text.delta", 1, 10)
	obs.RecordEvent("response.output_text.delta", 2, 20)
	obs.RecordVisibleOutput(10)
	snapshot := obs.Snapshot()
	require.Equal(t, 2, snapshot.EventIndex)
	require.Empty(t, snapshot.EventBody)
	require.True(t, snapshot.SemanticOutputSeen)
}

func TestStreamObservationRootCauseRequiresSufficientEvidence(t *testing.T) {
	obs := NewStreamObservation(StreamObservationInput{RequestID: "req-1"})
	obs.RecordFailure(StreamFailureStageUpstreamBodyRead, io.ErrUnexpectedEOF, false)

	snapshot := obs.Snapshot()
	require.Equal(t, "insufficient_evidence", snapshot.RootCause)
	require.Equal(t, StreamErrorClassUpstreamEOF, snapshot.ErrorClass)
	require.Equal(t, StreamFailureStageUpstreamBodyRead, snapshot.FailureStage)
	require.True(t, snapshot.CorrelationDegraded)
}

func TestAppendOpsUpstreamErrorEmbedsSanitizedStreamSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/responses", nil)
	obs := BeginStreamObservation(c, "gpt-5.6-sol", "gpt-5.6-sol", PlatformOpenAI, &Account{ID: 7, Name: "account-7"})
	obs.RecordEvent("response.output_text.delta", 1, 12)
	obs.RecordFailure(StreamFailureStageUpstreamBodyRead, errors.New("Bearer secret-token: unexpected EOF"), false)

	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{Kind: "stream_failure", Message: "stream failed"})
	value, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := value.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.NotNil(t, events[0].StreamObservation)
	raw, err := json.Marshal(events[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-token")
	require.NotContains(t, string(raw), "EventBody")
	require.Contains(t, string(raw), "unexpected EOF")
}
