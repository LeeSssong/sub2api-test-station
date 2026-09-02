package refund

import (
	"context"
	"errors"
	"time"
)

var ErrWorkerUnavailable = errors.New("refund worker unavailable")

type Job struct {
	AdjustmentID int64
	Attempt      int
	RequestKey   string
	ProviderID   string
	Payload      map[string]string
}

type ProviderResult struct {
	RefundID string
	State    string
	Snapshot map[string]any
}

type WorkerStore interface {
	Claim(context.Context) (*Job, error)
	Complete(context.Context, int64, ProviderResult) error
	Retry(context.Context, int64, time.Time, error) error
	DeadLetter(context.Context, int64, error) error
	MarkUnknown(context.Context, int64, ProviderResult) error
}

type Provider interface {
	Refund(context.Context, Job) (ProviderResult, error)
}

type Worker struct {
	Store    WorkerStore
	Provider Provider
	Now      func() time.Time
}

// ProcessOne claims durable work first, then invokes the provider after the
// claim operation has returned. Store implementations therefore cannot keep a
// database transaction or user-wallet lock open across the provider call.
func (w Worker) ProcessOne(ctx context.Context) (bool, error) {
	if w.Store == nil || w.Provider == nil {
		return false, ErrWorkerUnavailable
	}
	job, err := w.Store.Claim(ctx)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	result, providerErr := w.Provider.Refund(ctx, *job)
	if providerErr != nil {
		now := time.Now()
		if w.Now != nil {
			now = w.Now()
		}
		decision := DecideRetry(providerErr, job.Attempt, now)
		if decision.DeadLetter {
			return true, w.Store.DeadLetter(ctx, job.AdjustmentID, providerErr)
		}
		return true, w.Store.Retry(ctx, job.AdjustmentID, decision.NextRetryAt, providerErr)
	}
	switch result.State {
	case "succeeded", "completed":
		return true, w.Store.Complete(ctx, job.AdjustmentID, result)
	case "failed":
		return true, w.Store.DeadLetter(ctx, job.AdjustmentID, ErrPermanentProvider)
	default:
		return true, w.Store.MarkUnknown(ctx, job.AdjustmentID, result)
	}
}
