package store

import (
	"context"
	"encoding/json"
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

func TestFeishuGroupLockSerializesSameGroup(t *testing.T) {
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
		_, err := st.WithFeishuGroupLock(ctx, 3, func(context.Context) commands.Completion {
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
		_, err := st.WithFeishuGroupLock(ctx, 3, func(context.Context) commands.Completion {
			close(secondEntered)
			return commands.Completion{Status: commands.StatusSucceeded}
		})
		secondDone <- err
	}()
	select {
	case <-secondEntered:
		t.Fatal("second same-group command entered before first released")
	case <-time.After(100 * time.Millisecond):
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
