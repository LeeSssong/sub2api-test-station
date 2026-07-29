package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const stateSchemaVersion = 1

var (
	ErrOperationExists  = errors.New("an update operation already exists")
	ErrOperationRunning = errors.New("update operation is already running")
	ErrNoOperation      = errors.New("no update operation exists")
	ErrServiceClosed    = errors.New("updater service is closed")
)

type Operation struct {
	SchemaVersion int       `json:"schema_version"`
	OperationID   string    `json:"operation_id"`
	ActorID       int64     `json:"actor_id"`
	Mode          string    `json:"mode"`
	TargetVersion string    `json:"target_version"`
	Image         string    `json:"image"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Stage         string    `json:"stage"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type ExecutionResult struct {
	Stage          string
	Result         string
	Error          string
	Promoted       bool
	RolledBack     bool
	RollbackFailed bool
}

type Identity struct {
	ID     int64
	Role   string
	Status string
}

type Resolver interface {
	Resolve(context.Context, string) (string, error)
}

type Readiness struct {
	TargetVersion string `json:"target_version"`
	Ready         bool   `json:"ready"`
	Reason        string `json:"reason,omitempty"`
}

type Executor interface {
	Run(context.Context, Operation) (ExecutionResult, error)
}

// Service owns at most one non-terminal operation and one timer.
type Service struct {
	store    *Store
	resolver Resolver
	executor Executor
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc

	mu      sync.Mutex
	op      *Operation
	last    *Operation
	loadErr error
	timer   *time.Timer
	closed  bool
	wg      sync.WaitGroup
}

// NewService accepts an optional clock for deterministic callers and tests.
func NewService(store *Store, resolver Resolver, executor Executor, clocks ...func() time.Time) *Service {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		store:    store,
		resolver: resolver,
		executor: executor,
		now:      now,
		ctx:      ctx,
		cancel:   cancel,
	}
	if op, err := store.Load(); err != nil {
		s.loadErr = err
	} else if op != nil {
		switch op.Stage {
		case "scheduled", "running":
			s.op = op
			s.recoverLocked()
		default:
			s.last = op
		}
	}
	return s
}

func (s *Service) Schedule(ctx context.Context, actorID int64, mode, targetVersion string, scheduledAt time.Time) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Operation{}, ErrServiceClosed
	}
	if s.loadErr != nil {
		return Operation{}, s.loadErr
	}
	if s.op != nil {
		if sameOperation(*s.op, actorID, mode, targetVersion, scheduledAt) {
			return *s.op, nil
		}
		return Operation{}, ErrOperationExists
	}
	image, err := s.resolver.Resolve(ctx, targetVersion)
	if err != nil {
		return Operation{}, fmt.Errorf("resolve update image: %w", err)
	}
	now := s.now().UTC()
	if mode == "now" {
		scheduledAt = now
	}
	op := Operation{SchemaVersion: stateSchemaVersion, OperationID: newOperationID(), ActorID: actorID, Mode: mode, TargetVersion: targetVersion, Image: image, ScheduledAt: scheduledAt.UTC(), CreatedAt: now, Stage: "scheduled"}
	if err := s.store.Save(op); err != nil {
		return Operation{}, err
	}
	s.op = &op
	s.last = &op
	s.armLocked()
	return op, nil
}

func (s *Service) Readiness(ctx context.Context, targetVersion string) (Readiness, error) {
	if _, err := s.resolver.Resolve(ctx, targetVersion); err != nil {
		if errors.Is(err, ErrCandidateNotReady) {
			return Readiness{TargetVersion: targetVersion, Reason: "candidate_not_ready"}, nil
		}
		return Readiness{}, err
	}
	return Readiness{TargetVersion: targetVersion, Ready: true}, nil
}

func (s *Service) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	if s.op == nil {
		return ErrNoOperation
	}
	if s.op.Stage == "running" {
		return ErrOperationRunning
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if err := s.store.Clear(); err != nil {
		return err
	}
	s.op = nil
	return nil
}

func (s *Service) Status() (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return Operation{}, s.loadErr
	}
	if s.op == nil {
		if s.last == nil {
			return Operation{}, ErrNoOperation
		}
		return *s.last, nil
	}
	return *s.op, nil
}

func (s *Service) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
		if s.timer != nil {
			s.timer.Stop()
			s.timer = nil
		}
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) recoverLocked() {
	if s.op.Stage == "scheduled" {
		s.armLocked()
		return
	}
	if s.op.Stage == "running" {
		s.op.Stage = "failed"
		s.op.Result = "interrupted"
		s.op.CompletedAt = s.now().UTC()
		s.op.Error = "update interrupted by service restart"
		if err := s.store.Save(*s.op); err != nil {
			s.loadErr = fmt.Errorf("record interrupted update: %w", err)
		}
		s.last = s.op
		s.op = nil
	}
}

func (s *Service) armLocked() {
	if s.op == nil || s.op.Stage != "scheduled" || s.closed {
		return
	}
	delay := s.op.ScheduledAt.Sub(s.now())
	if delay < 0 {
		delay = 0
	}
	s.timer = time.AfterFunc(delay, s.start)
}

func (s *Service) start() {
	s.mu.Lock()
	if s.closed || s.op == nil || s.op.Stage != "scheduled" {
		s.mu.Unlock()
		return
	}
	s.op.Stage = "running"
	s.op.StartedAt = s.now().UTC()
	if err := s.store.Save(*s.op); err != nil {
		s.op.Stage = "failed"
		s.op.Error = err.Error()
		s.op.CompletedAt = s.now().UTC()
		_ = s.store.Save(*s.op)
		s.last = s.op
		s.op = nil
		s.mu.Unlock()
		return
	}
	op := *s.op
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	result, err := s.executor.Run(s.ctx, op)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.op == nil || s.op.OperationID != op.OperationID {
		return
	}
	s.op.Stage, s.op.Result, s.op.Error = executionOutcome(result, err)
	s.op.CompletedAt = s.now().UTC()
	_ = s.store.Save(*s.op)
	s.last = s.op
	s.op = nil
}

func executionOutcome(result ExecutionResult, err error) (stage, message, failure string) {
	stage, message, failure = result.Stage, result.Result, result.Error
	if err != nil {
		return "failed", message, err.Error()
	}
	if result.RollbackFailed {
		return "rollback_failed", message, failure
	}
	if result.RolledBack {
		return "rolled_back", message, failure
	}
	if result.Promoted {
		return "promoted", message, failure
	}
	switch stage {
	case "promoted", "rolled_back", "failed", "rollback_failed":
		return stage, message, failure
	}
	return "promoted", message, failure
}

func sameOperation(op Operation, actorID int64, mode, target string, at time.Time) bool {
	if mode == "now" {
		return op.ActorID == actorID && op.Mode == mode && op.TargetVersion == target
	}
	return op.ActorID == actorID && op.Mode == mode && op.TargetVersion == target && op.ScheduledAt.Equal(at.UTC())
}

func newOperationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("op-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
