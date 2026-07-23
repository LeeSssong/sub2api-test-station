package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/routingcontrol"
)

func TestWorkerEnforcesCommandMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantStatus  string
		wantCode    string
		wantDryRun  bool
		wantRouting int
	}{
		{"disabled", ModeDisabled, StatusRejected, ErrorCommandDisabled, false, 0},
		{"dry run", ModeDryRun, StatusSucceeded, "", true, 1},
		{"enabled", ModeEnabled, StatusSucceeded, "", false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeWorkerRepository{next: switchRecord()}
			router := &fakeRouter{}
			sender := &fakeSender{repository: repository}
			worker := testWorker(repository, router, sender, tt.mode)
			worked, err := worker.RunOnce(context.Background())
			if err != nil || !worked {
				t.Fatalf("RunOnce = %v, %v", worked, err)
			}
			if repository.completion.Status != tt.wantStatus || repository.completion.ErrorCode != tt.wantCode || len(router.switchDryRuns) != tt.wantRouting {
				t.Fatalf("completion=%#v routing=%v", repository.completion, router.switchDryRuns)
			}
			if tt.wantRouting == 1 && router.switchDryRuns[0] != tt.wantDryRun {
				t.Fatalf("dry run = %v", router.switchDryRuns)
			}
			wantLocks := RouteLockIDs{GroupID: 3, PrimaryAccountID: 11, BackupAccountID: 12}
			if tt.wantRouting == 1 && repository.lockedRoute != wantLocks {
				t.Fatalf("locked route = %#v, want %#v", repository.lockedRoute, wantLocks)
			}
		})
	}
}

func TestDisabledWorkerDoesNotRequireRouter(t *testing.T) {
	repository := &fakeWorkerRepository{next: switchRecord()}
	sender := &fakeSender{repository: repository}
	worker := testWorker(repository, &fakeRouter{}, sender, ModeDisabled)
	worker.Router = nil

	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	if repository.completion.Status != StatusRejected || repository.completion.ErrorCode != ErrorCommandDisabled {
		t.Fatalf("completion = %#v", repository.completion)
	}
}

func TestWorkerSendsStructuredCommandCardWithoutTextFallback(t *testing.T) {
	repository := &fakeWorkerRepository{next: switchRecord()}
	sender := &fakeSender{repository: repository}
	worker := testWorker(repository, &fakeRouter{}, sender, ModeDisabled)
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce=%v err=%v", worked, err)
	}
	if sender.message.MsgType != "interactive" || !json.Valid(sender.message.Content) {
		t.Fatalf("message=%#v", sender.message)
	}
	if sender.textCalls != 0 {
		t.Fatalf("text fallback calls=%d", sender.textCalls)
	}
}

func TestWorkerStatusIsReadOnlyAndUnknownCommandIsRejected(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		record := switchRecord()
		record.Command = "查询当前分组状态"
		record.ActionKind = ActionStatus
		record.GroupName = ""
		record.TargetRole = ""
		repository := &fakeWorkerRepository{next: record}
		router := &fakeRouter{}
		worked, err := testWorker(repository, router, &fakeSender{repository: repository}, ModeEnabled).RunOnce(context.Background())
		if err != nil || !worked || router.readCalls != 1 || len(router.switchDryRuns) != 0 || repository.completion.Status != StatusSucceeded {
			t.Fatalf("worked=%v err=%v router=%#v completion=%#v", worked, err, router, repository.completion)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		record := switchRecord()
		record.Command = ""
		record.ActionKind = ""
		record.ErrorCode = ErrorUnknownCommand
		repository := &fakeWorkerRepository{next: record}
		router := &fakeRouter{}
		worked, err := testWorker(repository, router, &fakeSender{repository: repository}, ModeEnabled).RunOnce(context.Background())
		if err != nil || !worked || repository.completion.Status != StatusRejected || repository.completion.ErrorCode != ErrorUnknownCommand || router.readCalls != 0 || len(router.switchDryRuns) != 0 {
			t.Fatalf("worked=%v err=%v router=%#v completion=%#v", worked, err, router, repository.completion)
		}
	})
}

func TestWorkerCompletesBeforeReplyAndRetriesDeliveryAtMostThreeTimes(t *testing.T) {
	t.Run("third attempt succeeds", func(t *testing.T) {
		repository := &fakeWorkerRepository{next: switchRecord()}
		sender := &fakeSender{repository: repository, failures: 2}
		worked, err := testWorker(repository, &fakeRouter{}, sender, ModeEnabled).RunOnce(context.Background())
		if err != nil || !worked || sender.calls != 3 || repository.replyCalls != 3 || !repository.lastDelivered {
			t.Fatalf("worked=%v err=%v sender=%d replies=%d delivered=%v", worked, err, sender.calls, repository.replyCalls, repository.lastDelivered)
		}
	})

	t.Run("three failures stop", func(t *testing.T) {
		repository := &fakeWorkerRepository{next: switchRecord()}
		sender := &fakeSender{repository: repository, failures: 5}
		worked, err := testWorker(repository, &fakeRouter{}, sender, ModeEnabled).RunOnce(context.Background())
		if !worked || err == nil || sender.calls != 3 || repository.replyCalls != 3 || repository.lastDelivered {
			t.Fatalf("worked=%v err=%v sender=%d replies=%d delivered=%v", worked, err, sender.calls, repository.replyCalls, repository.lastDelivered)
		}
	})
}

