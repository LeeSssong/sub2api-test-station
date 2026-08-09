package events

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	eventID1 = "550e8400-e29b-41d4-a716-446655440001"
	eventID2 = "550e8400-e29b-41d4-a716-446655440002"
	eventID3 = "550e8400-e29b-41d4-a716-446655440003"
)

type durableJournal struct {
	mu         sync.Mutex
	states     map[string]string
	watermarks map[string]Watermark
	dead       map[string]DeadLetter
}

func newDurableJournal() *durableJournal {
	return &durableJournal{
		states:     map[string]string{},
		watermarks: map[string]Watermark{},
		dead:       map[string]DeadLetter{},
	}
}

func (j *durableJournal) ClaimEvent(_ context.Context, event Event) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, exists := j.states[event.EventID]; exists {
		return false, nil
	}
	j.states[event.EventID] = "processing"
	return true, nil
}

func (j *durableJournal) CompleteEvent(_ context.Context, event Event, processedAt time.Time) (Watermark, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.states[event.EventID] = "processed"
	w := j.watermarks[event.SourceVersion]
	if ComparePosition(event.OccurredAt, event.EventID, w.OccurredAt, w.LastEventID) > 0 {
		w.Source = event.SourceVersion
		w.LastEventID = event.EventID
		w.OccurredAt = event.OccurredAt
	}
	w.ProcessedAt = processedAt
	w.Completeness = CompletenessComplete
	j.watermarks[event.SourceVersion] = w
	return w, nil
}

func (j *durableJournal) FailEvent(_ context.Context, event Event, processedAt time.Time, cause error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.states[event.EventID] = "dead"
	j.dead[event.EventID] = DeadLetter{Event: event, Error: cause.Error(), FailedAt: processedAt}
	return nil
}

func (j *durableJournal) LoadWatermark(_ context.Context, source string) (Watermark, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	w, ok := j.watermarks[source]
	return w, ok, nil
}

func (j *durableJournal) ListDeadLetters(context.Context) ([]DeadLetter, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := make([]DeadLetter, 0, len(j.dead))
	for _, dead := range j.dead {
		result = append(result, dead)
	}
	sort.Slice(result, func(i, k int) bool { return result[i].Event.EventID < result[k].Event.EventID })
	return result, nil
}

func TestConsumerPersistsIdempotencyAndResumesAfterRestart(t *testing.T) {
	journal := newDurableJournal()
	var handled []string
	handler := HandlerFunc(func(_ context.Context, event Event) error {
		handled = append(handled, event.EventID)
		return nil
	})
	first, err := NewPersistentConsumer(journal, handler)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPersistentConsumer(journal, handler)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	one := healthEvent(eventID1, at)
	two := healthEvent(eventID2, at.Add(time.Second))
	if err := first.Handle(context.Background(), one); err != nil {
		t.Fatal(err)
	}
	if err := second.Handle(context.Background(), one); err != nil {
		t.Fatalf("duplicate after restart: %v", err)
	}
	if err := second.Handle(context.Background(), two); err != nil {
		t.Fatalf("resume from persisted checkpoint: %v", err)
	}
	if want := []string{eventID1, eventID2}; !reflect.DeepEqual(handled, want) {
		t.Fatalf("handled = %v, want %v", handled, want)
	}
	w, found, err := second.LoadWatermark(context.Background(), "sub2api-v1")
	if err != nil || !found {
		t.Fatalf("watermark found=%v err=%v", found, err)
	}
	if w.LastEventID != eventID2 || !w.OccurredAt.Equal(two.OccurredAt) || w.Completeness != CompletenessComplete {
		t.Fatalf("watermark = %+v", w)
	}
	if w.ProcessedAt.Before(w.OccurredAt) {
		t.Fatalf("processed_at %s is before occurred_at %s", w.ProcessedAt, w.OccurredAt)
	}
}

func TestConsumerOrdersOutOfOrderEventsByOccurredAtThenEventID(t *testing.T) {
	journal := newDurableJournal()
	var handled []string
	consumer, err := NewPersistentConsumer(journal, HandlerFunc(func(_ context.Context, event Event) error {
		handled = append(handled, event.EventID)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	input := []Event{
		healthEvent(eventID3, at.Add(time.Second)),
		healthEvent(eventID2, at),
		healthEvent(eventID1, at),
	}
	if err := consumer.HandleBatch(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	want := []string{eventID1, eventID2, eventID3}
	if !reflect.DeepEqual(handled, want) {
		t.Fatalf("handled = %v, want %v", handled, want)
	}
}

func TestConsumerMovesHandlerFailureToPersistentDeadLetter(t *testing.T) {
	journal := newDurableJournal()
	consumer, err := NewPersistentConsumer(journal, HandlerFunc(func(context.Context, Event) error {
		return errors.New("invalid account projection")
	}))
	if err != nil {
		t.Fatal(err)
	}
	event := healthEvent(eventID1, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC))
	if err := consumer.Handle(context.Background(), event); err == nil {
		t.Fatal("expected handler failure")
	}

	restarted, err := NewPersistentConsumer(journal, HandlerFunc(func(context.Context, Event) error {
		t.Fatal("dead-lettered event was applied again")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Handle(context.Background(), event); err != nil {
		t.Fatalf("dead-letter duplicate must be acknowledged: %v", err)
	}
	dead, err := restarted.ListDeadLetters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dead) != 1 || dead[0].Event.EventID != eventID1 || dead[0].Error != "invalid account projection" || dead[0].FailedAt.IsZero() {
		t.Fatalf("dead letters = %+v", dead)
	}
}

func healthEvent(id string, at time.Time) Event {
	return Event{
		EventID:         id,
		Type:            AccountHealthChanged,
		OccurredAt:      at,
		SourceVersion:   "sub2api-v1",
		ContractVersion: ContractVersion,
		Payload:         []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:00Z"}`),
	}
}
