package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestExecAccountMonotonicUpdateUsesLockedDatabaseVersionExpression(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts SET schedulable = $1, updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond') WHERE deleted_at IS NULL AND id = $2")).
		WithArgs(false, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	updated, err := repo.execAccountMonotonicUpdate(context.Background(), 27, "schedulable = $1", "", false)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	require.NoError(t, mock.ExpectationsWereMet())
}
