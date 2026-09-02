package service

import (
	"context"
	"time"
)

// OpenAISchedulerAlgorithmVersion identifies the currently deployed
// multi-window quality selection semantics. It is server-owned data, never a
// configurable setting or a value supplied by clients.
const OpenAISchedulerAlgorithmVersion = "openai-multi-window-quality-v1"

// OpenAISchedulerLog is an immutable persisted scheduler decision event.
// Decision contains only the controlled event snapshot emitted by the
// scheduler; it never contains prompts, credentials, or upstream bodies.
type OpenAISchedulerLog struct {
	ID               int64          `json:"id"`
	EventAt          time.Time      `json:"event_at"`
	Platform         string         `json:"platform"`
	GroupID          *int64         `json:"group_id,omitempty"`
	LogicalRequestID string         `json:"logical_request_id"`
	AttemptID        string         `json:"attempt_id,omitempty"`
	AttemptNumber    int            `json:"attempt_number"`
	EventName        string         `json:"event_name"`
	AccountID        *int64         `json:"account_id,omitempty"`
	CanonicalModel   string         `json:"canonical_model,omitempty"`
	Outcome          string         `json:"outcome,omitempty"`
	FinalOutcome     string         `json:"final_outcome,omitempty"`
	SelectionLayer   string         `json:"selection_layer,omitempty"`
	AlgorithmVersion string         `json:"algorithm_version"`
	Decision         map[string]any `json:"decision,omitempty"`
}

type OpenAISchedulerLogCursor struct {
	EventAt time.Time
	ID      int64
}

type OpenAISchedulerLogListFilter struct {
	From      time.Time
	To        time.Time
	Cursor    *OpenAISchedulerLogCursor
	Limit     int
	GroupID   *int64
	AccountID *int64
	Outcome   string
	Mechanism string
	Query     string
}

type OpenAISchedulerLogList struct {
	Logs       []OpenAISchedulerLog `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type OpenAISchedulerLogTimeline struct {
	LogicalRequestID string               `json:"logical_request_id"`
	Attempts         []OpenAISchedulerLog `json:"attempts"`
}

// OpenAISchedulerLogRepository is isolated from usage and scheduler state.
// Persistence failures remain observability-only and cannot change routing.
type OpenAISchedulerLogRepository interface {
	BatchInsertOpenAISchedulerLogs(context.Context, []OpenAISchedulerLogInsert) (int, error)
	DeleteOpenAISchedulerLogsBefore(context.Context, time.Time, int) (int64, error)
	ListOpenAISchedulerLogs(context.Context, *OpenAISchedulerLogListFilter) (*OpenAISchedulerLogList, error)
	GetOpenAISchedulerLogTimeline(context.Context, string) (*OpenAISchedulerLogTimeline, error)
}
