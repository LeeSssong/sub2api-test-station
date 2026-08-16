package service

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

type ProbeKind string

const (
	ProbeKindMonitor   ProbeKind = "monitor"
	ProbeKindScheduled ProbeKind = "scheduled"
	ProbeKindManual    ProbeKind = "manual"
)

type ProbeUsageCompleteness string

const (
	ProbeUsageComplete ProbeUsageCompleteness = "complete"
	ProbeUsagePartial  ProbeUsageCompleteness = "partial"
	ProbeUsageUnknown  ProbeUsageCompleteness = "unknown"
)

type ProbeOutcome string

const (
	ProbeOutcomeSuccess ProbeOutcome = "success"
	ProbeOutcomeFailure ProbeOutcome = "failure"
)

var ErrAccountProbeCostConflict = errors.New("account probe cost payload conflicts with existing probe run")

type AccountProbeCostLog struct {
	ProbeRunID          string
	AccountID           int64
	GroupID             *int64
	ProbeKind           ProbeKind
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	AccountCost         *decimal.Decimal
	UsageCompleteness   ProbeUsageCompleteness
	ProbeOutcome        ProbeOutcome
	ErrorCode           *string
	CreatedAt           time.Time
}

type AccountProbeCostAggregate struct {
	GroupID           *int64
	AccountID         int64
	ProbeRequests     int64
	ProbeTokens       int64
	ProbeCost         *decimal.Decimal
	HasIncompleteCost bool
}

type AccountProbeCostRepository interface {
	Append(context.Context, AccountProbeCostLog) error
	ReadWindow(context.Context, time.Time, time.Time) ([]AccountProbeCostAggregate, error)
}
