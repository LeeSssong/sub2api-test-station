package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type AccountMonitorRunner struct {
	svc    *AccountMonitorService
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	interval time.Duration
	trigger  chan struct{}
	reload   chan struct{}
	started  bool
	stopped  bool
	wg       sync.WaitGroup
	runMu    sync.Mutex
}

func NewAccountMonitorRunner(svc *AccountMonitorService) *AccountMonitorRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountMonitorRunner{
		svc:     svc,
		ctx:     ctx,
		cancel:  cancel,
		trigger: make(chan struct{}, 1),
		reload:  make(chan struct{}, 1),
	}
}

func (r *AccountMonitorRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	settings, err := r.svc.loadSettings(r.ctx)
	if err != nil {
		slog.Error("account_monitor: load settings failed", "error", err)
		settings.IntervalSeconds = AccountMonitorDefaultIntervalSeconds
	}
	r.mu.Lock()
	r.interval = time.Duration(settings.IntervalSeconds) * time.Second
	r.mu.Unlock()

	r.wg.Add(1)
	go r.loop()
	r.TriggerNow()
}

func (r *AccountMonitorRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	r.cancel()
	r.wg.Wait()
}

func (r *AccountMonitorRunner) TriggerNow() {
	if r == nil {
		return
	}
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

func (r *AccountMonitorRunner) ReloadSettings(settings AccountMonitorSettings) {
	if r == nil {
		return
	}
	interval := settings.IntervalSeconds
	if interval < AccountMonitorMinIntervalSeconds {
		interval = AccountMonitorMinIntervalSeconds
	}
	if interval > AccountMonitorMaxIntervalSeconds {
		interval = AccountMonitorMaxIntervalSeconds
	}
	r.mu.Lock()
	r.interval = time.Duration(interval) * time.Second
	r.mu.Unlock()
	select {
	case r.reload <- struct{}{}:
	default:
	}
}

func (r *AccountMonitorRunner) loop() {
	defer r.wg.Done()
	for {
		r.mu.Lock()
		interval := r.interval
		r.mu.Unlock()
		if interval <= 0 {
			interval = AccountMonitorDefaultIntervalSeconds * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return
		case <-r.trigger:
			timer.Stop()
			r.runOnce()
		case <-r.reload:
			timer.Stop()
		case <-timer.C:
			r.runOnce()
		}
	}
}

func (r *AccountMonitorRunner) runOnce() {
	if !r.runMu.TryLock() {
		return
	}
	defer r.runMu.Unlock()
	if _, err := r.svc.RunAll(r.ctx, 0); err != nil {
		slog.Warn("account_monitor: run failed", "error", err)
	}
}
