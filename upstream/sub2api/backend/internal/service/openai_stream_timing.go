package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/tidwall/gjson"
)

// Observe the upstream event before normalization/redaction. Only lengths and
// existing classifier decisions cross into the timing collector; no content,
// arbitrary event names, tool names or provider errors are retained.
func observeOpenAIStreamTiming(ctx context.Context, data, eventType, ttftMode string) {
	if requesttiming.From(ctx) == nil {
		return
	}
	o := requesttiming.StreamEventObservation{Type: eventType, PayloadBytes: len(data)}
	trimmed := strings.TrimSpace(data)
	if trimmed == "[DONE]" {
		o.Type = "[DONE]"
	} else if !gjson.Valid(trimmed) {
		o.Type = "invalid"
	} else {
		payload := gjson.Parse(trimmed)
		if o.Type == "" {
			o.Type = strings.TrimSpace(payload.Get("type").String())
		}
		o.Reasoning = strings.HasPrefix(o.Type, "response.reasoning_")
		o.ContentBytes = openAIStreamTimingTextBytes(payload)
		for _, path := range []string{"item", "part"} {
			item := payload.Get(path)
			content, encrypted, reasoning := openAIStreamTimingItemBytes(item)
			o.ContentBytes += content
			o.EncryptedBytes += encrypted
			o.Reasoning = o.Reasoning || reasoning
		}
		for _, item := range payload.Get("response.output").Array() {
			content, encrypted, reasoning := openAIStreamTimingItemBytes(item)
			o.ContentBytes += content
			o.EncryptedBytes += encrypted
			o.Reasoning = o.Reasoning || reasoning
		}
	}
	o.Semantic = openAIStreamDataStartsSemanticTTFT(trimmed, eventType)
	o.Visible = openAIStreamDataStartsVisibleOutput(trimmed, eventType)
	if normalizeOpenAITTFTMode(ttftMode) == OpenAITTFTModeVisible {
		o.TTFT = o.Visible
	} else {
		o.TTFT = openAIStreamDataStartsTTFT(trimmed, eventType, ttftMode)
	}
	// The Chat bridge measures its first data document, including metadata.
	if ttftMode == "first_data" {
		o.TTFT = trimmed != "[DONE]"
	}
	requesttiming.StreamEvent(ctx, o)
}

func openAIStreamTimingTextBytes(value gjson.Result) int {
	n := 0
	for _, path := range []string{"delta", "text", "arguments", "input", "result", "transcript", "refusal", "partial_image_b64", "audio"} {
		part := value.Get(path)
		if part.Type == gjson.String {
			n += len(part.String())
		}
	}
	return n
}

func openAIStreamTimingItemBytes(item gjson.Result) (content, encrypted int, reasoning bool) {
	content = openAIStreamTimingTextBytes(item)
	encrypted = len(item.Get("encrypted_content").String())
	reasoning = item.Get("type").String() == "reasoning"
	for _, path := range []string{"content", "summary"} {
		for _, part := range item.Get(path).Array() {
			content += openAIStreamTimingTextBytes(part)
		}
	}
	return
}

func observeOpenAIChatStreamTiming(ctx context.Context, payload string, semantic, ttft bool) {
	if requesttiming.From(ctx) == nil {
		return
	}
	o := requesttiming.StreamEventObservation{Type: "chat.chunk", PayloadBytes: len(payload), Semantic: semantic, Visible: semantic, TTFT: ttft}
	if payload == "[DONE]" {
		o.Type = "[DONE]"
	} else if !gjson.Valid(payload) {
		o.Type = "invalid"
	} else {
		if isOpenAIChatUsageOnlyStreamChunk(payload) {
			o.Type = "chat.usage"
		}
		for _, choice := range gjson.Get(payload, "choices").Array() {
			delta := choice.Get("delta")
			o.ContentBytes += len(delta.Get("content").String())
			thought := delta.Get("reasoning_content").String()
			o.ContentBytes += len(thought)
			o.Reasoning = o.Reasoning || thought != ""
			for _, tool := range delta.Get("tool_calls").Array() {
				o.ContentBytes += len(tool.Get("function.arguments").String())
			}
		}
	}
	requesttiming.StreamEvent(ctx, o)
}
