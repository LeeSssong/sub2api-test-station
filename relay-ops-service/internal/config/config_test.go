package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesFixedMonitoringCadence(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeReadOnly {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeReadOnly)
	}
	if cfg.ProductionPageInterval != 5*time.Minute {
		t.Fatalf("ProductionPageInterval = %s", cfg.ProductionPageInterval)
	}
	if cfg.CandidateInterval != 6*time.Hour {
		t.Fatalf("CandidateInterval = %s", cfg.CandidateInterval)
	}
	if cfg.TimezoneName != "Asia/Shanghai" {
		t.Fatalf("TimezoneName = %q", cfg.TimezoneName)
	}
	if cfg.CandidateSecretDir != "/var/lib/relay-ops/candidate-keys" {
		t.Fatalf("CandidateSecretDir = %q", cfg.CandidateSecretDir)
	}
	if cfg.FastProfilePath != "/app/config/upstream-benchmarks/quality-first-fast-v1.yaml" {
		t.Fatalf("FastProfilePath = %q", cfg.FastProfilePath)
	}
	if cfg.AnalyzerCommandPath != "/app/ops/analyze-account-monitor.rb" {
		t.Fatalf("AnalyzerCommandPath = %q", cfg.AnalyzerCommandPath)
	}
	if cfg.ComparisonReportSetFile != "/var/lib/relay-ops/comparison-report-sets.jsonl" || cfg.CutoverStateFile != "/var/lib/relay-ops/cutover-state.jsonl" {
		t.Fatalf("cutover paths = %q %q", cfg.ComparisonReportSetFile, cfg.CutoverStateFile)
	}
}

func TestLoadRejectsRelativeCutoverAuthorityPaths(t *testing.T) {
	for key, value := range map[string]string{
		"RELAY_OPS_COMPARISON_REPORT_SET_FILE": "relative/sets.jsonl",
		"RELAY_OPS_CUTOVER_STATE_FILE":         "relative/state.jsonl",
	} {
		t.Run(key, func(t *testing.T) {
			env := validEnv(t)
			env[key] = value
			if _, err := Load(func(name string) string { return env[name] }); err == nil {
				t.Fatalf("accepted %s=%q", key, value)
			}
		})
	}
}

func TestLoadConfiguresExactTrustedProxyHost(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_TRUSTED_PROXY_HOST"] = "caddy"
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TrustedProxyHost != "caddy" {
		t.Fatalf("TrustedProxyHost = %q", cfg.TrustedProxyHost)
	}
}

func TestLoadRequiresStartDateWhenAccountingEnabled(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_ACCOUNTING_ENABLED"] = "true"
	if _, err := Load(func(key string) string { return env[key] }); err == nil ||
		!strings.Contains(err.Error(), "RELAY_OPS_ACCOUNTING_LEDGER_START_DATE") {
		t.Fatalf("error = %v, want start-date requirement", err)
	}
}

func TestLoadParsesAccountingExclusionIDs(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_ACCOUNTING_ENABLED"] = "true"
	env["RELAY_OPS_ACCOUNTING_LEDGER_START_DATE"] = "2026-08-02"
	env["RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS"] = "7, 9"
	env["RELAY_OPS_ACCOUNTING_INTERNAL_API_KEY_IDS"] = "11"
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AccountingEnabled || cfg.AccountingLedgerStartDate.Format("2006-01-02") != "2026-08-02" {
		t.Fatalf("accounting activation = %#v", cfg)
	}
	if got, want := cfg.AccountingInternalUserIDs, []int64{7, 9}; !equalInt64s(got, want) {
		t.Fatalf("internal user IDs = %v, want %v", got, want)
	}
	if got, want := cfg.AccountingInternalAPIKeyIDs, []int64{11}; !equalInt64s(got, want) {
		t.Fatalf("internal API key IDs = %v, want %v", got, want)
	}
}

