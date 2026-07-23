package ops

import (
	"context"
	"sync"
	"time"

	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/store"
)

type Scheduler struct {
	Store        *store.Store
	Credits      *credits.Service
	Reporter     *Reporter
	Registration interface {
		ReconcileRegistrations(context.Context) (int, error)
	}
	Alerter interface {
		Send(context.Context, Alert) error
	}
	Timezone   *time.Location
	TickC      <-chan time.Time
	mu         sync.Mutex
	running    bool
	lastDaily  string
	statusMu   sync.RWMutex
	lastTick   time.Time
	lastTickOK bool
}

type TickStatus struct {
	LastTick   time.Time
	LastTickOK bool
}

func (s *Scheduler) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	var ticker *time.Ticker
	var tickC <-chan time.Time
	if s.TickC != nil {
		tickC = s.TickC
	} else {
		ticker = time.NewTicker(time.Minute)
		defer ticker.Stop()
		tickC = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-tickC:
			err := s.Tick(ctx, now)
			s.statusMu.Lock()
			s.lastTick, s.lastTickOK = now, err == nil
			s.statusMu.Unlock()
			if err != nil && s.Alerter != nil {
				_ = s.Alerter.Send(ctx, Alert{Severity: "high", Code: "scheduler_tick_failed", Message: "后台任务执行失败", At: now})
			}
		}
	}
}

func (s *Scheduler) Status() TickStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return TickStatus{LastTick: s.lastTick, LastTickOK: s.lastTickOK}
}
func (s *Scheduler) Tick(ctx context.Context, now time.Time) error {
	if s.Credits.Mode == "write" && s.Registration != nil {
		if _, err := s.Registration.ReconcileRegistrations(ctx); err != nil {
			return err
		}
	}
	users, err := s.Store.ListInternalUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if _, err := s.Credits.ProcessUsage(ctx, u.UserID); err != nil {
			return err
		}
	}
	if now.Minute()%5 == 0 {
		for _, u := range users {
			if _, err := s.Credits.ReconcileUser(ctx, u.UserID); err != nil {
				return err
			}
		}
	}
	date := now.In(s.Timezone).Format("2006-01-02")
	if s.Reporter != nil && now.Minute() == 0 && s.lastDaily != date {
		if _, err := s.Reporter.Daily(ctx, now); err != nil {
			return err
		}
		s.lastDaily = date
	}
	return nil
}
