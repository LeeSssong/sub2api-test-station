package events

import "time"

const (
	CompletenessEmpty    = "empty"
	CompletenessPartial  = "partial"
	CompletenessComplete = "complete"
)

type Watermark struct {
	Source       string    `json:"source"`
	LastEventID  string    `json:"last_event_id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ProcessedAt  time.Time `json:"processed_at"`
	Completeness string    `json:"completeness"`
}

// ComparePosition orders integration events deterministically even when the
// source emits multiple events at the same timestamp.
func ComparePosition(leftAt time.Time, leftID string, rightAt time.Time, rightID string) int {
	if leftAt.Before(rightAt) {
		return -1
	}
	if leftAt.After(rightAt) {
		return 1
	}
	if leftID < rightID {
		return -1
	}
	if leftID > rightID {
		return 1
	}
	return 0
}

func (w Watermark) FreshnessSeconds(now time.Time) int64 {
	if w.ProcessedAt.IsZero() {
		return -1
	}
	seconds := int64(now.Sub(w.ProcessedAt).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}
