package commands

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	WithFeishuGroupLock(context.Context, int64, func(context.Context) Completion) (Completion, error)
}

type Router interface {
	Switch(context.Context, string, routingcontrol.Role, bool) routingcontrol.Result
	ReadAll(context.Context) ([]routingcontrol.GroupState, error)
}

type TextSender interface {
	SendText(context.Context, string, string) (string, error)
}

type Worker struct {
	Mode         string
	Repository   WorkerRepository
	Router       Router
	Sender       TextSender
	GroupIDs     map[string]int64
	Now          func() time.Time
	Lease        time.Duration
	PollInterval time.Duration
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
	text := renderReply(*record, completion)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		messageID, sendErr := w.Sender.SendText(ctx, record.ChatID, text)
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
	groupID := w.GroupIDs[record.GroupName]
	if groupID <= 0 {
		return Completion{Status: StatusRejected, ErrorCode: routingcontrol.ErrorConfigMismatch}, nil
	}
	completion, err := w.Repository.WithFeishuGroupLock(ctx, groupID, func(lockCtx context.Context) Completion {
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

func renderReply(record Record, completion Completion) string {
	auditID := shortHash(record.EventID)
	actorID := shortHash(record.SenderOpenID)
	switch completion.ErrorCode {
	case ErrorUnknownCommand:
		return "未识别命令。可用命令：\n切换 GPT-Pro 到灾备\n切换 GPT-Plus 到灾备\n恢复 GPT-Pro 主分组\n恢复 GPT-Plus 主分组\n查询当前分组状态\n审计：" + auditID
	case ErrorCommandDisabled:
		return "命令功能未启用。结果：rejected\n审计：" + auditID
	}
	parts := []string{
		"命令：" + record.Command,
		"执行者：" + actorID,
		"结果：" + completion.Status,
	}
	if record.GroupName != "" {
		parts = append(parts, "分组："+record.GroupName, "目标："+record.TargetRole)
	}
	if completion.ErrorCode != "" {
		parts = append(parts, "错误码："+completion.ErrorCode)
	}
	if completion.Status == StatusPartial {
		parts = append(parts, "需要人工复核当前账号绑定。")
	}
	parts = append(parts, "审计："+auditID)
	return strings.Join(parts, "\n")
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
