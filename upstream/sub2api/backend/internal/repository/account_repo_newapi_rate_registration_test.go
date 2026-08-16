package repository

import (
	"context"
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

func TestNewAPIRateRefreshRepositoryContract(t *testing.T) {
	var repo service.NewAPIRateRefreshRepository
	var _ = repo
	t.Skip("contract compile coverage; integration SQL tests provide execution coverage")

	ctx := context.Background()
	date := "2026-08-16"
	until := time.Date(2026, 8, 16, 10, 5, 0, 0, time.FixedZone("CST", 8*3600))
	claimed, err := repo.ClaimNewAPIRateRefresh(ctx, 1, date, "claim-token", until)
	if err != nil || !claimed {
		t.Fatalf("claim contract: claimed=%v err=%v", claimed, err)
	}
	if err := repo.CompleteNewAPIRateRefresh(ctx, service.NewAPIRateRefreshCompletion{
		AccountID: 1, ClaimToken: "claim-token", RefreshDate: date, GroupRatio: 0.17,
		ObservedAt: until, UsageLogID: 9,
	}); err != nil {
		t.Fatalf("complete contract: %v", err)
	}
	if err := repo.ReleaseNewAPIRateRefresh(ctx, 1, "claim-token"); err != nil {
		t.Fatalf("release contract: %v", err)
	}
}

func TestCompleteNewAPIRateRefreshRemovesNestedClaimFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*newapi_rate_registration.*claim_token.*claim_date.*claim_expires_at`).
		WithArgs(0.17, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7), service.AccountTypeAPIKey, service.AccountMonitorBalanceSourceNewAPI, service.UpstreamBillingProbeStatusUnsupported, service.UpstreamBillingProbeStatusOK, "claim", "2026-08-16").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.CompleteNewAPIRateRefresh(context.Background(), service.NewAPIRateRefreshCompletion{
		AccountID: 7, ClaimToken: "claim", RefreshDate: "2026-08-16", GroupRatio: 0.17,
		ObservedAt: time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC), UsageLogID: 9,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
