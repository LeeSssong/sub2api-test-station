package service

import (
	"errors"
	"testing"
	"time"
)

func TestAccountMonitorProbeResultRejectsSuccessfulEmptyStream(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(250 * time.Millisecond)

	result := buildAccountMonitorProbeResult(7, "gpt-4o-mini", startedAt, finishedAt, &accountMonitorProbeObserver{}, nil)
	if result.Status != "failed" || result.ErrorCode != "malformed_stream" || result.TTFTMS != nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestAccountMonitorProbeResultUsesFirstNonEmptyContentForTTFT(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	observer := &accountMonitorProbeObserver{}
	observer.observe(TestEvent{Type: "status", Text: "waiting"}, startedAt.Add(20*time.Millisecond))
	observer.observe(TestEvent{Type: "content", Text: "  "}, startedAt.Add(30*time.Millisecond))
	observer.observe(TestEvent{Type: "content", Text: "ok"}, startedAt.Add(80*time.Millisecond))
	observer.observe(TestEvent{Type: "content", Text: "later"}, startedAt.Add(120*time.Millisecond))

	result := buildAccountMonitorProbeResult(7, "gpt-4o-mini", startedAt, startedAt.Add(200*time.Millisecond), observer, nil)
	if result.Status != "success" || result.ErrorCode != "" || result.TTFTMS == nil || *result.TTFTMS != 80 {
		t.Fatalf("result = %#v", result)
	}
	if result.LatencyMS == nil || *result.LatencyMS != 200 {
		t.Fatalf("latency = %#v", result.LatencyMS)
	}
}

func TestAccountMonitorProbeResultClassifiesFatalErrorsWithHTTPStatus(t *testing.T) {
	startedAt := time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		message    string
		errorCode  string
		httpStatus int
	}{
		{name: "balance exhausted", message: "insufficient quota remaining", errorCode: "balance_exhausted"},
		{name: "http unauthorized", message: "API returned 401: invalid credentials", errorCode: "http_error", httpStatus: 401},
		{name: "antigravity chinese unauthorized", message: "API 返回 401: request rejected", errorCode: "http_error", httpStatus: 401},
		{name: "antigravity chinese payment required", message: "API 返回 402: request rejected", errorCode: "http_error", httpStatus: 402},
		{name: "antigravity chinese forbidden", message: "API 返回 403: request rejected", errorCode: "http_error", httpStatus: 403},
		{name: "antigravity chinese server error", message: "API 返回 500: upstream unavailable", errorCode: "http_error", httpStatus: 500},
		{name: "missing api key", message: "No API key available", errorCode: "invalid_auth"},
		{name: "authentication failed", message: "Chat Completions authentication failed", errorCode: "invalid_auth"},
		{name: "http server error", message: "Grok Responses API returned 500: upstream unavailable", errorCode: "http_error", httpStatus: 500},
		{name: "model name is not http status", message: "model gpt-401 unavailable", errorCode: "model_unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAccountMonitorProbeResult(
				7,
				"gpt-4o-mini",
				startedAt,
				startedAt.Add(200*time.Millisecond),
				&accountMonitorProbeObserver{},
				errors.New(tt.message),
			)
			if result.Status != "failed" || result.ErrorCode != tt.errorCode {
				t.Fatalf("result = %#v, want failed/%s", result, tt.errorCode)
			}
			if tt.httpStatus == 0 {
				if result.HTTPStatus != nil {
					t.Fatalf("http status = %#v, want nil", result.HTTPStatus)
				}
			} else if result.HTTPStatus == nil || *result.HTTPStatus != tt.httpStatus {
				t.Fatalf("http status = %#v, want %d", result.HTTPStatus, tt.httpStatus)
			}
		})
	}
}
