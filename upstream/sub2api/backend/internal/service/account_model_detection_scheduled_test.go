package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

type modelDetectionUsageStub struct {
	accountUsed bool
	err         error
}

type sequencedModelDetectionUsageStub struct {
	mu   sync.Mutex
	used []bool
	call int
}

func (s *sequencedModelDetectionUsageStub) HasAccountUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.used) == 0 {
		return false, nil
	}
	index := s.call
	if index >= len(s.used) {
		index = len(s.used) - 1
	}
	s.call++
	return s.used[index], nil
}

func (s *sequencedModelDetectionUsageStub) HasGroupUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	return false, nil
}

func (s *modelDetectionUsageStub) HasAccountUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	return s.accountUsed, s.err
}

func (s *modelDetectionUsageStub) HasGroupUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	return false, nil
}

func TestRunDueSlotsQueuesLowOnlyForActiveSchedulableEmptyBucketAccounts(t *testing.T) {
	accounts := []Account{
		{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}},
		{ID: 8, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}},
		{ID: 9, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}},
	}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: accounts}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	svc.now = func() time.Time { return time.Date(2026, 8, 27, 6, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	queued, err := svc.RunDueSlots(context.Background())
	if err != nil || queued != 1 {
		t.Fatalf("queued=%d err=%v, want one eligible account", queued, err)
	}
	if len(repo.runs) != 1 || repo.runs[0].Profile != AccountModelDetectionProfileLow || repo.runs[0].PlannedRequests != 19 {
		t.Fatalf("runs=%#v, want one low run", repo.runs)
	}
}

func TestRunDueSlotsDeduplicatesTheSameSixHourSlot(t *testing.T) {
	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: []Account{account}}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	svc.now = func() time.Time { return time.Date(2026, 8, 27, 6, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	first, err := svc.RunDueSlots(context.Background())
	second, err2 := svc.RunDueSlots(context.Background())
	if err != nil || err2 != nil || first != 1 || second != 0 || len(repo.runs) != 1 {
		t.Fatalf("first=%d second=%d runs=%d err=%v/%v, want one run for the slot", first, second, len(repo.runs), err, err2)
	}
}

func TestRunDueSlotsSkipsAccountWithCurrentBucketUsage(t *testing.T) {
	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: []Account{account}}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{accountUsed: true})
	svc.now = func() time.Time { return time.Date(2026, 8, 27, 6, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	queued, err := svc.RunDueSlots(context.Background())
	if err != nil || queued != 0 || len(repo.runs) != 0 {
		t.Fatalf("queued=%d runs=%#v err=%v, want skipped", queued, repo.runs, err)
	}
}

func TestRunDueSlotsSkipsWhenUsageReaderFails(t *testing.T) {
	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: []Account{account}}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{err: context.DeadlineExceeded})
	svc.now = func() time.Time { return time.Date(2026, 8, 27, 6, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	queued, err := svc.RunDueSlots(context.Background())
	if err != nil || queued != 0 || len(repo.runs) != 0 {
		t.Fatalf("queued=%d runs=%#v err=%v, want skipped", queued, repo.runs, err)
	}
}

func TestScheduledExecuteRechecksUsageBeforeCallingDetector(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{
		"api_key":       "secret",
		"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"},
	}}
	run := AccountModelDetectionRun{ID: "run-1", AccountID: account.ID, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Profile: AccountModelDetectionProfileLow, Mode: AccountModelDetectionModeMonitor, TriggerKind: "scheduled", Status: AccountModelDetectionStatusQueued}
	repo := &executionDetectionRepoStub{runs: map[string]AccountModelDetectionRun{run.ID: run}}
	sidecar := &countingDetectionSidecar{catalog: []string{"gpt-5.6-sol"}}
	usage := &sequencedModelDetectionUsageStub{used: []bool{false, true}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, sidecar)
	svc.SetActiveProbeUsageReader(usage)
	if used, err := usage.HasAccountUsageInWindow(context.Background(), account.ID, time.Time{}, time.Time{}); err != nil || used {
		t.Fatalf("initial bucket check used=%v err=%v, want empty bucket", used, err)
	}
	svc.execute(context.Background(), run.ID)
	if calls := sidecar.detectCallCount(); calls != 0 {
		t.Fatalf("detector calls=%d, want no upstream request when bucket became busy", calls)
	}
	completion := repo.completion(run.ID)
	if completion.response.Status != AccountModelDetectionStatusInsufficient {
		t.Fatalf("completion status=%q, want insufficient", completion.response.Status)
	}
}

