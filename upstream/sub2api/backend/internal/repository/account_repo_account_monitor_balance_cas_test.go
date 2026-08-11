package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestUpdateAccountMonitorBalanceRequiresSameIdentityStateAndSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+
		regexp.QuoteMeta("WHERE id = $2")+`.*`+
		regexp.QuoteMeta("AND platform = $3")+`.*`+
		regexp.QuoteMeta("AND type = $4")+`.*`+
		regexp.QuoteMeta("AND credentials = $5::jsonb")+`.*`+
		regexp.QuoteMeta("AND proxy_id IS NOT DISTINCT FROM $6")+`.*`+
		regexp.QuoteMeta("AND status = $7")+`.*`+
		regexp.QuoteMeta("AND schedulable = $8")+`.*`+
		regexp.QuoteMeta("COALESCE(extra -> 'account_monitor_balance', 'null'::jsonb) = $9::jsonb")).
		WithArgs(sqlmock.AnyArg(), int64(21), service.PlatformOpenAI, service.AccountTypeAPIKey,
			`{"api_key":"sk-test","base_url":"https://upstream.example"}`, nil, service.StatusActive, true,
			`{"status":"stale"}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	value := 12.5
	account := &service.Account{ID: 21, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example"},
		Extra:       map[string]any{service.AccountMonitorBalanceExtraKey: map[string]any{"status": "stale"}},
	}
	snapshot := &service.AccountMonitorBalance{Version: service.AccountMonitorBalanceVersion, ValueUSD: &value,
		Source: service.AccountMonitorBalanceSourceNewAPI, Status: service.AccountMonitorBalanceStatusOK,
		ObservedAt: &now, LastAttemptAt: &now}
	repo := newAccountRepositoryWithSQL(client, db, nil)
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("INSERT INTO scheduler_outbox")+`.*`).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(21), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.UpdateAccountMonitorBalance(dbent.NewTxContext(context.Background(), tx), account, snapshot))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAccountMonitorBalanceRejectsChangedPriorSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+
		regexp.QuoteMeta("COALESCE(extra -> 'account_monitor_balance', 'null'::jsonb) = $9::jsonb")).
		WithArgs(sqlmock.AnyArg(), int64(21), service.PlatformOpenAI, service.AccountTypeAPIKey,
			`{"api_key":"sk-test","base_url":"https://upstream.example"}`, nil, service.StatusActive, true,
			`{"status":"stale"}`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	value := 12.5
	account := &service.Account{ID: 21, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example"},
		Extra:       map[string]any{service.AccountMonitorBalanceExtraKey: map[string]any{"status": "stale"}},
	}
	snapshot := &service.AccountMonitorBalance{Version: service.AccountMonitorBalanceVersion, ValueUSD: &value,
		Source: service.AccountMonitorBalanceSourceNewAPI, Status: service.AccountMonitorBalanceStatusOK,
		ObservedAt: &now, LastAttemptAt: &now}
	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.UpdateAccountMonitorBalance(dbent.NewTxContext(context.Background(), tx), account, snapshot)
	require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulableBalanceVetoPredicateFailsOpenForMissingSnapshot(t *testing.T) {
	selector := entsql.Select().From(entsql.Table("accounts"))
	schedulableBalanceVetoPredicate()(selector)
	query, args := selector.Query()
	require.Contains(t, query, "COALESCE(`accounts`.`extra` -> 'account_monitor_balance' ->> 'status', '')")
	require.Contains(t, query, "COALESCE(`accounts`.`extra` -> 'account_monitor_balance' ->> 'failure_code', '')")
	require.Equal(t, []any{service.PlatformOpenAI, service.AccountTypeAPIKey, service.AccountMonitorBalanceStatusFailed, "balance_unavailable", fmt.Sprintf("%d", service.AccountMonitorBalanceVersion)}, args)
}

func TestListSchedulableCapacityByGroupIDsAppliesBalanceVeto(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectQuery(`(?s)SELECT.*account_monitor_balance`).
		WithArgs(sqlmock.AnyArg(), service.StatusActive, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "concurrency", "extra", "session_window_start", "session_window_end", "session_window_status"}))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.ListSchedulableCapacityByGroupIDs(context.Background(), []int64{7})
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}
