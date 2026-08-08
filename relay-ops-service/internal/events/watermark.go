package events

import "time"

type Watermark struct {
	Source       string    `json:"source"`
	LastEventID  string    `json:"last_event_id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ProcessedAt  time.Time `json:"processed_at"`
	Completeness string    `json:"completeness"`
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
