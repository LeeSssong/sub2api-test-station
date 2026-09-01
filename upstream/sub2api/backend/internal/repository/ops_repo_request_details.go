package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) ListRequestDetails(ctx context.Context, filter *service.OpsRequestDetailFilter) ([]*service.OpsRequestDetail, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("nil ops repository")
	}

	page, pageSize, startTime, endTime := filter.Normalize()
	offset := (page - 1) * pageSize

	conditions := make([]string, 0, 16)
	args := make([]any, 0, 24)

	// Placeholders $1/$2 reserved for time window inside the CTE.
	args = append(args, startTime.UTC(), endTime.UTC())

	addCondition := func(condition string, values ...any) {
		conditions = append(conditions, condition)
		args = append(args, values...)
	}

	if filter != nil {
		if kind := strings.TrimSpace(strings.ToLower(filter.Kind)); kind != "" && kind != "all" {
			if kind != string(service.OpsRequestKindSuccess) && kind != string(service.OpsRequestKindError) {
				return nil, 0, fmt.Errorf("invalid kind")
			}
			addCondition(fmt.Sprintf("kind = $%d", len(args)+1), kind)
		}

		if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
			addCondition(fmt.Sprintf("platform = $%d", len(args)+1), platform)
		}
		if filter.GroupID != nil && *filter.GroupID > 0 {
			addCondition(fmt.Sprintf("group_id = $%d", len(args)+1), *filter.GroupID)
		}

		if filter.UserID != nil && *filter.UserID > 0 {
			addCondition(fmt.Sprintf("user_id = $%d", len(args)+1), *filter.UserID)
		}
		if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
			addCondition(fmt.Sprintf("api_key_id = $%d", len(args)+1), *filter.APIKeyID)
		}
		if filter.AccountID != nil && *filter.AccountID > 0 {
			addCondition(fmt.Sprintf("account_id = $%d", len(args)+1), *filter.AccountID)
		}

		if model := strings.TrimSpace(filter.Model); model != "" {
			addCondition(fmt.Sprintf("model = $%d", len(args)+1), model)
		}
		if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
			idx := len(args) + 1
			addCondition(fmt.Sprintf("(request_id = $%d OR logical_request_id = $%d)", idx, idx), requestID)
		}
		if q := strings.TrimSpace(filter.Query); q != "" {
			like := "%" + strings.ToLower(q) + "%"
			startIdx := len(args) + 1
			addCondition(
				fmt.Sprintf("(LOWER(COALESCE(request_id,'')) LIKE $%d OR LOWER(COALESCE(logical_request_id,'')) LIKE $%d OR LOWER(COALESCE(model,'')) LIKE $%d OR LOWER(COALESCE(message,'')) LIKE $%d)",
					startIdx, startIdx+1, startIdx+2, startIdx+3,
				),
				like, like, like, like,
			)
		}

		if filter.MinDurationMs != nil {
			addCondition(fmt.Sprintf("duration_ms >= $%d", len(args)+1), *filter.MinDurationMs)
		}
		if filter.MaxDurationMs != nil {
			addCondition(fmt.Sprintf("duration_ms <= $%d", len(args)+1), *filter.MaxDurationMs)
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	cte := `
WITH usage_events AS (
  SELECT
    ul.id::BIGINT AS source_id,
    ul.created_at,
    ul.request_id,
    NULLIF(ul.logical_request_id, '') AS logical_request_id,
    COALESCE(NULLIF(ul.logical_request_id, ''), NULLIF(ul.request_id, ''), 'usage:' || ul.id::TEXT) AS request_key,
    CASE WHEN NULLIF(ul.logical_request_id, '') IS NULL THEN 'legacy_request_id' ELSE 'logical_request_id' END AS correlation_quality,
    NULLIF(ul.attempt_id, '') AS attempt_id,
    'usage'::TEXT AS source_kind,
    COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    ul.model,
    ul.duration_ms,
    NULL::INT AS status_code,
    NULL::BIGINT AS error_id,
    NULL::TEXT AS phase,
    NULL::TEXT AS severity,
    NULL::TEXT AS message,
    ul.user_id,
    ul.api_key_id,
    ul.account_id,
    ul.group_id,
    ul.stream,
    (COALESCE(NULLIF(ul.usage_completeness, ''), 'complete') = 'complete' AND ul.actual_cost > 0) AS terminal_success,
    FALSE AS terminal_error,
    COALESCE(NULLIF(ul.usage_completeness, ''), 'complete') AS usage_completeness,
    TRUE AS usage_present,
    COALESCE(ul.unsafe_to_replay, FALSE) AS unsafe_to_replay,
    FALSE AS switch_allowed,
    NULL::TEXT AS switch_reason,
    NULL::TEXT AS final_error_code,
    NULL::TEXT AS terminal_reason,
    NULL::TEXT AS error_owner,
    ul.first_token_ms::BIGINT AS time_to_first_token_ms
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2
), logical_keys AS (
  SELECT DISTINCT ON (request_id)
    request_id, request_key, logical_request_id, correlation_quality
  FROM usage_events
  WHERE NULLIF(request_id, '') IS NOT NULL
  ORDER BY request_id, created_at DESC, source_id DESC
), error_events AS (
  SELECT
    o.id::BIGINT AS source_id,
    o.created_at,
    o.request_id,
    lk.logical_request_id,
    COALESCE(lk.request_key, NULLIF(o.request_id, ''), 'error:' || o.id::TEXT) AS request_key,
    COALESCE(lk.correlation_quality, CASE WHEN NULLIF(o.request_id, '') IS NOT NULL THEN 'legacy_request_id' ELSE 'unknown' END) AS correlation_quality,
    NULL::TEXT AS attempt_id,
    'error'::TEXT AS source_kind,
    COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    o.model,
    o.duration_ms,
    o.status_code,
    o.id AS error_id,
    o.error_phase AS phase,
    o.severity,
    o.error_message AS message,
    o.user_id,
    o.api_key_id,
    o.account_id,
    o.group_id,
    o.stream,
    FALSE AS terminal_success,
    (COALESCE(o.status_code, 0) >= 400 OR o.error_type = 'cyber_policy') AS terminal_error,
    NULL::TEXT AS usage_completeness,
    FALSE AS usage_present,
    FALSE AS unsafe_to_replay,
    FALSE AS switch_allowed,
    NULL::TEXT AS switch_reason,
    COALESCE(NULLIF(o.provider_error_code, ''), NULLIF(o.error_type, '')) AS final_error_code,
    'user_visible_error' AS terminal_reason,
    o.error_owner,
    o.time_to_first_token_ms
  FROM ops_error_logs o
  LEFT JOIN logical_keys lk ON lk.request_id = NULLIF(o.request_id, '')
  LEFT JOIN groups g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= $1 AND o.created_at < $2
    AND COALESCE(o.is_count_tokens, FALSE) = FALSE
), events AS (
  SELECT * FROM usage_events
  UNION ALL
  SELECT * FROM error_events
), dedup_attempts AS (
  SELECT e.*, ROW_NUMBER() OVER (
    PARTITION BY e.request_key, COALESCE(e.attempt_id, NULLIF(e.request_id, ''), e.source_kind || ':' || e.source_id::TEXT)
    ORDER BY e.created_at DESC, e.source_id DESC
  ) AS attempt_position
  FROM events e
), distinct_attempts AS (
  SELECT * FROM dedup_attempts WHERE attempt_position = 1
), request_stats AS (
  SELECT
    request_key,
    COUNT(DISTINCT COALESCE(attempt_id, NULLIF(request_id, ''), source_kind || ':' || source_id::TEXT))::INT AS attempt_count,
    COUNT(DISTINCT COALESCE(attempt_id, NULLIF(request_id, ''), source_kind || ':' || source_id::TEXT)) FILTER (WHERE source_kind = 'error' AND (error_owner = 'provider' OR phase IN ('upstream', 'network')))::INT AS upstream_error_count,
    COUNT(DISTINCT account_id) FILTER (WHERE account_id IS NOT NULL)::INT - 1 AS failover_count,
    BOOL_OR(unsafe_to_replay) AS unsafe_to_replay,
    BOOL_OR(usage_present) AS usage_present,
    CASE
      WHEN BOOL_OR(usage_present AND usage_completeness = 'unknown') THEN 'unknown'
      WHEN BOOL_OR(usage_present AND usage_completeness = 'partial') THEN 'partial'
      WHEN BOOL_OR(usage_present AND usage_completeness = 'complete') THEN 'complete'
      ELSE NULL
    END AS usage_completeness,
    MIN(created_at) AS first_attempt_at
  FROM events
  GROUP BY request_key
), terminal_candidates AS (
  SELECT da.*, ROW_NUMBER() OVER (
    PARTITION BY da.request_key
    ORDER BY (da.terminal_success OR da.terminal_error) DESC, da.created_at DESC, da.terminal_success DESC, da.source_id DESC
  ) AS terminal_position
  FROM distinct_attempts da
), terminals AS (
  SELECT * FROM terminal_candidates WHERE terminal_position = 1
), request_projection AS (
  SELECT
    CASE WHEN t.terminal_success THEN 'success'::TEXT ELSE 'error'::TEXT END AS kind,
    t.created_at,
    COALESCE(NULLIF(t.request_id, ''), t.request_key) AS request_id,
    t.logical_request_id,
    t.correlation_quality,
    rs.attempt_count,
    GREATEST(rs.failover_count, 0) AS failover_count,
    rs.upstream_error_count,
    CASE WHEN t.terminal_success THEN '200' WHEN t.terminal_error THEN COALESCE(t.status_code::TEXT, '500') ELSE 'unknown' END AS final_status,
    CASE WHEN t.stream THEN 'stream' ELSE 'http' END AS final_protocol,
    CASE
      WHEN t.terminal_success AND rs.upstream_error_count > 0 THEN 'auto_retry_recovered'
      WHEN t.terminal_success THEN 'success'
      WHEN rs.unsafe_to_replay OR t.unsafe_to_replay THEN 'stopped_unsafe_to_replay'
      WHEN NOT t.terminal_error THEN 'incomplete_unknown'
      WHEN rs.attempt_count > 1 THEN 'retry_exhausted_user_visible'
      ELSE 'single_attempt_user_visible'
    END AS terminal_kind,
    CASE
      WHEN t.terminal_success AND rs.upstream_error_count > 0 THEN 'upstream_error_recovered'
      WHEN t.terminal_success THEN 'completed'
      WHEN NOT t.terminal_error THEN 'incomplete_unknown'
      WHEN rs.attempt_count > 1 THEN 'retry_exhausted'
      ELSE COALESCE(t.terminal_reason, 'user_visible_error')
    END AS terminal_reason,
    (NOT t.terminal_success AND t.terminal_error) AS user_visible,
    (t.terminal_success AND rs.upstream_error_count > 0) AS auto_retry_recovered,
    (NOT t.terminal_success AND t.terminal_error AND rs.attempt_count > 1) AS retry_exhausted,
    (NOT t.terminal_success AND (rs.unsafe_to_replay OR t.unsafe_to_replay)) AS stopped_unsafe_to_replay,
    (rs.unsafe_to_replay OR t.unsafe_to_replay) AS unsafe_to_replay,
    t.switch_allowed,
    t.switch_reason,
    rs.usage_completeness,
    rs.usage_present,
    rs.first_attempt_at,
    CASE WHEN t.terminal_success OR t.terminal_error THEN t.created_at ELSE NULL END AS completed_at,
    t.time_to_first_token_ms,
    t.final_error_code,
    t.platform,
    t.model,
    t.duration_ms,
    t.status_code,
    t.error_id,
    t.phase,
    t.severity,
    t.message,
    t.user_id,
    t.api_key_id,
    t.account_id,
    t.group_id,
    t.stream
  FROM terminals t
  JOIN request_stats rs ON rs.request_key = t.request_key
)
`

	countQuery := fmt.Sprintf(`%s SELECT COUNT(1) FROM request_projection %s`, cte, where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		if err == sql.ErrNoRows {
			total = 0
		} else {
			return nil, 0, err
		}
	}

	sort := "ORDER BY created_at DESC"
	if filter != nil {
		switch strings.TrimSpace(strings.ToLower(filter.Sort)) {
		case "", "created_at_desc":
			// default
		case "duration_desc":
			sort = "ORDER BY duration_ms DESC NULLS LAST, created_at DESC"
		default:
			return nil, 0, fmt.Errorf("invalid sort")
		}
	}

	listQuery := fmt.Sprintf(`
%s
SELECT
  kind,
  created_at,
  request_id,
  logical_request_id,
  correlation_quality,
  attempt_count,
  failover_count,
  upstream_error_count,
  final_status,
  final_protocol,
  terminal_kind,
  terminal_reason,
  user_visible,
  auto_retry_recovered,
  retry_exhausted,
  stopped_unsafe_to_replay,
  unsafe_to_replay,
  switch_allowed,
  switch_reason,
  usage_completeness,
  usage_present,
  first_attempt_at,
  completed_at,
  time_to_first_token_ms,
  final_error_code,
  platform,
  model,
  duration_ms,
  status_code,
  error_id,
  phase,
  severity,
  message,
  user_id,
  api_key_id,
  account_id,
  group_id,
  stream
FROM request_projection
%s
%s
LIMIT $%d OFFSET $%d
`, cte, where, sort, len(args)+1, len(args)+2)

	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	toIntPtr := func(v sql.NullInt64) *int {
		if !v.Valid {
			return nil
		}
		i := int(v.Int64)
		return &i
	}
	toInt64Ptr := func(v sql.NullInt64) *int64 {
		if !v.Valid {
			return nil
		}
		i := v.Int64
		return &i
	}

	out := make([]*service.OpsRequestDetail, 0, pageSize)
	for rows.Next() {
		var (
			kind                string
			createdAt           time.Time
			requestID           sql.NullString
			logicalRequestID    sql.NullString
			correlationQuality  sql.NullString
			attemptCount        int
			failoverCount       int
			upstreamErrorCount  int
			finalStatus         sql.NullString
			finalProtocol       sql.NullString
			terminalKind        string
			terminalReason      sql.NullString
			userVisible         bool
			autoRetryRecovered  bool
			retryExhausted      bool
			stoppedUnsafeReplay bool
			unsafeToReplay      bool
			switchAllowed       bool
			switchReason        sql.NullString
			usageCompleteness   sql.NullString
			usagePresent        bool
			firstAttemptAt      sql.NullTime
			completedAt         sql.NullTime
			timeToFirstTokenMs  sql.NullInt64
			finalErrorCode      sql.NullString
			platform            sql.NullString
			model               sql.NullString

			durationMs sql.NullInt64
			statusCode sql.NullInt64
			errorID    sql.NullInt64

			phase    sql.NullString
			severity sql.NullString
			message  sql.NullString

			userID    sql.NullInt64
			apiKeyID  sql.NullInt64
			accountID sql.NullInt64
			groupID   sql.NullInt64

			stream bool
		)

		if err := rows.Scan(
			&kind,
			&createdAt,
			&requestID,
			&logicalRequestID,
			&correlationQuality,
			&attemptCount,
			&failoverCount,
			&upstreamErrorCount,
			&finalStatus,
			&finalProtocol,
			&terminalKind,
			&terminalReason,
			&userVisible,
			&autoRetryRecovered,
			&retryExhausted,
			&stoppedUnsafeReplay,
			&unsafeToReplay,
			&switchAllowed,
			&switchReason,
			&usageCompleteness,
			&usagePresent,
			&firstAttemptAt,
			&completedAt,
			&timeToFirstTokenMs,
			&finalErrorCode,
			&platform,
			&model,
			&durationMs,
			&statusCode,
			&errorID,
			&phase,
			&severity,
			&message,
			&userID,
			&apiKeyID,
			&accountID,
			&groupID,
			&stream,
		); err != nil {
			return nil, 0, err
		}

		item := &service.OpsRequestDetail{
			Kind:                  service.OpsRequestKind(kind),
			CreatedAt:             createdAt,
			RequestID:             strings.TrimSpace(requestID.String),
			LogicalRequestID:      strings.TrimSpace(logicalRequestID.String),
			CorrelationQuality:    strings.TrimSpace(correlationQuality.String),
			AttemptCount:          attemptCount,
			FailoverCount:         failoverCount,
			UpstreamErrorCount:    upstreamErrorCount,
			FinalStatus:           strings.TrimSpace(finalStatus.String),
			FinalProtocol:         strings.TrimSpace(finalProtocol.String),
			TerminalKind:          terminalKind,
			TerminalReason:        strings.TrimSpace(terminalReason.String),
			UserVisible:           userVisible,
			AutoRetryRecovered:    autoRetryRecovered,
			RetryExhausted:        retryExhausted,
			StoppedUnsafeToReplay: stoppedUnsafeReplay,
			UnsafeToReplay:        unsafeToReplay,
			SwitchAllowed:         switchAllowed,
			SwitchReason:          strings.TrimSpace(switchReason.String),
			UsageCompleteness:     strings.TrimSpace(usageCompleteness.String),
			UsagePresent:          usagePresent,
			FinalErrorCode:        strings.TrimSpace(finalErrorCode.String),
			Platform:              strings.TrimSpace(platform.String),
			Model:                 strings.TrimSpace(model.String),

			DurationMs: toIntPtr(durationMs),
			StatusCode: toIntPtr(statusCode),
			ErrorID:    toInt64Ptr(errorID),
			Phase:      phase.String,
			Severity:   severity.String,
			Message:    message.String,

			UserID:    toInt64Ptr(userID),
			APIKeyID:  toInt64Ptr(apiKeyID),
			AccountID: toInt64Ptr(accountID),
			GroupID:   toInt64Ptr(groupID),

			Stream: stream,
		}
		if firstAttemptAt.Valid {
			value := firstAttemptAt.Time
			item.FirstAttemptAt = &value
		}
		if completedAt.Valid {
			value := completedAt.Time
			item.CompletedAt = &value
		}
		if timeToFirstTokenMs.Valid {
			value := int(timeToFirstTokenMs.Int64)
			item.TimeToFirstTokenMs = &value
		}

		if item.Platform == "" {
			item.Platform = "unknown"
		}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}
