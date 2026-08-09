package events

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrUnsupportedContract = errors.New("unsupported integration contract")
	ErrStaleClaim          = errors.New("stale event claim")
)

type Handler interface {
	Handle(context.Context, Event) error
}

type HandlerFunc func(context.Context, Event) error

func (f HandlerFunc) Handle(ctx context.Context, event Event) error { return f(ctx, event) }

type DeadLetter struct {
	Event    Event     `json:"event"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
}

type Claim struct {
	Acquired   bool
	Token      string
	Generation int64
}

// Journal is the durable ownership boundary for consumer idempotency,
// checkpoints and dead letters. Implementations must claim event IDs
// atomically across processes.
type Journal interface {
	ClaimEvent(context.Context, Event) (Claim, error)
	ApplyEvent(context.Context, Event, Claim, time.Time, func(context.Context) error) (Watermark, error)
	FailEvent(context.Context, Event, Claim, time.Time, error) error
	LoadWatermark(context.Context, string) (Watermark, bool, error)
	ListDeadLetters(context.Context) ([]DeadLetter, error)
}

type Consumer struct {
	journal  Journal
	handlers []Handler
	now      func() time.Time
}

// NewConsumer remains useful for isolated callers. Production consumers
// should use NewPersistentConsumer with the PostgreSQL store.
func NewConsumer(handlers ...Handler) *Consumer {
	consumer, _ := NewPersistentConsumer(newMemoryJournal(), handlers...)
	return consumer
}

func NewPersistentConsumer(journal Journal, handlers ...Handler) (*Consumer, error) {
	if journal == nil {
		return nil, errors.New("event journal is required")
	}
	for _, handler := range handlers {
		if handler == nil {
			return nil, errors.New("event handler is required")
		}
	}
	return &Consumer{journal: journal, handlers: append([]Handler(nil), handlers...), now: time.Now}, nil
}

func (c *Consumer) Handle(ctx context.Context, event Event) error {
	if c == nil || c.journal == nil {
		return errors.New("event consumer is not initialized")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	claim, err := c.journal.ClaimEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("claim event %s: %w", event.EventID, err)
	}
	if !claim.Acquired {
		return nil
	}
	var handlerErr error
	_, err = c.journal.ApplyEvent(ctx, event, claim, c.processedAt(event), func(applyCtx context.Context) error {
		for _, handler := range c.handlers {
			if err := handler.Handle(applyCtx, event); err != nil {
				handlerErr = err
				return err
			}
		}
		return nil
	})
	if handlerErr != nil {
		failedAt := c.processedAt(event)
		if journalErr := c.journal.FailEvent(ctx, event, claim, failedAt, handlerErr); journalErr != nil {
			return errors.Join(handlerErr, fmt.Errorf("dead-letter event %s: %w", event.EventID, journalErr))
		}
		return handlerErr
	}
	if err != nil {
		return fmt.Errorf("complete event %s: %w", event.EventID, err)
	}
	return nil
}

func (c *Consumer) processedAt(event Event) time.Time {
	processedAt := c.now().UTC()
	if processedAt.Before(event.OccurredAt) {
		return event.OccurredAt.UTC()
	}
	return processedAt
}

func (c *Consumer) HandleBatch(ctx context.Context, input []Event) error {
	ordered := append([]Event(nil), input...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ComparePosition(ordered[i].OccurredAt, ordered[i].EventID, ordered[j].OccurredAt, ordered[j].EventID) < 0
	})
	for _, event := range ordered {
		if err := c.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (c *Consumer) LoadWatermark(ctx context.Context, source string) (Watermark, bool, error) {
	if c == nil || c.journal == nil {
		return Watermark{}, false, errors.New("event consumer is not initialized")
	}
	return c.journal.LoadWatermark(ctx, source)
}

func (c *Consumer) ListDeadLetters(ctx context.Context) ([]DeadLetter, error) {
	if c == nil || c.journal == nil {
		return nil, errors.New("event consumer is not initialized")
	}
	return c.journal.ListDeadLetters(ctx)
}

// Compatibility helpers for existing in-process callers.
func (c *Consumer) DeadLetters() []DeadLetter {
	dead, _ := c.ListDeadLetters(context.Background())
	return dead
}

func (c *Consumer) Watermark(source string) (Watermark, bool) {
	w, found, _ := c.LoadWatermark(context.Background(), source)
	return w, found
}

type memoryJournal struct {
	mu         sync.Mutex
	states     map[string]memoryEventState
	watermarks map[string]Watermark
	dead       map[string]DeadLetter
}

type memoryEventState struct {
	status     string
	source     string
	claim      Claim
	generation int64
}

func newMemoryJournal() *memoryJournal {
	return &memoryJournal{
		states:     map[string]memoryEventState{},
		watermarks: map[string]Watermark{},
		dead:       map[string]DeadLetter{},
	}
}

func (j *memoryJournal) ClaimEvent(_ context.Context, event Event) (Claim, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, exists := j.states[event.EventID]; exists {
		return Claim{}, nil
	}
	claim := Claim{Acquired: true, Token: "memory-claim-" + event.EventID, Generation: 1}
	j.states[event.EventID] = memoryEventState{status: "processing", source: event.SourceVersion, claim: claim, generation: 1}
	if watermark, found := j.watermarks[event.SourceVersion]; found {
		watermark.Completeness = CompletenessPartial
		j.watermarks[event.SourceVersion] = watermark
	}
	return claim, nil
}

func (j *memoryJournal) ApplyEvent(ctx context.Context, event Event, claim Claim, processedAt time.Time, apply func(context.Context) error) (Watermark, error) {
	j.mu.Lock()
	state, found := j.states[event.EventID]
	j.mu.Unlock()
	if !found || state.status != "processing" || state.claim.Token != claim.Token || state.generation != claim.Generation {
		return Watermark{}, ErrStaleClaim
	}
	if err := apply(ctx); err != nil {
		return Watermark{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	state.status = "processed"
	j.states[event.EventID] = state
	w := j.watermarks[event.SourceVersion]
	if ComparePosition(event.OccurredAt, event.EventID, w.OccurredAt, w.LastEventID) > 0 {
		w.Source = event.SourceVersion
		w.LastEventID = event.EventID
		w.OccurredAt = event.OccurredAt.UTC()
	}
	w.ProcessedAt = processedAt.UTC()
	w.Completeness = CompletenessComplete
	for _, candidate := range j.states {
		if candidate.source == event.SourceVersion && candidate.status != "processed" {
			w.Completeness = CompletenessPartial
			break
		}
	}
	j.watermarks[event.SourceVersion] = w
	return w, nil
}

func (j *memoryJournal) FailEvent(_ context.Context, event Event, claim Claim, failedAt time.Time, cause error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	state, found := j.states[event.EventID]
	if !found || state.status != "processing" || state.claim.Token != claim.Token || state.generation != claim.Generation {
		return ErrStaleClaim
	}
	state.status = "dead"
	j.states[event.EventID] = state
	j.dead[event.EventID] = DeadLetter{Event: event, Error: cause.Error(), FailedAt: failedAt.UTC()}
	if watermark, found := j.watermarks[event.SourceVersion]; found {
		watermark.Completeness = CompletenessPartial
		j.watermarks[event.SourceVersion] = watermark
	}
	return nil
}

func (j *memoryJournal) LoadWatermark(_ context.Context, source string) (Watermark, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	w, found := j.watermarks[source]
	return w, found, nil
}

func (j *memoryJournal) ListDeadLetters(context.Context) ([]DeadLetter, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := make([]DeadLetter, 0, len(j.dead))
	for _, dead := range j.dead {
		result = append(result, dead)
	}
	sort.Slice(result, func(i, k int) bool { return result[i].Event.EventID < result[k].Event.EventID })
	return result, nil
}
