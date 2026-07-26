package commands

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/routingcontrol"
)

const (
	ModeDisabled = "disabled"
	ModeDryRun   = "dry_run"
	ModeEnabled  = "enabled"

	ErrorCommandDisabled = "command_disabled"
	ErrorRoutingFailed   = "routing_failed"
	ErrorReplyFailed     = "reply_failed"
)

type WorkerRepository interface {
	ClaimFeishuCommand(context.Context, time.Time, time.Duration) (*Record, error)
	CompleteFeishuCommand(context.Context, Completion) error
	RecordFeishuReply(context.Context, string, string, bool, string) error
	WithFeishuRouteLock(context.Context, RouteLockIDs, func(context.Context) Completion) (Completion, error)
}

type RouteLockIDs struct {
	GroupID          int64
	PrimaryAccountID int64
	BackupAccountID  int64
}

type Router interface {
	Switch(context.Context, string, routingcontrol.Role, bool) routingcontrol.Result
	ReadAll(context.Context) ([]routingcontrol.GroupState, error)
}

type MessageSender interface {
	SendMessage(context.Context, string, feishuapi.OutboundMessage) (string, error)
}

type Worker struct {
	Mode         string
	Repository   WorkerRepository
	Router       Router
	Sender       MessageSender
	RouteLocks   map[string]RouteLockIDs
	Now          func() time.Time
	Lease        time.Duration
	PollInterval time.Duration
	// ReplyBackoff waits between reply attempts. Tests replace it so the retry
	// path stays instant; production leaves it nil and gets a real wait.
	ReplyBackoff func(context.Context, time.Duration) error
}

func (w *Worker) Run(ctx context.Context) error {
	interval := w.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	for {
		worked, err := w.RunOnce(ctx)
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	if w.Repository == nil || w.Sender == nil || (w.Mode != ModeDisabled && w.Router == nil) {
		return false, errors.New("Feishu command worker dependencies are unavailable")
	}
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	lease := w.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	record, err := w.Repository.ClaimFeishuCommand(ctx, now().UTC(), lease)
	if err != nil || record == nil {
		return false, err
	}
	startedAt := now()
	completion, err := w.execute(ctx, *record)
	if err != nil {
		return true, err
	}
	completion.EventID = record.EventID
	completion.CompletedAt = now().UTC()
	completion.Duration = completion.CompletedAt.Sub(startedAt)
	if completion.Duration < 0 {
		completion.Duration = 0
	}
	if err := w.Repository.CompleteFeishuCommand(ctx, completion); err != nil {
		return true, err
	}
	message := renderReply(*record, completion, w.Mode == ModeDryRun)
	payload, err := message.Outbound()
	if err != nil {
		return true, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// Back-to-back retries are the one thing that cannot help here:
			// the failures worth retrying are throttling and transient Feishu
			// faults, and three sends inside the same millisecond spend the
			// remaining quota reproducing the same rejection.
			if waitErr := w.pauseBeforeRetry(ctx, replyRetryDelay(attempt)); waitErr != nil {
				return true, errors.New("Feishu reply retry was interrupted")
			}
		}
		messageID, sendErr := w.Sender.SendMessage(ctx, record.ChatID, payload)
		delivered := sendErr == nil
		errorCode := ""
		if !delivered {
			errorCode = ErrorReplyFailed
			lastErr = sendErr
		}
		if err := w.Repository.RecordFeishuReply(ctx, record.EventID, messageID, delivered, errorCode); err != nil {
			return true, err
		}
		if delivered {
			return true, nil
		}
	}
	if lastErr != nil {
		return true, errors.New("Feishu reply failed after 3 attempts")
	}
	return true, nil
}

func replyRetryDelay(attempt int) time.Duration {
	return time.Duration(1<<(attempt-1)) * time.Second
}

func (w *Worker) pauseBeforeRetry(ctx context.Context, delay time.Duration) error {
	if w.ReplyBackoff != nil {
		return w.ReplyBackoff(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w *Worker) execute(ctx context.Context, record Record) (Completion, error) {
	if record.ErrorCode == ErrorUnknownCommand || record.ActionKind == "" {
		return Completion{Status: StatusRejected, ErrorCode: ErrorUnknownCommand}, nil
	}
	if w.Mode == ModeDisabled {
		return Completion{Status: StatusRejected, ErrorCode: ErrorCommandDisabled}, nil
	}
	if w.Mode != ModeDryRun && w.Mode != ModeEnabled {
		return Completion{Status: StatusRejected, ErrorCode: ErrorCommandDisabled}, nil
	}
	if record.ActionKind == ActionStatus {
		states, err := w.Router.ReadAll(ctx)
		if err != nil {
			return Completion{Status: StatusFailed, ErrorCode: ErrorRoutingFailed}, nil
		}
		snapshot, err := json.Marshal(map[string]any{"groups": states})
		if err != nil {
			return Completion{}, errors.New("encode routing status")
		}
		return Completion{Status: StatusSucceeded, BeforeState: snapshot, AfterState: snapshot}, nil
	}
	if record.ActionKind != ActionSwitch {
		return Completion{Status: StatusRejected, ErrorCode: ErrorUnknownCommand}, nil
	}
	locks, exists := w.RouteLocks[record.GroupName]
	if !exists || locks.GroupID <= 0 || locks.PrimaryAccountID <= 0 || locks.BackupAccountID <= 0 {
		return Completion{Status: StatusRejected, ErrorCode: routingcontrol.ErrorConfigMismatch}, nil
	}
	completion, err := w.Repository.WithFeishuRouteLock(ctx, locks, func(lockCtx context.Context) Completion {
		result := w.Router.Switch(lockCtx, record.GroupName, routingcontrol.Role(record.TargetRole), w.Mode == ModeDryRun)
		return routingCompletion(result)
	})
	if err != nil {
		return Completion{}, err
	}
	return completion, nil
}

func routingCompletion(result routingcontrol.Result) Completion {
	before, beforeErr := json.Marshal(map[string]any{"groups": []routingcontrol.GroupState{result.Before}})
	after, afterErr := json.Marshal(map[string]any{"groups": []routingcontrol.GroupState{result.After}})
	if beforeErr != nil || afterErr != nil {
		return Completion{Status: StatusFailed, ErrorCode: ErrorRoutingFailed}
	}
	return Completion{
		Status: result.Status, ErrorCode: result.ErrorCode,
		BeforeState: before, AfterState: after,
	}
}

func renderReply(record Record, completion Completion, dryRun bool) notify.FeishuMessage {
	auditID := shortHash(record.EventID)
	return notify.RenderCommand(notify.CommandView{
		Command: record.Command, ActorID: record.SenderOpenID, Status: completion.Status,
		ErrorCode: completion.ErrorCode, GroupName: record.GroupName, TargetRole: record.TargetRole,
		AuditID: auditID, DryRun: dryRun, Unknown: completion.ErrorCode == ErrorUnknownCommand,
	})
}

func shortHash(value string) string {
	return shortHashWithKey(value, processHashKey)
}

func shortHashWithKey(value string, key []byte) string {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(value))
	return hex.EncodeToString(digest.Sum(nil)[:6])
}

var processHashKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("Feishu command hash key is unavailable")
	}
	return key
}()
