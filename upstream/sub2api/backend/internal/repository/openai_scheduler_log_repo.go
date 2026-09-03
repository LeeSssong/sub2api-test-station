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
	cursorClause := ""
	if filter.Cursor != nil {
		cursorClause = fmt.Sprintf("WHERE (cursor_event_at, cursor_id) < ($%d, $%d)", len(args)+1, len(args)+2)
		args = append(args, filter.Cursor.EventAt.UTC(), filter.Cursor.ID)
	}
	args = append(args, limit+1)
	query := `WITH matching_requests AS (
		SELECT logical_request_id, MAX(event_at) AS cursor_event_at, MAX(id) AS cursor_id
		FROM openai_scheduler_logs WHERE ` + strings.Join(clauses, " AND ") + `
		GROUP BY logical_request_id
	)
	SELECT logical_request_id, cursor_event_at, cursor_id
	FROM matching_requests ` + cursorClause +
		fmt.Sprintf(" ORDER BY cursor_event_at DESC, cursor_id DESC LIMIT $%d", len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type requestPageEntry struct {
		id      string
		eventAt time.Time
		rowID   int64
	}
	page := make([]requestPageEntry, 0, limit+1)
	for rows.Next() {
		var entry requestPageEntry
		if err := rows.Scan(&entry.id, &entry.eventAt, &entry.rowID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		page = append(page, entry)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(page) == 0 {
		return &service.OpenAISchedulerLogList{Logs: []service.OpenAISchedulerLog{}}, nil
	}
	result := &service.OpenAISchedulerLogList{}
	if len(page) > limit {
		page = page[:limit]
		last := page[len(page)-1]
		result.NextCursor = encodeOpenAISchedulerLogCursor(service.OpenAISchedulerLogCursor{EventAt: last.eventAt, ID: last.rowID})
	}
	requestArgs := make([]any, 0, len(page))
	placeholders := make([]string, 0, len(page))
	for index, entry := range page {
		requestArgs = append(requestArgs, entry.id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
	}
	eventQuery := `SELECT id, event_at, platform, group_id, logical_request_id, attempt_id, attempt_number, event_name,
		account_id, canonical_model, outcome, final_outcome, selection_layer, algorithm_version, decision
		FROM openai_scheduler_logs WHERE logical_request_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY event_at DESC, id DESC`
	eventRows, err := r.db.QueryContext(ctx, eventQuery, requestArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = eventRows.Close() }()
	result.Logs, err = scanOpenAISchedulerLogs(eventRows)
	if err != nil {
		return nil, err
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
	timeline := &service.OpenAISchedulerLogTimeline{LogicalRequestID: logicalRequestID, Attempts: logs, FinalOutcome: "unknown"}
	for _, event := range logs {
		if event.AlgorithmVersion != "" {
			timeline.AlgorithmVersion = event.AlgorithmVersion
		}
		if budget := schedulerLogDecisionInt(event.Decision, "runtime_retry_budget"); budget > timeline.RuntimeRetryBudget {
			timeline.RuntimeRetryBudget = budget
		}
		if switches := schedulerLogDecisionInt(event.Decision, "switch_count"); switches > timeline.SwitchCount {
			timeline.SwitchCount = switches
		}
		if event.FinalOutcome != "" {
			timeline.FinalOutcome = event.FinalOutcome
		}
	}
	return timeline, nil
}

func schedulerLogDecisionInt(decision map[string]any, key string) int {
	switch value := decision[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
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
