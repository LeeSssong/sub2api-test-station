package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"github.com/stretchr/testify/require"
)

func TestAccountFinancialReconciliationUsageLogUniqueIndexes(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, schema := range spec.Schemas {
		schemas[schema.Name] = schema
	}

	requireUniqueIndexWithStorageKey(
		t,
		requireSchema(t, schemas, "UsageUpstreamCostEvidence"),
		"usage_upstream_cost_evidence_usage_log_id_key",
		"usage_log_id",
	)
	requireUniqueIndexWithStorageKey(
		t,
		requireSchema(t, schemas, "UsageCostReview"),
		"usage_cost_reviews_usage_log_id_key",
		"usage_log_id",
	)
}

func requireUniqueIndexWithStorageKey(t *testing.T, schema *load.Schema, storageKey string, fields ...string) {
	t.Helper()

	for _, index := range schema.Indexes {
		if !index.Unique || index.StorageKey != storageKey {
			continue
		}
		require.Equal(t, fields, index.Fields)
		return
	}

	require.Failf(t, "missing unique index", "schema %s should include unique index %s on %v", schema.Name, storageKey, fields)
}
