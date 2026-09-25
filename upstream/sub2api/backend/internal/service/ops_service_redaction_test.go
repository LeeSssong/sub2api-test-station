package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareErrorLogInput_RedactsUpstreamMessages(t *testing.T) {
	t.Parallel()
	message := "Authorization: Bearer top-message-secret\nprovider unavailable"
	entry := &OpsInsertErrorLogInput{
		UpstreamErrorMessage: &message,
		UpstreamErrors: []*OpsUpstreamErrorEvent{{
			UpstreamStatusCode: 502,
			Message:            "X-Goog-Api-Key: event-message-secret\nupstream timeout",
		}},
	}
	prepared, ok, err := (&OpsService{opsRepo: &opsRepoMock{}}).prepareErrorLogInput(nil, entry)
	if err != nil || !ok {
		t.Fatalf("prepare error log: ok=%v err=%v", ok, err)
	}
	if strings.Contains(*prepared.UpstreamErrorMessage, "top-message-secret") ||
		strings.Contains(*prepared.UpstreamErrorsJSON, "event-message-secret") {
		t.Fatalf("upstream messages retained credentials: message=%q events=%q", *prepared.UpstreamErrorMessage, *prepared.UpstreamErrorsJSON)
	}
	if !strings.Contains(*prepared.UpstreamErrorMessage, "provider unavailable") ||
		!strings.Contains(*prepared.UpstreamErrorsJSON, "upstream timeout") {
		t.Fatalf("diagnostic context lost: message=%q events=%q", *prepared.UpstreamErrorMessage, *prepared.UpstreamErrorsJSON)
	}
}

func TestSanitizeErrorBodyForStorage_RedactsPlainTextCredentials(t *testing.T) {
	t.Parallel()
	raw := "Authorization: Bearer raw-body-secret\nX-Goog-Api-Key: raw-upstream-secret\nprovider unavailable"
	out, _ := sanitizeErrorBodyForStorage(raw, 10*1024)
	if strings.Contains(out, "raw-body-secret") || strings.Contains(out, "raw-upstream-secret") {
		t.Fatalf("plain-text credentials were not redacted: %q", out)
	}
	if !strings.Contains(out, "provider unavailable") {
		t.Fatalf("non-sensitive diagnostic content lost: %q", out)
	}
}

func TestSanitizeErrorBodyForStorage_RedactsCredentialsInsideJSONMessage(t *testing.T) {
	t.Parallel()
	raw := `{"message":"Authorization: Bearer nested-secret","detail":"X-Goog-Api-Key: another-secret","max_tokens":128}`
	out, _ := sanitizeErrorBodyForStorage(raw, 10*1024)
	if strings.Contains(out, "nested-secret") || strings.Contains(out, "another-secret") {
		t.Fatalf("JSON message credentials were not redacted: %q", out)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("sanitized JSON must remain valid: %v", err)
	}
	if decoded["max_tokens"] != float64(128) {
		t.Fatalf("token budget changed: %#v", decoded["max_tokens"])
	}
}

func TestIsSensitiveKey_TokenBudgetKeysNotRedacted(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		"max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"max_tokens_to_sample",
		"budget_tokens",
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
	} {
		if isSensitiveKey(key) {
			t.Fatalf("expected key %q to NOT be treated as sensitive", key)
		}
	}

	for _, key := range []string{
		"authorization",
		"Authorization",
		"x-goog-api-key",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"token",
		"client_secret",
		"private_key",
		"signature",
	} {
		if !isSensitiveKey(key) {
			t.Fatalf("expected key %q to be treated as sensitive", key)
		}
	}
}

func TestSanitizeAndTrimJSONPayload_PreservesTokenBudgetFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"model":"claude-3","max_tokens":123,"thinking":{"type":"enabled","budget_tokens":456},"access_token":"abc","messages":[{"role":"user","content":"hi"}]}`)
	out, _, _ := sanitizeAndTrimJSONPayload(raw, 10*1024)
	if out == "" {
		t.Fatalf("expected non-empty sanitized output")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal sanitized output: %v", err)
	}

	if got, ok := decoded["max_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_tokens=123, got %#v", decoded["max_tokens"])
	}

	thinking, ok := decoded["thinking"].(map[string]any)
	if !ok || thinking == nil {
		t.Fatalf("expected thinking object to be preserved, got %#v", decoded["thinking"])
	}
	if got, ok := thinking["budget_tokens"].(float64); !ok || got != 456 {
		t.Fatalf("expected thinking.budget_tokens=456, got %#v", thinking["budget_tokens"])
	}

	if got := decoded["access_token"]; got != "[REDACTED]" {
		t.Fatalf("expected access_token to be redacted, got %#v", got)
	}
}

func TestSanitizeAndTrimJSONPayload_RedactsProviderAPIKey(t *testing.T) {
	t.Parallel()
	out, _, _ := sanitizeAndTrimJSONPayload([]byte(`{"x-goog-api-key":"provider-secret","status":"unavailable"}`), 10*1024)
	if strings.Contains(out, "provider-secret") || !strings.Contains(out, "unavailable") {
		t.Fatalf("provider API key redaction failed: %q", out)
	}
}

func TestShrinkToEssentials_IncludesThinking(t *testing.T) {
	t.Parallel()

	root := map[string]any{
		"model":      "claude-3",
		"max_tokens": 100,
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 200,
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "first"},
			map[string]any{"role": "user", "content": "last"},
		},
	}

	out := shrinkToEssentials(root)
	if _, ok := out["thinking"]; !ok {
		t.Fatalf("expected thinking to be included in essentials: %#v", out)
	}
}
