package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type accountLastUsedPayloadMatcher struct {
	accountID int64
	notBefore time.Time
}

func (m accountLastUsedPayloadMatcher) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		return false
	}
	var payload struct {
		LastUsed map[string]int64 `json:"last_used"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	lastUsed, ok := payload.LastUsed[strconv.FormatInt(m.accountID, 10)]
	return ok && len(payload.LastUsed) == 1 && lastUsed >= m.notBefore.Unix()
}

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

func TestUpdateLastUsedAdvancesDurableAccountVersionAndPublishesLastUsedEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)
	accountID := int64(47)
	started := time.Now()

	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts SET last_used_at = $1, updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond') WHERE deleted_at IS NULL AND id = $2")).
		WithArgs(sqlmock.AnyArg(), accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountLastUsed, accountID, nil, accountLastUsedPayloadMatcher{accountID: accountID, notBefore: started}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.UpdateLastUsed(context.Background(), accountID))
	require.NoError(t, mock.ExpectationsWereMet())
}
