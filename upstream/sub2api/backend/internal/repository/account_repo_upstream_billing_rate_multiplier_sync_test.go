package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type upstreamBillingMultiplierAuditCapture struct {
	service.AuditLogRepository
	logs []*service.AuditLog
}

func (r *upstreamBillingMultiplierAuditCapture) BatchInsert(_ context.Context, logs []*service.AuditLog) (int64, error) {
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}

type upstreamBillingMultiplierSchedulerCapture struct {
	service.SchedulerCache
	accounts []*service.Account
}

func (c *upstreamBillingMultiplierSchedulerCapture) SetAccount(_ context.Context, account *service.Account) error {
	c.accounts = append(c.accounts, account)
	return nil
}

func TestUpdateUpstreamBillingProbeSnapshotSynchronizesManagedMultiplierAuditsAndRefreshesScheduler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	observedAt := time.Date(2026, time.July, 31, 9, 30, 0, 0, time.UTC)
	oldMultiplier := 1.0
	probeMultiplier := 0.249975
	newMultiplier := 0.25
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+regexp.QuoteMeta("SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb")+`.*`+regexp.QuoteMeta("COALESCE(extra -> 'upstream_billing_probe_enabled', 'null'::jsonb) = $8::jsonb")).
		WithArgs(sqlmock.AnyArg(), int64(17), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"sk-test"}`, nil, "null", "null").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts SET rate_multiplier = $1")+`.*`+regexp.QuoteMeta("COALESCE(extra ->> 'upstream_billing_rate_multiplier_policy', 'manual_override') = 'upstream_managed'")).
		WithArgs(newMultiplier, int64(17), oldMultiplier).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)SELECT .* FROM "accounts" WHERE "accounts"\."id" = \$1.*"accounts"\."deleted_at" IS NULL`).
		WithArgs(int64(17)).
		WillReturnRows(updatedAccountRowsWithRate(17, newMultiplier, `{"upstream_billing_probe":{"status":"ok"}}`))
	mock.ExpectQuery(`(?s)SELECT .* FROM "account_groups" WHERE "account_groups"\."account_id" IN \(\$1\)`).
		WithArgs(int64(17)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}))

	auditRepo := &upstreamBillingMultiplierAuditCapture{}
	auditLog := service.NewAuditLogService(auditRepo, nil)
	auditLog.Start()
	t.Cleanup(auditLog.Stop)
	scheduler := &upstreamBillingMultiplierSchedulerCapture{}
	repo := newAccountRepositoryWithSQLAndAudit(client, db, scheduler, auditLog)
	account := &service.Account{
		ID:             17,
		Platform:       service.PlatformOpenAI,
		Type:           service.AccountTypeAPIKey,
		Credentials:    map[string]any{"api_key": "sk-test"},
		RateMultiplier: &oldMultiplier,
		Extra: map[string]any{
			service.UpstreamBillingRateMultiplierPolicyExtraKey: service.UpstreamBillingRateMultiplierPolicyManaged,
		},
	}

	err = repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), account, &service.UpstreamBillingProbeSnapshot{
		Status:     service.UpstreamBillingProbeStatusOK,
		ReceivedAt: &observedAt,
		Data:       map[string]any{"effective_rate_multiplier": probeMultiplier},
	})

	require.NoError(t, err)
	auditLog.Stop()
	require.Len(t, scheduler.accounts, 1)
	require.NotNil(t, scheduler.accounts[0].RateMultiplier)
	require.Equal(t, newMultiplier, *scheduler.accounts[0].RateMultiplier)
	require.Len(t, auditRepo.logs, 1)
	require.Equal(t, "system.accounts.rate_multiplier.sync", auditRepo.logs[0].Action)
	require.Equal(t, "system", auditRepo.logs[0].ActorEmail)
	require.Equal(t, map[string]any{
		"account_id":          int64(17),
		"old_rate_multiplier": oldMultiplier,
		"new_rate_multiplier": newMultiplier,
		"source":              "native_billing",
		"probe_timestamp":     observedAt.Format(time.RFC3339Nano),
		"trigger":             "scheduled",
		"policy":              service.UpstreamBillingRateMultiplierPolicyManaged,
		"actor":               "system",
	}, auditRepo.logs[0].Extra)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUpstreamBillingProbeSnapshotCommitFailureDoesNotRefreshSchedulerOrAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	oldMultiplier := 1.0
	newMultiplier := 0.07
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)` + regexp.QuoteMeta("UPDATE accounts") + `.*` + regexp.QuoteMeta("SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET rate_multiplier = $1")).
		WithArgs(newMultiplier, int64(18), oldMultiplier).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(18), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	auditRepo := &upstreamBillingMultiplierAuditCapture{}
	auditLog := service.NewAuditLogService(auditRepo, nil)
	auditLog.Start()
	t.Cleanup(auditLog.Stop)
	scheduler := &upstreamBillingMultiplierSchedulerCapture{}
	repo := newAccountRepositoryWithSQLAndAudit(client, db, scheduler, auditLog)
	err = repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), &service.Account{
		ID:             18,
		Platform:       service.PlatformOpenAI,
		Type:           service.AccountTypeAPIKey,
		Credentials:    map[string]any{"api_key": "sk-test"},
		RateMultiplier: &oldMultiplier,
		Extra: map[string]any{
			service.UpstreamBillingRateMultiplierPolicyExtraKey: service.UpstreamBillingRateMultiplierPolicyManaged,
		},
	}, &service.UpstreamBillingProbeSnapshot{
		Status: service.UpstreamBillingProbeStatusOK,
		Data:   map[string]any{"effective_rate_multiplier": newMultiplier},
	})

	require.EqualError(t, err, "commit failed")
	auditLog.Stop()
	require.Empty(t, scheduler.accounts)
	require.Empty(t, auditRepo.logs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUpstreamBillingProbeSnapshotPreservesSnapshotWithoutMultiplierChange(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		multiplier float64
		snapshot   *service.UpstreamBillingProbeSnapshot
	}{
		{
			name:       "manual override",
			policy:     service.UpstreamBillingRateMultiplierPolicyManualOverride,
			multiplier: 1.0,
			snapshot: &service.UpstreamBillingProbeSnapshot{
				Status: service.UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.07},
			},
		},
		{
			name:       "unchanged managed value",
			multiplier: 0.07,
			snapshot: &service.UpstreamBillingProbeSnapshot{
				Status: service.UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.07},
			},
		},
		{
			name:       "high precision probe matching persisted four decimal value",
			multiplier: 0.25,
			snapshot: &service.UpstreamBillingProbeSnapshot{
				Status: service.UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0.249975},
			},
		},
		{
			name:       "invalid probe value",
			multiplier: 1.0,
			snapshot: &service.UpstreamBillingProbeSnapshot{
				Status: service.UpstreamBillingProbeStatusOK,
				Data:   map[string]any{"effective_rate_multiplier": 0},
			},
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
			mock.ExpectExec(`(?s)` + regexp.QuoteMeta("UPDATE accounts") + `.*` + regexp.QuoteMeta("SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb")).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
				WithArgs(service.SchedulerOutboxEventAccountChanged, int64(19), nil, nil, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()

			auditRepo := &upstreamBillingMultiplierAuditCapture{}
			auditLog := service.NewAuditLogService(auditRepo, nil)
			auditLog.Start()
			t.Cleanup(auditLog.Stop)
			repo := newAccountRepositoryWithSQLAndAudit(client, db, nil, auditLog)
			extra := map[string]any{}
			if tt.policy != "" {
				extra[service.UpstreamBillingRateMultiplierPolicyExtraKey] = tt.policy
			}
			err = repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), &service.Account{
				ID:             19,
				Platform:       service.PlatformOpenAI,
				Type:           service.AccountTypeAPIKey,
				Credentials:    map[string]any{"api_key": "sk-test"},
				RateMultiplier: &tt.multiplier,
				Extra:          extra,
			}, tt.snapshot)

			require.NoError(t, err)
			auditLog.Stop()
			require.Empty(t, auditRepo.logs)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpstreamBillingMultiplierAuditUsesLifecycleTriggerFromContext(t *testing.T) {
	oldMultiplier := 1.0
	newMultiplier := 0.07
	probeAt := time.Date(2026, time.July, 31, 10, 0, 0, 0, time.UTC)
	entry := newUpstreamBillingRateMultiplierAuditLog(
		service.WithUpstreamBillingRateMultiplierSyncTrigger(context.Background(), service.UpstreamBillingRateMultiplierSyncTriggerLifecycle),
		&service.Account{ID: 23, RateMultiplier: &oldMultiplier},
		&service.UpstreamBillingProbeSnapshot{ReceivedAt: &probeAt},
		service.UpstreamBillingRateMultiplierPolicyManaged,
		oldMultiplier,
		newMultiplier,
	)

	require.Equal(t, service.UpstreamBillingRateMultiplierSyncTriggerLifecycle, entry.Extra["trigger"])
}

func updatedAccountRowsWithRate(id int64, rate float64, extra string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(dbaccount.Columns).AddRow(
		id, now, now, nil, "test", nil, service.PlatformOpenAI, service.AccountTypeAPIKey,
		[]byte(`{"api_key":"sk-test"}`), []byte(extra), nil, nil, 1, nil, 1, rate,
		service.StatusActive, nil, nil, nil, false, true, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, service.QuotaDimensionGlobal,
	)
}
