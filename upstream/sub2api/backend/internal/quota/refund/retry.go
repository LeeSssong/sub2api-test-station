package refund

import (
	"errors"
	"net"
	"strings"
	"time"
)

// ErrPermanentProvider marks a provider response that cannot succeed by
// retrying (for example, an invalid refund or an unsupported request).
var ErrPermanentProvider = errors.New("permanent refund provider error")

// RetryDecision is the worker's durable decision for a failed provider call.
// Retryable failures remain in pending/unknown/reconciling and must not be
// projected to the terminal failed state by the caller.
type RetryDecision struct {
	Retry       bool
	DeadLetter  bool
	Terminal    bool
	NextRetryAt time.Time
}

const (
	initialRetryDelay = 5 * time.Second
	maxRetryDelay     = time.Hour
)

// DecideRetry classifies provider errors without treating ordinary timeout,
// connection, or context failures as a permanent refund failure.
func DecideRetry(err error, attempt int, now time.Time) RetryDecision {
	if err == nil {
		return RetryDecision{}
	}
	if errors.Is(err, ErrPermanentProvider) {
		return RetryDecision{DeadLetter: true, Terminal: true}
	}

	return RetryDecision{Retry: true, NextRetryAt: now.Add(retryDelay(attempt))}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := initialRetryDelay
	for i := 1; i < attempt && delay < maxRetryDelay; i++ {
		if delay > maxRetryDelay/2 {
			return maxRetryDelay
		}
		delay *= 2
	}
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

// Kept as a small helper for future provider-specific classifiers. Transport
// and context-shaped errors are intentionally treated as uncertain outcomes.
func isTransientRefundError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"timeout", "timed out", "deadline exceeded", "connection reset", "connection refused", "connection closed", "temporary", "eof"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
