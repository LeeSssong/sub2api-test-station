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