func TestRoutingCompletionUsesTheAuditedSnapshotEnvelope(t *testing.T) {
	completion := routingCompletion(routingcontrol.Result{
		Status: routingcontrol.StatusSucceeded,
		Before: routingcontrol.GroupState{GroupName: "GPT-Pro", GroupID: 3, CurrentRole: routingcontrol.RolePrimary},
		After:  routingcontrol.GroupState{GroupName: "GPT-Pro", GroupID: 3, CurrentRole: routingcontrol.RoleBackup},
	})
	for name, payload := range map[string]json.RawMessage{"before": completion.BeforeState, "after": completion.AfterState} {
		var snapshot struct {
			Groups []routingcontrol.GroupState `json:"groups"`
		}
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			t.Fatalf("%s snapshot = %s: %v", name, payload, err)
		}
		if len(snapshot.Groups) != 1 || snapshot.Groups[0].GroupName != "GPT-Pro" {
			t.Fatalf("%s groups = %#v", name, snapshot.Groups)
		}
	}
}

func TestWorkerRunStopsWhenContextIsCancelled(t *testing.T) {
	repository := &fakeWorkerRepository{}
	worker := testWorker(repository, &fakeRouter{}, &fakeSender{repository: repository}, ModeDisabled)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestShortHashIsStableOnlyForTheSameProcessKey(t *testing.T) {
	first := shortHashWithKey("ou-user", []byte("process-key-one"))
	repeated := shortHashWithKey("ou-user", []byte("process-key-one"))
	otherKey := shortHashWithKey("ou-user", []byte("process-key-two"))
	if first != repeated || first == otherKey || len(first) != 12 {
		t.Fatalf("first=%q repeated=%q other=%q", first, repeated, otherKey)
	}
}

func TestWorkerRunStopsWhenContextIsCanceled(t *testing.T) {
	repository := &fakeWorkerRepository{}
	worker := testWorker(repository, &fakeRouter{}, &fakeSender{repository: repository}, ModeDisabled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

type fakeWorkerRepository struct {
	next          Record
	claimed       bool
	completion    Completion
	completed     bool
	lockedRoute   RouteLockIDs
	replyCalls    int
	lastDelivered bool
}

func (f *fakeWorkerRepository) ClaimFeishuCommand(context.Context, time.Time, time.Duration) (*Record, error) {
	if f.claimed || f.next.EventID == "" {
		return nil, nil
	}
	f.claimed = true
	record := f.next
	record.Status = StatusRunning
	return &record, nil
}

func (f *fakeWorkerRepository) CompleteFeishuCommand(_ context.Context, completion Completion) error {
	f.completion = completion
	f.completed = true
	return nil
}

func (f *fakeWorkerRepository) RecordFeishuReply(_ context.Context, _ string, _ string, delivered bool, _ string) error {
	f.replyCalls++
	f.lastDelivered = delivered
	return nil
}

func (f *fakeWorkerRepository) WithFeishuRouteLock(ctx context.Context, route RouteLockIDs, fn func(context.Context) Completion) (Completion, error) {
	f.lockedRoute = route
	return fn(ctx), nil
}

type fakeRouter struct {
	switchDryRuns []bool
	readCalls     int
}

func (f *fakeRouter) Switch(_ context.Context, groupName string, role routingcontrol.Role, dryRun bool) routingcontrol.Result {
	f.switchDryRuns = append(f.switchDryRuns, dryRun)
	return routingcontrol.Result{
		Status: routingcontrol.StatusSucceeded, GroupName: groupName, TargetRole: role, DryRun: dryRun,
		Before: routingcontrol.GroupState{GroupName: groupName, GroupID: 3, CurrentRole: routingcontrol.RolePrimary},
		After:  routingcontrol.GroupState{GroupName: groupName, GroupID: 3, CurrentRole: role},
	}
}

func (f *fakeRouter) ReadAll(context.Context) ([]routingcontrol.GroupState, error) {
	f.readCalls++
	return []routingcontrol.GroupState{
		{GroupName: "GPT-Pro", GroupID: 3, CurrentRole: routingcontrol.RolePrimary},
		{GroupName: "GPT-Plus", GroupID: 4, CurrentRole: routingcontrol.RolePrimary},
	}, nil
}

type fakeSender struct {
	repository *fakeWorkerRepository
	failures   int
	calls      int
	message    feishuapi.OutboundMessage
	textCalls  int
}

func (f *fakeSender) SendMessage(_ context.Context, _ string, message feishuapi.OutboundMessage) (string, error) {
	f.calls++
	f.message = message
	if !f.repository.completed {
		return "", errors.New("reply attempted before completion")
	}
	if f.calls <= f.failures {
		return "", errors.New("delivery failed")
	}
	return "om-reply", nil
}

func switchRecord() Record {
	return Record{
		EventID: "evt-worker", MessageID: "msg-worker", ChatID: "chat-worker", SenderOpenID: "ou-worker",
		Command: "切换 GPT-Pro 到灾备", ActionKind: ActionSwitch, GroupName: "GPT-Pro", TargetRole: RoleBackup,
		Status: StatusReceived, ReceivedAt: fixedNow(),
	}
}

func testWorker(repository *fakeWorkerRepository, router *fakeRouter, sender *fakeSender, mode string) *Worker {
	return &Worker{
		Mode: mode, Repository: repository, Router: router, Sender: sender,
		RouteLocks: map[string]RouteLockIDs{
			"GPT-Pro":  {GroupID: 3, PrimaryAccountID: 11, BackupAccountID: 12},
			"GPT-Plus": {GroupID: 4, PrimaryAccountID: 21, BackupAccountID: 22},
		},
		Now:   fixedNow,
		Lease: time.Minute, PollInterval: time.Millisecond,
	}
}
