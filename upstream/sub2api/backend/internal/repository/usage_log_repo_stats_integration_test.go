//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLog_ReadAccountFinancialUsage_NativeContract(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	from := time.Date(2037, 8, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	var balanceBefore float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(balance), 0) FROM users WHERE deleted_at IS NULL`).Scan(&balanceBefore))
	user := mustCreateUser(t, client, &service.User{Email: "native-financial-" + suffix + "@example.com", Balance: 12.5})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-native-financial-" + suffix, Name: "native-financial"})
	deletedUser := mustCreateUser(t, client, &service.User{Email: "native-financial-deleted-" + suffix + "@example.com", Balance: 40})
	require.NoError(t, client.User.UpdateOneID(deletedUser.ID).SetDeletedAt(from).Exec(ctx))

	activeAccount := mustCreateAccount(t, client, &service.Account{Name: "native-active-account-" + suffix, Type: service.AccountTypeAPIKey, Platform: service.PlatformOpenAI})
	historicalAccount := mustCreateAccount(t, client, &service.Account{Name: "native-historical-account-" + suffix, Type: service.AccountTypeOAuth, Platform: service.PlatformAnthropic})
	zeroAccount := mustCreateAccount(t, client, &service.Account{Name: "native-zero-account-" + suffix, Type: service.AccountTypeAPIKey, Platform: service.PlatformOpenAI})
	activeGroup := mustCreateGroup(t, client, &service.Group{Name: "native-active-group-" + suffix})
	secondGroup := mustCreateGroup(t, client, &service.Group{Name: "native-second-group-" + suffix})
	historicalGroup := mustCreateGroup(t, client, &service.Group{Name: "native-historical-group-" + suffix})
	zeroGroup := mustCreateGroup(t, client, &service.Group{Name: "native-zero-group-" + suffix})

	create := func(account *service.Account, groupID *int64, at time.Time, input, output, cacheCreation, cacheRead int, accountCost, accountStatsCost, accountRateMultiplier *float64, totalCost, actualCost float64) {
		t.Helper()
		inserted, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID, GroupID: groupID,
			RequestID: fmt.Sprintf("native-financial-%s-%d", suffix, time.Now().UnixNano()), Model: "native-financial",
			InputTokens: input, OutputTokens: output, CacheCreationTokens: cacheCreation, CacheReadTokens: cacheRead,
			AccountCost: accountCost, AccountStatsCost: accountStatsCost, AccountRateMultiplier: accountRateMultiplier,
			TotalCost: totalCost, ActualCost: actualCost, CreatedAt: at,
		})
		require.NoError(t, err)
		require.True(t, inserted)
	}

	accountCostA := 1.25
	accountCostB := 0.75
	create(activeAccount, &activeGroup.ID, from, 1, 2, 3, 4, &accountCostA, nil, nil, 9, 2)
	create(activeAccount, &activeGroup.ID, from.Add(time.Minute), 2, 1, 0, 1, &accountCostB, nil, nil, 9, 1.5)
	accountStatsCost := 2.0
	rateMultiplier := 1.5
	create(activeAccount, &secondGroup.ID, from.Add(2*time.Minute), 1, 1, 1, 1, nil, &accountStatsCost, &rateMultiplier, 9, 4)
	fallbackMultiplier := 0.5
	create(activeAccount, &secondGroup.ID, from.Add(3*time.Minute), 2, 0, 1, 1, nil, nil, &fallbackMultiplier, 4, 2.5)
	unassignedCost := 0.5
	create(activeAccount, nil, from.Add(4*time.Minute), 1, 1, 1, 1, &unassignedCost, nil, nil, 9, 1)
	historicalCost := 7.0
	create(historicalAccount, &historicalGroup.ID, from.Add(5*time.Minute), 0, 1, 1, 2, &historicalCost, nil, nil, 9, 8)
	excludedCost := 99.0
	create(activeAccount, &activeGroup.ID, to, 99, 99, 99, 99, &excludedCost, nil, nil, 99, 99)
	require.NoError(t, client.Account.UpdateOneID(historicalAccount.ID).SetDeletedAt(from.Add(6*time.Minute)).Exec(ctx))
	require.NoError(t, client.Group.UpdateOneID(historicalGroup.ID).SetDeletedAt(from.Add(6*time.Minute)).Exec(ctx))

	snapshot, err := repo.ReadAccountFinancialUsage(ctx, from, to)
	require.NoError(t, err)
	require.InDelta(t, balanceBefore+12.5, snapshot.UserBalanceCNY, 1e-9)

	accounts := make(map[int64]service.AccountFinancialUsageAccount, len(snapshot.Accounts))
	for _, account := range snapshot.Accounts {
		accounts[account.ID] = account
	}
	require.Equal(t, service.AccountFinancialUsageAccount{ID: activeAccount.ID, Name: activeAccount.Name, Type: service.AccountTypeAPIKey, Platform: service.PlatformOpenAI, Active: true}, accounts[activeAccount.ID])
	require.Equal(t, service.AccountFinancialUsageAccount{ID: historicalAccount.ID, Name: historicalAccount.Name, Type: service.AccountTypeOAuth, Platform: service.PlatformAnthropic, Active: false}, accounts[historicalAccount.ID])
	require.True(t, accounts[zeroAccount.ID].Active)

	groups := make(map[int64]service.AccountFinancialUsageGroup, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groups[group.ID] = group
	}
	require.Equal(t, service.AccountFinancialUsageGroup{ID: activeGroup.ID, Name: activeGroup.Name, Active: true}, groups[activeGroup.ID])
	require.Equal(t, service.AccountFinancialUsageGroup{ID: historicalGroup.ID, Name: historicalGroup.Name, Active: false}, groups[historicalGroup.ID])
	require.True(t, groups[zeroGroup.ID].Active)

	pairs := make(map[string]service.AccountFinancialUsageRow, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		groupKey := "unassigned"
		if row.GroupID != nil {
			groupKey = fmt.Sprintf("%d", *row.GroupID)
		}
		pairs[groupKey+":"+fmt.Sprintf("%d", row.AccountID)] = row
	}
	require.Len(t, pairs, 4)
	activePair := pairs[fmt.Sprintf("%d:%d", activeGroup.ID, activeAccount.ID)]
	require.Equal(t, int64(2), activePair.Requests)
	require.Equal(t, int64(14), activePair.Tokens)
	require.InDelta(t, 2.0, activePair.Cost, 1e-9)
	require.InDelta(t, 3.5, activePair.UserCost, 1e-9)
	secondPair := pairs[fmt.Sprintf("%d:%d", secondGroup.ID, activeAccount.ID)]
	require.Equal(t, int64(2), secondPair.Requests)
	require.Equal(t, int64(8), secondPair.Tokens)
	require.InDelta(t, 5.0, secondPair.Cost, 1e-9)
	require.InDelta(t, 6.5, secondPair.UserCost, 1e-9)
	unassignedPair := pairs[fmt.Sprintf("unassigned:%d", activeAccount.ID)]
	require.Nil(t, unassignedPair.GroupID)
	require.Equal(t, int64(1), unassignedPair.Requests)
	require.Equal(t, int64(4), unassignedPair.Tokens)
	require.InDelta(t, 0.5, unassignedPair.Cost, 1e-9)
	historicalPair := pairs[fmt.Sprintf("%d:%d", historicalGroup.ID, historicalAccount.ID)]
	require.Equal(t, historicalGroup.Name, historicalPair.GroupName)
	require.Equal(t, historicalAccount.Name, historicalPair.AccountName)
	require.Equal(t, service.AccountTypeOAuth, historicalPair.AccountType)
	require.Equal(t, service.PlatformAnthropic, historicalPair.AccountPlatform)
	require.Equal(t, int64(1), historicalPair.Requests)
	require.Equal(t, int64(4), historicalPair.Tokens)
	require.InDelta(t, 7.0, historicalPair.Cost, 1e-9)
	require.InDelta(t, 8.0, historicalPair.UserCost, 1e-9)
}

