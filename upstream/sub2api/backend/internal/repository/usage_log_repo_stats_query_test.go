package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetStatsWithFiltersUsesSingleScopedAggregate(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		checks := []struct {
			ok      bool
			message string
		}{
			{!strings.Contains(actual, "%!"), "query contains a missing fmt argument marker"},
			{strings.Count(actual, "FROM usage_logs") == 1, "query must read usage_logs exactly once"},
			{strings.Contains(actual, "FROM scoped"), "aggregate must read the scoped CTE"},
			{strings.Contains(actual, "GROUP BY GROUPING SETS"), "aggregate must return totals and endpoint dimensions"},
			{strings.Contains(actual, usageLogNormalQueryFilter), "query must preserve the normal usage filter"},
			{strings.Contains(actual, effectiveAccountCostSQL("")), "query must preserve the effective account cost expression"},
		}
		for _, check := range checks {
			if !check.ok {
				return fmt.Errorf("%s: %s", check.message, actual)
			}
		}
		return nil
	})

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	columns := []string{
		"inbound_grouped", "upstream_grouped", "inbound_endpoint", "upstream_endpoint",
		"requests", "input_tokens", "output_tokens", "cache_tokens", "cache_creation_tokens", "cache_read_tokens",
		"cost", "actual_cost", "account_cost", "average_duration_ms",
	}
	mock.ExpectQuery("stats").
		WithArgs(int64(23), start, end).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(1, 1, nil, nil, 45, 100, 200, 50, 30, 20, 8.586858, 1.030423, 0.75, 125.0).
			AddRow(0, 1, "/v1/responses", nil, 45, 100, 200, 50, 30, 20, 8.586858, 1.030423, 0.75, 125.0).
			AddRow(1, 0, nil, "/v1/responses", 45, 100, 200, 50, 30, 20, 8.586858, 1.030423, 0.75, 125.0).
			AddRow(0, 0, "/v1/responses", "/v1/responses", 45, 100, 200, 50, 30, 20, 8.586858, 1.030423, 0.75, 125.0))

	stats, err := newUsageLogRepositoryWithSQL(nil, db).GetStatsWithFilters(context.Background(), UsageLogFilters{
		UserID: 23, StartTime: &start, EndTime: &end,
	})
	require.NoError(t, err)
	require.Equal(t, int64(45), stats.TotalRequests)
	require.Equal(t, int64(50), stats.TotalCacheTokens)
	require.Equal(t, int64(350), stats.TotalTokens)
	require.InDelta(t, 1.030423, stats.TotalActualCost, 0.0000001)
	require.Len(t, stats.Endpoints, 1)
	require.Equal(t, "/v1/responses", stats.Endpoints[0].Endpoint)
	require.Len(t, stats.UpstreamEndpoints, 1)
	require.Len(t, stats.EndpointPaths, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
