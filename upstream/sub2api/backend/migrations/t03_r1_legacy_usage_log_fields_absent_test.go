package migrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestT03R1LegacyUsageLogFieldsAreAbsent(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
	for _, path := range []string{
		filepath.Join(backendRoot, "ent", "schema", "usage_log.go"),
		filepath.Join(backendRoot, "ent", "usagelog", "usagelog.go"),
		filepath.Join(backendRoot, "ent", "usagelog_create.go"),
		filepath.Join(backendRoot, "ent", "usagelog_update.go"),
		filepath.Join(backendRoot, "internal", "repository", "usage_log_repo_insert.go"),
		filepath.Join(backendRoot, "internal", "repository", "usage_log_repo_query.go"),
		filepath.Join(backendRoot, "internal", "service", "usage_log.go"),
	} {
		data, err := os.ReadFile(path)
		require.NoError(t, err, path)
		usageSchemaAndGeneratedCode := string(data)
		for _, forbidden := range []string{
			"upstream_actual_cost",
			"upstream_cost_status",
			"upstream_cost_reason",
			"upstream_cost_recorded_at",
			"profit",
		} {
			require.NotContains(t, usageSchemaAndGeneratedCode, forbidden, path)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(currentFile))
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(filepath.Dir(currentFile), entry.Name()))
		require.NoError(t, readErr, entry.Name())
		sql := strings.ToLower(string(data))
		for _, forbidden := range []string{
			"upstream_actual_cost",
			"upstream_cost_status",
			"upstream_cost_reason",
			"upstream_cost_recorded_at",
		} {
			require.NotContains(t, sql, forbidden, entry.Name())
		}
	}
	require.NoFileExists(t, filepath.Join(filepath.Dir(currentFile), "221_usage_log_upstream_cost_persistence.sql"))
}