func TestLoadRejectsInvalidAccountingIDsAndDate(t *testing.T) {
	t.Parallel()

	for name, overrides := range map[string]string{
		"duplicate":  "1,1",
		"zero":       "0",
		"nonnumeric": "x",
	} {
		t.Run(name, func(t *testing.T) {
			env := validEnv(t)
			env["RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS"] = overrides
			if _, err := Load(func(key string) string { return env[key] }); err == nil {
				t.Fatalf("accepted invalid accounting IDs %q", overrides)
			}
		})
	}
	env := validEnv(t)
	env["RELAY_OPS_ACCOUNTING_ENABLED"] = "true"
	env["RELAY_OPS_ACCOUNTING_LEDGER_START_DATE"] = "2026-02-30"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("accepted invalid accounting start date")
	}
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoadAcceptsAbsoluteUpstreamGroupMappingFile(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE"] = "/run/relay-ops/upstream-group-mapping.json"
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UpstreamGroupMappingFile != env["RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE"] {
		t.Fatalf("UpstreamGroupMappingFile = %q", cfg.UpstreamGroupMappingFile)
	}
}

func TestLoadRejectsRelativeUpstreamGroupMappingFile(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE"] = "config/upstream-group-mapping.json"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("Load accepted a relative upstream group mapping file")
	}
}

func TestLoadDefaultsUpstreamGroupMappingFileToDisabled(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UpstreamGroupMappingFile != "" {
		t.Fatalf("UpstreamGroupMappingFile = %q, want empty（不配就当没这功能）", cfg.UpstreamGroupMappingFile)
	}
}

func TestLoadAcceptsAbsoluteCandidateSecretDirectory(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_CANDIDATE_SECRET_DIR"] = "/srv/relay-ops/managed-candidate-keys"
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CandidateSecretDir != env["RELAY_OPS_CANDIDATE_SECRET_DIR"] {
		t.Fatalf("CandidateSecretDir = %q", cfg.CandidateSecretDir)
	}
}

func TestLoadRejectsRelativeCandidateSecretDirectory(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_CANDIDATE_SECRET_DIR"] = "secrets/candidate-keys"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("Load accepted a relative candidate secret directory")
	}
}

func TestLoadAcceptsOnlyKnownModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{ModeReadOnly, ModeProbe, ModeClosed} {
		env := validEnv(t)
		env["RELAY_OPS_MODE"] = mode
		if _, err := Load(func(key string) string { return env[key] }); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	env := validEnv(t)
	env["RELAY_OPS_MODE"] = "write"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("write mode unexpectedly accepted")
	}
}

func TestLoadRejectsWritableSecretFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	if err := os.Chmod(env["RELAY_OPS_SUB2API_ADMIN_KEY_FILE"], 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("writable admin key unexpectedly accepted")
	}
}

func TestLoadRejectsWorldReadableSecretFiles(t *testing.T) {
	t.Parallel()
	env := validEnv(t)
	if err := os.Chmod(env["RELAY_OPS_SUB2API_ADMIN_KEY_FILE"], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("world-readable admin key unexpectedly accepted")
	}
}

func TestLoadAcceptsOnlyAbsoluteAccountQualityResultPath(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE"] = "/run/relay-ops/account-quality/account-quality-result.json"
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountQualityResultFile != env["RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE"] {
		t.Fatalf("account quality result file = %q", cfg.AccountQualityResultFile)
	}

	env["RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE"] = "relative.json"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("relative account quality result path was accepted")
	}
}

func TestLoadKeepsReadOnlyExternalizationOptIn(t *testing.T) {
	t.Parallel()
	env := validEnv(t)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExternalizationEnabled {
		t.Fatal("externalization unexpectedly enabled")
	}
	if cfg.CoreDatabaseURLFile != "" {
		t.Fatalf("core database file=%q", cfg.CoreDatabaseURLFile)
	}
}

func validEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	databaseURL := filepath.Join(dir, "database-url")
	adminKey := filepath.Join(dir, "sub2api-admin-key")
	for path, value := range map[string]string{databaseURL: "postgres://relay:secret@postgres/relay_ops", adminKey: "admin-test-key"} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return map[string]string{
		"RELAY_OPS_MODE":                   ModeReadOnly,
		"RELAY_OPS_DATABASE_URL_FILE":      databaseURL,
		"RELAY_OPS_SUB2API_ADMIN_KEY_FILE": adminKey,
		"RELAY_OPS_SUB2API_URL":            "http://sub2api:8080",
		"RELAY_OPS_TIMEZONE":               "Asia/Shanghai",
		"RELAY_OPS_LISTEN_ADDRESS":         ":8100",
	}
}
