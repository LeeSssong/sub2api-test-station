package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func probeCostLog() service.AccountProbeCostLog {
	cost := decimal.RequireFromString("1234567890.1234567890")
	groupID := int64(9)
	return service.AccountProbeCostLog{
		ProbeRunID: "probe-1", AccountID: 17, GroupID: &groupID,
		ProbeKind: service.ProbeKindMonitor, Model: "gpt-5",
		InputTokens: 10, OutputTokens: 20, CacheCreationTokens: 3, CacheReadTokens: 4,
		AccountCost: &cost, UsageCompleteness: service.ProbeUsageComplete,
		ProbeOutcome: service.ProbeOutcomeSuccess, CreatedAt: time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC),
	}
}

func TestAccountProbeCostRepositoryAppendCompleteRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	log := probeCostLog()
	mock.ExpectQuery("INSERT INTO account_probe_cost_logs").WillReturnRows(sqlmock.NewRows([]string{"probe_run_id"}).AddRow(log.ProbeRunID))

	err = NewAccountProbeCostRepository(db).Append(context.Background(), log)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryAppendPartialNullCost(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	log := probeCostLog()
	log.ProbeRunID = "probe-partial"
	log.AccountCost = nil
	log.UsageCompleteness = service.ProbeUsagePartial
	mock.ExpectQuery("INSERT INTO account_probe_cost_logs").WillReturnRows(sqlmock.NewRows([]string{"probe_run_id"}).AddRow(log.ProbeRunID))

	err = NewAccountProbeCostRepository(db).Append(context.Background(), log)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryAppendDuplicateIdenticalIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	log := probeCostLog()
	log.CreatedAt = log.CreatedAt.Add(789 * time.Nanosecond)
	mock.ExpectQuery("INSERT INTO account_probe_cost_logs").WillReturnRows(sqlmock.NewRows([]string{"probe_run_id"}))
	mock.ExpectQuery("SELECT probe_run_id").WithArgs(log.ProbeRunID).WillReturnRows(sqlmock.NewRows([]string{
		"probe_run_id", "account_id", "group_id", "probe_kind", "model", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "account_cost", "usage_completeness", "probe_outcome", "error_code", "created_at",
	}).AddRow(log.ProbeRunID, log.AccountID, *log.GroupID, string(log.ProbeKind), log.Model, log.InputTokens, log.OutputTokens, log.CacheCreationTokens, log.CacheReadTokens, log.AccountCost.String(), string(log.UsageCompleteness), string(log.ProbeOutcome), nil, log.CreatedAt.UTC().Truncate(time.Microsecond)))

	require.NoError(t, NewAccountProbeCostRepository(db).Append(context.Background(), log))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryAppendDuplicateConflictIsTyped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	log := probeCostLog()
	mock.ExpectQuery("INSERT INTO account_probe_cost_logs").WillReturnRows(sqlmock.NewRows([]string{"probe_run_id"}))
	mock.ExpectQuery("SELECT probe_run_id").WithArgs(log.ProbeRunID).WillReturnRows(sqlmock.NewRows([]string{
		"probe_run_id", "account_id", "group_id", "probe_kind", "model", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "account_cost", "usage_completeness", "probe_outcome", "error_code", "created_at",
	}).AddRow(log.ProbeRunID, 99, *log.GroupID, string(log.ProbeKind), log.Model, log.InputTokens, log.OutputTokens, log.CacheCreationTokens, log.CacheReadTokens, log.AccountCost.String(), string(log.UsageCompleteness), string(log.ProbeOutcome), nil, log.CreatedAt))

	err = NewAccountProbeCostRepository(db).Append(context.Background(), log)
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrAccountProbeCostConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryReadWindowAggregatesHalfOpenWindowAndNullableCost(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	mock.ExpectQuery("(?s)SELECT\\s+group_id,\\s+account_id.*created_at\\s+>=\\s+\\$1\\s+AND\\s+created_at\\s+<\\s+\\$2").WithArgs(from, to).WillReturnRows(sqlmock.NewRows([]string{
		"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost",
	}).AddRow(int64(9), int64(17), int64(2), int64(37), nil, true))

	rows, err := NewAccountProbeCostRepository(db).ReadWindow(context.Background(), from, to)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].ProbeRequests)
	require.Equal(t, int64(37), rows[0].ProbeTokens)
	require.Nil(t, rows[0].ProbeCost)
	require.True(t, rows[0].HasIncompleteCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryReadWindowPreservesTenDecimalPlaces(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	mock.ExpectQuery("(?s)SELECT\\s+group_id,\\s+account_id.*created_at\\s+>=\\s+\\$1\\s+AND\\s+created_at\\s+<\\s+\\$2").WithArgs(from, to).WillReturnRows(sqlmock.NewRows([]string{
		"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost",
	}).AddRow(int64(9), int64(17), int64(2), int64(37), "1234567890.1234567890", false))

	rows, err := NewAccountProbeCostRepository(db).ReadWindow(context.Background(), from, to)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].ProbeCost)
	require.Equal(t, "1234567890.1234567890", rows[0].ProbeCost.StringFixed(10))
	require.False(t, rows[0].HasIncompleteCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryReadWindowNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	from, to := time.Now().Add(-time.Hour), time.Now()
	mock.ExpectQuery("(?s)SELECT\\s+group_id,\\s+account_id.*created_at\\s+>=\\s+\\$1\\s+AND\\s+created_at\\s+<\\s+\\$2").WithArgs(from, to).WillReturnRows(sqlmock.NewRows([]string{
		"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost",
	}))
	rows, err := NewAccountProbeCostRepository(db).ReadWindow(context.Background(), from, to)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProbeCostRepositoryReadWindowPropagatesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	queryErr := errors.New("database unavailable")
	from, to := time.Now().Add(-time.Hour), time.Now()
	mock.ExpectQuery("(?s)SELECT\\s+group_id,\\s+account_id.*created_at\\s+>=\\s+\\$1\\s+AND\\s+created_at\\s+<\\s+\\$2").WithArgs(from, to).WillReturnError(queryErr)
	_, err = NewAccountProbeCostRepository(db).ReadWindow(context.Background(), from, to)
	require.ErrorIs(t, err, queryErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