func TestUsageLog_UpstreamModelMismatchFilterAndPartialIndex(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "model-audit@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-model-audit", Name: "model-audit"})
	account := mustCreateAccount(t, client, &service.Account{Name: "model-audit-account"})
	now := time.Now().UTC()
	responseModel := "gpt-5.4"
	for _, mismatch := range []bool{true, false} {
		mismatchValue := mismatch
		_, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			Model: "gpt-5.5", InputTokens: 1, OutputTokens: 1,
			UpstreamResponseModel: &responseModel, UpstreamModelMismatch: &mismatchValue,
			CreatedAt: now,
		})
		require.NoError(t, err)
	}

	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	trueValue := true
	stats, err := repo.GetStatsWithFilters(ctx, usagestats.UsageLogFilters{
		UserID: user.ID, StartTime: &start, EndTime: &end, UpstreamModelMismatch: &trueValue,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalRequests)
	require.Equal(t, []usagestats.EndpointStat{{
		Endpoint: "unknown", Requests: 1, TotalTokens: 2,
	}}, stats.Endpoints)
	require.Equal(t, []usagestats.EndpointStat{{
		Endpoint: "unknown", Requests: 1, TotalTokens: 2,
	}}, stats.UpstreamEndpoints)
	require.Equal(t, []usagestats.EndpointStat{{
		Endpoint: "unknown -> unknown", Requests: 1, TotalTokens: 2,
	}}, stats.EndpointPaths)

	trend, err := repo.GetUsageTrendWithUsageFilters(ctx, start, end, "hour", usagestats.UsageLogFilters{
		UserID: user.ID, UpstreamModelMismatch: &trueValue,
	})
	require.NoError(t, err)
	require.Len(t, trend, 1)
	require.Equal(t, int64(1), trend[0].Requests)

	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)
	assertPlanUsesIndex := func(query, indexName string, args ...any) {
		rows, queryErr := tx.QueryContext(ctx, query, args...)
		require.NoError(t, queryErr)
		var planLines []string
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			planLines = append(planLines, line)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		require.Contains(t, strings.Join(planLines, "\n"), indexName)
	}
	assertPlanUsesIndex(`
EXPLAIN (COSTS OFF)
SELECT id
FROM usage_logs
WHERE upstream_model_mismatch IS TRUE
ORDER BY created_at DESC, id DESC
LIMIT 100
`, usageLogsUpstreamModelMismatchIndex)
	assertPlanUsesIndex(`
EXPLAIN (COSTS OFF)
SELECT id
FROM usage_logs
WHERE COALESCE(NULLIF(TRIM(requested_model), ''), model) = $1
  AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC, id DESC
LIMIT 100
`, usageLogsEffectiveRequestedModelIndex, "gpt-5.5", start, end)
	assertPlanUsesIndex(`
EXPLAIN (COSTS OFF)
SELECT id
FROM usage_logs
WHERE COALESCE(NULLIF(TRIM(upstream_model), ''), model) = $1
  AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC, id DESC
LIMIT 100
`, usageLogsEffectiveUpstreamModelIndex, "gpt-5.5", start, end)
}

