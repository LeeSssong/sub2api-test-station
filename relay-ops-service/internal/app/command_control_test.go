package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMountHandlersExposesOnlyTheExactFeishuCallback(t *testing.T) {
	operationsCalls := 0
	callbackCalls := 0
	operations := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		operationsCalls++
		w.WriteHeader(http.StatusTeapot)
	})
	callback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callbackCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mountHandlers(operations, callback)

	for _, test := range []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantOps    int
		wantCalls  int
	}{
		{name: "exact post", method: http.MethodPost, path: "/relay-ops/api/feishu/events", wantStatus: http.StatusNoContent, wantOps: 0, wantCalls: 1},
		{name: "get stays protected", method: http.MethodGet, path: "/relay-ops/api/feishu/events", wantStatus: http.StatusTeapot, wantOps: 1, wantCalls: 1},
		{name: "adjacent post stays protected", method: http.MethodPost, path: "/relay-ops/api/feishu/events/extra", wantStatus: http.StatusTeapot, wantOps: 2, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != test.wantStatus || operationsCalls != test.wantOps || callbackCalls != test.wantCalls {
				t.Fatalf("status=%d operations=%d callback=%d", recorder.Code, operationsCalls, callbackCalls)
			}
		})
	}
}

func TestMountHandlersOmitsCallbackWhenCommandControlIsUnconfigured(t *testing.T) {
	operations := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	recorder := httptest.NewRecorder()
	mountHandlers(operations, nil).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil),
	)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}
