package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type schedulerResolver struct {
	image string
	calls int
}

func (r *schedulerResolver) Resolve(context.Context, string) (string, error) {
	r.calls++
	return r.image, nil
}

type readinessResolver struct {
	image string
	err   error
}

func (r readinessResolver) Resolve(context.Context, string) (string, error) {
	return r.image, r.err
}

type schedulerExecutor struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
	result  ExecutionResult
	err     error
}

func (e *schedulerExecutor) Run(context.Context, Operation) (ExecutionResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	if e.entered != nil {
		e.entered <- struct{}{}
	}
	if e.release != nil {
		<-e.release
	}
	return e.result, e.err
}
func (e *schedulerExecutor) count() int { e.mu.Lock(); defer e.mu.Unlock(); return e.calls }

type cancellationExecutor struct {
	entered   chan struct{}
	cancelled chan struct{}
}

type fakeCandidatePreparer struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
	err     error
}

type deadlineCandidatePreparer struct {
	entered chan struct{}
}

func (p *deadlineCandidatePreparer) Prepare(ctx context.Context, _ string) error {
	close(p.entered)
	<-ctx.Done()
	return ctx.Err()
}

func (p *fakeCandidatePreparer) Prepare(context.Context, string) error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.release != nil {
		<-p.release
	}
	return p.err
}

func (p *fakeCandidatePreparer) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (e *cancellationExecutor) Run(ctx context.Context, _ Operation) (ExecutionResult, error) {
	close(e.entered)
	<-ctx.Done()
	close(e.cancelled)
	return ExecutionResult{}, ctx.Err()
}

func newSchedulerService(t *testing.T, now func() time.Time, executor *schedulerExecutor) (*Service, *schedulerResolver, *Store) {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	resolver := &schedulerResolver{image: "weishaw/sub2api:1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	service := NewService(store, resolver, executor, now)
	t.Cleanup(service.Close)
	return service, resolver, store
}

func TestServiceReadinessReportsReadyWithoutCreatingOperation(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	executor := &schedulerExecutor{}
	service := NewService(
		NewStore(statePath),
		readinessResolver{image: "xingqiao-sub2api:upstream-1.2.3"},
		executor,
	)
	t.Cleanup(service.Close)

	readiness, err := service.Readiness(context.Background(), "1.2.3")
	if err != nil || !readiness.Ready || readiness.TargetVersion != "1.2.3" {
		t.Fatalf("readiness=%#v err=%v", readiness, err)
	}
	if _, err := service.Status(); !errors.Is(err, ErrNoOperation) {
		t.Fatalf("readiness created operation: %v", err)
	}
	if executor.count() != 0 {
		t.Fatalf("readiness invoked executor %d times", executor.count())
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readiness created state file: %v", err)
	}
}

func TestServiceReadinessReportsCandidateNotReadyWithoutState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	executor := &schedulerExecutor{}
	service := NewService(NewStore(statePath), readinessResolver{err: ErrCandidateNotReady}, executor)
	t.Cleanup(service.Close)

	readiness, err := service.Readiness(context.Background(), "1.2.3")
	if err != nil || readiness.Ready || readiness.TargetVersion != "1.2.3" || readiness.Reason != "candidate_not_ready" {
		t.Fatalf("readiness=%#v err=%v", readiness, err)
	}
	if _, err := service.Status(); !errors.Is(err, ErrNoOperation) {
		t.Fatalf("readiness created operation: %v", err)
	}
	if executor.count() != 0 {
		t.Fatalf("readiness invoked executor %d times", executor.count())
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readiness created state file: %v", err)
	}
}

func TestServiceSchedulePersistsImmutableImageAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	s, resolver, store := newSchedulerService(t, func() time.Time { return now }, &schedulerExecutor{})
	first, err := s.Schedule(context.Background(), 3, "schedule", "1.2.3", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Schedule(context.Background(), 3, "schedule", "1.2.3", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID {
		t.Fatalf("ids = %q and %q", first.OperationID, second.OperationID)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolve calls = %d, want 1", resolver.calls)
	}
	stored, err := store.Load()
	if err != nil || stored == nil || stored.Image != resolver.image {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}
}

func TestServiceAllowsOnlyOneOperationUntilCancelled(t *testing.T) {
	now := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	s, _, _ := newSchedulerService(t, func() time.Time { return now }, &schedulerExecutor{})
	if _, err := s.Schedule(context.Background(), 1, "schedule", "1.2.3", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Schedule(context.Background(), 2, "schedule", "1.2.3", now.Add(2*time.Hour)); !errors.Is(err, ErrOperationExists) {
		t.Fatalf("error = %v", err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Schedule(context.Background(), 2, "schedule", "1.2.3", now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestServiceTimerFiresOnce(t *testing.T) {
	now := time.Now().UTC()
	executor := &schedulerExecutor{entered: make(chan struct{}, 2)}
	s, _, _ := newSchedulerService(t, time.Now, executor)
	if _, err := s.Schedule(context.Background(), 1, "schedule", "1.2.3", now.Add(25*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("operation did not run")
	}
	time.Sleep(50 * time.Millisecond)
	if got := executor.count(); got != 1 {
		t.Fatalf("execution count = %d", got)
	}
	op, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if op.Stage != "promoted" {
		t.Fatalf("stage = %q", op.Stage)
	}
}

func TestServiceCancellationBeforeStartPreventsExecution(t *testing.T) {
	now := time.Now().UTC()
	executor := &schedulerExecutor{}
	s, _, _ := newSchedulerService(t, time.Now, executor)
	if _, err := s.Schedule(context.Background(), 1, "schedule", "1.2.3", now.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := executor.count(); got != 0 {
		t.Fatalf("execution count = %d", got)
	}
}

func TestServiceCancellationLosesRaceToRunningJob(t *testing.T) {
	executor := &schedulerExecutor{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s, _, _ := newSchedulerService(t, time.Now, executor)
	if _, err := s.Schedule(context.Background(), 1, "now", "1.2.3", time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("operation did not begin")
	}
	if err := s.Cancel(); !errors.Is(err, ErrOperationRunning) {
		t.Fatalf("error = %v", err)
	}
	close(executor.release)
}

func TestServiceRecoversScheduledOperationAndExecutesOverdueOnStart(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	op := sampleOperation()
	op.ScheduledAt = time.Now().Add(-time.Second)
	op.Stage = "scheduled"
	if err := store.Save(op); err != nil {
		t.Fatal(err)
	}
	executor := &schedulerExecutor{entered: make(chan struct{}, 1)}
	s := NewService(store, &schedulerResolver{}, executor, time.Now)
	defer s.Close()
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("overdue operation did not execute")
	}
	if got := executor.count(); got != 1 {
		t.Fatalf("execution count = %d", got)
	}
}

func TestServiceRecoversInterruptedRunningOperationAsTerminalFailure(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	op := sampleOperation()
	op.Stage = "running"
	op.StartedAt = time.Now().Add(-time.Minute)
	if err := store.Save(op); err != nil {
		t.Fatal(err)
	}

	s := NewService(store, &schedulerResolver{}, &schedulerExecutor{}, time.Now)
	defer s.Close()

	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Stage != "failed" || status.Result != "interrupted" || status.Error == "" {
		t.Fatalf("status = %#v", status)
	}
	if status.CompletedAt.IsZero() {
		t.Fatal("interrupted operation has no completion time")
	}
}

func TestExecutionOutcomePreservesAdministratorIntervention(t *testing.T) {
	result := ExecutionResult{
		Stage:                "intervention_required",
		Result:               "migration_set_changed",
		Error:                "candidate migration set differs from the active release",
		InterventionRequired: true,
	}
	stage, message, failure := executionOutcome(result, nil)
	if stage != "intervention_required" || message != result.Result || failure != result.Error {
		t.Fatalf("outcome = (%q, %q, %q)", stage, message, failure)
	}
}

func TestServiceCloseCancelsRunningExecutor(t *testing.T) {
	executor := &cancellationExecutor{
		entered:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
	s, _, _ := newSchedulerService(t, time.Now, &schedulerExecutor{})
	s.executor = executor
	if _, err := s.Schedule(context.Background(), 1, "now", "1.2.3", time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("operation did not begin")
	}

	s.Close()
	select {
	case <-executor.cancelled:
	case <-time.After(time.Second):
		t.Fatal("running executor was not cancelled")
	}
}

func TestServiceAllowsNewOperationAfterTerminalResult(t *testing.T) {
	now := time.Now().UTC()
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	resolver := &schedulerResolver{image: "weishaw/sub2api:1.2.3@sha256:" + strings.Repeat("a", 64)}
	executor := &schedulerExecutor{
		entered: make(chan struct{}, 2),
		result:  ExecutionResult{Stage: "promoted", Result: "promoted", Promoted: true},
	}
	s := NewService(store, resolver, executor, time.Now)
	defer s.Close()

	if _, err := s.Schedule(context.Background(), 1, "now", "1.2.3", now); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("first operation did not run")
	}
	deadline := time.Now().Add(time.Second)
	for {
		status, err := s.Status()
		if err == nil && status.Stage == "promoted" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first operation did not reach terminal state: err=%v status=%#v", err, status)
		}
		time.Sleep(time.Millisecond)
	}

	executor.result = ExecutionResult{Stage: "promoted", Result: "promoted", Promoted: true}
	if _, err := s.Schedule(context.Background(), 2, "now", "1.2.3", time.Now()); err != nil {
		t.Fatalf("new operation after terminal result failed: %v", err)
	}
}

func TestServicePrepareCandidateIsIdempotentWhileInFlight(t *testing.T) {
	preparer := &fakeCandidatePreparer{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer s.Close()

	first, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil || first.Stage != "preparing" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	select {
	case <-preparer.entered:
	case <-time.After(time.Second):
		t.Fatal("preparer did not start")
	}
	second, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil || second.Stage != "preparing" || second.TargetVersion != first.TargetVersion {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if got := preparer.count(); got != 1 {
		t.Fatalf("prepare calls=%d, want 1", got)
	}

	close(preparer.release)
	deadline := time.Now().Add(time.Second)
	for {
		status, err := s.CandidateStatus()
		if err == nil && status.Stage == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate did not become ready: status=%#v err=%v", status, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServiceSurfacesCandidateTerminalPersistenceFailures(t *testing.T) {
	tests := []struct {
		name     string
		preparer CandidatePreparer
		terminal string
	}{
		{
			name:     "unavailable preparer",
			terminal: "failed",
		},
		{
			name:     "ready",
			preparer: &fakeCandidatePreparer{entered: make(chan struct{}, 1), release: make(chan struct{})},
			terminal: "ready",
		},
		{
			name:     "failed",
			preparer: &fakeCandidatePreparer{entered: make(chan struct{}, 1), release: make(chan struct{}), err: errors.New("candidate image pull failed")},
			terminal: "failed",
		},
		{
			name:     "target changed",
			preparer: &fakeCandidatePreparer{entered: make(chan struct{}, 1), release: make(chan struct{}), err: ErrTargetChanged},
			terminal: "target_changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(filepath.Join(t.TempDir(), "state.json"))
			service := NewServiceWithPreparer(store, &schedulerResolver{}, &schedulerExecutor{}, tt.preparer, time.Now)
			defer service.Close()

			if tt.preparer == nil {
				if err := os.Mkdir(store.candidatePath(), 0o700); err != nil {
					t.Fatal(err)
				}
				_, err := service.PrepareCandidate(context.Background(), 1, "1.2.3")
				if err == nil {
					t.Fatal("PrepareCandidate succeeded despite durable state write failure")
				}
				if _, err := service.CandidateStatus(); err == nil {
					t.Fatal("CandidateStatus returned an in-memory state after failed persistence")
				}
				return
			}

			preparer := tt.preparer.(*fakeCandidatePreparer)
			if _, err := service.PrepareCandidate(context.Background(), 1, "1.2.3"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-preparer.entered:
			case <-time.After(time.Second):
				t.Fatal("preparer did not start")
			}
			if err := os.Remove(store.candidatePath()); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(store.candidatePath(), 0o700); err != nil {
				t.Fatal(err)
			}
			close(preparer.release)

			deadline := time.Now().Add(time.Second)
			for {
				_, err := service.CandidateStatus()
				if err != nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("CandidateStatus returned in-memory %s state after failed persistence", tt.terminal)
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := service.Readiness(context.Background(), "1.2.3"); err == nil {
				t.Fatalf("Readiness returned in-memory %s state after failed persistence", tt.terminal)
			}
		})
	}
}

func TestServiceMarksHungCandidatePreparationFailedAfterConfiguredTimeout(t *testing.T) {
	preparer := &deadlineCandidatePreparer{entered: make(chan struct{})}
	service, err := NewServiceWithPreparerConfig(
		NewStore(filepath.Join(t.TempDir(), "state.json")),
		&schedulerResolver{}, &schedulerExecutor{}, preparer,
		ServiceConfig{CandidatePreparationTimeout: 25 * time.Millisecond}, time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	if _, err := service.PrepareCandidate(context.Background(), 1, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-preparer.entered:
	case <-time.After(time.Second):
		t.Fatal("preparer did not start")
	}

	deadline := time.Now().Add(time.Second)
	for {
		status, err := service.CandidateStatus()
		if err == nil && status.Stage == "failed" {
			if status.Reason != "candidate preparation timed out after 25ms" {
				t.Fatalf("timeout reason = %q", status.Reason)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("hung candidate did not fail: status=%#v err=%v", status, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServiceRejectsInvalidCandidatePreparationTimeout(t *testing.T) {
	_, err := NewServiceWithPreparerConfig(
		NewStore(filepath.Join(t.TempDir(), "state.json")),
		&schedulerResolver{}, &schedulerExecutor{}, &fakeCandidatePreparer{},
		ServiceConfig{CandidatePreparationTimeout: 0}, time.Now,
	)
	if err == nil || !strings.Contains(err.Error(), "candidate preparation timeout") {
		t.Fatalf("invalid timeout error = %v", err)
	}
}

func TestServicePrepareCandidateRecordsReadableFailureAndTargetChange(t *testing.T) {
	preparer := &fakeCandidatePreparer{err: errors.New("candidate image pull failed")}
	s := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer s.Close()
	if _, err := s.PrepareCandidate(context.Background(), 1, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		status, err := s.CandidateStatus()
		if err == nil && status.Stage == "failed" {
			if status.Reason != "candidate image pull failed" {
				t.Fatalf("reason=%q", status.Reason)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate did not fail: status=%#v err=%v", status, err)
		}
		time.Sleep(time.Millisecond)
	}

	preparer.err = ErrTargetChanged
	if _, err := s.PrepareCandidate(context.Background(), 1, "1.2.4"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		status, err := s.CandidateStatus()
		if err == nil && status.TargetVersion == "1.2.4" && status.Stage == "target_changed" {
			if status.Reason == "" {
				t.Fatal("target_changed status has no reason")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate did not record target change: status=%#v err=%v", status, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServicePrepareCandidateRetriesSameTargetAfterFailure(t *testing.T) {
	preparer := &fakeCandidatePreparer{err: errors.New("candidate image pull failed")}
	s := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer s.Close()
	first, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	waitForCandidateStage(t, s, "failed")

	preparer.err = nil
	second, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil || second.Stage != "preparing" || second.PreparationID == first.PreparationID {
		t.Fatalf("retry=%#v err=%v first=%#v", second, err, first)
	}
	waitForCandidateStage(t, s, "ready")
	if got := preparer.count(); got != 2 {
		t.Fatalf("prepare calls=%d, want 2", got)
	}
}

func TestServicePrepareCandidateRetriesSameTargetAfterTargetChange(t *testing.T) {
	preparer := &fakeCandidatePreparer{err: ErrTargetChanged}
	s := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer s.Close()
	first, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	waitForCandidateStage(t, s, "target_changed")

	preparer.err = nil
	second, err := s.PrepareCandidate(context.Background(), 1, "1.2.3")
	if err != nil || second.Stage != "preparing" || second.PreparationID == first.PreparationID {
		t.Fatalf("retry=%#v err=%v first=%#v", second, err, first)
	}
	waitForCandidateStage(t, s, "ready")
	if got := preparer.count(); got != 2 {
		t.Fatalf("prepare calls=%d, want 2", got)
	}
}

func TestServiceRestoresCandidatePreparationAfterRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	preparer := &fakeCandidatePreparer{}
	first := NewServiceWithPreparer(NewStore(statePath), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	if _, err := first.PrepareCandidate(context.Background(), 1, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	waitForCandidateStage(t, first, "ready")
	first.Close()

	second := NewServiceWithPreparer(NewStore(statePath), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer second.Close()
	status, err := second.CandidateStatus()
	if err != nil || status.Stage != "ready" || status.TargetVersion != "1.2.3" {
		t.Fatalf("restored status=%#v err=%v", status, err)
	}
}

func TestServiceRestoresCandidateFailureReasonAfterRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	preparer := &fakeCandidatePreparer{err: errors.New("candidate image pull failed")}
	first := NewServiceWithPreparer(NewStore(statePath), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	if _, err := first.PrepareCandidate(context.Background(), 1, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	waitForCandidateStage(t, first, "failed")
	first.Close()

	second := NewServiceWithPreparer(NewStore(statePath), &schedulerResolver{}, &schedulerExecutor{}, preparer, time.Now)
	defer second.Close()
	status, err := second.CandidateStatus()
	if err != nil || status.Stage != "failed" || status.Reason != "candidate image pull failed" {
		t.Fatalf("restored failure=%#v err=%v", status, err)
	}
}

func TestServiceMarksInterruptedCandidatePreparationFailedAfterRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(statePath)
	started := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	if err := store.SaveCandidate(CandidatePreparation{
		PreparationID: "preparing-id",
		TargetVersion: "1.2.3",
		Stage:         "preparing",
		StartedAt:     started,
	}); err != nil {
		t.Fatal(err)
	}

	service := NewServiceWithPreparer(store, &schedulerResolver{}, &schedulerExecutor{}, &fakeCandidatePreparer{}, func() time.Time { return started.Add(time.Minute) })
	defer service.Close()
	status, err := service.CandidateStatus()
	if err != nil || status.Stage != "failed" || status.Reason != "candidate preparation interrupted by updater restart" {
		t.Fatalf("interrupted status=%#v err=%v", status, err)
	}
}

func waitForCandidateStage(t *testing.T, service *Service, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		status, err := service.CandidateStatus()
		if err == nil && status.Stage == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate stage=%#v err=%v, want %q", status, err, want)
		}
		time.Sleep(time.Millisecond)
	}
}
