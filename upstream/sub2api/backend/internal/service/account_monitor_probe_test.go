package service

import (
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
