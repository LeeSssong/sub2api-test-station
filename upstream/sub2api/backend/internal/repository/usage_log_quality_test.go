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
			"account_id", "attempt_count", "success_count", "success_rate",
			"ttft_sample_count", "ttft_trimmed_mean_ms", "latency_sample_count", "latency_trimmed_mean_ms",
		}).
			AddRow(int64(7), int64(2), int64(1), 0.5, int64(1), 1200.0, int64(1), 2400.0).
			AddRow(int64(8), int64(1), int64(0), 0.0, int64(0), nil, int64(0), nil))

	got, err := newUsageLogRepositoryWithSQL(nil, db).ListOpenAIAccountQuality(context.Background(), from, to)
	require.NoError(t, err)
	require.Equal(t, []service.OpenAIAccountQuality{
		{AccountID: 7, AttemptCount: 2, SuccessCount: 1, SuccessRate: qualityFloatPtr(0.5), TTFTSampleCount: 1, TTFTTrimmedMeanMS: qualityFloatPtr(1200), LatencySampleCount: 1, LatencyTrimmedMeanMS: qualityFloatPtr(2400)},
		{AccountID: 8, AttemptCount: 1, SuccessCount: 0, SuccessRate: qualityFloatPtr(0), TTFTSampleCount: 0, LatencySampleCount: 0},
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

func TestOpenAIAccountQualityQueryContainsPhysicalAttemptAndTrimmedMeanContract(t *testing.T) {
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
		"floor(t.ttft_n * 0.05)",
		"floor(l.latency_n * 0.05)",
		"created_at >= $1",
		"created_at < $2",
		"from ops_error_logs",
		"coalesce(o.is_business_limited, false) = false",
	} {
		require.Contains(t, query, fragment)
	}
}

func qualityFloatPtr(value float64) *float64 { return &value }
