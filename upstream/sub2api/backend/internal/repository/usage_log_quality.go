package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// openAIAccountQualityQuery combines successful native usage rows with
// account-owned failures from the native operations error log. A physical
// attempt is deduplicated by persisted request identity, while successful
// latency samples are trimmed independently at both ends by floor(n*5%).
const openAIAccountQualityQuery = `
WITH usage_attempts AS (
    SELECT DISTINCT ON (
        u.account_id,
        u.api_key_id,
        COALESCE(NULLIF(u.attempt_id, ''), NULLIF(u.request_id, ''), 'usage:' || u.id::text)
    )
        u.account_id,
        u.logical_request_id,
        u.attempt_id,
        u.request_id,
        COALESCE(NULLIF(u.usage_completeness, ''), 'complete') AS usage_completeness,
        u.actual_cost,
        u.first_token_ms::double precision AS first_token_ms,
        u.duration_ms::double precision AS duration_ms
    FROM usage_logs u
    WHERE u.created_at >= $1
      AND u.created_at < $2
      AND u.account_id IS NOT NULL
      AND COALESCE(NULLIF(u.usage_completeness, ''), 'complete') <> 'unknown'
      AND COALESCE(NULLIF(u.billing_mode, ''), 'token') NOT IN ('image', 'video')
    ORDER BY
        u.account_id,
        u.api_key_id,
        COALESCE(NULLIF(u.attempt_id, ''), NULLIF(u.request_id, ''), 'usage:' || u.id::text),
        u.id DESC
),
error_attempts AS (
    SELECT DISTINCT ON (
        o.account_id,
        COALESCE(NULLIF(o.request_id, ''), NULLIF(o.client_request_id, ''), 'error:' || o.id::text)
    )
        o.account_id,
        NULL::text AS logical_request_id,
        NULL::text AS attempt_id,
        o.request_id,
        'failed'::text AS usage_completeness,
        0::numeric AS actual_cost,
        NULL::double precision AS first_token_ms,
        NULL::double precision AS duration_ms
    FROM ops_error_logs o
    WHERE o.created_at >= $1
      AND o.created_at < $2
      AND o.account_id IS NOT NULL
      AND COALESCE(o.is_business_limited, FALSE) = FALSE
      AND COALESCE(o.is_count_tokens, FALSE) = FALSE
      AND COALESCE(o.status_code, 0) >= 400
      AND COALESCE(o.error_owner, '') <> 'client'
      AND NOT (COALESCE(o.error_phase, '') = 'request' AND COALESCE(o.error_source, '') = 'client_request')
      AND lower(CONCAT_WS(' ', o.error_type, o.error_message, o.error_body, o.upstream_error_message, o.upstream_error_detail)) NOT LIKE ALL (ARRAY[
        '%model not supported%', '%unsupported model%', '%not supported by any configured account%',
        '%does not support the requested model%', '%model_not_supported%', '%model_unsupported%',
        '%model_not_found%', '%本站暂不支持%'
      ])
      AND NOT EXISTS (
        SELECT 1
        FROM usage_logs u
        WHERE u.account_id = o.account_id
          AND u.created_at >= $1
          AND u.created_at < $2
          AND NULLIF(u.request_id, '') = NULLIF(o.request_id, '')
          AND COALESCE(NULLIF(u.usage_completeness, ''), 'complete') <> 'unknown'
      )
    ORDER BY
        o.account_id,
        COALESCE(NULLIF(o.request_id, ''), NULLIF(o.client_request_id, ''), 'error:' || o.id::text),
        o.id DESC
),
all_attempts AS (
    SELECT * FROM usage_attempts
    UNION ALL
    SELECT * FROM error_attempts
),
physical_attempts AS (
    SELECT * FROM all_attempts
),
successful AS (
    SELECT p.*
    FROM physical_attempts p
    WHERE p.usage_completeness = 'complete'
      AND p.actual_cost > 0
),
ttft_samples AS (
    SELECT
        s.account_id,
        s.first_token_ms,
        row_number() OVER (PARTITION BY s.account_id ORDER BY s.first_token_ms) AS ttft_rn,
        count(*) OVER (PARTITION BY s.account_id) AS ttft_n
    FROM successful s
    WHERE s.first_token_ms IS NOT NULL
),
latency_samples AS (
    SELECT
        s.account_id,
        s.duration_ms,
        row_number() OVER (PARTITION BY s.account_id ORDER BY s.duration_ms) AS latency_rn,
        count(*) OVER (PARTITION BY s.account_id) AS latency_n
    FROM successful s
    WHERE s.duration_ms IS NOT NULL
),
attempt_aggregates AS (
    SELECT
        p.account_id,
        count(*)::bigint AS attempt_count,
        count(*) FILTER (WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0)::bigint AS success_count
    FROM physical_attempts p
    GROUP BY p.account_id
),
ttft_aggregates AS (
    SELECT
        t.account_id,
        count(*)::bigint AS ttft_sample_count,
        avg(t.first_token_ms) FILTER (
            WHERE t.ttft_rn > floor(t.ttft_n * 0.05)::bigint
              AND t.ttft_rn <= t.ttft_n - floor(t.ttft_n * 0.05)::bigint
        ) AS ttft_trimmed_mean_ms
    FROM ttft_samples t
    GROUP BY t.account_id
),
latency_aggregates AS (
    SELECT
        l.account_id,
        count(*)::bigint AS latency_sample_count,
        avg(l.duration_ms) FILTER (
            WHERE l.latency_rn > floor(l.latency_n * 0.05)::bigint
              AND l.latency_rn <= l.latency_n - floor(l.latency_n * 0.05)::bigint
        ) AS latency_trimmed_mean_ms
    FROM latency_samples l
    GROUP BY l.account_id
)
SELECT
    a.account_id,
    a.attempt_count,
    a.success_count,
    CASE WHEN a.attempt_count > 0
        THEN a.success_count::double precision / a.attempt_count
        ELSE NULL
    END AS success_rate,
    COALESCE(t.ttft_sample_count, 0)::bigint AS ttft_sample_count,
    t.ttft_trimmed_mean_ms,
    COALESCE(l.latency_sample_count, 0)::bigint AS latency_sample_count,
    l.latency_trimmed_mean_ms
FROM attempt_aggregates a
LEFT JOIN ttft_aggregates t ON t.account_id = a.account_id
LEFT JOIN latency_aggregates l ON l.account_id = a.account_id
ORDER BY a.account_id
`

