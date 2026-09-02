package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const openAISchedulerLogCleanupBatchSize = 1000

type openAISchedulerLogRepository struct {
	db *sql.DB
}

func NewOpenAISchedulerLogRepository(db *sql.DB) service.OpenAISchedulerLogRepository {
	return &openAISchedulerLogRepository{db: db}
}

func (r *openAISchedulerLogRepository) BatchInsertOpenAISchedulerLogs(ctx context.Context, inputs []service.OpenAISchedulerLogInsert) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil openai scheduler log repository")
	}
	if len(inputs) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO openai_scheduler_logs (
			event_at, platform, group_id, logical_request_id, attempt_id, attempt_number, event_name,
			account_id, canonical_model, outcome, final_outcome, selection_layer, algorithm_version, decision
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)
	`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	inserted := 0
	for _, input := range inputs {
		if strings.TrimSpace(input.LogicalRequestID) == "" {
			continue
		}
		var accountID any
		if input.AccountID > 0 {
			accountID = input.AccountID
		}
		decision := strings.TrimSpace(input.DecisionJSON)
		if decision == "" {
			decision = "{}"
		}
		version := strings.TrimSpace(input.AlgorithmVersion)
		if version == "" {
			version = service.OpenAISchedulerAlgorithmVersion
		}
		eventAt := input.EventAt.UTC()
		if eventAt.IsZero() {
			eventAt = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, eventAt, input.Platform, input.GroupID, input.LogicalRequestID,
			openAISchedulerLogNullableString(input.AttemptID), input.AttemptNumber, input.EventName, accountID,
			openAISchedulerLogNullableString(input.CanonicalModel), openAISchedulerLogNullableString(input.Outcome), openAISchedulerLogNullableString(input.FinalOutcome),
			openAISchedulerLogNullableString(input.SelectionLayer), version, decision); err != nil {
			return 0, err
		}
		inserted++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return inserted, nil
}

func (r *openAISchedulerLogRepository) DeleteOpenAISchedulerLogsBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil openai scheduler log repository")
	}
	if limit <= 0 {
		limit = openAISchedulerLogCleanupBatchSize
	}
	result, err := r.db.ExecContext(ctx, `
		WITH doomed AS (
			SELECT id FROM openai_scheduler_logs
			WHERE event_at < $1
			ORDER BY event_at ASC, id ASC
			LIMIT $2
		)
		DELETE FROM openai_scheduler_logs AS logs
		USING doomed
		WHERE logs.id = doomed.id
	`, cutoff.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *openAISchedulerLogRepository) ListOpenAISchedulerLogs(ctx context.Context, filter *service.OpenAISchedulerLogListFilter) (*service.OpenAISchedulerLogList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil openai scheduler log repository")
	}
	if filter == nil {
		filter = &service.OpenAISchedulerLogListFilter{}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	clauses := []string{"event_at >= $1", "event_at <= $2"}
	args := []any{filter.From.UTC(), filter.To.UTC()}
	add := func(sql string, values ...any) {
		clauses = append(clauses, sql)
		args = append(args, values...)
	}
	if filter.GroupID != nil {
		add(fmt.Sprintf("group_id = $%d", len(args)+1), *filter.GroupID)
	}
	if filter.AccountID != nil {
		add(fmt.Sprintf("account_id = $%d", len(args)+1), *filter.AccountID)
	}
	if value := strings.TrimSpace(filter.Outcome); value != "" {
		add(fmt.Sprintf("COALESCE(final_outcome, outcome, '') = $%d", len(args)+1), value)
	}
	if value := strings.TrimSpace(filter.Mechanism); value != "" {
		add(fmt.Sprintf("selection_layer = $%d", len(args)+1), value)
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		add(fmt.Sprintf("(logical_request_id ILIKE $%d OR canonical_model ILIKE $%d)", len(args)+1, len(args)+2), "%"+value+"%", "%"+value+"%")
	}
	if filter.Cursor != nil {
		add(fmt.Sprintf("(event_at, id) < ($%d, $%d)", len(args)+1, len(args)+2), filter.Cursor.EventAt.UTC(), filter.Cursor.ID)
	}
	args = append(args, limit+1)
	query := `SELECT id, event_at, platform, group_id, logical_request_id, attempt_id, attempt_number, event_name,
		account_id, canonical_model, outcome, final_outcome, selection_layer, algorithm_version, decision
		FROM openai_scheduler_logs WHERE ` + strings.Join(clauses, " AND ") +
		fmt.Sprintf(" ORDER BY event_at DESC, id DESC LIMIT $%d", len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	logs, err := scanOpenAISchedulerLogs(rows)
	if err != nil {
		return nil, err
	}
	result := &service.OpenAISchedulerLogList{Logs: logs}
	if len(result.Logs) > limit {
		last := result.Logs[limit-1]
		result.Logs = result.Logs[:limit]
		result.NextCursor = encodeOpenAISchedulerLogCursor(service.OpenAISchedulerLogCursor{EventAt: last.EventAt, ID: last.ID})
	}
	return result, nil
}

func (r *openAISchedulerLogRepository) GetOpenAISchedulerLogTimeline(ctx context.Context, logicalRequestID string) (*service.OpenAISchedulerLogTimeline, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil openai scheduler log repository")
	}
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if logicalRequestID == "" {
		return nil, fmt.Errorf("logical request id is required")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, event_at, platform, group_id, logical_request_id, attempt_id, attempt_number, event_name,
		account_id, canonical_model, outcome, final_outcome, selection_layer, algorithm_version, decision
		FROM openai_scheduler_logs WHERE logical_request_id = $1 ORDER BY event_at ASC, id ASC`, logicalRequestID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	logs, err := scanOpenAISchedulerLogs(rows)
	if err != nil {
		return nil, err
	}
	return &service.OpenAISchedulerLogTimeline{LogicalRequestID: logicalRequestID, Attempts: logs}, nil
}

