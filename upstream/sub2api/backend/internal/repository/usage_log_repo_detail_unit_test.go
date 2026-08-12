package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestUsageLogRepositoryGetByID加载详情所需摘要关系(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:usage_log_detail?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	for _, statement := range []string{
		"ALTER TABLE usage_logs ADD COLUMN image_output_tokens integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_output_cost real NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_input_tokens integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_input_cost real NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN request_type integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN openai_ws_mode boolean NOT NULL DEFAULT false",
		"ALTER TABLE usage_logs ADD COLUMN service_tier text NULL",
		"ALTER TABLE usage_logs ADD COLUMN reasoning_effort text NULL",
		"ALTER TABLE usage_logs ADD COLUMN inbound_endpoint text NULL",
		"ALTER TABLE usage_logs ADD COLUMN upstream_endpoint text NULL",
		"ALTER TABLE usage_logs ADD COLUMN account_stats_cost real NULL",
		"ALTER TABLE usage_logs ADD COLUMN account_cost real NULL",
		"ALTER TABLE usage_logs ADD COLUMN session_id text NULL",
	} {
		_, err = db.Exec(statement)
		require.NoError(t, err)
	}

	user, err := client.User.Create().
		SetEmail("usage-detail@test.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("详情分组").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	apiKey, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-详情测试").
		SetName("详情密钥").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	account, err := client.Account.Create().
		SetName("详情上游账号").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeAPIKey).
		SetStatus(service.StatusActive).
		SetCredentials(map[string]any{"api_key": "上游密钥"}).
		Save(ctx)
	require.NoError(t, err)

	created, err := client.UsageLog.Create().
		SetUserID(user.ID).
		SetAPIKeyID(apiKey.ID).
		SetAccountID(account.ID).
		SetGroupID(group.ID).
		SetRequestID("req-relation-detail").
		SetModel("gpt-5.4").
		Save(ctx)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE usage_logs SET account_cost = 0.06 WHERE id = ?", created.ID)
	require.NoError(t, err)

	repo := newUsageLogRepositoryWithSQL(client, db)
	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got.AccountCost)
	require.InEpsilon(t, 0.06, *got.AccountCost, 1e-9)
	require.NotNil(t, got.APIKey)
	require.Equal(t, apiKey.ID, got.APIKey.ID)
	require.Equal(t, "详情密钥", got.APIKey.Name)
	require.NotNil(t, got.Group)
	require.Equal(t, group.ID, got.Group.ID)
	require.Equal(t, "详情分组", got.Group.Name)
	require.NotNil(t, got.Account)
	require.Equal(t, account.ID, got.Account.ID)
	require.Equal(t, "详情上游账号", got.Account.Name)
}

func TestUsageLogRepositoryUpstreamCostPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:usage_log_upstream_cost?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	for _, statement := range []string{
		"ALTER TABLE usage_logs ADD COLUMN image_output_tokens integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_output_cost real NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_input_tokens integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN image_input_cost real NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN request_type integer NOT NULL DEFAULT 0",
		"ALTER TABLE usage_logs ADD COLUMN openai_ws_mode boolean NOT NULL DEFAULT false",
		"ALTER TABLE usage_logs ADD COLUMN service_tier text NULL",
		"ALTER TABLE usage_logs ADD COLUMN reasoning_effort text NULL",
		"ALTER TABLE usage_logs ADD COLUMN inbound_endpoint text NULL",
		"ALTER TABLE usage_logs ADD COLUMN upstream_endpoint text NULL",
		"ALTER TABLE usage_logs ADD COLUMN account_stats_cost real NULL",
		"ALTER TABLE usage_logs ADD COLUMN account_cost real NULL",
		"ALTER TABLE usage_logs ADD COLUMN session_id text NULL",
	} {
		_, err = db.Exec(statement)
		require.NoError(t, err)
	}

	user, err := client.User.Create().SetEmail("usage-upstream-cost@test.com").SetPasswordHash("test-password-hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	apiKey, err := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-upstream-cost").SetName("cost key").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().SetName("cost account").SetPlatform(service.PlatformOpenAI).SetType(service.AccountTypeAPIKey).SetStatus(service.StatusActive).SetCredentials(map[string]any{"api_key": "upstream-key"}).Save(ctx)
	require.NoError(t, err)
	created, err := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(apiKey.ID).SetAccountID(account.ID).SetRequestID("req-upstream-cost-round-trip").SetModel("gpt-5").Save(ctx)
	require.NoError(t, err)

	recordedAt := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	_, err = db.Exec(`UPDATE usage_logs
SET upstream_actual_cost = ?, upstream_cost_status = ?, upstream_cost_reason = ?, profit = ?, upstream_cost_recorded_at = ?
WHERE id = ?`, 0.004, service.UsageUpstreamCostStatusConfirmed, "", 0.00288, recordedAt, created.ID)
	require.NoError(t, err)

	repo := newUsageLogRepositoryWithSQL(client, db)
	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got.UpstreamActualCost)
	require.InEpsilon(t, 0.004, *got.UpstreamActualCost, 1e-9)
	require.NotNil(t, got.UpstreamCostStatus)
	require.Equal(t, service.UsageUpstreamCostStatusConfirmed, *got.UpstreamCostStatus)
	require.NotNil(t, got.UpstreamCostReason)
	require.Equal(t, "", *got.UpstreamCostReason)
	require.NotNil(t, got.Profit)
	require.InEpsilon(t, 0.00288, *got.Profit, 1e-9)
	require.NotNil(t, got.UpstreamCostRecordedAt)
	require.True(t, got.UpstreamCostRecordedAt.Equal(recordedAt))

	logs, _, err := repo.ListByUser(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].UpstreamActualCost)
	require.InEpsilon(t, 0.004, *logs[0].UpstreamActualCost, 1e-9)
	require.NotNil(t, logs[0].UpstreamCostStatus)
	require.Equal(t, service.UsageUpstreamCostStatusConfirmed, *logs[0].UpstreamCostStatus)
	require.NotNil(t, logs[0].UpstreamCostReason)
	require.Equal(t, "", *logs[0].UpstreamCostReason)
	require.NotNil(t, logs[0].Profit)
	require.InEpsilon(t, 0.00288, *logs[0].Profit, 1e-9)
	require.NotNil(t, logs[0].UpstreamCostRecordedAt)
	require.True(t, logs[0].UpstreamCostRecordedAt.Equal(recordedAt))

	historical, err := client.UsageLog.Create().
		SetUserID(user.ID).
		SetAPIKeyID(apiKey.ID).
		SetAccountID(account.ID).
		SetRequestID("req-upstream-cost-historical").
		SetModel("gpt-5").
		Save(ctx)
	require.NoError(t, err)
	historicalLog, err := repo.GetByID(ctx, historical.ID)
	require.NoError(t, err)
	require.Nil(t, historicalLog.UpstreamActualCost)
	require.Nil(t, historicalLog.UpstreamCostStatus)
	require.Nil(t, historicalLog.UpstreamCostReason)
	require.Nil(t, historicalLog.Profit)
	require.Nil(t, historicalLog.UpstreamCostRecordedAt)
}
