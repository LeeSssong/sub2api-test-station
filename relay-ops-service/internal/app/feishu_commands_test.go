package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/commands"
	"example.invalid/relay-ops-service/internal/config"
)

func TestAttachFeishuCommandHandlerExposesOnlyExactPOSTPath(t *testing.T) {
	callbackCalls := 0
	callback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callbackCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	fallbackCalls := 0
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalls++
		w.WriteHeader(http.StatusTeapot)
	})
	handler := AttachFeishuCommandHandler(fallback, callback)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodPost, "/relay-ops/api/feishu/events", http.StatusNoContent},
		{http.MethodGet, "/relay-ops/api/feishu/events", http.StatusTeapot},
		{http.MethodPost, "/relay-ops/api/feishu/events/extra", http.StatusTeapot},
		{http.MethodPost, "/relay-ops/api/acceptance/synthetic", http.StatusTeapot},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
		if recorder.Code != tt.want {
			t.Fatalf("%s %s status=%d want=%d", tt.method, tt.path, recorder.Code, tt.want)
		}
	}
	if callbackCalls != 1 || fallbackCalls != 3 {
		t.Fatalf("callback=%d fallback=%d", callbackCalls, fallbackCalls)
	}
}

func TestConfigureFeishuCommandsLeavesExistingHandlerUntouchedWhenUnconfigured(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	runtime, err := ConfigureFeishuCommands(config.Config{FeishuCommandMode: config.FeishuCommandDisabled}, nil, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Worker != nil {
		t.Fatal("unconfigured disabled runtime unexpectedly created a worker")
	}
	recorder := httptest.NewRecorder()
	runtime.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil))
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("fallback status = %d", recorder.Code)
	}
}

func TestConfigureDisabledFeishuCommandsDoesNotRequireRoutingOrSub2API(t *testing.T) {
	dir := t.TempDir()
	secret := func(name, value string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cfg := config.Config{
		FeishuCommandMode:      config.FeishuCommandDisabled,
		FeishuAppIDFile:        secret("app-id", "cli_test_app"),
		FeishuAppSecretFile:    secret("app-secret", "app-secret"),
		FeishuVerificationFile: secret("verification-token", "verification-token"),
		FeishuEncryptKeyFile:   secret("encrypt-key", "encrypt-key"),
	}
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	runtime, err := ConfigureFeishuCommands(cfg, &FeishuCommandDependencies{Repository: disabledFeishuRepository{}}, fallback)
	if err != nil {
		t.Fatalf("ConfigureFeishuCommands: %v", err)
	}
	if runtime.Worker == nil || runtime.Worker.Router != nil {
		t.Fatalf("worker = %#v", runtime.Worker)
	}
}

type disabledFeishuRepository struct{}

func (disabledFeishuRepository) InsertFeishuEvent(context.Context, commands.Record) (bool, error) {
	return true, nil
}

func (disabledFeishuRepository) ClaimFeishuCommand(context.Context, time.Time, time.Duration) (*commands.Record, error) {
	return nil, nil
}

func (disabledFeishuRepository) CompleteFeishuCommand(context.Context, commands.Completion) error {
	return nil
}

func (disabledFeishuRepository) RecordFeishuReply(context.Context, string, string, bool, string) error {
	return nil
}

func (disabledFeishuRepository) WithFeishuGroupLock(ctx context.Context, _ int64, fn func(context.Context) commands.Completion) (commands.Completion, error) {
	return fn(ctx), nil
}
