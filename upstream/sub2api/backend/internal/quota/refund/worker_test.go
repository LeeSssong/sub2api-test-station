package refund

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWorkerCompletesClaimedRefund(t *testing.T) {
	store := &workerStore{job: &Job{AdjustmentID: 7, Attempt: 1}}
	provider := &workerProvider{result: ProviderResult{RefundID: "r-1", State: "succeeded"}}
	w := Worker{Store: store, Provider: provider, Now: func() time.Time { return time.Unix(100, 0) }}
	processed, err := w.ProcessOne(context.Background())
	if err != nil || !processed || store.completed != 7 || provider.calls != 1 {
		t.Fatalf("processed=%v err=%v store=%+v provider=%+v", processed, err, store, provider)
	}
}

func TestWorkerKeepsTimeoutRetryable(t *testing.T) {
	store := &workerStore{job: &Job{AdjustmentID: 8, Attempt: 2}}
	provider := &workerProvider{err: errors.New("connection timeout")}
	w := Worker{Store: store, Provider: provider, Now: func() time.Time { return time.Unix(100, 0) }}
	processed, err := w.ProcessOne(context.Background())
	if err != nil || !processed || store.retryID != 8 || !store.retryAt.After(time.Unix(100, 0)) || store.deadLetter != 0 {
		t.Fatalf("unexpected retry outcome: %+v err=%v", store, err)
	}
}

func TestWorkerDeadLettersExplicitPermanentProviderFailure(t *testing.T) {
	store := &workerStore{job: &Job{AdjustmentID: 9, Attempt: 1}}
	w := Worker{Store: store, Provider: &workerProvider{err: ErrPermanentProvider}, Now: time.Now}
	processed, err := w.ProcessOne(context.Background())
	if err != nil || !processed || store.deadLetter != 9 || store.retryID != 0 {
		t.Fatalf("unexpected dead-letter outcome: %+v err=%v", store, err)
	}
}

func TestWorkerMarksAmbiguousProviderStateUnknown(t *testing.T) {
	store := &workerStore{job: &Job{AdjustmentID: 10, Attempt: 1}}
	w := Worker{Store: store, Provider: &workerProvider{result: ProviderResult{RefundID: "r-2", State: "pending"}}, Now: time.Now}
	processed, err := w.ProcessOne(context.Background())
	if err != nil || !processed || store.unknown != 10 {
		t.Fatalf("unexpected unknown outcome: %+v err=%v", store, err)
	}
}

type workerStore struct {
	job        *Job
	completed  int64
	retryID    int64
	retryAt    time.Time
	deadLetter int64
	unknown    int64
}

func (s *workerStore) Claim(context.Context) (*Job, error) { j := s.job; s.job = nil; return j, nil }
func (s *workerStore) Complete(_ context.Context, id int64, _ ProviderResult) error {
	s.completed = id
	return nil
}
func (s *workerStore) Retry(_ context.Context, id int64, at time.Time, _ error) error {
	s.retryID, s.retryAt = id, at
	return nil
}
func (s *workerStore) DeadLetter(_ context.Context, id int64, _ error) error {
	s.deadLetter = id
	return nil
}
func (s *workerStore) MarkUnknown(_ context.Context, id int64, _ ProviderResult) error {
	s.unknown = id
	return nil
}

type workerProvider struct {
	result ProviderResult
	err    error
	calls  int
}

func (p *workerProvider) Refund(context.Context, Job) (ProviderResult, error) {
	p.calls++
	return p.result, p.err
}
