package repository

import (
	"context"
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

func TestUpdateUpstreamBillingProbeSnapshotUsesOfficialSyncSwitchForMultiplierWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	observedAt := time.Date(2026, time.July, 31, 9, 30, 0, 0, time.UTC)
	oldMultiplier := 1.0
	newMultiplier := 0.25
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+regexp.QuoteMeta("COALESCE(extra -> 'upstream_billing_rate_sync_enabled', 'null'::jsonb) = $9::jsonb")).
		WithArgs(sqlmock.AnyArg(), int64(17), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"sk-test"}`, nil, "null", "true", "true").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts SET rate_multiplier = $1")+`.*`+regexp.QuoteMeta("COALESCE((extra ->> 'upstream_billing_rate_sync_enabled')::boolean, false) = true")).
		WithArgs(newMultiplier, int64(17), oldMultiplier).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)SELECT .* FROM "accounts" WHERE "accounts"\."id" = \$1.*"accounts"\."deleted_at" IS NULL`).
		WithArgs(int64(17)).
		WillReturnRows(updatedAccountRowsWithRate(17, newMultiplier, `{"upstream_billing_probe":{"status":"ok"},"upstream_billing_rate_sync_enabled":true}`))
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
			service.UpstreamBillingProbeEnabledExtraKey:    true,
			service.UpstreamBillingRateSyncEnabledExtraKey: true,
		},
	}

	err = repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), account, &service.UpstreamBillingProbeSnapshot{
		Status: service.UpstreamBillingProbeStatusOK, ReceivedAt: &observedAt,
	}, &newMultiplier)

	require.NoError(t, err)
	auditLog.Stop()
	require.Len(t, scheduler.accounts, 1)
	require.Len(t, auditRepo.logs, 1)
	require.Equal(t, map[string]any{
		"account_id": int64(17), "old_rate_multiplier": oldMultiplier, "new_rate_multiplier": newMultiplier,
		"source": "native_billing", "probe_timestamp": observedAt.Format(time.RFC3339Nano),
		"trigger": "scheduled", "actor": "system",
	}, auditRepo.logs[0].Extra)
	require.NoError(t, mock.ExpectationsWereMet())
}

func updatedAccountRowsWithRate(id int64, rate float64, extra string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(dbaccount.Columns).AddRow(
		id, now, now, nil, "test", nil, service.PlatformOpenAI, service.AccountTypeAPIKey,
		[]byte(`{"api_key":"sk-test"}`), []byte(extra), nil, nil, 1, nil, 1, rate,
		nil, nil, nil, service.StatusActive, nil, nil, nil, false, true, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, service.QuotaDimensionGlobal,
	)
}
