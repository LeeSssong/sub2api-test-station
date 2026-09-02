package refund

import (
	"errors"
	"testing"
	"time"
)

func TestRetryDecisionKeepsTransientFailuresRetryable(t *testing.T) {
	for _, err := range []error{contextError{}, errors.New("timeout"), errors.New("connection reset") } {
		d := DecideRetry(err, 1, time.Unix(100, 0))
		if d.DeadLetter || d.Terminal { t.Fatalf("transient error became terminal: %+v", d) }
		if !d.Retry || !d.NextRetryAt.After(time.Unix(100, 0)) { t.Fatalf("invalid retry decision: %+v", d) }
	}
}

func TestRetryDecisionDeadLettersExplicitPermanentErrors(t *testing.T) {
	d := DecideRetry(ErrPermanentProvider, 1, time.Unix(100, 0))
	if !d.DeadLetter || !d.Terminal || d.Retry { t.Fatalf("expected dead letter: %+v", d) }
}

type contextError struct{}
func (contextError) Error() string { return "context deadline exceeded" }
