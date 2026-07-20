package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