func scanOpenAISchedulerLogs(rows *sql.Rows) ([]service.OpenAISchedulerLog, error) {
	logs := make([]service.OpenAISchedulerLog, 0)
	for rows.Next() {
		var (
			log                                            service.OpenAISchedulerLog
			groupID, accountID                             sql.NullInt64
			attemptID, model, outcome, finalOutcome, layer sql.NullString
			decisionRaw                                    []byte
		)
		if err := rows.Scan(&log.ID, &log.EventAt, &log.Platform, &groupID, &log.LogicalRequestID, &attemptID, &log.AttemptNumber, &log.EventName,
			&accountID, &model, &outcome, &finalOutcome, &layer, &log.AlgorithmVersion, &decisionRaw); err != nil {
			return nil, err
		}
		if groupID.Valid {
			value := groupID.Int64
			log.GroupID = &value
		}
		if accountID.Valid {
			value := accountID.Int64
			log.AccountID = &value
		}
		if attemptID.Valid {
			log.AttemptID = attemptID.String
		}
		if model.Valid {
			log.CanonicalModel = model.String
		}
		if outcome.Valid {
			log.Outcome = outcome.String
		}
		if finalOutcome.Valid {
			log.FinalOutcome = finalOutcome.String
		}
		if layer.Valid {
			log.SelectionLayer = layer.String
		}
		if len(decisionRaw) > 0 {
			if err := json.Unmarshal(decisionRaw, &log.Decision); err != nil {
				return nil, fmt.Errorf("decode scheduler decision: %w", err)
			}
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func openAISchedulerLogNullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func encodeOpenAISchedulerLogCursor(cursor service.OpenAISchedulerLogCursor) string {
	payload, _ := json.Marshal(struct {
		EventAt time.Time `json:"event_at"`
		ID      int64     `json:"id"`
	}{cursor.EventAt.UTC(), cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func DecodeOpenAISchedulerLogCursor(value string) (*service.OpenAISchedulerLogCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	var raw struct {
		EventAt time.Time `json:"event_at"`
		ID      int64     `json:"id"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil || raw.EventAt.IsZero() || raw.ID <= 0 {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &service.OpenAISchedulerLogCursor{EventAt: raw.EventAt.UTC(), ID: raw.ID}, nil
}
