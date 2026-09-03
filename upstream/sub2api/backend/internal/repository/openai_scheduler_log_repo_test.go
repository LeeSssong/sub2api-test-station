package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAISchedulerLogRepositoryBatchInsertAndCleanup(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectPrepare("INSERT INTO openai_scheduler_logs").ExpectExec().WithArgs(
		sqlmock.AnyArg(), "openai", nil, "req-1", "attempt-1", 1, service.OpenAIEventSchedulerSelection,
		int64(7), nil, "selected", nil, "unified_quality", service.OpenAISchedulerAlgorithmVersion, sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := NewOpenAISchedulerLogRepository(db)
	inserted, err := repo.BatchInsertOpenAISchedulerLogs(context.Background(), []service.OpenAISchedulerLogInsert{{
		EventAt: time.Now(), Platform: "openai", LogicalRequestID: "req-1", AttemptID: "attempt-1", AttemptNumber: 1,
		EventName: service.OpenAIEventSchedulerSelection, AccountID: 7, Outcome: "selected", SelectionLayer: "unified_quality",
		AlgorithmVersion: service.OpenAISchedulerAlgorithmVersion, DecisionJSON: "{}",
	}})
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	mock.ExpectExec("WITH doomed").WithArgs(sqlmock.AnyArg(), 1000).WillReturnResult(sqlmock.NewResult(0, 2))
	deleted, err := repo.DeleteOpenAISchedulerLogsBefore(context.Background(), time.Now(), 0)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDecodeOpenAISchedulerLogCursorRejectsMalformedValue(t *testing.T) {
	_, err := DecodeOpenAISchedulerLogCursor("bad")
	require.Error(t, err)
}

func TestOpenAISchedulerLogRepositoryPaginatesLogicalRequestsAndReturnsCompleteEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	from := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	latest := from.Add(2 * time.Minute)
	mock.ExpectQuery("WITH matching_requests AS").WithArgs(from, to, 2).WillReturnRows(
		sqlmock.NewRows([]string{"logical_request_id", "cursor_event_at", "cursor_id"}).
			AddRow("req-1", latest, int64(12)).
			AddRow("req-2", from.Add(time.Minute), int64(9)),
	)
	mock.ExpectQuery("WHERE logical_request_id IN").WithArgs("req-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "event_at", "platform", "group_id", "logical_request_id", "attempt_id", "attempt_number", "event_name", "account_id", "canonical_model", "outcome", "final_outcome", "selection_layer", "algorithm_version", "decision"}).
			AddRow(int64(11), from, "openai", int64(2), "req-1", "attempt-1", 1, service.OpenAIEventSchedulerSelection, int64(131), "gpt-5.6", "selected", nil, "unified_quality", service.OpenAISchedulerAlgorithmVersion, []byte(`{"runtime_retry_budget":4}`)).
			AddRow(int64(12), latest, "openai", int64(2), "req-1", "attempt-1", 1, service.OpenAIEventSchedulerRequestOutcome, int64(131), "gpt-5.6", "success", "success", nil, service.OpenAISchedulerAlgorithmVersion, []byte(`{"switch_count":0}`)),
	)

	repo := NewOpenAISchedulerLogRepository(db)
	result, err := repo.ListOpenAISchedulerLogs(context.Background(), &service.OpenAISchedulerLogListFilter{From: from, To: to, Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Logs, 2)
	require.Equal(t, "req-1", result.Logs[0].LogicalRequestID)
	require.NotEmpty(t, result.NextCursor)
	require.NoError(t, mock.ExpectationsWereMet())
}
