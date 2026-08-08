package events

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var ErrUnsupportedContract = errors.New("unsupported integration contract")

type Handler interface {
	Handle(context.Context, Event) error
}
type HandlerFunc func(context.Context, Event) error

func (f HandlerFunc) Handle(ctx context.Context, e Event) error { return f(ctx, e) }

type DeadLetter struct {
	Event Event
	Error string
}

type Consumer struct {
	mu         sync.Mutex
	seen       map[string]struct{}
	dead       []DeadLetter
	handlers   []Handler
	watermarks map[string]Watermark
}

func NewConsumer(handlers ...Handler) *Consumer {
	return &Consumer{seen: map[string]struct{}{}, handlers: handlers, watermarks: map[string]Watermark{}}
}

func (c *Consumer) Handle(ctx context.Context, event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	if _, ok := c.seen[event.EventID]; ok {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	for _, handler := range c.handlers {
		if err := handler.Handle(ctx, event); err != nil {
			c.mu.Lock()
			c.dead = append(c.dead, DeadLetter{Event: event, Error: err.Error()})
			c.mu.Unlock()
			return err
		}
	}
	c.mu.Lock()
	c.seen[event.EventID] = struct{}{}
	w := c.watermarks[event.SourceVersion]
	if w.OccurredAt.Before(event.OccurredAt) || (w.OccurredAt.Equal(event.OccurredAt) && event.EventID > w.LastEventID) {
		w.Source, w.LastEventID, w.OccurredAt = event.SourceVersion, event.EventID, event.OccurredAt
	}
	w.ProcessedAt = event.OccurredAt
	w.Completeness = "complete"
	c.watermarks[event.SourceVersion] = w
	c.mu.Unlock()
	return nil
}

func (c *Consumer) HandleBatch(ctx context.Context, input []Event) error {
	events := append([]Event(nil), input...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})
	for _, event := range events {
		if err := c.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (c *Consumer) DeadLetters() []DeadLetter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]DeadLetter(nil), c.dead...)
}
func (c *Consumer) Watermark(source string) (Watermark, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.watermarks[source]
	return w, ok
}
