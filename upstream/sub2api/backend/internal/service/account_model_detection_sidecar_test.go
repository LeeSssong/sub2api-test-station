package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPAccountModelDetectionSidecarCatalogAndDetect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "4.1.1", "models": []map[string]any{{"id": "gpt-5.6-sol", "supported": true}, {"id": "legacy", "supported": false}}})
		case "/v1/detect":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["api_key"] != "sk-private" || body["base_url"] != "https://relay.example/v1" {
				t.Fatalf("request body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "abnormal", "juice_status": "mismatch", "fingerprint_candidate": "gpt-5.6-luna", "fingerprint_similarity": map[string]any{"luna": 0.98}, "detector_version": "4.1.1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPAccountModelDetectionSidecar(server.URL, "private-token", server.Client())
	catalog, err := client.Catalog(context.Background())
	if err != nil || len(catalog.Models) != 1 || catalog.Models[0] != "gpt-5.6-sol" {
		t.Fatalf("catalog = %#v err=%v", catalog, err)
	}
	response, err := client.Detect(context.Background(), AccountModelDetectionRequest{RunID: "run-1", DeclaredModel: "gpt-5.6-sol", RequestModel: "gpt-5.6-sol", APIKey: "sk-private", BaseURL: "https://relay.example/v1"})
	if err != nil || response.Status != AccountModelDetectionStatusAbnormal || response.FingerprintCandidate != "gpt-5.6-luna" {
		t.Fatalf("response = %#v err=%v", response, err)
	}
}

func TestHTTPAccountModelDetectionSidecarReturnsStableErrorsWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`upstream rejected sk-leaked-secret at https://secret.example/v1`))
	}))
	defer server.Close()
	client := NewHTTPAccountModelDetectionSidecar(server.URL, "", server.Client())
	_, err := client.Detect(context.Background(), AccountModelDetectionRequest{APIKey: "sk-leaked-secret", BaseURL: "https://secret.example/v1"})
	if err == nil {
		t.Fatal("expected sidecar error")
	}
	if strings.Contains(err.Error(), "sk-leaked-secret") || strings.Contains(err.Error(), "secret.example") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestHTTPAccountModelDetectionSidecarRejectsInvalidStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "confirmed_replacement"})
	}))
	defer server.Close()
	client := NewHTTPAccountModelDetectionSidecar(server.URL, "", server.Client())
	if _, err := client.Detect(context.Background(), AccountModelDetectionRequest{}); err == nil {
		t.Fatal("expected invalid response error")
	}
}

func TestHTTPAccountModelDetectionSidecarSanitizesSummaryFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "normal",
			"juice_summary": map[string]any{
				"score":   0.9,
				"api_key": "sk-secret",
				"nested":  map[string]any{"authorization": "Bearer secret", "label": "safe"},
			},
			"fingerprint_similarity": map[string]any{
				"gpt-5.6-sol": 0.98,
				"response":    "full upstream output",
			},
		})
	}))
	defer server.Close()

	client := NewHTTPAccountModelDetectionSidecar(server.URL, "", server.Client())
	response, err := client.Detect(context.Background(), AccountModelDetectionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.JuiceSummary["api_key"]; ok {
		t.Fatalf("juice summary retained api_key: %#v", response.JuiceSummary)
	}
	nested, ok := response.JuiceSummary["nested"].(map[string]any)
	if !ok || nested["label"] != "safe" {
		t.Fatalf("nested summary = %#v", response.JuiceSummary["nested"])
	}
	if _, ok := nested["authorization"]; ok {
		t.Fatalf("nested summary retained authorization: %#v", nested)
	}
	if _, ok := response.FingerprintSimilarity["response"]; ok {
		t.Fatalf("fingerprint summary retained response: %#v", response.FingerprintSimilarity)
	}
}
