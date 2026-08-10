package billing

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeWriter struct {
	id     int64
	fields map[string]any
	calls  int
}

func (f *fakeWriter) SendAccountUpdateCommand(_ context.Context, command AccountUpdateCommand) error {
	f.calls++
	f.id = command.AccountID
	f.fields = command.Fields
	return nil
}

func TestBalanceSnapshotAndOfficialWriteContract(t *testing.T) {
	now := time.Now().UTC()
	if err := (BalanceSnapshot{AccountID: 1, Amount: "1.20", Currency: "USD", ObservedAt: now, FreshUntil: now.Add(time.Minute), Source: "provider"}).Validate(); err != nil {
		t.Fatal(err)
	}
	w := &fakeWriter{}
	if err := ApplyAccountUpdate(context.Background(), w, AccountUpdateCommand{CommandID: "cmd-1", ActorID: 9, AccountID: 1, IdempotencyKey: "idem-1", Fields: map[string]any{"rate_multiplier": "1.1"}}); err != nil {
		t.Fatal(err)
	}
	if w.id != 1 {
		t.Fatal("official writer not called")
	}
}

func TestCommandReplaysSameIdempotencyKeyWithoutSecondOfficialWrite(t *testing.T) {
	writer := &fakeWriter{}
	audit := &commandAudit{claimed: map[string]bool{}}
	command := AccountUpdateCommand{CommandID: "cmd-1", ActorID: 9, AccountID: 1, IdempotencyKey: "account:1:rate:1", Fields: map[string]any{"rate_multiplier": 1.1}}
	for range 2 {
		if err := ExecuteAccountUpdate(context.Background(), writer, audit, command); err != nil {
			t.Fatal(err)
		}
	}
	if writer.calls != 1 || audit.completed != 1 {
		t.Fatalf("official writes=%d completions=%d", writer.calls, audit.completed)
	}
}

func TestCommandRejectsUnauthorizedOrUnsafeFieldsBeforeOfficialWrite(t *testing.T) {
	writer := &fakeWriter{}
	audit := &commandAudit{claimed: map[string]bool{}}
	for _, command := range []AccountUpdateCommand{
		{CommandID: "cmd-unauthorized", AccountID: 1, IdempotencyKey: "account:1:no-actor", Fields: map[string]any{"priority": 2}},
		{CommandID: "cmd-unsafe", ActorID: 9, AccountID: 1, IdempotencyKey: "account:1:credentials", Fields: map[string]any{"credentials": "secret"}},
	} {
		if err := ExecuteAccountUpdate(context.Background(), writer, audit, command); err == nil {
			t.Fatalf("command %#v was accepted", command)
		}
	}
	if writer.calls != 0 {
		t.Fatalf("unsafe command reached official writer: %d", writer.calls)
	}
}

func TestCommandFailedReplayReturnsSameFailure(t *testing.T) {
	writer := &failingWriter{}
	audit := &failedReplayAudit{}
	command := AccountUpdateCommand{CommandID: "cmd-failed", ActorID: 9, AccountID: 1, IdempotencyKey: "failed:1", Fields: map[string]any{"priority": 2}}
	if err := ExecuteAccountUpdate(context.Background(), writer, audit, command); err == nil {
		t.Fatal("initial failure was accepted")
	}
	if err := ExecuteAccountUpdate(context.Background(), writer, audit, command); !errors.Is(err, ErrAccountUpdateFailed) {
		t.Fatalf("replay=%v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("calls=%d", writer.calls)
	}
}

type failingWriter struct{ calls int }

func (w *failingWriter) SendAccountUpdateCommand(context.Context, AccountUpdateCommand) error {
	w.calls++
	return errors.New("official failure")
}

type failedReplayAudit struct{ failed bool }

func (a *failedReplayAudit) ClaimAccountUpdateCommand(context.Context, AccountUpdateCommand, string, int) (bool, string, error) {
	if a.failed {
		return false, "failed", nil
	}
	return true, "pending", nil
}
func (a *failedReplayAudit) CompleteAccountUpdateCommand(_ context.Context, _ AccountUpdateCommand, result string, _ int) error {
	a.failed = result == "failed"
	return nil
}

type commandAudit struct {
	claimed   map[string]bool
	completed int
}

func (a *commandAudit) ClaimAccountUpdateCommand(_ context.Context, command AccountUpdateCommand, _ string, _ int) (bool, string, error) {
	if a.claimed[command.IdempotencyKey] {
		return false, "accepted", nil
	}
	a.claimed[command.IdempotencyKey] = true
	return true, "pending", nil
}
func (a *commandAudit) CompleteAccountUpdateCommand(_ context.Context, _ AccountUpdateCommand, result string, _ int) error {
	if result != "accepted" {
		return errors.New("unexpected command failure")
	}
	a.completed++
	return nil
}

func (a *commandAudit) ClaimExternalizationCommand(_ context.Context, _, _ int64, key, _ string, _ int) (bool, string, error) {
	if a.claimed[key] {
		return false, "accepted", nil
	}
	a.claimed[key] = true
	return true, "pending", nil
}

func (a *commandAudit) CompleteExternalizationCommand(_ context.Context, _, _ int64, _ string, result string, _ int) error {
	if result != "accepted" {
		return errors.New("unexpected command failure")
	}
	a.completed++
	return nil
}
