//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usagecostreview"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountFinancialRepositoryPostgresTransactions(t *testing.T) {
	t.Run("filtered batch rollback and newer exclusion", func(t *testing.T) {
		ctx := context.Background()
		client := testEntClient(t)
		prefix := fmt.Sprintf("financial-pg-%d", time.Now().UnixNano())
		cleanupFinancialPG(t, prefix)
		now := time.Now().UTC()
		ensureFinancialSetting(t, ctx, client, now.Add(-time.Hour))
		user := client.User.Create().SetEmail(prefix + "@example.com").SetPasswordHash("x").SaveX(ctx)
		key := client.APIKey.Create().SetUserID(user.ID).SetKey(prefix).SetName(prefix).SaveX(ctx)
		account := client.Account.Create().SetName(prefix).SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
		makePending := func(s string) *ent.UsageLog {
			u := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID(prefix + s).SetModel("m").SetActualCost(10).SetCreatedAt(now).SaveX(ctx)
			client.UsageUpstreamCostEvidence.Create().SetUsageLogID(u.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
			return u
		}
		one, two := makePending("-1"), makePending("-2")
		repo := &accountFinancialRepository{client: client, now: func() time.Time { return now }, reviewBeforeCreate: func(id int64) error {
			if id == two.ID {
				return errors.New("injected batch failure")
			}
			return nil
		}}
		_, err := repo.ReviewFiltered(ctx, service.ReviewFilteredInput{Filter: service.ReviewFilter{AccountID: &account.ID}, ReviewedBy: user.ID, ReviewedAt: now})
		require.ErrorContains(t, err, "injected batch failure")
		count := client.UsageCostReview.Query().Where(usagecostreview.UsageLogIDIn(one.ID, two.ID)).CountX(ctx)
		require.Zero(t, count, "failed batch must roll back all rows")
		repo.reviewBeforeCreate = nil
		cutoff := two.ID
		newer := makePending("-3")
		res, err := repo.ReviewFiltered(ctx, service.ReviewFilteredInput{Filter: service.ReviewFilter{AccountID: &account.ID}, MaxUsageLogID: cutoff, ReviewedBy: user.ID, ReviewedAt: now})
		require.NoError(t, err)
		require.Equal(t, 2, res.Updated)
		require.False(t, client.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(newer.ID)).ExistX(ctx))
	})

	t.Run("daily override concurrent unique and truthful cost old new", func(t *testing.T) {
		ctx := context.Background()
		client := testEntClient(t)
		prefix := fmt.Sprintf("financial-day-%d", time.Now().UnixNano())
		cleanupFinancialPG(t, prefix)
		loc, _ := time.LoadLocation("Asia/Shanghai")
		now := time.Date(2026, 8, 13, 12, 0, 0, 0, loc)
		account := client.Account.Create().SetName(prefix).SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
		repo := NewAccountFinancialRepositoryWithClock(client, func() time.Time { return now })
		four := 4.0
		_, err := repo.SetTodayOverride(ctx, service.TodayOverrideInput{AccountID: account.ID, BusinessDate: "2026-08-13", CostCNY: &four, ActorUserID: 1})
		require.NoError(t, err)
		six := 6.0
		truth, err := repo.SetTodayOverride(ctx, service.TodayOverrideInput{AccountID: account.ID, BusinessDate: "2026-08-13", CostCNY: &six, ActorUserID: 1})
		require.NoError(t, err)
		require.Equal(t, "cost", truth.MutationKind)
		require.Equal(t, 4.0, *truth.OldValue)
		require.Equal(t, 6.0, *truth.NewValue)
		vals := []float64{7, 8}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range vals {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = repo.SetTodayOverride(context.Background(), service.TodayOverrideInput{AccountID: account.ID, BusinessDate: "2026-08-13", CostCNY: &vals[i], ActorUserID: 1})
			}(i)
		}
		wg.Wait()
		success := 0
		for _, e := range errs {
			if e == nil {
				success++
			}
		}
		require.GreaterOrEqual(t, success, 1)
		var rows int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM account_daily_financial_values WHERE account_id=$1 AND business_date=$2", account.ID, "2026-08-13").Scan(&rows))
		require.Equal(t, 1, rows)
	})
}

func ensureFinancialSetting(t *testing.T, ctx context.Context, client *ent.Client, enabled time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(ctx, "INSERT INTO account_financial_settings(key,enabled_at,created_at,updated_at) VALUES($1,$2,now(),now()) ON CONFLICT(key) DO UPDATE SET enabled_at=EXCLUDED.enabled_at", accountFinancialSettingKey, enabled)
	require.NoError(t, err)
}
func cleanupFinancialPG(t *testing.T, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM usage_logs WHERE request_id LIKE $1", prefix+"%")
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE key=$1", prefix)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE email=$1", prefix+"@example.com")
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE name=$1", prefix)
	})
}
