//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountProcurementAuditParametersUsePostgreSQLTypes(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	t.Run("save rolls back parser failure before casts", func(t *testing.T) {
		prefix := fmt.Sprintf("t35-save-%d", time.Now().UnixNano())
		account := createT35ProcurementAccount(t, client, prefix)
		requestID := prefix + "-request"
		cost, quota := 120.0, 60.0

		err := service.NewAccountProfitabilityService(integrationDB).UpdateProcurementConfig(ctx, service.ProcurementConfigInput{
			AccountID: account.ID, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: requestID,
		})
		if err != nil {
			require.ErrorContains(t, err, "could not determine data type of parameter")
			require.Zero(t, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM account_procurement_cost_versions WHERE account_id=$1 AND request_id=$2`, account.ID, requestID))
			var costIsNull, quotaIsNull bool
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT procurement_cost_cny IS NULL, estimated_usable_quota_usd IS NULL FROM accounts WHERE id=$1`, account.ID).Scan(&costIsNull, &quotaIsNull))
			require.True(t, costIsNull)
			require.True(t, quotaIsNull)
			require.Zero(t, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM audit_logs WHERE request_id=$1`, requestID))
		}
		require.NoError(t, err)

		var savedCost, savedQuota float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT procurement_cost_cny::double precision, estimated_usable_quota_usd::double precision FROM accounts WHERE id=$1`, account.ID).Scan(&savedCost, &savedQuota))
		require.Equal(t, cost, savedCost)
		require.Equal(t, quota, savedQuota)
		var accountType, costType, quotaType string
		var savedAccountID int64
		var auditCost, auditQuota float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT jsonb_typeof(extra->'account_id'), (extra->>'account_id')::bigint, jsonb_typeof(extra->'cost_cny'), (extra->>'cost_cny')::double precision, jsonb_typeof(extra->'quota_usd'), (extra->>'quota_usd')::double precision FROM audit_logs WHERE request_id=$1 ORDER BY id DESC LIMIT 1`, requestID).Scan(&accountType, &savedAccountID, &costType, &auditCost, &quotaType, &auditQuota))
		require.Equal(t, "number", accountType)
		require.Equal(t, account.ID, savedAccountID)
		require.Equal(t, "number", costType)
		require.Equal(t, cost, auditCost)
		require.Equal(t, "number", quotaType)
		require.Equal(t, quota, auditQuota)
	})

	t.Run("clear rolls back parser failure before casts", func(t *testing.T) {
		prefix := fmt.Sprintf("t35-clear-%d", time.Now().UnixNano())
		account := createT35ProcurementAccount(t, client, prefix)
		effective := time.Now().UTC().Add(-time.Hour)
		account.Update().SetProcurementCostCny(100).SetEstimatedUsableQuotaUsd(50).SetProcurementCostEffectiveAt(effective).SaveX(ctx)
		seedT35ProcurementVersion(t, ctx, account.ID, 1, 100, 50, effective, "active", prefix+"-seed")
		requestID := prefix + "-request"

		err := service.NewAccountProfitabilityService(integrationDB).UpdateProcurementConfig(ctx, service.ProcurementConfigInput{
			AccountID: account.ID, ActorUserID: 77, RequestID: requestID,
		})
		if err != nil {
			require.ErrorContains(t, err, "could not determine data type of parameter")
			var status string
			var ended bool
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, ended_at IS NOT NULL FROM account_procurement_cost_versions WHERE account_id=$1 AND version_no=1`, account.ID).Scan(&status, &ended))
			require.Equal(t, "active", status)
			require.False(t, ended)
			require.Equal(t, 0, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM account_procurement_cost_versions WHERE account_id=$1 AND status='cost_pending'`, account.ID))
			var costIsNull, quotaIsNull bool
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT procurement_cost_cny IS NULL, estimated_usable_quota_usd IS NULL FROM accounts WHERE id=$1`, account.ID).Scan(&costIsNull, &quotaIsNull))
			require.False(t, costIsNull)
			require.False(t, quotaIsNull)
			require.Zero(t, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM audit_logs WHERE request_id=$1`, requestID))
		}
		require.NoError(t, err)

		var status string
		var ended bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, ended_at IS NOT NULL FROM account_procurement_cost_versions WHERE account_id=$1 AND version_no=1`, account.ID).Scan(&status, &ended))
		require.Equal(t, "ended", status)
		require.True(t, ended)
		require.Equal(t, 1, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM account_procurement_cost_versions WHERE account_id=$1 AND status='cost_pending' AND request_id=$2`, account.ID, requestID))
		var costIsNull, quotaIsNull bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT procurement_cost_cny IS NULL, estimated_usable_quota_usd IS NULL FROM accounts WHERE id=$1`, account.ID).Scan(&costIsNull, &quotaIsNull))
		require.True(t, costIsNull)
		require.True(t, quotaIsNull)
		var accountType string
		var savedAccountID int64
		var cleared string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT jsonb_typeof(extra->'account_id'), (extra->>'account_id')::bigint, extra->>'cleared' FROM audit_logs WHERE request_id=$1 ORDER BY id DESC LIMIT 1`, requestID).Scan(&accountType, &savedAccountID, &cleared))
		require.Equal(t, "number", accountType)
		require.Equal(t, account.ID, savedAccountID)
		require.Equal(t, "true", cleared)
	})

	t.Run("settle rolls back parser failure before casts", func(t *testing.T) {
		prefix := fmt.Sprintf("t35-settle-%d", time.Now().UnixNano())
		account := createT35ProcurementAccount(t, client, prefix)
		account.Update().SetStatus(service.StatusError).SaveX(ctx)
		effective := time.Now().UTC().Add(-time.Hour)
		seedT35ProcurementVersion(t, ctx, account.ID, 1, 100, 50, effective, "active", prefix+"-seed")
		requestID := prefix + "-request"

		ok, err := service.NewAccountProfitabilityService(integrationDB).SettleProcurement(ctx, service.ProcurementSettlementInput{
			AccountID: account.ID, RequestID: requestID, Reason: "administrator_confirmed_expired", ActorUserID: 77,
		})
		if err != nil {
			require.False(t, ok)
			require.ErrorContains(t, err, "could not determine data type of parameter")
			var status string
			var settlementRequest sql.NullString
			var loss float64
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, settlement_request_id, loss_cny::double precision FROM account_procurement_cost_versions WHERE account_id=$1 AND version_no=1`, account.ID).Scan(&status, &settlementRequest, &loss))
			require.Equal(t, "active", status)
			require.False(t, settlementRequest.Valid)
			require.Zero(t, loss)
			require.Zero(t, t35ProcurementCount(t, ctx, `SELECT COUNT(*) FROM audit_logs WHERE request_id=$1`, requestID))
		}
		require.NoError(t, err)
		require.True(t, ok)

		var status string
		var settlementRequest string
		var loss float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, settlement_request_id, loss_cny::double precision FROM account_procurement_cost_versions WHERE account_id=$1 AND version_no=1`, account.ID).Scan(&status, &settlementRequest, &loss))
		require.Equal(t, "settled", status)
		require.Equal(t, requestID, settlementRequest)
		require.Equal(t, 100.0, loss)
		var accountType, reasonType, reason string
		var savedAccountID int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT jsonb_typeof(extra->'account_id'), (extra->>'account_id')::bigint, jsonb_typeof(extra->'reason'), extra->>'reason' FROM audit_logs WHERE request_id=$1 ORDER BY id DESC LIMIT 1`, requestID).Scan(&accountType, &savedAccountID, &reasonType, &reason))
		require.Equal(t, "number", accountType)
		require.Equal(t, account.ID, savedAccountID)
		require.Equal(t, "string", reasonType)
		require.Equal(t, "administrator_confirmed_expired", reason)
	})
}

func createT35ProcurementAccount(t *testing.T, client *ent.Client, name string) *ent.Account {
	t.Helper()
	return client.Account.Create().SetName(name).SetPlatform(service.PlatformOpenAI).SetType(service.AccountTypeOAuth).SetStatus(service.StatusActive).SaveX(context.Background())
}

func seedT35ProcurementVersion(t *testing.T, ctx context.Context, accountID int64, version int, cost, quota float64, effective time.Time, status, requestID string) {
	t.Helper()
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO account_procurement_cost_versions(account_id,version_no,cost_cny,estimated_usable_quota_usd,effective_at,status,request_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$5,$5)`, accountID, version, cost, quota, effective, status, requestID)
	require.NoError(t, err)
}

func t35ProcurementCount(t *testing.T, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, query, args...).Scan(&count))
	return count
}
