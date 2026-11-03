package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"time"
)

// The production API and singleton worker run in different processes. Upstream
// CRUD callbacks still serve full nodes; the worker also discovers durable
// changes here without introducing another scheduler or changing probe policy.
func (r *ChannelMonitorRunner) startReconciliation() {
	r.mu.Lock()
	if !r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.parentCtx.Done():
				return
			case <-ticker.C:
				r.reconcileEnabledMonitors()
			}
		}
	}()
}

func monitorScheduleHash(m *ChannelMonitor) ([32]byte, error) {
	copy := *m
	// MarkChecked updates UpdatedAt too. Neither timestamp may reset a timer,
	// otherwise a completed check would immediately schedule another check.
	copy.LastCheckedAt = nil
	copy.UpdatedAt = time.Time{}
	copy.CreatedAt = time.Time{}
	copy.HistoryStartedAt = time.Time{}
	encoded, err := json.Marshal(copy)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func (r *ChannelMonitorRunner) reconcileEnabledMonitors() {
	ctx, cancel := context.WithTimeout(r.parentCtx, monitorStartupLoadTimeout)
	defer cancel()
	enabled, err := r.svc.ListEnabledMonitors(ctx)
	if err != nil {
		// Keep existing timers on a transient DB failure.
		slog.Warn("channel_monitor: worker configuration refresh failed", "error", err)
		return
	}
	present := make(map[int64]bool, len(enabled))
	for _, monitor := range enabled {
		if monitor == nil {
			continue
		}
		present[monitor.ID] = true
		if !monitor.Enabled || monitor.APIKeyDecryptFailed {
			r.Unschedule(monitor.ID)
			continue
		}
		hash, err := monitorScheduleHash(monitor)
		if err != nil {
			r.Unschedule(monitor.ID)
			continue
		}
		r.mu.Lock()
		task, exists := r.tasks[monitor.ID]
		changed := !exists || task.configHash != hash
		r.mu.Unlock()
		if changed {
			r.Schedule(monitor)
		}
	}
	r.mu.Lock()
	var removed []int64
	for id := range r.tasks {
		if !present[id] {
			removed = append(removed, id)
		}
	}
	r.mu.Unlock()
	for _, id := range removed {
		r.Unschedule(id)
	}
}
