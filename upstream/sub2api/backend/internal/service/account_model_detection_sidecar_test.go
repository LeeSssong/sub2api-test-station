package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPAccountModelDetectionSidecarReportsUnconfigured(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("", "", nil)
	if _, err := client.Catalog(context.Background()); !errors.Is(err, ErrAccountModelDetectorNotConfigured) {
		t.Fatalf("Catalog error = %v, want detector not configured", err)
	}
	if _, err := client.Detect(context.Background(), AccountModelDetectionRequest{}); !errors.Is(err, ErrAccountModelDetectorNotConfigured) {
		t.Fatalf("Detect error = %v, want detector not configured", err)
	}
}

type accountModelDetectionRoundTripFunc func(*http.Request) (*http.Response, error)

func (f accountModelDetectionRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func accountModelDetectionJSONResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

func TestHTTPAccountModelDetectionSidecarCatalogAndDetect(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "private-token", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/catalog":
			return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{"version": "4.1.1", "models": []map[string]any{{"id": "gpt-5.6-sol", "supported": true}, {"id": "legacy", "supported": false}}}), nil
		case "/v1/detect":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["api_key"] != "sk-private" || body["base_url"] != "https://relay.example/v1" {
				t.Fatalf("request body = %#v", body)
			}
			return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{"status": "abnormal", "juice_status": "mismatch", "fingerprint_candidate": "gpt-5.6-luna", "fingerprint_similarity": map[string]any{"luna": 0.98}, "detector_version": "4.1.1"}), nil
		default:
			return accountModelDetectionJSONResponse(http.StatusNotFound, map[string]any{"error": "not found"}), nil
		}
	})})
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
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return accountModelDetectionJSONResponse(http.StatusBadGateway, map[string]any{"error": "upstream rejected sk-leaked-secret at https://secret.example/v1"}), nil
	})})
	_, err := client.Detect(context.Background(), AccountModelDetectionRequest{APIKey: "sk-leaked-secret", BaseURL: "https://secret.example/v1"})
	if err == nil {
		t.Fatal("expected sidecar error")
	}
	if strings.Contains(err.Error(), "sk-leaked-secret") || strings.Contains(err.Error(), "secret.example") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestHTTPAccountModelDetectionSidecarRejectsInvalidStatus(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{"status": "confirmed_replacement"}), nil
	})})
	if _, err := client.Detect(context.Background(), AccountModelDetectionRequest{}); err == nil {
		t.Fatal("expected invalid response error")
	}
}

func TestHTTPAccountModelDetectionSidecarSanitizesSummaryFields(t *testing.T) {
	client := NewHTTPAccountModelDetectionSidecar("http://detector.test", "", &http.Client{Transport: accountModelDetectionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return accountModelDetectionJSONResponse(http.StatusOK, map[string]any{
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
		}), nil
	})})
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
