package repository

import (
	"context"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateUpstreamMultiplierMeasurementSnapshotUsesIdentityCAS(t *testing.T) {
	tests := []struct {
		name     string
		affected int64
		wantErr  error
	}{
		{name: "same identity and snapshot", affected: 1},
		{
			name:     "identity or snapshot changed",
			affected: 0,
			wantErr:  service.ErrUpstreamBillingProbeIdentityChanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })

			mock.ExpectBegin()
			mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+
				regexp.QuoteMeta("AND credentials = $5::jsonb")+`.*`+
				regexp.QuoteMeta("AND proxy_id IS NOT DISTINCT FROM $6")+`.*`+
				regexp.QuoteMeta("AND status = $7")+`.*`+
				regexp.QuoteMeta("AND schedulable = $8")+`.*`+
				regexp.QuoteMeta("COALESCE(extra -> 'upstream_multiplier_measurement_v1', 'null'::jsonb) = $9::jsonb")).
				WithArgs(
					sqlmock.AnyArg(),
					int64(17),
					service.PlatformOpenAI,
					service.AccountTypeAPIKey,
					`{"api_key":"sk-test","base_url":"https://newapi.example/v1"}`,
					nil,
					service.StatusActive,
					true,
					`{"observed_at":"2026-07-25T00:00:00Z","schema_version":1,"status":"stale"}`,
				).
				WillReturnResult(sqlmock.NewResult(0, tt.affected))
			if tt.affected > 0 {
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
					WithArgs(
						service.SchedulerOutboxEventAccountChanged,
						int64(17),
						nil,
						nil,
						sqlmock.AnyArg(),
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}
			if tt.wantErr == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			repo := newAccountRepositoryWithSQL(client, db, nil)
			account := &service.Account{
				ID:          17,
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://newapi.example/v1",
				},
				Extra: map[string]any{
					service.UpstreamMultiplierMeasurementExtraKey: map[string]any{
						"schema_version": float64(1),
						"status":         "stale",
						"observed_at":    "2026-07-25T00:00:00Z",
					},
				},
			}

			err = repo.UpdateUpstreamMultiplierMeasurementSnapshot(
				context.Background(),
				account,
				&service.UpstreamMultiplierMeasurementSnapshot{
					SchemaVersion: service.UpstreamMultiplierMeasurementSchemaVersion,
					Status:        service.AccountMonitorMultiplierStatusOK,
				},
			)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
