package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	accountModelDetectionScheduleInterval   = 30 * time.Second
	accountModelDetectionQueueInterval      = time.Second
	accountMonitorTerminalWatchdogInterval  = time.Minute
	accountMonitorV4SnapshotRefreshInterval = 5 * time.Minute
)

const (
	accountMonitorV4SnapshotLeaderLockTTL = 10 * time.Minute
	accountMonitorV4SnapshotLeaderLockKey = "account-monitor-v4-snapshot"
)

type AccountMonitorRunner struct {
	svc                 *AccountMonitorService
	detector            *AccountModelDetectionService
	balanceNotification upstreamBalanceNotificationTrigger
	ctx                 context.Context
	cancel              context.CancelFunc

	mu                sync.Mutex
	interval          time.Duration
	trigger           chan struct{}
	reload            chan struct{}
	started           bool
	stopped           bool
	wg                sync.WaitGroup
	runMu             sync.Mutex
	snapshotMu        sync.Mutex
	snapshotRefresher MonitorV4SnapshotRefresher
	lockCache         LeaderLockCache
	db                *sql.DB
	instanceID        string
}

type upstreamBalanceNotificationTrigger interface {
	TriggerEvaluate()
}

func (r *AccountMonitorRunner) SetModelDetectionService(detector *AccountModelDetectionService) {
	if r != nil {
		r.detector = detector
	}
}

func (r *AccountMonitorRunner) SetUpstreamBalanceNotificationTrigger(trigger upstreamBalanceNotificationTrigger) {
	if r != nil {
		r.balanceNotification = trigger
	}
}

func (r *AccountMonitorRunner) SetMonitorV4SnapshotRefresher(refresher MonitorV4SnapshotRefresher) {
	if r != nil {
		r.snapshotRefresher = refresher
	}
}

func (r *AccountMonitorRunner) SetMonitorV4SnapshotCoordination(cache LeaderLockCache, db *sql.DB) {
	if r != nil {
		r.lockCache, r.db = cache, db
	}
}

func NewAccountMonitorRunner(svc *AccountMonitorService) *AccountMonitorRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountMonitorRunner{
		svc:        svc,
		ctx:        ctx,
		cancel:     cancel,
		trigger:    make(chan struct{}, 1),
		reload:     make(chan struct{}, 1),
		instanceID: uuid.NewString(),
	}
}

func (r *AccountMonitorRunner) Start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	if r.svc != nil {
		settings, err := r.svc.loadSettings(r.ctx)
		if err != nil {
			slog.Error("account_monitor: load settings failed", "error", err)
			settings.IntervalSeconds = AccountMonitorDefaultIntervalSeconds
		}
		r.mu.Lock()
		r.interval = time.Duration(settings.IntervalSeconds) * time.Second
		r.mu.Unlock()
	}

	if r.svc != nil {
		r.wg.Add(1)
		go r.loop()
	}
	if r.detector != nil {
		r.wg.Add(1)
		go r.detectionLoop()
	}
	if r.snapshotRefresher != nil {
		r.wg.Add(1)
		go r.snapshotLoop()
	}
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
	terminalTicker := time.NewTicker(accountMonitorTerminalWatchdogInterval)
	defer terminalTicker.Stop()
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
		case <-terminalTicker.C:
			timer.Stop()
			r.settleOnce()
		case <-timer.C:
			r.runOnce()
		}
	}
}

func (r *AccountMonitorRunner) detectionLoop() {
	defer r.wg.Done()
	if r.detector == nil {
		return
	}
	r.runDueDetectionSlots()
	r.runQueuedDetections()
	scheduleTicker := time.NewTicker(accountModelDetectionScheduleInterval)
	defer scheduleTicker.Stop()
	queueTicker := time.NewTicker(accountModelDetectionQueueInterval)
	defer queueTicker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-scheduleTicker.C:
			r.runDueDetectionSlots()
			r.runQueuedDetections()
		case <-queueTicker.C:
			r.runQueuedDetections()
		}
	}
}

func (r *AccountMonitorRunner) snapshotLoop() {
	defer r.wg.Done()
	r.refreshSnapshotOnce()
	ticker := time.NewTicker(accountMonitorV4SnapshotRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.refreshSnapshotOnce()
		}
	}
}

func (r *AccountMonitorRunner) refreshSnapshotOnce() {
	if r == nil || r.snapshotRefresher == nil || !r.snapshotMu.TryLock() {
		return
	}
	defer r.snapshotMu.Unlock()
	ctx, cancel := context.WithTimeout(r.ctx, 4*time.Minute)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(ctx, r.lockCache, r.db, accountMonitorV4SnapshotLeaderLockKey, r.instanceID, accountMonitorV4SnapshotLeaderLockTTL)
	if !acquired {
		return
	}
	defer release()
	if err := r.snapshotRefresher.RefreshMonitorV4Snapshots(ctx, time.Now().UTC()); err != nil {
		slog.Warn("account_monitor_v4_snapshot: refresh failed", "phase", "refresh", "error_category", "refresh_failed")
	}
}

func (r *AccountMonitorRunner) runDueDetectionSlots() {
	if _, err := r.detector.RunDueSlots(r.ctx); err != nil {
		slog.Warn("account_model_detection: schedule failed", "error", err)
	}
}

func (r *AccountMonitorRunner) runQueuedDetections() {
	if _, err := r.detector.RunQueued(r.ctx); err != nil {
		slog.Warn("account_model_detection: queue poll failed", "error", err)
	}
}

func (r *AccountMonitorRunner) runOnce() {
	if !r.runMu.TryLock() {
		return
	}
	defer r.runMu.Unlock()
	if r.balanceNotification != nil {
		defer r.balanceNotification.TriggerEvaluate()
	}
	if _, err := r.svc.RunAll(r.ctx, 0); err != nil {
		slog.Warn("account_monitor: run failed", "error", err)
	}
}

func (r *AccountMonitorRunner) settleOnce() {
	if r == nil || r.svc == nil || !r.runMu.TryLock() {
		return
	}
	defer r.runMu.Unlock()
	if err := r.svc.SettleDueProbeBuckets(r.ctx); err != nil {
		slog.Warn("account_monitor: terminal watchdog failed", "error", err)
	}
}
