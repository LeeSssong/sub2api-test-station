package refund

import "fmt"

var retryable = map[string]bool{"pending": true, "unknown": true, "reconciling": true}

func ValidateTransition(from, to string) error {
	if from == to {
		return nil
	}
	if from == "completed" || from == "failed" {
		return fmt.Errorf("terminal refund state %q cannot transition", from)
	}
	if to == "failed" {
		if from != "not_started" && !retryable[from] {
			return fmt.Errorf("invalid refund transition %s -> %s", from, to)
		}
		if retryable[from] {
			return fmt.Errorf("retryable refund state %q requires explicit non-retryable classification", from)
		}
		return nil
	}
	if to == "completed" && (from == "not_started" || from == "requested" || retryable[from]) {
		return nil
	}
	if from == "not_started" && to == "requested" {
		return nil
	}
	if retryable[from] && retryable[to] {
		return nil
	}
	return fmt.Errorf("invalid refund transition %s -> %s", from, to)
}
