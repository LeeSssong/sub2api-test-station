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
