package events

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/integration"
	"github.com/stretchr/testify/require"
)

func TestOutboxAppendValidatesAndUsesCallerTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO externalization_outbox").WithArgs(
		"550e8400-e29b-41d4-a716-446655440000", integration.EventTypeAccountHealthChanged,
		sqlmock.AnyArg(), "sub2api-test", integration.ContractVersion, sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	event := integration.Event{EventID: "550e8400-e29b-41d4-a716-446655440000", Type: integration.EventTypeAccountHealthChanged,
		OccurredAt: time.Now(), SourceVersion: "sub2api-test", ContractVersion: integration.ContractVersion, Payload: []byte(`{"account_id":1,"status":"healthy","checked_at":"2026-08-09T00:00:00Z"}`)}
	require.NoError(t, NewOutbox(db).Append(context.Background(), tx, event))
	mock.ExpectCommit()
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOutboxClaimBatchUsesSkipLockedAndReturnsEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("WITH candidates").WithArgs("relay-ops-1", 10, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(
		[]string{"event_id", "event_type", "occurred_at", "source_version", "contract_version", "payload"},
	).AddRow("550e8400-e29b-41d4-a716-446655440000", integration.EventTypeAccountHealthChanged, time.Now(), "sub2api-test", integration.ContractVersion, []byte(`{"account_id":1}`)))
	mock.ExpectCommit()
	events, err := NewOutbox(db).ClaimBatch(context.Background(), "relay-ops-1", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", events[0].EventID)
	require.NoError(t, mock.ExpectationsWereMet())
}
