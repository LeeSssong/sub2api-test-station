package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/commands"
)

func TestFeishuCommandEventIsIdempotentAndLeaseIsRecoverable(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	record := commands.Record{
		EventID: "evt-1", MessageID: "msg-1", ChatID: "chat-1", SenderOpenID: "ou-user",
		Command: "切换 GPT-Pro 到灾备", ActionKind: commands.ActionSwitch, GroupName: "GPT-Pro",
		TargetRole: commands.RoleBackup, Status: commands.StatusReceived, ReceivedAt: receivedAt,
	}
	inserted, err := st.InsertFeishuEvent(ctx, record)
	if err != nil || !inserted {
		t.Fatalf("first insert = %v, %v", inserted, err)
	}
	inserted, err = st.InsertFeishuEvent(ctx, record)
	if err != nil || inserted {
		t.Fatalf("duplicate insert = %v, %v", inserted, err)
	}

	claimed, err := st.ClaimFeishuCommand(ctx, receivedAt, time.Minute)
	if err != nil || claimed == nil || claimed.EventID != record.EventID || claimed.Status != commands.StatusRunning {
		t.Fatalf("first claim = %#v, %v", claimed, err)
	}
	claimedAgain, err := st.ClaimFeishuCommand(ctx, receivedAt.Add(30*time.Second), time.Minute)
	if err != nil || claimedAgain != nil {
		t.Fatalf("active lease claim = %#v, %v", claimedAgain, err)
	}
	recovered, err := st.ClaimFeishuCommand(ctx, receivedAt.Add(61*time.Second), time.Minute)
	if err != nil || recovered == nil || recovered.EventID != record.EventID {
		t.Fatalf("expired lease claim = %#v, %v", recovered, err)
	}
}

func TestFeishuCommandEventRejectsNonWhitelistAuditValues(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := commands.Record{
		EventID: "evt-invalid-audit", MessageID: "msg-invalid-audit", ChatID: "chat-invalid-audit",
		SenderOpenID: "ou-user", Command: "执行任意动作", ActionKind: commands.ActionKind("shell"),
		GroupName: "arbitrary", TargetRole: commands.RoleBackup, Status: commands.StatusReceived,
		ErrorCode: "INVALID VALUE", ReceivedAt: time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
	}
	if inserted, err := st.InsertFeishuEvent(ctx, record); err == nil || inserted {
		t.Fatalf("invalid audit values = %v, %v", inserted, err)
	}
}

func TestFeishuConcurrentClaimsReturnOneCommand(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	inserted, err := st.InsertFeishuEvent(ctx, commands.Record{
		EventID: "evt-concurrent", MessageID: "msg-concurrent", ChatID: "chat-concurrent",
		SenderOpenID: "ou-user", Command: "查询当前分组状态", ActionKind: commands.ActionStatus,
		Status: commands.StatusReceived, ReceivedAt: now,
	})
	if err != nil || !inserted {
		t.Fatalf("InsertFeishuEvent = %v, %v", inserted, err)
	}

	start := make(chan struct{})
	results := make(chan *commands.Record, 2)
	errors := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			record, claimErr := st.ClaimFeishuCommand(ctx, now, time.Minute)
			results <- record
			errors <- claimErr
		}()
	}
	ready.Wait()
	close(start)

	claimed := 0
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		if record := <-results; record != nil {
			claimed++
			if record.EventID != "evt-concurrent" {
				t.Fatalf("claimed event = %q", record.EventID)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed count = %d, want 1", claimed)
	}
}

