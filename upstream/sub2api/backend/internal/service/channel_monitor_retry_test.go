//go:build unit

package service

import (
	"context"
	"testing"
)

func TestRunChannelMonitorCheckWithRetry_StopsAfterFirstSuccessfulResponse(t *testing.T) {
	attempts := 0
	result := runChannelMonitorCheckWithRetry(context.Background(), func(context.Context) *CheckResult {
		attempts++
		return &CheckResult{Status: MonitorStatusOperational}
	})

	if attempts != 1 {
		t.Fatalf("expected one attempt, got %d", attempts)
	}
	if result.Status != MonitorStatusOperational {
		t.Fatalf("expected operational result, got %q", result.Status)
	}
}

func TestRunChannelMonitorCheckWithRetry_RetriesFiveTimesUntilSixthAttemptSucceeds(t *testing.T) {
	attempts := 0
	result := runChannelMonitorCheckWithRetry(context.Background(), func(context.Context) *CheckResult {
		attempts++
		if attempts == monitorChannelCheckMaxAttempts {
			return &CheckResult{Status: MonitorStatusDegraded}
		}
		return &CheckResult{Status: MonitorStatusError}
	})

	if attempts != 6 {
		t.Fatalf("expected six total attempts, got %d", attempts)
	}
	if result.Status != MonitorStatusDegraded {
		t.Fatalf("expected degraded successful response, got %q", result.Status)
	}
}

func TestRunChannelMonitorCheckWithRetry_ReturnsFinalFailureAfterSixAttempts(t *testing.T) {
	attempts := 0
	result := runChannelMonitorCheckWithRetry(context.Background(), func(context.Context) *CheckResult {
		attempts++
		return &CheckResult{Status: MonitorStatusFailed, Message: string(rune('0' + attempts))}
	})

	if attempts != 6 {
		t.Fatalf("expected six total attempts, got %d", attempts)
	}
	if result.Message != "6" {
		t.Fatalf("expected final failure result, got message %q", result.Message)
	}
}

func TestRunChannelMonitorCheckWithRetry_StopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	result := runChannelMonitorCheckWithRetry(ctx, func(context.Context) *CheckResult {
		attempts++
		cancel()
		return &CheckResult{Status: MonitorStatusError}
	})

	if attempts != 1 {
		t.Fatalf("expected cancellation to stop retries, got %d attempts", attempts)
	}
	if result.Status != MonitorStatusError {
		t.Fatalf("expected first failed result, got %q", result.Status)
	}
}
