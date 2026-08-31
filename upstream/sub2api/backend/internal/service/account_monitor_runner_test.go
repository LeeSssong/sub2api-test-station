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

	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{}
	detector := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: &account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	detector.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	detector.now = func() time.Time { return time.Date(2026, 8, 17, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
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

func TestAccountMonitorRunnerSettleOnceRunsTerminalWatchdog(t *testing.T) {
	repo := &accountMonitorRepoStub{
		groups: []AccountMonitorGroup{{ID: 7, Status: StatusActive}},
	}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	runner := NewAccountMonitorRunner(svc)
	runner.settleOnce()
	if len(repo.probeBucketTerminals) < 1 || len(repo.probeBucketTerminals) > 2 {
		t.Fatalf("terminal writes = %d, want one previous-bucket write and optionally the current final-minute bucket", len(repo.probeBucketTerminals))
	}
	if repo.probeBucketTerminals[0].groupID != 7 {
		t.Fatalf("watchdog group id = %d, want 7", repo.probeBucketTerminals[0].groupID)
	}
}

func TestAccountMonitorRunnerTriggersBalanceEvaluationAfterNativeRun(t *testing.T) {
	repo := &accountMonitorRepoStub{settings: AccountMonitorSettings{IntervalSeconds: 60}}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	runner := NewAccountMonitorRunner(svc)
	trigger := &upstreamBalanceNotificationTriggerStub{}
	runner.SetUpstreamBalanceNotificationTrigger(trigger)

	runner.runOnce()

	if trigger.calls != 1 {
		t.Fatalf("balance evaluation triggers = %d, want 1", trigger.calls)
	}
}

type upstreamBalanceNotificationTriggerStub struct{ calls int }

func (s *upstreamBalanceNotificationTriggerStub) TriggerEvaluate() { s.calls++ }
