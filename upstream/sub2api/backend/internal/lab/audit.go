package lab

import (
	"fmt"
	"strings"
	"time"
)

type AuditEvent struct {
	Action     string
	Source     string
	Payload    map[string]any
	OccurredAt time.Time
}

func NewAuditEvent(action string, payload map[string]any) (AuditEvent, error) {
	action = strings.TrimSpace(action)
	if action == "" {
		return AuditEvent{}, fmt.Errorf("lab audit action is required")
	}
	redacted := make(map[string]any, len(payload))
	for k, v := range payload {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") || strings.Contains(lk, "api_key") || strings.Contains(lk, "authorization") || strings.Contains(lk, "cookie") {
			redacted[k] = "[REDACTED]"
			continue
		}
		redacted[k] = v
	}
	return AuditEvent{Action: action, Source: "LAB_ONLY", Payload: redacted, OccurredAt: time.Now().UTC()}, nil
}
