package store

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"example.invalid/relay-ops-service/internal/commands"
	"github.com/jackc/pgx/v5"
)

//go:embed migrations/002_feishu_commands.sql
var feishuCommandMigration string

func init() {
	initialMigration += "\n" + feishuCommandMigration
}

func (s *Store) InsertFeishuEvent(ctx context.Context, record commands.Record) (bool, error) {
	command, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.feishu_command_events
			(event_id, message_id, chat_id, sender_open_id, command_text, action_kind,
			 group_name, target_role, status, error_code, received_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
			NULLIF($8, ''), $9, NULLIF($10, ''), $11)
		ON CONFLICT (event_id) DO NOTHING`,
		record.EventID, record.MessageID, record.ChatID, record.SenderOpenID, record.Command,
		string(record.ActionKind), record.GroupName, record.TargetRole, record.Status,
		record.ErrorCode, record.ReceivedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert Feishu command event: %w", err)
	}
	return command.RowsAffected() == 1, nil
}

func (s *Store) ClaimFeishuCommand(ctx context.Context, now time.Time, lease time.Duration) (*commands.Record, error) {
	if lease <= 0 {
		return nil, errors.New("Feishu command lease must be positive")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin Feishu command claim: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT event_id
			FROM relay_ops.feishu_command_events
			WHERE status = 'received'
			   OR (status = 'running' AND lease_expires_at < $1)
			ORDER BY received_at, event_id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE relay_ops.feishu_command_events AS job
		SET status='running', lease_expires_at=$2,
			started_at=COALESCE(started_at, $1), updated_at=$1
		FROM candidate
		WHERE job.event_id=candidate.event_id
		RETURNING job.event_id, job.message_id, job.chat_id, job.sender_open_id,
			COALESCE(job.command_text, ''), COALESCE(job.action_kind, ''),
			COALESCE(job.group_name, ''), COALESCE(job.target_role, ''), job.status,
			COALESCE(job.error_code, ''), job.received_at, job.lease_expires_at,
			job.reply_attempts`, now, now.Add(lease))
	var record commands.Record
	var actionKind string
	if err := row.Scan(
		&record.EventID, &record.MessageID, &record.ChatID, &record.SenderOpenID,
		&record.Command, &actionKind, &record.GroupName, &record.TargetRole,
		&record.Status, &record.ErrorCode, &record.ReceivedAt, &record.LeaseExpiresAt,
		&record.ReplyAttempts,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, tx.Commit(ctx)
		}
		return nil, fmt.Errorf("claim Feishu command: %w", err)
	}
	record.ActionKind = commands.ActionKind(actionKind)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Feishu command claim: %w", err)
	}
	return &record, nil
}

func (s *Store) CompleteFeishuCommand(ctx context.Context, completion commands.Completion) error {
	if !validRouteSnapshot(completion.BeforeState) {
		return errors.New("Feishu command before state is invalid JSON")
	}
	if !validRouteSnapshot(completion.AfterState) {
		return errors.New("Feishu command after state is invalid JSON")
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE relay_ops.feishu_command_events
		SET status=$2, error_code=NULLIF($3, ''), before_state=$4, after_state=$5,
			duration_ms=$6, completed_at=$7, lease_expires_at=NULL, updated_at=$7
		WHERE event_id=$1 AND status='running'`, completion.EventID, completion.Status,
		completion.ErrorCode, nullableJSON(completion.BeforeState), nullableJSON(completion.AfterState),
		completion.Duration.Milliseconds(), completion.CompletedAt)
	if err != nil {
		return fmt.Errorf("complete Feishu command: %w", err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("Feishu command completion did not match a running job")
	}
	return nil
}

func (s *Store) RecordFeishuReply(ctx context.Context, eventID, messageID string, delivered bool, errorCode string) error {
	command, err := s.pool.Exec(ctx, `
		UPDATE relay_ops.feishu_command_events
		SET reply_attempts=reply_attempts+1, reply_delivered=$2,
			reply_message_id=NULLIF($3, ''), reply_error_code=NULLIF($4, ''), updated_at=NOW()
		WHERE event_id=$1 AND reply_attempts < 3`, eventID, delivered, messageID, errorCode)
	if err != nil {
		return fmt.Errorf("record Feishu command reply: %w", err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("Feishu command reply attempt was not recorded")
	}
	return nil
}

func (s *Store) WithFeishuRouteLock(ctx context.Context, ids commands.RouteLockIDs, fn func(context.Context) commands.Completion) (commands.Completion, error) {
	if ids.GroupID <= 0 || ids.PrimaryAccountID <= 0 || ids.BackupAccountID <= 0 || fn == nil {
		return commands.Completion{}, errors.New("Feishu command route lock input is invalid")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return commands.Completion{}, fmt.Errorf("begin Feishu command route lock: %w", err)
	}
	defer tx.Rollback(ctx)
	keys := []string{
		fmt.Sprintf("relay_ops_feishu_group:%d", ids.GroupID),
		fmt.Sprintf("relay_ops_feishu_account:%d", ids.PrimaryAccountID),
		fmt.Sprintf("relay_ops_feishu_account:%d", ids.BackupAccountID),
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index > 0 && key == keys[index-1] {
			continue
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
			return commands.Completion{}, fmt.Errorf("acquire Feishu command route lock: %w", err)
		}
	}
	completion := fn(ctx)
	if err := tx.Commit(ctx); err != nil {
		return commands.Completion{}, fmt.Errorf("release Feishu command route lock: %w", err)
	}
	return completion, nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

type routeSnapshot struct {
	Groups []routeSnapshotGroup `json:"groups"`
}

type routeSnapshotGroup struct {
	GroupName       string  `json:"group_name"`
	GroupID         int64   `json:"group_id,omitempty"`
	CurrentRole     string  `json:"current_role"`
	PrimaryBound    bool    `json:"primary_bound,omitempty"`
	BackupBound     bool    `json:"backup_bound,omitempty"`
	PrimaryEligible bool    `json:"primary_eligible,omitempty"`
	BackupEligible  bool    `json:"backup_eligible,omitempty"`
	RateMultiplier  float64 `json:"rate_multiplier,omitempty"`
}

func validRouteSnapshot(value json.RawMessage) bool {
	if len(value) == 0 {
		return true
	}
	if len(value) > 32<<10 {
		return false
	}
	var snapshot routeSnapshot
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false
	}
	if len(snapshot.Groups) == 0 || len(snapshot.Groups) > 2 {
		return false
	}
	seen := make(map[string]struct{}, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		if group.GroupName != "GPT-Pro" && group.GroupName != "GPT-Plus" {
			return false
		}
		if _, exists := seen[group.GroupName]; exists {
			return false
		}
		seen[group.GroupName] = struct{}{}
		switch group.CurrentRole {
		case "primary", "backup", "mixed", "none":
		default:
			return false
		}
	}
	return true
}
