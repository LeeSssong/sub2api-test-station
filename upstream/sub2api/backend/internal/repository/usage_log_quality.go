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
// attempt is deduplicated by persisted request identity and assigned to one
// mutually exclusive scheduling window in the same seven-day scan.
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
		u.created_at,
        u.first_token_ms::double precision AS first_token_ms,
		u.duration_ms::double precision AS duration_ms,
		u.output_tokens::double precision AS output_tokens
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
		o.created_at,
        NULL::double precision AS first_token_ms,
		NULL::double precision AS duration_ms,
		NULL::double precision AS output_tokens
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
    SELECT
		a.*,
		CASE
			WHEN a.created_at >= $2 - interval '1 hour' THEN 'w1'
			WHEN a.created_at >= $2 - interval '24 hours' THEN 'w24'
			ELSE 'w7'
		END AS quality_window
	FROM all_attempts a
),
successful AS (
    SELECT p.*
    FROM physical_attempts p
    WHERE p.usage_completeness = 'complete'
      AND p.actual_cost > 0
),
window_aggregates AS (
    SELECT
        p.account_id,
		p.quality_window,
        count(*)::bigint AS attempt_count,
		count(*) FILTER (WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0)::bigint AS success_count,
		count(p.first_token_ms) FILTER (WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0)::bigint AS ttft_sample_count,
		PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY p.first_token_ms)
			FILTER (WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0 AND p.first_token_ms IS NOT NULL) AS ttft_p50_ms,
		PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY p.first_token_ms)
			FILTER (WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0 AND p.first_token_ms IS NOT NULL) AS ttft_p90_ms,
		count(*) FILTER (
			WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0
			  AND p.output_tokens > 0 AND p.first_token_ms IS NOT NULL
			  AND p.duration_ms > p.first_token_ms
		)::bigint AS output_rate_sample_count,
		PERCENTILE_CONT(0.50) WITHIN GROUP (
			ORDER BY p.output_tokens / ((p.duration_ms - p.first_token_ms) / 1000.0)
		) FILTER (
			WHERE p.usage_completeness = 'complete' AND p.actual_cost > 0
			  AND p.output_tokens > 0 AND p.first_token_ms IS NOT NULL
			  AND p.duration_ms > p.first_token_ms
		) AS output_rate_tokens_per_second
    FROM physical_attempts p
	GROUP BY p.account_id, p.quality_window
)
SELECT
	w.account_id,
	w.quality_window,
	w.attempt_count,
	w.success_count,
	CASE WHEN w.attempt_count > 0
		THEN w.success_count::double precision / w.attempt_count
        ELSE NULL
    END AS success_rate,
	w.ttft_sample_count,
	w.ttft_p50_ms,
	w.ttft_p90_ms,
	w.output_rate_sample_count,
	w.output_rate_tokens_per_second
FROM window_aggregates w
ORDER BY w.account_id, w.quality_window
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
	byAccount := make(map[int64]int)
	for rows.Next() {
		var (
			accountID                                 int64
			windowName                                string
			metrics                                   service.OpenAIQualityWindowMetrics
			successRate, ttftP50, ttftP90, outputRate sql.NullFloat64
		)
		if err := rows.Scan(
			&accountID,
			&windowName,
			&metrics.AttemptCount,
			&metrics.SuccessCount,
			&successRate,
			&metrics.TTFTSampleCount,
			&ttftP50,
			&ttftP90,
			&metrics.OutputRateSampleCount,
			&outputRate,
		); err != nil {
			return nil, err
		}
		if successRate.Valid {
			metrics.SuccessRate = &successRate.Float64
		}
		if ttftP50.Valid {
			metrics.TTFTP50MS = &ttftP50.Float64
		}
		if ttftP90.Valid {
			metrics.TTFTP90MS = &ttftP90.Float64
		}
		if outputRate.Valid {
			metrics.OutputRateTokensPerSecond = &outputRate.Float64
		}
		index, ok := byAccount[accountID]
		if !ok {
			index = len(result)
			byAccount[accountID] = index
			result = append(result, service.OpenAIAccountQuality{AccountID: accountID, Windows: make(map[service.OpenAIQualityWindow]service.OpenAIQualityWindowMetrics)})
		}
		result[index].Windows[service.OpenAIQualityWindow(windowName)] = metrics
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}
