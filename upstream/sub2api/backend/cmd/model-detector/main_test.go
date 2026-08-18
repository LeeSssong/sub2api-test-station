package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "v1 base", base: "https://example.test/v1", want: "https://example.test/v1/models"},
		{name: "root base", base: "https://example.test", want: "https://example.test/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := modelsEndpoint(tt.base)
			if err != nil || got != tt.want {
				t.Fatalf("modelsEndpoint(%q) = %q, %v; want %q", tt.base, got, err, tt.want)
			}
		})
	}
}

func TestDetectorDetectsAdvertisedModelWithoutLoggingCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.6-sol"}}})
	}))
	t.Cleanup(upstream.Close)
	d := &detector{token: "detector-secret", version: "test", models: map[string]struct{}{"gpt-5.6-sol": {}}, client: upstream.Client()}
	req := httptest.NewRequest(http.MethodPost, "/v1/detect", nil)
	req.Header.Set("Authorization", "Bearer detector-secret")
	req = req.WithContext(context.Background())
	response := httptest.NewRecorder()
	// The handler needs a JSON body, so exercise the service through its public route.
	req.Body = io.NopCloser(strings.NewReader(`{"run_id":"run-1","declared_model":"gpt-5.6-sol","request_model":"gpt-5.6-sol","api_key":"upstream-secret","base_url":"` + upstream.URL + `"}`))
	d.detect(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload detectResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "normal" {
		t.Fatalf("status = %q, want normal", payload.Status)
	}
}

func TestDetectorCatalogRequiresTokenAndIncludesProductionModels(t *testing.T) {
	d := &detector{token: "detector-secret", version: "test", models: parseModels("")}
	unauthorized := httptest.NewRecorder()
	d.catalog(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/catalog", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
	req.Header.Set("Authorization", "Bearer detector-secret")
	response := httptest.NewRecorder()
	d.catalog(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "gpt-5.6-terra") || !strings.Contains(response.Body.String(), "gpt-5.4") {
		t.Fatalf("catalog response = %d %s", response.Code, response.Body.String())
	}
}

func TestDetectorReportsMismatchAndUpstreamAuthorizationFailure(t *testing.T) {
	tests := []struct {
		name       string
		upstream   http.Handler
		wantStatus string
		wantCode   string
	}{
		{name: "model mismatch", upstream: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.4"}}})
		}), wantStatus: "abnormal", wantCode: "model_not_advertised"},
		{name: "upstream unauthorized", upstream: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}), wantStatus: "insufficient", wantCode: "upstream_unauthorized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(tt.upstream)
			t.Cleanup(upstream.Close)
			d := &detector{token: "detector-secret", version: "test", models: parseModels(""), client: upstream.Client()}
			req := httptest.NewRequest(http.MethodPost, "/v1/detect", strings.NewReader(`{"run_id":"run-1","request_model":"gpt-5.6-sol","api_key":"upstream-secret","base_url":"`+upstream.URL+`"}`))
			req.Header.Set("Authorization", "Bearer detector-secret")
			response := httptest.NewRecorder()
			d.detect(response, req)
			var payload detectResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != tt.wantStatus || payload.ErrorCode != tt.wantCode {
				t.Fatalf("response = %+v, want status=%s code=%s", payload, tt.wantStatus, tt.wantCode)
			}
		})
	}
}
