package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConsumerIsIdempotentAndDeadLettersFailures(t *testing.T) {
	n := 0
	c := NewConsumer(HandlerFunc(func(context.Context, Event) error {
		n++
		if n == 1 {
			return errors.New("temporary")
		}
		return nil
	}))
	e := Event{EventID: "e1", Type: AccountHealthChanged, OccurredAt: time.Now(), SourceVersion: "core", ContractVersion: ContractVersion, Payload: []byte(`{"account_id":1}`)}
	if c.Handle(context.Background(), e) == nil {
		t.Fatal("expected failure")
	}
	if len(c.DeadLetters()) != 1 {
		t.Fatal("missing dead letter")
	}
	if c.Handle(context.Background(), e) != nil {
		t.Fatal("retry should succeed")
	}
	if c.Handle(context.Background(), e) != nil {
		t.Fatal("duplicate should be ignored")
	}
	if n != 2 {
		t.Fatalf("handler calls=%d", n)
	}
}
