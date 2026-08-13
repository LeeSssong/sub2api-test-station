package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageCostEvidenceRepositoryCreatesOnceAndIgnoresConflict(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     *sqlmock.Rows
		inserted bool
	}{
		{name: "inserted", rows: sqlmock.NewRows([]string{"id"}).AddRow(1), inserted: true},
		{name: "conflict", rows: sqlmock.NewRows([]string{"id"}), inserted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery("INSERT INTO usage_upstream_cost_evidence").WillReturnRows(tc.rows)
			repo := NewUsageCostEvidenceRepository(db)
			cost := 0.004
			profit := 0.002
			inserted, err := repo.CreateOnce(context.Background(), &service.UsageCostEvidence{
				UsageLogID: 41, Source: service.UsageCostEvidenceSourceSub,
				NormalizedCostCNY: &cost, ProfitCNY: &profit,
				Status: service.UsageCostEvidenceStatusConfirmed, RecordedAt: time.Now(),
			})

			require.NoError(t, err)
			require.Equal(t, tc.inserted, inserted)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountFinancialActivationRepositoryEnabledAt(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows *sqlmock.Rows
		want bool
	}{
		{name: "setting absent", rows: sqlmock.NewRows([]string{"enabled_at"}), want: false},
		{name: "enabled at absent", rows: sqlmock.NewRows([]string{"enabled_at"}).AddRow(nil), want: false},
		{name: "enabled", rows: sqlmock.NewRows([]string{"enabled_at"}).AddRow(time.Now()), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery("SELECT enabled_at FROM account_financial_settings").WithArgs("t03_r1_account_financial").WillReturnRows(tc.rows)

			enabledAt, err := NewAccountFinancialActivationRepository(db).EnabledAt(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, enabledAt != nil)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
