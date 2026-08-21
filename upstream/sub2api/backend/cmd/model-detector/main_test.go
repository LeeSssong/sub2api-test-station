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

func TestResponsesEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "v1 base", base: "https://example.test/v1", want: "https://example.test/v1/responses"},
		{name: "root base", base: "https://example.test", want: "https://example.test/v1/responses"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := responsesEndpoint(tt.base)
			if err != nil || got != tt.want {
				t.Fatalf("responsesEndpoint(%q) = %q, %v; want %q", tt.base, got, err, tt.want)
			}
		})
	}
}

func TestDetectorDetectsAdvertisedModelWithoutLoggingCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.6-sol"}}})
		case "/v1/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_1", "model": "gpt-5.6-sol", "output": []any{}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
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

func TestDetectorCapturesUpstreamResponseModelEvidence(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.6-sol"}, {"id": "gpt-5.4"}}})
		case "/v1/responses":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "gpt-5.6-sol" || body["input"] != "Reply with exactly OK." || body["max_output_tokens"] != float64(8) || body["stream"] != false {
				t.Fatalf("probe body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "resp_1",
				"model":  "gpt-5.4",
				"output": []any{map[string]any{"content": "must-not-persist"}},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(upstream.Close)
	d := &detector{token: "detector-secret", version: "test", models: parseModels(""), client: upstream.Client()}
	req := httptest.NewRequest(http.MethodPost, "/v1/detect", strings.NewReader(`{"run_id":"run-1","request_model":"gpt-5.6-sol","api_key":"upstream-secret","base_url":"`+upstream.URL+`"}`))
	req.Header.Set("Authorization", "Bearer detector-secret")
	response := httptest.NewRecorder()
	d.detect(response, req)

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "abnormal" || payload["error_code"] != "response_or_fingerprint_mismatch" {
		t.Fatalf("payload = %#v", payload)
	}
	summary, ok := payload["juice_summary"].(map[string]any)
	if !ok || summary["verdict"] != verdictSuspectedReplacement {
		t.Fatalf("juice_summary = %#v", payload["juice_summary"])
	}
	active, ok := summary["active_response"].(map[string]any)
	if !ok || active["returned_model"] != "gpt-5.4" || active["status"] != evidenceMismatch {
		t.Fatalf("active_response = %#v", summary["active_response"])
	}
	if strings.Contains(response.Body.String(), "must-not-persist") || strings.Contains(response.Body.String(), "upstream-secret") || strings.Contains(response.Body.String(), upstream.URL) {
		t.Fatalf("response leaked sensitive or raw output data: %s", response.Body.String())
	}
}

func TestDetectorMarksMissingResponseModelWithoutBorrowingCatalog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.6-sol"}, {"id": "gpt-5.4"}}})
		case "/v1/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_1", "output": []any{map[string]any{"model": "nested-must-not-count"}}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(upstream.Close)
	d := &detector{token: "detector-secret", version: "test", models: parseModels(""), client: upstream.Client()}
	req := httptest.NewRequest(http.MethodPost, "/v1/detect", strings.NewReader(`{"run_id":"run-1","request_model":"gpt-5.6-sol","api_key":"upstream-secret","base_url":"`+upstream.URL+`"}`))
	req.Header.Set("Authorization", "Bearer detector-secret")
	response := httptest.NewRecorder()
	d.detect(response, req)

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "insufficient" {
		t.Fatalf("payload = %#v", payload)
	}
	summary := payload["juice_summary"].(map[string]any)
	active := summary["active_response"].(map[string]any)
	if active["status"] != evidenceMissing || active["returned_model"] != "" {
		t.Fatalf("active_response = %#v", active)
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
		{name: "model mapping", upstream: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/models":
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.4"}}})
			case "/v1/responses":
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_1", "model": "gpt-5.6-sol"})
			default:
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
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
