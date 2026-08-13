package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountFinancialReconciliationMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("222_account_financial_reconciliation.sql")
	require.NoError(t, err)
	sql := strings.ToLower(strings.Join(strings.Fields(string(sqlBytes)), " "))

	for _, table := range []string{
		"create table if not exists usage_upstream_cost_evidence",
		"create table if not exists usage_cost_reviews",
		"create table if not exists account_daily_financial_values",
		"create table if not exists account_financial_settings",
	} {
		require.Contains(t, sql, table)
	}
	require.Equal(t, 2, strings.Count(sql, "usage_log_id bigint not null unique"))
	for _, fragment := range []string{
		"unique (account_id, business_date)",
		"business_date date not null",
		"revenue_evidence_cutoff_id bigint",
		"revenue_review_cutoff_id bigint",
		"cost_evidence_cutoff_id bigint",
		"cost_review_cutoff_id bigint",
		"create index if not exists idx_usage_upstream_cost_evidence_status_usage_log_id",
		"create index if not exists idx_usage_cost_reviews_usage_log_id",
		"create index if not exists idx_account_daily_financial_values_account_id_business_date",
	} {
		require.Contains(t, sql, fragment)
	}
	for _, column := range []string{
		"sub_actual_cost",
		"newapi_quota",
		"newapi_quota_per_unit",
		"normalized_cost_cny",
		"profit_cny",
		"manual_cost_cny",
		"manual_profit_cny",
		"oauth_cost_cny",
		"revenue_override_cny",
		"cost_override_cny",
	} {
		require.Contains(t, sql, column+" numeric(20,10)")
	}
	for _, forbidden := range []string{
		"alter table usage_logs",
		"update usage_logs",
		"delete from usage_logs",
		"drop table",
		"truncate",
	} {
		require.NotContains(t, sql, forbidden)
	}
	for _, sensitive := range []string{
		"raw_response",
		"request_body",
		"credentials",
		"api_key",
	} {
		require.NotContains(t, sql, sensitive)
	}
}
