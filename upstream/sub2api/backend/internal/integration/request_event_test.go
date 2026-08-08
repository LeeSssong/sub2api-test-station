package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestCompletedEventContainsActualResponseModelAndNoPrompt(t *testing.T) {
	event, err := NewRequestCompletedEvent("core", time.Now(), RequestCompleted{RequestID: "r-1", AccountID: 1, Model: "requested", RequestedModel: "requested", UpstreamModel: "upstream", ActualResponseModel: "actual", PromptTokens: 1, CompletionTokens: 2, UserCharge: "1.00", ActualCost: "0.50", Currency: "USD", LatencyMS: 25})
	require.NoError(t, err)
	require.Contains(t, string(event.Payload), "actual_response_model")
	require.NotContains(t, string(event.Payload), "prompt_text")
}
