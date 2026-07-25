package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
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

	repo := newUsageLogRepositoryWithSQL(client, db)
	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
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