func TestUsageLog_GetStatsWithFilters_AggregatesAndEndpoints(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "stats@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-stats-1", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-stats"})

	now := time.Now().UTC()
	inboundEndpoint := "/v1/messages"
	upstreamEndpoint := "/v1/responses"
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			Model: "claude-3", InputTokens: 2, OutputTokens: 3,
			CacheCreationTokens: 4, CacheReadTokens: 5,
			TotalCost: 0.5, ActualCost: 0.4, CreatedAt: now,
			InboundEndpoint: &inboundEndpoint, UpstreamEndpoint: &upstreamEndpoint,
		})
		require.NoError(t, err)
	}

	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)
	// 按本测试创建的 user 维度过滤:集成库为共享实例,其它用 testEntClient 的兄弟测试会留下
	// 已提交的 usage_log 行(含零 token 的失败请求),不限定 user 会把它们计入 TotalRequests。
	stats, err := repo.GetStatsWithFilters(ctx, usagestats.UsageLogFilters{UserID: user.ID, StartTime: &start, EndTime: &end})
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TotalRequests)
	require.Equal(t, int64(6), stats.TotalInputTokens)
	require.Equal(t, int64(9), stats.TotalOutputTokens)
	require.Equal(t, int64(27), stats.TotalCacheTokens)
	require.Equal(t, int64(12), stats.TotalCacheCreationTokens)
	require.Equal(t, int64(15), stats.TotalCacheReadTokens)
	require.InDelta(t, 1.2, stats.TotalActualCost, 1e-9)
	require.NotEmpty(t, stats.Endpoints)
	require.NotEmpty(t, stats.UpstreamEndpoints)
	require.NotEmpty(t, stats.EndpointPaths)
}
