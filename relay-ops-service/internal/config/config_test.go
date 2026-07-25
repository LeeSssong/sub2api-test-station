package config

import (
	"os"
	"path/filepath"
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
	if cfg.FeishuCommandMode != FeishuCommandDisabled {
		t.Fatalf("FeishuCommandMode = %q, want %q", cfg.FeishuCommandMode, FeishuCommandDisabled)
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

func TestLoadAcceptsKnownFeishuCommandModesWithCompleteFiles(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{FeishuCommandDisabled, FeishuCommandDryRun, FeishuCommandEnabled} {
		env := validEnv(t)
		addFeishuCommandFiles(t, env)
		env["RELAY_OPS_FEISHU_COMMAND_MODE"] = mode
		cfg, err := Load(func(key string) string { return env[key] })
		if err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
		if cfg.FeishuCommandMode != mode {
			t.Fatalf("mode = %q, want %q", cfg.FeishuCommandMode, mode)
		}
	}
}

func TestLoadDisabledAcceptsCallbackFilesWithoutRouting(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuCallbackFiles(t, env)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("disabled callback configuration rejected: %v", err)
	}
	if cfg.FeishuRoutingFile != "" {
		t.Fatalf("routing file = %q, want empty", cfg.FeishuRoutingFile)
	}
}

func TestLoadAcceptsFeishuAlertChatWithCompleteAppFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuCallbackFiles(t, env)
	chatIDFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatIDFile, []byte("oc_alert_group"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE"] = chatIDFile
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("alert configuration rejected: %v", err)
	}
	if cfg.FeishuAlertChatIDFile != chatIDFile {
		t.Fatalf("alert chat file = %q, want %q", cfg.FeishuAlertChatIDFile, chatIDFile)
	}
}

func TestLoadRejectsFeishuAlertChatWithoutCompleteAppFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	chatIDFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatIDFile, []byte("oc_alert_group"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE"] = chatIDFile
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("alert chat without Feishu App files unexpectedly accepted")
	}
}

func TestLoadRejectsUnknownFeishuCommandMode(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuCommandFiles(t, env)
	env["RELAY_OPS_FEISHU_COMMAND_MODE"] = "write"
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("unknown Feishu command mode unexpectedly accepted")
	}
}

func TestLoadRequiresCompleteFeishuCommandFiles(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{FeishuCommandDryRun, FeishuCommandEnabled} {
		env := validEnv(t)
		addFeishuCommandFiles(t, env)
		env["RELAY_OPS_FEISHU_COMMAND_MODE"] = mode
		delete(env, "RELAY_OPS_FEISHU_APP_SECRET_FILE")
		if _, err := Load(func(key string) string { return env[key] }); err == nil {
			t.Fatalf("mode %q accepted an incomplete file set", mode)
		}
	}
}

func TestLoadDisabledRejectsPartialFeishuCommandFiles(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuCommandFiles(t, env)
	delete(env, "RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE")
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("disabled mode accepted a partial file set")
	}
}

func TestLoadRejectsInsecureFeishuCommandFile(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	addFeishuCommandFiles(t, env)
	env["RELAY_OPS_FEISHU_COMMAND_MODE"] = FeishuCommandDryRun
	if err := os.Chmod(env["RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE"], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(func(key string) string { return env[key] }); err == nil {
		t.Fatal("world-readable Feishu secret unexpectedly accepted")
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

func addFeishuCommandFiles(t *testing.T, env map[string]string) {
	t.Helper()
	addFeishuCallbackFiles(t, env)
	dir := t.TempDir()
	path := filepath.Join(dir, "RELAY_OPS_FEISHU_ROUTING_FILE")
	if err := os.WriteFile(path, []byte(`{"groups":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env["RELAY_OPS_FEISHU_ROUTING_FILE"] = path
}

func addFeishuCallbackFiles(t *testing.T, env map[string]string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"RELAY_OPS_FEISHU_APP_ID_FILE":             "cli_test_app",
		"RELAY_OPS_FEISHU_APP_SECRET_FILE":         "app-secret",
		"RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE": "verification-token",
		"RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE":        "encrypt-key",
	}
	for key, value := range files {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		env[key] = path
	}
}
