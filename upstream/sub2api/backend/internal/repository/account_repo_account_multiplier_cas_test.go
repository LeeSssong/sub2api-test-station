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

func TestUpdateAccountMultiplierMeasurementRequiresSameIdentityStateAndSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		affected int64
		wantErr  error
	}{
		{name: "same account snapshot", affected: 1},
		{name: "identity state or prior measurement changed", affected: 0, wantErr: service.ErrUpstreamBillingProbeIdentityChanged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				regexp.QuoteMeta("COALESCE(extra -> 'account_monitor_multiplier_measurement', 'null'::jsonb) = $9::jsonb")).
				WithArgs(
					sqlmock.AnyArg(),
					int64(21),
					service.PlatformOpenAI,
					service.AccountTypeAPIKey,
					`{"api_key":"sk-test","base_url":"https://new-api.example"}`,
					nil,
					service.StatusActive,
					true,
					`{"status":"stale"}`,
				).
				WillReturnResult(sqlmock.NewResult(0, tt.affected))

			repo := newAccountRepositoryWithSQL(client, db, nil)
			account := &service.Account{
				ID:          21,
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://new-api.example",
				},
				Extra: map[string]any{
					service.AccountMultiplierMeasurementExtraKey: map[string]any{"status": "stale"},
				},
			}
			now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
			value := 0.25
			snapshot := &service.AccountMultiplierMeasurementSnapshot{
				Version:       service.AccountMultiplierMeasurementVersion,
				Status:        service.AccountMonitorMultiplierStatusOK,
				Source:        service.AccountMonitorMultiplierSourceMeasured,
				Value:         &value,
				SampleCount:   3,
				ObservedAt:    &now,
				FreshUntil:    timePointer(now.Add(service.AccountMultiplierMeasurementTTL)),
				LastAttemptAt: now,
			}

			err = repo.UpdateAccountMultiplierMeasurement(dbent.NewTxContext(context.Background(), tx), account, snapshot)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
