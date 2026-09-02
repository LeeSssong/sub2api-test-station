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
