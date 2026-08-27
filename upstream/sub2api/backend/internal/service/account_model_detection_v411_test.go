package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDetectionProfileContractsUseBoundedRequestPlans(t *testing.T) {
	for _, tc := range []struct {
		profile string
		want    int
	}{
		{AccountModelDetectionProfileLow, 19},
		{AccountModelDetectionProfileMedium, 49},
		{AccountModelDetectionProfileHigh, 158},
	} {
		run := newAccountModelDetectionRun(7, "gpt-5.6-sol", tc.profile, AccountModelDetectionModeMonitor, AccountModelDetectionTriggerScheduled)
		if run.Profile != tc.profile || run.PlannedRequests != tc.want || run.Mode != AccountModelDetectionModeMonitor {
			t.Fatalf("run=%#v, want profile=%q planned=%d mode=monitor", run, tc.profile, tc.want)
		}
	}
}

func TestEnqueueImmediateCreatesManualLowRunWithReason(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Credentials: map[string]any{
		"api_key": "secret", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"},
	}, Extra: map[string]any{}}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})

	run, reused, err := svc.EnqueueImmediate(context.Background(), account.ID, 9)
	if err != nil || reused {
		t.Fatalf("run=%#v reused=%v err=%v", run, reused, err)
	}
	if run.Profile != AccountModelDetectionProfileLow || run.Mode != AccountModelDetectionModeManual || run.TriggerReason != AccountModelDetectionTriggerManual || run.PlannedRequests != 19 {
		t.Fatalf("run=%#v, want low/manual/manual/19", run)
	}
}

func TestCompleteRunPreservesRunMetadataWhenDetectionFailsBeforeSidecar(t *testing.T) {
	repo := &executionDetectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, nil, nil)
	run := AccountModelDetectionRun{ID: "run-1", Profile: AccountModelDetectionProfileMedium, PlannedRequests: 49}
	if err := svc.completeRun(context.Background(), run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "detector_unavailable", "检测器不可用"); err != nil {
		t.Fatal(err)
	}
	completion := repo.completion("run-1")
	if completion.response.Profile != AccountModelDetectionProfileMedium || completion.response.PlannedRequests != 49 || completion.response.EvidenceState != AccountModelDetectionEvidenceUnavailable {
		t.Fatalf("completion=%#v, want run metadata and unavailable evidence", completion)
	}
}

func TestDetectionHistoryPageCursorFiltersWithoutDuplicates(t *testing.T) {
	now := time.Date(2026, 8, 26, 2, 0, 0, 0, time.UTC)
	repo := &detectionRepoStub{recent: []AccountModelDetectionRun{
		{ID: "run-3", AccountID: 7, Status: AccountModelDetectionStatusAbnormal, Profile: AccountModelDetectionProfileHigh, Mode: AccountModelDetectionModeEscalation, CreatedAt: now},
		{ID: "run-2", AccountID: 7, Status: AccountModelDetectionStatusInsufficient, Profile: AccountModelDetectionProfileMedium, Mode: AccountModelDetectionModeMonitor, CreatedAt: now.Add(-time.Minute)},
		{ID: "run-1", AccountID: 7, Status: AccountModelDetectionStatusNormal, Profile: AccountModelDetectionProfileLow, Mode: AccountModelDetectionModeManual, CreatedAt: now.Add(-2 * time.Minute)},
	}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{}, &detectionSidecarStub{})
	page, err := svc.RecentPage(context.Background(), 7, 2, "", "", "", "", "", "")
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page=%#v err=%v, want two items and cursor", page, err)
	}
	if page.Items[0].ID != "run-3" || page.Items[1].ID != "run-2" {
		t.Fatalf("items=%#v", page.Items)
	}
	next, err := svc.RecentPage(context.Background(), 7, 2, page.NextCursor, "", "", "", "", "")
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "run-1" {
		t.Fatalf("next=%#v err=%v, want only run-1", next, err)
	}
}

func TestShouldEscalateDetectionUsesTieredReasons(t *testing.T) {
	finished := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		runs   []AccountModelDetectionRun
		want   bool
		reason string
	}{
		{name: "first run", want: true, reason: AccountModelDetectionTriggerFirstRun},
		{name: "first normal result", runs: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusNormal, FinishedAt: &finished}}, want: true, reason: AccountModelDetectionTriggerFirstRun},
		{name: "two abnormal", runs: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}, {Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}}, want: true, reason: AccountModelDetectionTriggerConsecutiveAbnormal},
		{name: "two insufficient", runs: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusInsufficient, FinishedAt: &finished}, {Status: AccountModelDetectionStatusInsufficient, FinishedAt: &finished}}, want: true, reason: AccountModelDetectionTriggerInsufficient},
		{name: "model conflict", runs: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusNormal, JuiceStatus: "mismatch", FinishedAt: &finished}}, want: true, reason: AccountModelDetectionTriggerModelConflict},
		{name: "single insufficient", runs: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusInsufficient, FinishedAt: &finished}}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := shouldEscalateDetection(tc.runs)
			if got != tc.want || reason != tc.reason {
				t.Fatalf("got=%v reason=%q, want=%v %q", got, reason, tc.want, tc.reason)
			}
		})
	}
}

func TestRunDueSlotsQueuesLowMonitorRuns(t *testing.T) {
	account := Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: []Account{account}}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	svc.now = func() time.Time { return time.Date(2026, 8, 17, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }
	if _, err := svc.RunDueSlots(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.runs) != 1 || repo.runs[0].Profile != AccountModelDetectionProfileLow || repo.runs[0].Mode != AccountModelDetectionModeMonitor || repo.runs[0].PlannedRequests != 19 {
		t.Fatalf("runs=%#v", repo.runs)
	}
}

func TestQueueEscalationDedupesActiveHighRun(t *testing.T) {
	finished := time.Now().UTC()
	repo := &detectionRepoStub{recent: []AccountModelDetectionRun{{Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}, {Status: AccountModelDetectionStatusAbnormal, FinishedAt: &finished}}}
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	if _, _, err := svc.EnqueueEscalationHigh(context.Background(), account.ID, AccountModelDetectionTriggerConsecutiveAbnormal); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.EnqueueEscalationHigh(context.Background(), account.ID, AccountModelDetectionTriggerConsecutiveAbnormal); err != nil {
		t.Fatal(err)
	}
	if len(repo.runs) != 1 || repo.runs[0].Profile != AccountModelDetectionProfileHigh || repo.runs[0].Mode != AccountModelDetectionModeEscalation {
		t.Fatalf("runs=%#v", repo.runs)
	}
}

func TestSidecarResponseCarriesV411EvidenceFields(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{"status": "normal", "profile": "medium", "planned_requests": 49, "valid_samples": 46, "evidence_state": "complete", "fingerprint_status": "strong_match"}), nil
	})})
	response, err := client.Detect(context.Background(), AccountModelDetectionRequest{})
	if err != nil || response.Profile != AccountModelDetectionProfileMedium || response.PlannedRequests != 49 || response.ValidSamples != 46 || response.EvidenceState != AccountModelDetectionEvidenceComplete || response.FingerprintStatus != "strong_match" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestSidecarRejectsUnboundedEvidenceCounts(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{"status": "normal", "profile": "high", "planned_requests": 999999, "valid_samples": 999999}), nil
	})})
	if _, err := client.Detect(context.Background(), AccountModelDetectionRequest{}); !errors.Is(err, ErrAccountModelDetectorUnavailable) {
		t.Fatalf("error=%v, want bounded payload rejection", err)
	}
}
