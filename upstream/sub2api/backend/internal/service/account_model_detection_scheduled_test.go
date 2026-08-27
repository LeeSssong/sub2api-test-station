package service

import (
	"context"
	"testing"
	"time"
)

type modelDetectionUsageStub struct {
	accountUsed bool
	err         error
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
