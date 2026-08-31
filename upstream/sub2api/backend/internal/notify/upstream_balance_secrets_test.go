package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUpstreamBalanceSecretsAcceptsProtectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := UpstreamBalanceSecretPaths{
		AppID:      writeProtectedFile(t, dir, "app-id", "fake-app-id"),
		AppSecret:  writeProtectedFile(t, dir, "app-secret", "fake-app-secret"),
		ChatID:     writeProtectedFile(t, dir, "chat-id", "oc_fake_team"),
		Recipients: writeProtectedFile(t, dir, "recipients.json", `{"open_ids":["ou-fake-a","ou-fake-b"]}`),
		Registry: writeProtectedFile(t, dir, "registry.json", `{
          "version": 1,
          "entries": {
            "HTTPS://UPSTREAM.example/": {"login_account":"registry-user.invalid","login_password":"fake-password-value"}
          }
        }`),
	}
	secrets, err := LoadUpstreamBalanceSecrets(paths)
	if err != nil {
		t.Fatal(err)
	}
	if secrets.AppID != "fake-app-id" || secrets.AppSecret != "fake-app-secret" || secrets.ChatID != "oc_fake_team" || len(secrets.RecipientOpenIDs) != 2 {
		t.Fatalf("loaded secrets have wrong non-sensitive shape")
	}
	account, password, ok := secrets.Registry.Lookup("https://upstream.example")
	if !ok || account != "registry-user.invalid" || password != "fake-password-value" {
		t.Fatalf("registry lookup = %q, %q, %v", account, password, ok)
	}
	account, password, ok = secrets.Registry.Lookup("https://missing.example")
	if ok || account != UnregisteredValue || password != UnregisteredValue {
		t.Fatalf("missing registry lookup = %q, %q, %v", account, password, ok)
	}
}

func TestLoadUpstreamBalanceSecretsFailsClosedForUnsafeOrInvalidRegistry(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		mutate   func(t *testing.T, paths *UpstreamBalanceSecretPaths)
		wantCode string
	}{
		{name: "unknown field", registry: `{"version":1,"entries":{},"extra":true}`, wantCode: SecretErrorInvalidJSON},
		{name: "trailing JSON", registry: `{"version":1,"entries":{}} {}`, wantCode: SecretErrorInvalidJSON},
		{name: "duplicate normalized URL", registry: `{"version":1,"entries":{"https://same.example":{"login_account":"one","login_password":"fake-one"},"HTTPS://SAME.example/":{"login_account":"two","login_password":"fake-two"}}}`, wantCode: SecretErrorRegistryConflict},
		{name: "empty URL", registry: `{"version":1,"entries":{"":{"login_account":"one","login_password":"fake-one"}}}`, wantCode: SecretErrorRegistryInvalidKey},
		{name: "unsafe file mode", registry: `{"version":1,"entries":{}}`, mutate: func(t *testing.T, paths *UpstreamBalanceSecretPaths) {
			if err := os.Chmod(paths.Registry, 0o644); err != nil {
				t.Fatal(err)
			}
		}, wantCode: SecretErrorUnsafeFile},
		{name: "unsafe parent mode", registry: `{"version":1,"entries":{}}`, mutate: func(t *testing.T, paths *UpstreamBalanceSecretPaths) {
			if err := os.Chmod(filepath.Dir(paths.Registry), 0o755); err != nil {
				t.Fatal(err)
			}
		}, wantCode: SecretErrorUnsafeParent},
		{name: "symlink", registry: `{"version":1,"entries":{}}`, mutate: func(t *testing.T, paths *UpstreamBalanceSecretPaths) {
			target := paths.Registry
			link := filepath.Join(filepath.Dir(target), "registry-link.json")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			paths.Registry = link
		}, wantCode: SecretErrorUnsafeFile},
		{name: "oversized", registry: `{"version":1,"entries":{}}`, mutate: func(t *testing.T, paths *UpstreamBalanceSecretPaths) {
			if err := os.WriteFile(paths.Registry, []byte(strings.Repeat("x", (1<<20)+1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, wantCode: SecretErrorUnsafeFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			paths := completeSecretPaths(t, dir, tt.registry)
			if tt.mutate != nil {
				tt.mutate(t, &paths)
			}
			_, err := LoadUpstreamBalanceSecrets(paths)
			if err == nil || SecretErrorCode(err) != tt.wantCode {
				t.Fatalf("error = %v code=%q, want %q", err, SecretErrorCode(err), tt.wantCode)
			}
			if strings.Contains(err.Error(), "fake-password") || strings.Contains(err.Error(), "registry-user") {
				t.Fatalf("error leaked protected content: %v", err)
			}
		})
	}
}

func completeSecretPaths(t *testing.T, dir, registry string) UpstreamBalanceSecretPaths {
	t.Helper()
	return UpstreamBalanceSecretPaths{
		AppID: writeProtectedFile(t, dir, "app-id", "fake-app-id"), AppSecret: writeProtectedFile(t, dir, "app-secret", "fake-app-secret"),
		ChatID: writeProtectedFile(t, dir, "chat-id", "oc_fake"), Recipients: writeProtectedFile(t, dir, "recipients.json", `{"open_ids":["ou-fake"]}`),
		Registry: writeProtectedFile(t, dir, "registry.json", registry),
	}
}

func writeProtectedFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
