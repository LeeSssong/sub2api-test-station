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
	require.Equal(t, "gpt-5.6-terra", ExtractOpenAIResponseModelSSEEvent("response.completed", []byte(`{"response":{"model":"gpt-5.6-terra"}}`)))
	require.Equal(t, "gpt-5.6-terra", ExtractOpenAIResponseModelSSEEvent("response.output_item.done", []byte(`{"model":"gpt-5.6-terra"}`)))
	require.Equal(t, "gpt-5.6-terra", ExtractOpenAIResponseModelSSEEvent("message", []byte(`{"model":"gpt-5.6-terra"}`)))
	require.Empty(t, ExtractOpenAIResponseModelSSEEvent("response.output_text.delta", []byte(`{"model":"gpt-5.6-terra"}`)))
	require.Empty(t, ExtractOpenAIResponseModelSSEEvent("response.completed", []byte(`not-json`)))
}
