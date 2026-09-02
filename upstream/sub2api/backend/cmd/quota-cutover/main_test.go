package main

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunDryRunIsReadOnlyAndBlocksResidual(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT user_id, paid_quota_balance_usd`).WillReturnRows(sqlmock.NewRows([]string{"user_id", "paid", "gift"}).AddRow(int64(1), "1", "0"))
	mock.ExpectQuery(`SELECT id, user_id, paid_quota_usd`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "paid", "gift", "pc", "gc", "pr", "gd", "prs", "off"}))
	mock.ExpectQuery(`SELECT id::text, user_id, delta_usd`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "delta", "paid", "gift", "status", "pa", "ga"}))
	mock.ExpectQuery(`SELECT po.id::text`).WillReturnRows(sqlmock.NewRows([]string{"id", "refunded", "adjusted"}))
	mock.ExpectQuery(`SELECT user_id::text`).WillReturnRows(sqlmock.NewRows([]string{"key"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM billing_usage_entries`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_quota_adjustments WHERE status`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_quota_adjustments a`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM`).WillReturnRows(sqlmock.NewRows([]string{"residual"}).AddRow("2"))
	_, _ = runDryRun(context.Background(), db, "b1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
