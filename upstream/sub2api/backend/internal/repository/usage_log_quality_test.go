package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryListOpenAIAccountQualityCombinesUsageAndAccountOwnedErrors(t *testing.T) {
	var captured string
	matcher := sqlmock.QueryMatcherFunc(func(_, actual string) error {
		captured = actual
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	from := time.Date(2026, 8, 24, 0, 30, 0, 0, time.UTC)
	to := from.Add(7 * 24 * time.Hour)
	mock.ExpectQuery("quality").
		WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "quality_window", "attempt_count", "success_count", "success_rate",
			"ttft_sample_count", "ttft_p50_ms", "ttft_p90_ms", "output_rate_sample_count", "output_rate_tokens_per_second",
		}).
			AddRow(int64(7), "w1", int64(2), int64(1), 0.5, int64(1), 1200.0, 1800.0, int64(1), 25.0).
			AddRow(int64(7), "w24", int64(10), int64(9), 0.9, int64(8), 800.0, 1400.0, int64(7), 40.0).
			AddRow(int64(8), "w7", int64(1), int64(0), 0.0, int64(0), nil, nil, int64(0), nil))

	got, err := newUsageLogRepositoryWithSQL(nil, db).ListOpenAIAccountQuality(context.Background(), from, to)
	require.NoError(t, err)
	require.Equal(t, []service.OpenAIAccountQuality{
		{AccountID: 7, Windows: map[service.OpenAIQualityWindow]service.OpenAIQualityWindowMetrics{
			service.OpenAIQualityWindow1H:  {AttemptCount: 2, SuccessCount: 1, SuccessRate: qualityFloatPtr(0.5), TTFTSampleCount: 1, TTFTP50MS: qualityFloatPtr(1200), TTFTP90MS: qualityFloatPtr(1800), OutputRateSampleCount: 1, OutputRateTokensPerSecond: qualityFloatPtr(25)},
			service.OpenAIQualityWindow24H: {AttemptCount: 10, SuccessCount: 9, SuccessRate: qualityFloatPtr(0.9), TTFTSampleCount: 8, TTFTP50MS: qualityFloatPtr(800), TTFTP90MS: qualityFloatPtr(1400), OutputRateSampleCount: 7, OutputRateTokensPerSecond: qualityFloatPtr(40)},
		}},
		{AccountID: 8, Windows: map[service.OpenAIQualityWindow]service.OpenAIQualityWindowMetrics{
			service.OpenAIQualityWindow7D: {AttemptCount: 1, SuccessCount: 0, SuccessRate: qualityFloatPtr(0)},
		}},
	}, got)
	require.Contains(t, strings.ToLower(captured), "from usage_logs")
	require.Contains(t, strings.ToLower(captured), "from ops_error_logs")
	require.Contains(t, strings.ToLower(captured), "is_business_limited")
	require.Contains(t, strings.ToLower(captured), "error_owner")
	require.Contains(t, strings.ToLower(captured), "client_request")
	require.Contains(t, strings.ToLower(captured), "not exists")
	require.NotContains(t, strings.ToLower(captured), "account_probe_cost_logs")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryListOpenAIAccountQualityRejectsInvalidWindow(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	from := time.Date(2026, 8, 24, 0, 30, 0, 0, time.UTC)
	_, err = newUsageLogRepositoryWithSQL(nil, db).ListOpenAIAccountQuality(context.Background(), from, from)
	require.Error(t, err)
}

func TestOpenAIAccountQualityQueryContainsExclusiveWindowAndPercentileContract(t *testing.T) {
	query := strings.ToLower(openAIAccountQualityQuery)
	for _, fragment := range []string{
		"physical_attempts as",
		"error_attempts as",
		"all_attempts as",
		"distinct on",
		"u.account_id,\n        u.api_key_id",
		"attempt_id",
		"logical_request_id",
		"usage_completeness",
		"actual_cost > 0",
		"interval '1 hour'",
		"interval '24 hours'",
		"'w1'",
		"'w24'",
		"'w7'",
		"percentile_cont(0.50)",
		"percentile_cont(0.90)",
		"output_tokens",
		"p.duration_ms > p.first_token_ms",
		"created_at >= $1",
		"created_at < $2",
		"from ops_error_logs",
		"coalesce(o.is_business_limited, false) = false",
	} {
		require.Contains(t, query, fragment)
	}
}

func qualityFloatPtr(value float64) *float64 { return &value }
