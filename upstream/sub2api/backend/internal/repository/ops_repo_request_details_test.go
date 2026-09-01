package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetailsProjectsOneRecoveredLogicalRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewOpsRepository(db)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	completed := end.Add(-time.Minute)

	query := `(?s)WITH usage_events AS.*logical_keys AS.*terminal_kind.*auto_retry_recovered.*`
	columns := []string{
		"kind", "created_at", "request_id", "logical_request_id", "correlation_quality", "attempt_count", "failover_count", "upstream_error_count",
		"final_status", "final_protocol", "terminal_kind", "terminal_reason", "user_visible", "auto_retry_recovered", "retry_exhausted",
		"stopped_unsafe_to_replay", "unsafe_to_replay", "switch_allowed", "switch_reason", "usage_completeness", "usage_present",
		"first_attempt_at", "completed_at", "time_to_first_token_ms", "final_error_code", "platform", "model", "duration_ms", "status_code",
		"error_id", "phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
	}
	addRow := func(rows *sqlmock.Rows) *sqlmock.Rows {
		return rows.AddRow(
			"success", completed, "attempt-b", "logical-1", "logical_request_id", 2, 1, 1,
			"200", "responses", "auto_retry_recovered", "upstream_error_recovered", false, true, false,
			false, false, true, "upstream_502", "complete", true, start.Add(10*time.Minute), completed, 450, "", "openai", "gpt-5.5", 900, 200,
			nil, "", "", "", int64(7), int64(8), int64(12), int64(3), true,
		)
	}
	mock.ExpectQuery(query+`SELECT COUNT\(1\) FROM request_projection`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(query+`LIMIT \$[0-9]+ OFFSET \$[0-9]+`).
		WithArgs(start, end, 50, 0).
		WillReturnRows(addRow(sqlmock.NewRows(columns)))

	got, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start, EndTime: &end, Page: 1, PageSize: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	require.Equal(t, service.OpsRequestKindSuccess, got[0].Kind)
	require.Equal(t, "auto_retry_recovered", got[0].TerminalKind)
	require.True(t, got[0].AutoRetryRecovered)
	require.Equal(t, 2, got[0].AttemptCount)
	require.Equal(t, int64(12), *got[0].AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetailsKeepsFinalFailureForPartialUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewOpsRepository(db)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	failedAt := end.Add(-time.Minute)

	query := regexp.QuoteMeta("WITH usage_events AS") + `.*` + regexp.QuoteMeta("logical_keys AS")
	columns := []string{
		"kind", "created_at", "request_id", "logical_request_id", "correlation_quality", "attempt_count", "failover_count", "upstream_error_count",
		"final_status", "final_protocol", "terminal_kind", "terminal_reason", "user_visible", "auto_retry_recovered", "retry_exhausted",
		"stopped_unsafe_to_replay", "unsafe_to_replay", "switch_allowed", "switch_reason", "usage_completeness", "usage_present",
		"first_attempt_at", "completed_at", "time_to_first_token_ms", "final_error_code", "platform", "model", "duration_ms", "status_code",
		"error_id", "phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
	}
	addRow := func(rows *sqlmock.Rows) *sqlmock.Rows {
		return rows.AddRow(
			"error", failedAt, "request-c", "logical-2", "logical_request_id", 2, 1, 2,
			"503", "chat", "retry_exhausted_user_visible", "retry_exhausted", true, false, true,
			false, false, false, "retry_budget_exhausted", "partial", true, start.Add(20*time.Minute), failedAt, nil, "upstream_unavailable", "openai", "gpt-5.5", 1100, 503,
			int64(99), "upstream", "P2", "temporarily unavailable", int64(7), int64(8), int64(13), int64(3), true,
		)
	}
	mock.ExpectQuery(`(?s)`+query+`.*SELECT COUNT\(1\) FROM request_projection`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)`+query+`.*LIMIT \$[0-9]+ OFFSET \$[0-9]+`).
		WithArgs(start, end, 50, 0).
		WillReturnRows(addRow(sqlmock.NewRows(columns)))

	got, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start, EndTime: &end, Page: 1, PageSize: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	require.Equal(t, service.OpsRequestKindError, got[0].Kind)
	require.Equal(t, "retry_exhausted_user_visible", got[0].TerminalKind)
	require.True(t, got[0].UserVisible)
	require.False(t, got[0].AutoRetryRecovered)
	require.NoError(t, mock.ExpectationsWereMet())
}