var _ service.OpenAIAccountQualityRepository = (*usageLogRepository)(nil)

func (r *usageLogRepository) ListOpenAIAccountQuality(ctx context.Context, start, end time.Time) ([]service.OpenAIAccountQuality, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("OpenAI account quality requires SQL executor")
	}
	if !end.After(start) {
		return nil, fmt.Errorf("OpenAI account quality window end must be after start")
	}
	rows, err := r.sql.QueryContext(ctx, openAIAccountQualityQuery, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]service.OpenAIAccountQuality, 0)
	for rows.Next() {
		var (
			quality              service.OpenAIAccountQuality
			successRate          sql.NullFloat64
			ttftTrimmedMeanMS    sql.NullFloat64
			latencyTrimmedMeanMS sql.NullFloat64
		)
		if err := rows.Scan(
			&quality.AccountID,
			&quality.AttemptCount,
			&quality.SuccessCount,
			&successRate,
			&quality.TTFTSampleCount,
			&ttftTrimmedMeanMS,
			&quality.LatencySampleCount,
			&latencyTrimmedMeanMS,
		); err != nil {
			return nil, err
		}
		if successRate.Valid {
			quality.SuccessRate = &successRate.Float64
		}
		if ttftTrimmedMeanMS.Valid {
			quality.TTFTTrimmedMeanMS = &ttftTrimmedMeanMS.Float64
		}
		if latencyTrimmedMeanMS.Valid {
			quality.LatencyTrimmedMeanMS = &latencyTrimmedMeanMS.Float64
		}
		result = append(result, quality)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}
