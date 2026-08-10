//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageLogBestEffortExternalizationCommitsUsageAndOutboxTogether(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("externalization-%s@example.test", suffix)})
	account := mustCreateAccount(t, client, &service.Account{Name: "externalization-" + suffix})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-externalization-" + suffix, Name: "externalization"})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM externalization_outbox WHERE payload->>'request_id' = $1", "req-"+suffix)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM usage_logs WHERE request_id = $1", "req-"+suffix)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	repo := newUsageLogRepositoryWithSQL(nil, integrationDB)
	log := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "req-" + suffix, Model: "gpt-test", InputTokens: 1, OutputTokens: 2, TotalCost: 1, ActualCost: .5, CreatedAt: time.Now().UTC()}
	require.NoError(t, repo.CreateBestEffort(ctx, log))
	var usages, events int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1", log.RequestID).Scan(&usages))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM externalization_outbox WHERE payload->>'request_id' = $1", log.RequestID).Scan(&events))
	require.Equal(t, 1, usages)
	require.Equal(t, 1, events)
}

func TestUsageLogBestEffortExternalizationRollsBackOnAppendFailure(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("externalization-rollback-%s@example.test", suffix)})
	account := mustCreateAccount(t, client, &service.Account{Name: "externalization-rollback-" + suffix})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-externalization-rollback-" + suffix, Name: "externalization"})
	triggerName := "externalization_outbox_fail_" + strings.ReplaceAll(suffix, "-", "")
	functionName := triggerName + "_fn"
	require.NoError(t, applyExternalizationOutboxFailureTrigger(ctx, triggerName, functionName))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON externalization_outbox")
		_, _ = integrationDB.ExecContext(ctx, "DROP FUNCTION IF EXISTS "+functionName+"()")
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM usage_logs WHERE request_id = $1", "req-rollback-"+suffix)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	repo := newUsageLogRepositoryWithSQL(nil, integrationDB)
	log := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "req-rollback-" + suffix, Model: "gpt-test", TotalCost: 1, ActualCost: .5, CreatedAt: time.Now().UTC()}
	require.Error(t, repo.CreateBestEffort(ctx, log))
	var usages, events int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1", log.RequestID).Scan(&usages))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM externalization_outbox WHERE payload->>'request_id' = $1", log.RequestID).Scan(&events))
	require.Zero(t, usages)
	require.Zero(t, events)
}

func applyExternalizationOutboxFailureTrigger(ctx context.Context, triggerName, functionName string) error {
	_, err := integrationDB.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'intentional outbox append failure'; END;
		$$;
		CREATE TRIGGER %s BEFORE INSERT ON externalization_outbox
		FOR EACH ROW EXECUTE FUNCTION %s();`, functionName, triggerName, functionName))
	return err
}