func TestFeishuCommandCompletionAndReplyAreAudited(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	record := commands.Record{
		EventID: "evt-2", MessageID: "msg-2", ChatID: "chat-2", SenderOpenID: "ou-user",
		Command: "查询当前分组状态", ActionKind: commands.ActionStatus,
		Status: commands.StatusReceived, ReceivedAt: receivedAt,
	}
	if inserted, err := st.InsertFeishuEvent(ctx, record); err != nil || !inserted {
		t.Fatalf("InsertFeishuEvent = %v, %v", inserted, err)
	}
	if _, err := st.ClaimFeishuCommand(ctx, receivedAt, time.Minute); err != nil {
		t.Fatal(err)
	}
	before := json.RawMessage(`{"groups":[{"group_name":"GPT-Pro","current_role":"primary"}]}`)
	after := json.RawMessage(`{"groups":[{"group_name":"GPT-Pro","current_role":"primary"}]}`)
	if err := st.CompleteFeishuCommand(ctx, commands.Completion{
		EventID: "evt-2", Status: commands.StatusSucceeded, BeforeState: before, AfterState: after,
		CompletedAt: receivedAt.Add(2 * time.Second), Duration: 2 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordFeishuReply(ctx, "evt-2", "om-reply", true, ""); err != nil {
		t.Fatal(err)
	}

	var status string
	var storedBefore, storedAfter []byte
	var attempts int
	var delivered bool
	var replyMessageID string
	err := st.pool.QueryRow(ctx, `
		SELECT status, before_state, after_state, reply_attempts, reply_delivered, reply_message_id
		FROM relay_ops.feishu_command_events WHERE event_id='evt-2'`).Scan(
		&status, &storedBefore, &storedAfter, &attempts, &delivered, &replyMessageID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != commands.StatusSucceeded || attempts != 1 || !delivered || replyMessageID != "om-reply" || !json.Valid(storedBefore) || !json.Valid(storedAfter) {
		t.Fatalf("audit = %q %d %v %q %s %s", status, attempts, delivered, replyMessageID, storedBefore, storedAfter)
	}
}

func TestFeishuCompletionRejectsUnknownSnapshotFields(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	inserted, err := st.InsertFeishuEvent(ctx, commands.Record{
		EventID: "evt-sensitive", MessageID: "msg-sensitive", ChatID: "chat-sensitive",
		SenderOpenID: "ou-user", Command: "查询当前分组状态", ActionKind: commands.ActionStatus,
		Status: commands.StatusReceived, ReceivedAt: now,
	})
	if err != nil || !inserted {
		t.Fatalf("InsertFeishuEvent = %v, %v", inserted, err)
	}
	if _, err := st.ClaimFeishuCommand(ctx, now, time.Minute); err != nil {
		t.Fatal(err)
	}

	snapshot := json.RawMessage(`{"groups":[{"group_name":"GPT-Pro","current_role":"primary","api_key":"must-not-be-stored"}]}`)
	err = st.CompleteFeishuCommand(ctx, commands.Completion{
		EventID: "evt-sensitive", Status: commands.StatusSucceeded,
		BeforeState: snapshot, AfterState: snapshot, CompletedAt: now.Add(time.Second), Duration: time.Second,
	})
	if err == nil {
		t.Fatal("CompleteFeishuCommand accepted an unknown snapshot field")
	}
	if got := err.Error(); got == "" || contains(got, "must-not-be-stored") {
		t.Fatalf("unsafe completion error = %q", got)
	}
}

func TestFeishuRouteLockSerializesSharedAccount(t *testing.T) {
	st := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := st.WithFeishuRouteLock(ctx, commands.RouteLockIDs{GroupID: 3, PrimaryAccountID: 11, BackupAccountID: 12}, func(context.Context) commands.Completion {
			close(firstEntered)
			<-releaseFirst
			return commands.Completion{Status: commands.StatusSucceeded}
		})
		firstDone <- err
	}()
	select {
	case <-firstEntered:
	case err := <-firstDone:
		t.Fatalf("first group lock failed before callback: %v", err)
	case <-ctx.Done():
		t.Fatal("first group lock did not enter callback")
	}

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		_, err := st.WithFeishuRouteLock(ctx, commands.RouteLockIDs{GroupID: 4, PrimaryAccountID: 21, BackupAccountID: 12}, func(context.Context) commands.Completion {
			close(secondEntered)
			return commands.Completion{Status: commands.StatusSucceeded}
		})
		secondDone <- err
	}()
	select {
	case <-secondEntered:
		t.Fatal("second shared-account command entered before first released")
	case <-time.After(100 * time.Millisecond):
	}

	differentEntered := make(chan struct{})
	differentDone := make(chan error, 1)
	go func() {
		_, err := st.WithFeishuRouteLock(ctx, commands.RouteLockIDs{GroupID: 5, PrimaryAccountID: 31, BackupAccountID: 32}, func(context.Context) commands.Completion {
			close(differentEntered)
			return commands.Completion{Status: commands.StatusSucceeded}
		})
		differentDone <- err
	}()
	select {
	case <-differentEntered:
	case <-ctx.Done():
		t.Fatal("unrelated route lock was blocked by another route")
	}
	if err := <-differentDone; err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondEntered:
	case <-ctx.Done():
		t.Fatal("second command never acquired group lock")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func contains(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
