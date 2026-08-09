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

func TestLoadAcceptsFeishuAlertChatWithCompleteAppFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	chatIDFile, recipientsFile := addFeishuAlertFiles(t, env)
	policyFile := addNotificationPolicy(t, env)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("alert configuration rejected: %v", err)
	}
	if cfg.FeishuAlertChatIDFile != chatIDFile {
		t.Fatalf("alert chat file = %q, want %q", cfg.FeishuAlertChatIDFile, chatIDFile)
	}
	if cfg.FeishuAlertRecipientsFile != recipientsFile {
		t.Fatalf("alert recipients file = %q, want %q", cfg.FeishuAlertRecipientsFile, recipientsFile)
	}
	if cfg.NotificationPolicyFile != policyFile {
		t.Fatalf("notification policy file = %q, want %q", cfg.NotificationPolicyFile, policyFile)
	}
}

func TestLoadAcceptsOutboundAlertAppWithoutCallbackSecrets(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	appID, appSecret, chatID, recipients := addOutboundFeishuAlertFiles(t, env)
	policy := addNotificationPolicy(t, env)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("outbound-only alert configuration rejected: %v", err)
	}
	if cfg.FeishuAppIDFile != appID || cfg.FeishuAppSecretFile != appSecret ||
		cfg.FeishuAlertChatIDFile != chatID ||
		cfg.FeishuAlertRecipientsFile != recipients ||
		cfg.NotificationPolicyFile != policy {
		t.Fatalf("outbound Feishu config = %#v", cfg)
	}
}

func TestLoadRequiresNotificationPolicyForConfiguredAlertTransport(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuAlertFiles(t, env)
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("configured alert transport without notification policy was accepted")
	}
}

func TestLoadAcceptsNoPolicyWhenNoAlertTransportExists(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.NotificationPolicyFile != "" {
		t.Fatalf("notification policy file = %q, want empty", cfg.NotificationPolicyFile)
	}
}

func TestLoadRejectsInvalidNotificationPolicyPermissions(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuAlertFiles(t, env)
	path := addNotificationPolicy(t, env)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("world-readable notification policy was accepted")
	}
}

func TestLoadRejectsFeishuAlertChatWithoutRecipients(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addOutboundFeishuAlertFiles(t, env)
	chatIDFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatIDFile, []byte("oc_alert_group"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE"] = chatIDFile
	delete(env, "RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE")
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("alert chat without recipients unexpectedly accepted")
	}
}

func TestLoadRejectsFeishuAlertChatWithoutCompleteAppFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	chatIDFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatIDFile, []byte("oc_alert_group"), 0o600); err != nil {
		t.Fatal(err)
	}
	recipientsFile := filepath.Join(t.TempDir(), "feishu-alert-recipients.json")
	if err := os.WriteFile(recipientsFile, []byte(`{"open_ids":["operator"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE"] = chatIDFile
	env["RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE"] = recipientsFile
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("alert chat without Feishu App files unexpectedly accepted")
	}
}

func TestLoadRejectsIncompleteFeishuAppFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	appID, _, _, _ := addOutboundFeishuAlertFiles(t, env)
	delete(env, "RELAY_OPS_FEISHU_APP_SECRET_FILE")
	env["RELAY_OPS_FEISHU_APP_ID_FILE"] = appID
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("incomplete Feishu App file pair unexpectedly accepted")
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

func TestLoadRequiresNativeOpsAlertReadDatabaseURLFileWhenEnabled(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addNotificationPolicy(t, env)
	delete(env, "RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE")
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("native ops alerts without read database URL file were accepted")
	}
}

func TestLoadRejectsRelativeNativeOpsAlertReadDatabaseURLFile(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addNotificationPolicy(t, env)
	env["RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE"] = "relative-secret"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("relative native ops alert read database URL file was accepted")
	}
}

func TestLoadAcceptsNativeOpsAlertReadDatabaseURLFile(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addNotificationPolicy(t, env)
	path := filepath.Join(t.TempDir(), "sub2api-alert-read-database-url")
	if err := os.WriteFile(path, []byte("postgres://reader:secret@postgres/sub2api"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE"] = path
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sub2APIAlertReadDatabaseURLFile != path {
		t.Fatalf("Sub2APIAlertReadDatabaseURLFile = %q, want %q", cfg.Sub2APIAlertReadDatabaseURLFile, path)
	}
}

func TestLoadAllowsNoNativeOpsAlertReadDatabaseURLFileWhenDisabled(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	path := addNotificationPolicy(t, env)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), `"native_ops_alerts_enabled": true`, `"native_ops_alerts_enabled": false`, 1))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	delete(env, "RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE")
	if _, err := Load(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("disabled native ops alerts rejected without secret path: %v", err)
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

func addFeishuAlertFiles(t *testing.T, env map[string]string) (string, string) {
	t.Helper()
	_, _, chatIDFile, recipientsFile := addOutboundFeishuAlertFiles(t, env)
	return chatIDFile, recipientsFile
}

func addOutboundFeishuAlertFiles(t *testing.T, env map[string]string) (string, string, string, string) {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"RELAY_OPS_FEISHU_APP_ID_FILE":           "cli_test_app",
		"RELAY_OPS_FEISHU_APP_SECRET_FILE":       "app-secret",
		"RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE":    "oc_alert_group",
		"RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE": `{"open_ids":["operator"]}`,
	}
	paths := make(map[string]string, len(files))
	for key, value := range files {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		env[key] = path
		paths[key] = path
	}
	return paths["RELAY_OPS_FEISHU_APP_ID_FILE"],
		paths["RELAY_OPS_FEISHU_APP_SECRET_FILE"],
		paths["RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE"],
		paths["RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE"]
}

func addNotificationPolicy(t *testing.T, env map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notification-policy.json")
	body := `{
		"version": 1,
		"delivery_mode": "shadow",
		"feishu_notifications": {
			"group_runtime_enabled": true,
			"group_capacity_enabled": true,
			"account_impact_enabled": true,
			"native_monitor_evidence_enabled": true,
			"pricing_notice_enabled": true,
			"daily_digest_enabled": true,
			"incident_escalation_enabled": true,
			"native_ops_alerts_enabled": true
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_NOTIFICATION_POLICY_FILE"] = path
	readURLPath := filepath.Join(t.TempDir(), "sub2api-alert-read-database-url")
	if err := os.WriteFile(readURLPath, []byte("postgres://reader:secret@postgres/sub2api"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE"] = readURLPath
	return path
}
