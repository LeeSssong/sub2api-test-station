package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClassifyOpenAIUpstreamFailure_PreOutputTransientIsReplayable(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(http.StatusBadGateway, "bad gateway", nil, false, false)
	require.True(t, got.Transient)
	require.True(t, got.Retryable)
	require.True(t, got.SafeToReplay)
	require.False(t, got.Hard)
	require.False(t, got.OutputStarted)
}

func TestClassifyOpenAIUpstreamFailure_HardAuthNeverReplayable(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(http.StatusUnauthorized, "invalid api key", []byte(`{"error":{"type":"authentication_error"}}`), false, false)
	require.True(t, got.Hard)
	require.False(t, got.Transient)
	require.False(t, got.Retryable)
	require.False(t, got.SafeToReplay)
}

func TestClassifyOpenAIUpstreamFailure_HardFailedEventMarkersNeverFailover(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "model_not_found", body: `{"error":{"code":"model_not_found","message":"model not found"}}`},
		{name: "permission", body: `{"error":{"type":"permission_error","message":"permission denied"}}`},
		{name: "balance", body: `{"error":{"code":"insufficient_balance","message":"insufficient balance"}}`},
		{name: "forbidden", body: `{"error":{"message":"forbidden"}}`},
		{name: "unauthorized", body: `{"error":{"message":"unauthorized"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.body)
			message := extractOpenAISSEErrorMessage(payload)
			require.False(t, openAIStreamFailedEventShouldFailover(payload, message))
		})
	}
}

func TestUpstreamFailoverError_PostOutputDefensivelyStopsRetry(t *testing.T) {
	require.False(t, (&UpstreamFailoverError{
		OutputStarted:     true,
		NextAccountAction: NextAccountRetry,
	}).ShouldRetryNextAccount())
	require.True(t, (&UpstreamFailoverError{
		OutputStarted:            true,
		SafeToFailoverAfterWrite: true,
		NextAccountAction:        NextAccountRetry,
	}).ShouldRetryNextAccount())
}

func TestClassifyOpenAIUpstreamFailure_PostOutputReaderFailureIsNotFailoverable(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(0, "connection reset by peer", nil, true, false)
	require.True(t, got.OutputStarted)
	require.True(t, got.Transient)
	require.False(t, got.Retryable)
	require.False(t, got.SafeToFailoverAfterWrite)
}

func TestClassifyOpenAIUpstreamFailure_StatusZeroRequiresTransportSignature(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(0, "upstream timeout was reported by provider", nil, false, false)
	require.False(t, got.Transient)

	got = ClassifyOpenAIUpstreamFailure(0, "read tcp 10.0.0.1:443: i/o timeout", nil, false, false)
	require.True(t, got.Transient)
}

func TestClassifyOpenAIUpstreamFailure_CapacityPressureIsTransientEvenAfterDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, message, subtype string
		status                 int
	}{
		{"pending", "Too many pending requests", "pending_requests", 0},
		{"concurrency", "Concurrency limit exceeded for account", "account_concurrency", 429},
		{"rate", "Upstream rate limit exceeded", "rate_limit", 429},
		{"unavailable", "Service temporarily unavailable", "temporary_unavailable", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyOpenAIUpstreamFailure(tc.status, tc.message, nil, true, true)
			require.True(t, got.CapacityPressure)
			require.Equal(t, tc.subtype, got.CapacitySubtype)
			require.True(t, got.Transient)
			require.False(t, got.Retryable)
		})
	}
}

func TestClassifyOpenAIUpstreamFailure_DoesNotTreatArbitraryTextAsCapacity(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(0, "client cancelled while waiting", nil, false, false)
	require.False(t, got.CapacityPressure)
	require.False(t, got.Transient)

	got = ClassifyOpenAIUpstreamFailure(0, "request failed", []byte(`{"input":"Too many pending requests","error":{"type":"unknown","message":"request failed"}}`), false, false)
	require.False(t, got.CapacityPressure)
}

func TestClassifyOpenAIUpstreamFailure_NonModel404BlocksReplayWhenRequestWasSent(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(http.StatusNotFound, "endpoint route not found", []byte(`{"error":{"message":"endpoint route not found"}}`), false, false)

	if !got.Transient || got.Retryable || got.SafeToReplay {
		t.Fatalf("non-model 404 should retain diagnostics but block replay after a sent request, got %+v", got)
	}
	if got.Hard {
		t.Fatalf("non-model 404 must not be hard account/model failure: %+v", got)
	}
}

func TestClassifyOpenAIUpstreamFailure_Model404KeepsModelFailureSemantics(t *testing.T) {
	got := ClassifyOpenAIUpstreamFailure(http.StatusNotFound, "model not found", []byte(`{"error":{"code":"model_not_found"}}`), false, false)

	if got.Transient || got.Retryable || got.SafeToReplay || !got.Hard {
		t.Fatalf("model 404 should remain a hard model failure, got %+v", got)
	}
}

func TestNewRetryableOpenAIStreamErrorCarriesRecoveryContract(t *testing.T) {
	err := NewRetryableOpenAIStreamError(10*time.Second, "resp_123", true)
	var recovery *OpenAIStreamRecoveryError
	require.True(t, errors.As(err, &recovery))
	payload := recovery.RecoveryPayload()
	require.Equal(t, "upstream_stream_error", payload.Type)
	require.Equal(t, "Upstream response stream was interrupted", payload.Message)
	require.True(t, payload.Retryable)
	require.True(t, payload.ResumeSupported)
	require.Equal(t, 10, payload.RetryAfterSeconds)
	_, marshalErr := json.Marshal(payload)
	require.NoError(t, marshalErr)
	info, ok := OpenAIStreamRecoveryDetails(err)
	require.True(t, ok)
	require.True(t, info.OutputStarted)
	require.True(t, info.UsageKnown)
	require.Equal(t, "resp_123", info.ResponseID)
}
