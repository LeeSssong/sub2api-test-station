package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestCompletedEventContainsActualResponseModelAndNoPrompt(t *testing.T) {
	at := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	payload := RequestCompleted{RequestID: "r-1", AccountID: 1, Model: "requested", RequestedModel: "requested", UpstreamModel: "upstream", ActualResponseModel: "actual", PromptTokens: 1, CompletionTokens: 2, UserCharge: "1.00", ActualCost: "0.50", Currency: "USD", LatencyMS: 25}
	event, err := NewRequestCompletedEvent("core", at, payload)
	require.NoError(t, err)
	require.Contains(t, string(event.Payload), "actual_response_model")
	require.NotContains(t, string(event.Payload), "prompt_text")
	retry, err := NewRequestCompletedEvent("core", at, payload)
	require.NoError(t, err)
	require.Equal(t, event.EventID, retry.EventID, "request identity must produce an idempotent event id")
}

func TestHealthChangedEventUsesStableMinimalFacts(t *testing.T) {
	at := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	payload := HealthChanged{AccountID: 7, Status: "degraded", ErrorCategory: "rate_limited", ObservedAt: at, ProbeVersion: "probe-v2"}
	event, err := NewHealthChangedEvent("core", at, payload)
	require.NoError(t, err)
	require.Contains(t, string(event.Payload), "error_category")
	require.Contains(t, string(event.Payload), "probe_version")
	require.NotContains(t, string(event.Payload), "authorization")
	retry, err := NewHealthChangedEvent("core", at, payload)
	require.NoError(t, err)
	require.Equal(t, event.EventID, retry.EventID)
}

func TestHealthChangedEventIdentityUsesStructuredFields(t *testing.T) {
	at := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	a, err := NewHealthChangedEvent("core", at, HealthChanged{AccountID: 1, Status: "a:b", ErrorCategory: "c", ObservedAt: at, ProbeVersion: "v"})
	require.NoError(t, err)
	b, err := NewHealthChangedEvent("core", at, HealthChanged{AccountID: 1, Status: "a", ErrorCategory: "b:c", ObservedAt: at, ProbeVersion: "v"})
	require.NoError(t, err)
	require.NotEqual(t, a.EventID, b.EventID)
	c, err := NewHealthChangedEvent("core", at, HealthChanged{AccountID: 1, Status: "a:b", ErrorCategory: "c", ObservedAt: at, ProbeVersion: "v2"})
	require.NoError(t, err)
	require.Equal(t, a.EventID, c.EventID, "probe version is not an immutable health fact")
}