func TestScheduledDetectionDoesNotEscalate(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "secret", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}}
	run := AccountModelDetectionRun{ID: "run-1", AccountID: account.ID, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Profile: AccountModelDetectionProfileMedium, Status: AccountModelDetectionStatusQueued}
	repo := &executionDetectionRepoStub{runs: map[string]AccountModelDetectionRun{run.ID: run}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &countingDetectionSidecar{catalog: []string{"gpt-5.6-sol"}})
	svc.execute(context.Background(), run.ID)
	if got := repo.enqueueCount(); got != 0 {
		t.Fatalf("automatic high escalation enqueued %d runs, want 0", got)
	}
}

func TestDueDetectionSlotUsesSixHourSchedule(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, tc := range []struct {
		at, want string
	}{
		{"2026-08-27T00:29:59+08:00", "2026-08-27T00:00"},
		{"2026-08-27T06:05:00+08:00", "2026-08-27T06:00"},
		{"2026-08-27T10:00:00+08:00", ""},
		{"2026-08-27T18:30:01+08:00", ""},
	} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		if got := dueDetectionSlot(at.In(loc)); got != tc.want {
			t.Fatalf("dueDetectionSlot(%s)=%q want %q", tc.at, got, tc.want)
		}
	}
}

func TestIsSuspiciousDetectionRequiresExplicitConflictEvidence(t *testing.T) {
	finished := time.Now().UTC()
	if !isSuspiciousDetection(AccountModelDetectionRun{JuiceStatus: "mismatch", FinishedAt: &finished}) {
		t.Fatal("juice mismatch must be suspicious")
	}
	if !isSuspiciousDetection(AccountModelDetectionRun{FingerprintStatus: "strong_conflict", FinishedAt: &finished}) {
		t.Fatal("fingerprint conflict must be suspicious")
	}
	if !isSuspiciousDetection(AccountModelDetectionRun{ClaimedModel: "gpt-5.6-sol", FingerprintCandidate: "gpt-5.6-terra", FinishedAt: &finished}) {
		t.Fatal("candidate mismatch must be suspicious")
	}
	if isSuspiciousDetection(AccountModelDetectionRun{Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}) {
		t.Fatal("abnormal without explicit conflict evidence must not be suspicious")
	}
}

func TestNextScheduledDetectionProfileUsesNextSixHourWindowPolicy(t *testing.T) {
	finished := time.Now().UTC()
	for _, tc := range []struct {
		name string
		run  AccountModelDetectionRun
		want string
	}{
		{name: "first", want: AccountModelDetectionProfileLow},
		{name: "low suspicious", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileLow, JuiceStatus: "mismatch", FinishedAt: &finished}, want: AccountModelDetectionProfileMedium},
		{name: "medium suspicious", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileMedium, FingerprintStatus: "strong_conflict", FinishedAt: &finished}, want: AccountModelDetectionProfileHigh},
		{name: "high final resets", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileHigh, Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}, want: AccountModelDetectionProfileLow},
		{name: "normal resets", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileMedium, Status: AccountModelDetectionStatusNormal, FinishedAt: &finished}, want: AccountModelDetectionProfileLow},
		{name: "insufficient holds", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileMedium, Status: AccountModelDetectionStatusInsufficient, FinishedAt: &finished}, want: AccountModelDetectionProfileMedium},
		{name: "failed holds", run: AccountModelDetectionRun{Profile: AccountModelDetectionProfileMedium, Status: AccountModelDetectionStatusFailed, FinishedAt: &finished}, want: AccountModelDetectionProfileMedium},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := nextScheduledDetectionProfile([]AccountModelDetectionRun{tc.run})
			if got != tc.want {
				t.Fatalf("profile=%q want %q", got, tc.want)
			}
		})
	}
}

func TestActiveProbeSwitchesDefaultOnAndFailClosedWhenDisabled(t *testing.T) {
	account := &Account{ID: 7}
	if !account.ActiveProbeEnabled() || !accountActiveProbeEnabledByGroups(account) {
		t.Fatal("missing account/group settings must preserve enabled default")
	}
	account.Extra = map[string]any{ActiveProbeEnabledExtraKey: false}
	if account.ActiveProbeEnabled() {
		t.Fatal("account switch=false must disable automatic probes")
	}
	account.Extra = map[string]any{}
	account.Groups = []*Group{{ID: 9, ActiveProbeEnabled: false}}
	if accountActiveProbeEnabledByGroups(account) {
		t.Fatal("any disabled group must disable automatic probes")
	}
}
