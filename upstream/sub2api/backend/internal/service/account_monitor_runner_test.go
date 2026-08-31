package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type monitorV4SnapshotRefresherStub struct {
	calls   atomic.Int32
	started chan struct{}
	block   <-chan struct{}
}

func (s *monitorV4SnapshotRefresherStub) RefreshMonitorV4Snapshots(context.Context, time.Time) error {
	s.calls.Add(1)
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		<-s.block
	}
	return nil
}

type monitorV4LeaderLockStub struct {
	held atomic.Bool
	mu   sync.Mutex
}

func (s *monitorV4LeaderLockStub) TryAcquireLeaderLock(context.Context, string, string, time.Duration) (bool, error) {
	return !s.held.Load(), nil
}
func (s *monitorV4LeaderLockStub) ReleaseLeaderLock(context.Context, string, string) error {
	return nil
}

func TestMonitorV4SnapshotRunnerLifecycle(t *testing.T) {
	previous := accountMonitorV4SnapshotRefreshInterval
	accountMonitorV4SnapshotRefreshInterval = 5 * time.Millisecond
	defer func() { accountMonitorV4SnapshotRefreshInterval = previous }()
	refresher := &monitorV4SnapshotRefresherStub{started: make(chan struct{}, 8)}
	runner := NewAccountMonitorRunner(nil)
	runner.SetMonitorV4SnapshotRefresher(refresher)
	runner.Start()
	defer runner.Stop()
	select {
	case <-refresher.started:
	case <-time.After(time.Second):
		t.Fatal("immediate snapshot refresh did not run")
	}
	select {
	case <-refresher.started:
	case <-time.After(time.Second):
		t.Fatal("ticker snapshot refresh did not run")
	}
	if got := refresher.calls.Load(); got < 2 {
		t.Fatalf("snapshot refresh calls = %d, want at least 2", got)
	}
}

func TestMonitorV4SnapshotRunnerDoesNotOverlapAndStops(t *testing.T) {
	previous := accountMonitorV4SnapshotRefreshInterval
	accountMonitorV4SnapshotRefreshInterval = time.Millisecond
	defer func() { accountMonitorV4SnapshotRefreshInterval = previous }()
	block := make(chan struct{})
	refresher := &monitorV4SnapshotRefresherStub{started: make(chan struct{}, 4), block: block}
	runner := NewAccountMonitorRunner(nil)
	runner.SetMonitorV4SnapshotRefresher(refresher)
	runner.Start()
	select {
	case <-refresher.started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	time.Sleep(10 * time.Millisecond)
	if got := refresher.calls.Load(); got != 1 {
		t.Fatalf("overlapping refresh calls = %d, want 1", got)
	}
	close(block)
	runner.Stop()
	calls := refresher.calls.Load()
	time.Sleep(10 * time.Millisecond)
	if refresher.calls.Load() != calls {
		t.Fatal("snapshot refresh occurred after Stop")
	}
}

func TestMonitorV4SnapshotRunnerSkipsPeerLeader(t *testing.T) {
	refresher := &monitorV4SnapshotRefresherStub{started: make(chan struct{}, 1)}
	lock := &monitorV4LeaderLockStub{}
	lock.held.Store(true)
	runner := NewAccountMonitorRunner(nil)
	runner.SetMonitorV4SnapshotRefresher(refresher)
	runner.SetMonitorV4SnapshotCoordination(lock, nil)
	runner.Start()
	defer runner.Stop()
	time.Sleep(20 * time.Millisecond)
	if got := refresher.calls.Load(); got != 0 {
		t.Fatalf("peer-held lock refresh calls = %d, want 0", got)
	}
}

func TestMonitorV4SnapshotRunnerNilRefresherHasNoLoop(t *testing.T) {
	runner := NewAccountMonitorRunner(nil)
	runner.Start()
	runner.Stop()
	if !runner.started {
		t.Fatal("runner did not start")
	}
}

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
