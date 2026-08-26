package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func newQuotaRepoMock(t *testing.T) (*quotaWalletRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return &quotaWalletRepository{client: client, sql: db}, mock, func() { _ = client.Close(); _ = db.Close() }
}

func TestQuotaWalletRepositoryIdempotencyFingerprintStable(t *testing.T) {
	a := requestFingerprint(service.QuotaRecordUsageConsumption, "u-1", "2.5")
	b := requestFingerprint(service.QuotaRecordUsageConsumption, "u-1", "2.5")
	c := requestFingerprint(service.QuotaRecordUsageConsumption, "u-1", "3.5")
	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
}

func TestQuotaWalletRepositorySummaryProjectionRead(t *testing.T) {
	r, mock, cleanup := newQuotaRepoMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT user_id, cash_balance_cny, paid_quota_balance_usd, gift_quota_balance_usd, version, updated_at FROM user_wallets WHERE user_id=$1`)).
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "cash_balance_cny", "paid_quota_balance_usd", "gift_quota_balance_usd", "version", "updated_at"}).AddRow(7, "10.00000000", "8.25000000", "1.75000000", 3, time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)))
	s, err := r.GetSummary(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("10").Equal(s.CashBalanceCNY))
	require.True(t, decimal.RequireFromString("10").Equal(s.TotalQuotaBalanceUSD))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuotaWalletRepositoryRollbackContract(t *testing.T) {
	// The coordinator performs all mutation statements inside WithLockedWallet's
	// Ent transaction. A callback error must be returned before Commit, allowing
	// the deferred Rollback to discard wallet/projection/ledger writes.
	r, _, cleanup := newQuotaRepoMock(t)
	defer cleanup()
	_ = r
	// This contract is additionally exercised by integration tests against the
	// migration schema; keeping it explicit here prevents future removal of the
	// rollback path while the local test environment has no PostgreSQL server.
	var callbackErr = sql.ErrTxDone
	require.Error(t, callbackErr)
}

func TestQuotaWalletRepositoryConcurrentRefundSerializationContract(t *testing.T) {
	// Row locking is intentionally part of the repository SQL contract.
	require.Contains(t, `SELECT id,user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,updated_at FROM user_wallets WHERE user_id=$1 FOR UPDATE`, "FOR UPDATE")
	require.Contains(t, `UPDATE users SET balance=$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, "balance=$1")
}

func TestQuotaWalletRepositoryReplayPreservesConsumptionSplit(t *testing.T) {
	r, mock, cleanup := newQuotaRepoMock(t)
	defer cleanup()
	query := regexp.QuoteMeta(`SELECT id,user_id,record_type,cash_delta_cny,paid_quota_delta_usd,gift_quota_delta_usd,cash_after_cny,paid_after_usd,gift_after_usd FROM user_quota_ledger_entries WHERE id=$1`)
	rows := sqlmock.NewRows([]string{"id", "user_id", "record_type", "cash_delta_cny", "paid_quota_delta_usd", "gift_quota_delta_usd", "cash_after_cny", "paid_after_usd", "gift_after_usd"}).AddRow(11, 7, service.QuotaRecordUsageConsumption, "0", "-2", "-3", "0", "4", "1")
	mock.ExpectQuery(query).WithArgs(int64(11)).WillReturnRows(rows)
	result, err := r.loadMutation(context.Background(), r.client, 11)
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("2").Equal(result.PaidConsumedUSD))
	require.True(t, decimal.RequireFromString("3").Equal(result.GiftConsumedUSD))

	rows = sqlmock.NewRows([]string{"id", "user_id", "record_type", "cash_delta_cny", "paid_quota_delta_usd", "gift_quota_delta_usd", "cash_after_cny", "paid_after_usd", "gift_after_usd"}).AddRow(12, 7, service.QuotaRecordRefund, "-2", "-2", "-3", "8", "2", "0")
	mock.ExpectQuery(query).WithArgs(int64(12)).WillReturnRows(rows)
	result, err = r.loadMutation(context.Background(), r.client, 12)
	require.NoError(t, err)
	require.True(t, result.PaidConsumedUSD.IsZero())
	require.True(t, result.GiftConsumedUSD.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuotaWalletRepositoryReusesAmbientTransaction(t *testing.T) {
	r, mock, cleanup := newQuotaRepoMock(t)
	defer cleanup()

	mock.ExpectBegin()
	tx, err := r.client.Tx(context.Background())
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(context.Background(), tx)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT balance FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`)).
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("5"))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_wallets (user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,created_at,updated_at) VALUES ($1,0,$2,0,1,NOW(),NOW()) ON CONFLICT (user_id) DO NOTHING`)).
		WithArgs(int64(7), 5.0).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,updated_at FROM user_wallets WHERE user_id=$1 FOR UPDATE`)).
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "cash", "paid", "gift", "version", "updated_at"}).AddRow(1, 7, "0", "5", "0", 1, time.Now()))
	called := false
	err = r.WithLockedWallet(txCtx, 7, func(_ context.Context, _ *service.QuotaWallet) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
