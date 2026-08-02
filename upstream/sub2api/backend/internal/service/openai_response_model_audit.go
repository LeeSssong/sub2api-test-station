package service

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const maxActualResponseModelLength = 100

// ExtractOpenAIResponseModelJSON extracts only the model string from an OpenAI JSON response.
func ExtractOpenAIResponseModelJSON(body []byte) string {
	var envelope struct {
		Response json.RawMessage `json:"response"`
		Model    string          `json:"model"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	if len(envelope.Response) > 0 {
		var nested struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(envelope.Response, &nested) == nil {
			if model := normalizeActualResponseModel(nested.Model); model != "" {
				return model
			}
		}
	}
	return normalizeActualResponseModel(envelope.Model)
}

// ExtractOpenAIResponseModelSSEEvent extracts models only from terminal/relevant OpenAI SSE events.
func ExtractOpenAIResponseModelSSEEvent(eventType string, data []byte) string {
	event := strings.ToLower(strings.TrimSpace(eventType))
	if event == "" {
		// Standard Chat Completions streams omit an SSE event name. Restrict
		// extraction to the documented chunk object so arbitrary unnamed JSON
		// payloads cannot be mistaken for OpenAI response events.
		var chunk struct {
			Object string `json:"object"`
			Model  string `json:"model"`
		}
		if json.Unmarshal(data, &chunk) != nil || chunk.Object != "chat.completion.chunk" {
			return ""
		}
		return normalizeActualResponseModel(chunk.Model)
	}

	switch event {
	case "message", "response.completed", "response.done", "response.failed", "response.incomplete", "response.output_item.done":
		return ExtractOpenAIResponseModelJSON(data)
	default:
		return ""
	}
}

func normalizeActualResponseModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || !utf8.ValidString(model) {
		return ""
	}
	runes := []rune(model)
	if len(runes) > maxActualResponseModelLength {
		model = string(runes[:maxActualResponseModelLength])
	}
	return model
}
