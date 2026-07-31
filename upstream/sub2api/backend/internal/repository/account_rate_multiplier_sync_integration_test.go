//go:build integration

package repository

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestManagedBillingMultiplierPersistsQuantizedValueIdempotentlyAndRefreshesCache(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)
	auditRepo := &upstreamBillingMultiplierAuditCapture{}
	auditLog := service.NewAuditLogService(auditRepo, nil)
	auditLog.Start()
	auditStopped := false
	defer func() {
		if !auditStopped {
			auditLog.Stop()
		}
	}()

	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:        fmt.Sprintf("rate-multiplier-quantized-%d", time.Now().UnixNano()),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-integration"},
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_groups WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
	})
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		UPDATE accounts
		SET rate_multiplier = 1.0000, updated_at = NOW()
		WHERE id = $1
		RETURNING rate_multiplier
	`, account.ID).Scan(new(float64)))

	repo := newAccountRepositoryWithSQLAndAudit(integrationEntClient, integrationDB, cache, auditLog)
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	observedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	snapshot := &service.UpstreamBillingProbeSnapshot{
		Status:     service.UpstreamBillingProbeStatusOK,
		ReceivedAt: &observedAt,
		Data:       map[string]any{"effective_rate_multiplier": 0.249975},
	}
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(ctx, loaded, snapshot))

	var persisted string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT rate_multiplier::text FROM accounts WHERE id = $1", account.ID,
	).Scan(&persisted))
	require.Equal(t, "0.2500", persisted)

	reloaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, 0.25, reloaded.BillingRateMultiplier())
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(ctx, reloaded, snapshot), "a repeated high-precision probe must be a multiplier no-op")

	auditLog.Stop()
	auditStopped = true
	require.Len(t, auditRepo.logs, 1, "only the first probe should change and audit the multiplier")
	require.Equal(t, 0.25, auditRepo.logs[0].Extra["new_rate_multiplier"])

	cached, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.Equal(t, 0.25, cached.BillingRateMultiplier())
}

func TestSchedulerCacheNewerManualSnapshotWinsAgainstStaleManagedWriteIntegration(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)
	base := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	managedRate := 0.25
	manualRate := 0.8
	staleManaged := service.Account{
		ID:             99117,
		Name:           "stale-managed",
		Platform:       service.PlatformOpenAI,
		Type:           service.AccountTypeAPIKey,
		RateMultiplier: &managedRate,
		UpdatedAt:      base,
	}
	newerManual := staleManaged
	newerManual.Name = "newer-manual"
	newerManual.RateMultiplier = &manualRate
	newerManual.UpdatedAt = base.Add(time.Second)

	require.NoError(t, cache.SetAccount(ctx, &newerManual))
	require.NoError(t, cache.SetAccount(ctx, &staleManaged))

	cached, err := cache.GetAccount(ctx, staleManaged.ID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.Equal(t, newerManual.Name, cached.Name)
	require.Equal(t, manualRate, cached.BillingRateMultiplier())

	id := strconv.FormatInt(staleManaged.ID, 10)
	fullRaw, err := rdb.Get(ctx, schedulerAccountKey(id)).Result()
	require.NoError(t, err)
	metaRaw, err := rdb.Get(ctx, schedulerAccountMetaKey(id)).Result()
	require.NoError(t, err)
	full, err := decodeCachedAccount(fullRaw)
	require.NoError(t, err)
	meta, err := decodeCachedAccount(metaRaw)
	require.NoError(t, err)
	require.Equal(t, newerManual.Name, full.Name)
	require.Equal(t, manualRate, full.BillingRateMultiplier())
	require.Equal(t, newerManual.Name, meta.Name)
	require.Equal(t, manualRate, meta.BillingRateMultiplier())
}

func TestProbeTransactionStartedBeforeNormalEditPublishesStrictlyNewerCacheVersion(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, cache)
	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:        fmt.Sprintf("probe-version-order-%d", time.Now().UnixNano()),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-version-order"},
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey:         true,
			service.UpstreamBillingRateMultiplierPolicyExtraKey: service.UpstreamBillingRateMultiplierPolicyManaged,
		},
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_groups WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
	})

	probeInput, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	probeTx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	probeCommitted := false
	t.Cleanup(func() {
		if !probeCommitted {
			_ = probeTx.Rollback()
		}
	})

	rows, err := probeTx.Client().QueryContext(ctx, "SELECT transaction_timestamp()")
	require.NoError(t, err)
	require.True(t, rows.Next())
	var probeTransactionStartedAt time.Time
	require.NoError(t, rows.Scan(&probeTransactionStartedAt))
	require.NoError(t, rows.Close())

	editInput, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	editInput.Name += "-edited"
	require.NoError(t, repo.Update(ctx, editInput))
	editUpdatedAt := editInput.UpdatedAt
	require.True(t, editUpdatedAt.After(probeTransactionStartedAt),
		"the normal edit must commit after the probe transaction has already started")

	cachedAfterEdit, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, cachedAfterEdit)
	require.Equal(t, editInput.Name, cachedAfterEdit.Name)
	require.Equal(t, editUpdatedAt, cachedAfterEdit.UpdatedAt)
	require.NotContains(t, cachedAfterEdit.Extra, service.UpstreamBillingProbeExtraKey)

	observedAt := time.Now().UTC()
	snapshot := &service.UpstreamBillingProbeSnapshot{
		Status:        service.UpstreamBillingProbeStatusOK,
		LastAttemptAt: observedAt,
		ReceivedAt:    &observedAt,
		Data:          map[string]any{"source": "version-order-integration"},
	}
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(dbent.NewTxContext(ctx, probeTx), probeInput, snapshot))
	require.NoError(t, probeTx.Commit())
	probeCommitted = true

	finalAccount, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, finalAccount.UpdatedAt.After(editUpdatedAt),
		"a probe committing later must receive a strictly newer row version than the intervening edit")
	require.Equal(t, editInput.Name, finalAccount.Name)
	require.Contains(t, finalAccount.Extra, service.UpstreamBillingProbeExtraKey)

	// The cache still holds the edit version until the committed probe publishes
	// its final database snapshot. Its strictly newer row version must be accepted.
	repo.syncSchedulerAccountSnapshot(ctx, account.ID)
	cachedFinal, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, cachedFinal)
	require.Equal(t, finalAccount.UpdatedAt, cachedFinal.UpdatedAt)
	require.Equal(t, editInput.Name, cachedFinal.Name)
	require.Contains(t, cachedFinal.Extra, service.UpstreamBillingProbeExtraKey)
}
