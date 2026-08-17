package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAccountMonitorRunnerReloadSettingsResetsCadenceWithoutTriggeringProbe(t *testing.T) {
	runner := NewAccountMonitorRunner(nil)
	runner.ReloadSettings(AccountMonitorSettings{IntervalSeconds: 60})

	runner.mu.Lock()
	interval := runner.interval
	runner.mu.Unlock()
	if interval != 60*time.Second {
		t.Fatalf("interval = %s", interval)
	}

	select {
	case <-runner.trigger:
		t.Fatal("settings update must not trigger a real probe")
	default:
	}
	select {
	case <-runner.reload:
	default:
		t.Fatal("settings update did not notify the cadence loop")
	}
}

func TestAccountMonitorRunnerDetectionLoopRunsImmediatelyAndStopsWithRunner(t *testing.T) {
	previousInterval := accountModelDetectionScheduleInterval
	accountModelDetectionScheduleInterval = 5 * time.Millisecond
	defer func() { accountModelDetectionScheduleInterval = previousInterval }()

	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{}
	detector := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: &account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	detector.now = func() time.Time { return time.Date(2026, 8, 17, 10, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	runner := NewAccountMonitorRunner(nil)
	runner.detector = detector
	runner.wg.Add(1)
	go runner.detectionLoop()

	deadline := time.After(250 * time.Millisecond)
	for {
		repo.mu.Lock()
		queued := len(repo.runs)
		repo.mu.Unlock()
		if queued > 0 {
			break
		}
		select {
		case <-deadline:
			runner.cancel()
			runner.wg.Wait()
			t.Fatal("detection loop did not run immediately")
		case <-time.After(time.Millisecond):
		}
	}
	runner.cancel()
	runner.wg.Wait()
	if err := runner.ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("runner context error = %v", err)
	}
}
