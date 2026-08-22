package lab

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// NotificationEvent is an in-memory lab observation. It is deliberately not a
// production notification fact source or a transport queue.
type NotificationEvent struct {
	Sequence   uint64
	Type       string
	Source     string
	Payload    map[string]any
	OccurredAt time.Time
}

// NotificationOutbox captures notification side effects for isolated lab tests
// without any SMTP, webhook, Feishu, SMS, or object-storage egress.
type NotificationOutbox struct {
	mu     sync.Mutex
	seq    uint64
	events []NotificationEvent
}

// NewNotificationOutbox only constructs the adapter for an explicit lab
// process. Any other value fails closed before an event can be emitted.
func NewNotificationOutbox(labOnly string) (*NotificationOutbox, error) {
	if strings.TrimSpace(labOnly) != "1" {
		return nil, fmt.Errorf("lab notification outbox rejected: LAB_ONLY must be 1")
	}
	return &NotificationOutbox{}, nil
}

// Enqueue records a redacted observation and never performs network I/O.
func (o *NotificationOutbox) Enqueue(ctx context.Context, event NotificationEvent) (NotificationEvent, error) {
	if o == nil {
		return NotificationEvent{}, fmt.Errorf("lab notification outbox is nil")
	}
	if err := ctx.Err(); err != nil {
		return NotificationEvent{}, err
	}
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		return NotificationEvent{}, fmt.Errorf("lab notification event type is required")
	}
	redacted := make(map[string]any, len(event.Payload))
	for key, value := range event.Payload {
		lower := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.seq++
	stored := NotificationEvent{Sequence: o.seq, Type: event.Type, Source: "LAB_ONLY", Payload: redacted, OccurredAt: time.Now().UTC()}
	o.events = append(o.events, stored)
	return cloneNotificationEvent(stored), nil
}

// Events returns a defensive snapshot for assertions and lab UI inspection.
func (o *NotificationOutbox) Events() []NotificationEvent {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make([]NotificationEvent, len(o.events))
	for i, event := range o.events {
		result[i] = cloneNotificationEvent(event)
	}
	return result
}

func cloneNotificationEvent(event NotificationEvent) NotificationEvent {
	payload := make(map[string]any, len(event.Payload))
	for key, value := range event.Payload {
		payload[key] = value
	}
	event.Payload = payload
	return event
}
