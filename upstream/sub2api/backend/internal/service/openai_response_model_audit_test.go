package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIResponseModelJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"nested response", `{"response":{"model":" gpt-5.6-terra "}}`, "gpt-5.6-terra"},
		{"top level", `{"model":"gpt-5.6-sol"}`, "gpt-5.6-sol"},
		{"missing", `{"id":"resp_1"}`, ""},
		{"malformed", `{"model":`, ""},
		{"max length", `{"model":"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"}`, "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuv"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ExtractOpenAIResponseModelJSON([]byte(tc.body)))
		})
	}
}

func TestExtractOpenAIResponseModelSSEEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      string
		want      string
	}{
		{"responses completed", "response.completed", `{"response":{"model":"gpt-5.6-terra"}}`, "gpt-5.6-terra"},
		{"responses failed terminal", "response.failed", `{"response":{"model":"gpt-5.6-terra"}}`, "gpt-5.6-terra"},
		{"output item done", "response.output_item.done", `{"model":"gpt-5.6-terra"}`, "gpt-5.6-terra"},
		{"named message", "message", `{"model":"gpt-5.6-terra"}`, "gpt-5.6-terra"},
		{"chat completions chunk without event name", "", `{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-5.6-terra"}`, "gpt-5.6-terra"},
		{"chat completions done marker", "", `[DONE]`, ""},
		{"chat completions invalid data", "", `not-json`, ""},
		{"unnamed non-chat payload", "", `{"object":"response","model":"gpt-5.6-terra"}`, ""},
		{"delta event", "response.output_text.delta", `{"model":"gpt-5.6-terra"}`, ""},
		{"created event", "response.created", `{"response":{"model":"gpt-5.6-terra"}}`, ""},
		{"output item added", "response.output_item.added", `{"model":"gpt-5.6-terra"}`, ""},
		{"unknown event", "response.some_future_event", `{"model":"gpt-5.6-terra"}`, ""},
		{"malformed completion", "response.completed", `not-json`, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ExtractOpenAIResponseModelSSEEvent(tc.eventType, []byte(tc.data)))
		})
	}
}
