package service

import (
	"context"
	"time"
)

type OpsRequestKind string

const (
	OpsRequestKindSuccess OpsRequestKind = "success"
	OpsRequestKindError   OpsRequestKind = "error"
)

// OpsRequestDetail is a request-level view across success (usage_logs) and error (ops_error_logs).
// It powers "request drilldown" UIs without exposing full request bodies for successful requests.
type OpsRequestDetail struct {
	Kind                  OpsRequestKind `json:"kind"`
	CreatedAt             time.Time      `json:"created_at"`
	RequestID             string         `json:"request_id"`
	LogicalRequestID      string         `json:"logical_request_id,omitempty"`
	CorrelationQuality    string         `json:"correlation_quality,omitempty"`
	AttemptCount          int            `json:"attempt_count"`
	FailoverCount         int            `json:"failover_count"`
	UpstreamErrorCount    int            `json:"upstream_error_count"`
	FinalStatus           string         `json:"final_status,omitempty"`
	FinalProtocol         string         `json:"final_protocol,omitempty"`
	TerminalKind          string         `json:"terminal_kind"`
	TerminalReason        string         `json:"terminal_reason,omitempty"`
	UserVisible           bool           `json:"user_visible"`
	AutoRetryRecovered    bool           `json:"auto_retry_recovered"`
	RetryExhausted        bool           `json:"retry_exhausted"`
	StoppedUnsafeToReplay bool           `json:"stopped_unsafe_to_replay"`
	UnsafeToReplay        bool           `json:"unsafe_to_replay"`
	SwitchAllowed         bool           `json:"switch_allowed"`
	SwitchReason          string         `json:"switch_reason,omitempty"`
	UsageCompleteness     string         `json:"usage_completeness,omitempty"`
	UsagePresent          bool           `json:"usage_present"`
	FirstAttemptAt        *time.Time     `json:"first_attempt_at,omitempty"`
	CompletedAt           *time.Time     `json:"completed_at,omitempty"`
	TimeToFirstTokenMs    *int           `json:"time_to_first_token_ms,omitempty"`
	FinalErrorCode        string         `json:"final_error_code,omitempty"`

	Platform string `json:"platform,omitempty"`
	Model    string `json:"model,omitempty"`

	DurationMs *int `json:"duration_ms,omitempty"`
	StatusCode *int `json:"status_code,omitempty"`

	// When Kind == "error", ErrorID links to /admin/ops/errors/:id.
	ErrorID *int64 `json:"error_id,omitempty"`

	Phase    string `json:"phase,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`

	UserID    *int64 `json:"user_id,omitempty"`
	APIKeyID  *int64 `json:"api_key_id,omitempty"`
	AccountID *int64 `json:"account_id,omitempty"`
	GroupID   *int64 `json:"group_id,omitempty"`

	Stream bool `json:"stream"`
}

type OpsRequestDetailFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	// kind: success|error|all
	Kind string

	Platform string
	GroupID  *int64

	UserID    *int64
	APIKeyID  *int64
	AccountID *int64

	Model     string
	RequestID string
	Query     string

	MinDurationMs *int
	MaxDurationMs *int

	// Sort: created_at_desc (default) or duration_desc.
	Sort string

	Page     int
	PageSize int
}

func (f *OpsRequestDetailFilter) Normalize() (page, pageSize int, startTime, endTime time.Time) {
	page = 1
	pageSize = 50
	endTime = time.Now()
	startTime = endTime.Add(-1 * time.Hour)

	if f == nil {
		return page, pageSize, startTime, endTime
	}

	if f.Page > 0 {
		page = f.Page
	}
	if f.PageSize > 0 {
		pageSize = f.PageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}

	if f.EndTime != nil {
		endTime = *f.EndTime
	}
	if f.StartTime != nil {
		startTime = *f.StartTime
	} else if f.EndTime != nil {
		startTime = endTime.Add(-1 * time.Hour)
	}

	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
	}

	return page, pageSize, startTime, endTime
}

type OpsRequestDetailList struct {
	Items    []*OpsRequestDetail `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

func (s *OpsService) ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) (*OpsRequestDetailList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return &OpsRequestDetailList{
			Items:    []*OpsRequestDetail{},
			Total:    0,
			Page:     1,
			PageSize: 50,
		}, nil
	}

	page, pageSize, startTime, endTime := filter.Normalize()
	filterCopy := &OpsRequestDetailFilter{}
	if filter != nil {
		*filterCopy = *filter
	}
	filterCopy.Page = page
	filterCopy.PageSize = pageSize
	filterCopy.StartTime = &startTime
	filterCopy.EndTime = &endTime

	items, total, err := s.opsRepo.ListRequestDetails(ctx, filterCopy)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*OpsRequestDetail{}
	}

	return &OpsRequestDetailList{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
