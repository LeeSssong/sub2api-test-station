package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultListenAddress = ":8090"
	defaultCatalog       = "gpt-5.6-terra,gpt-5.6-sol,gpt-5.4,gpt-5.6,gpt-5.6-codex,claude-3-7-sonnet"
	maxBodySize          = 2 << 20
)

type detector struct {
	token   string
	models  map[string]struct{}
	version string
	client  *http.Client
}

type detectRequest struct {
	RunID         string `json:"run_id"`
	DeclaredModel string `json:"declared_model"`
	RequestModel  string `json:"request_model"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
}

type detectResponse struct {
	Status      string `json:"status"`
	JuiceStatus string `json:"juice_status,omitempty"`
	DetectorVer string `json:"detector_version,omitempty"`
	Fingerprint string `json:"fingerprint_candidate,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
}

func main() {
	d := &detector{
		token:   strings.TrimSpace(os.Getenv("SUB2API_MODEL_DETECTOR_TOKEN")),
		models:  parseModels(os.Getenv("MODEL_DETECTOR_MODELS")),
		version: strings.TrimSpace(os.Getenv("MODEL_DETECTOR_VERSION")),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	if d.version == "" {
		d.version = "native-1"
	}
	if len(d.models) == 0 {
		log.Fatal("MODEL_DETECTOR_MODELS must contain at least one model")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", d.healthz)
	mux.HandleFunc("/v1/catalog", d.catalog)
	mux.HandleFunc("/v1/detect", d.detect)
	server := &http.Server{Addr: envOr("MODEL_DETECTOR_LISTEN_ADDRESS", defaultListenAddress), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second}
	log.Fatal(server.ListenAndServe())
}

func (d *detector) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (d *detector) catalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !d.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	models := make([]string, 0, len(d.models))
	for model := range d.models {
		models = append(models, model)
	}
	sort.Strings(models)
	items := make([]map[string]any, 0, len(models))
	for _, model := range models {
		items = append(items, map[string]any{"id": model, "supported": true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": d.version, "models": items})
}

func (d *detector) detect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !d.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var request detectRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	request.APIKey = strings.TrimSpace(request.APIKey)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	model := strings.TrimSpace(request.RequestModel)
	if model == "" {
		model = strings.TrimSpace(request.DeclaredModel)
	}
	if model == "" || request.APIKey == "" || request.BaseURL == "" {
		writeJSON(w, http.StatusOK, detectResponse{Status: "insufficient", DetectorVer: d.version, ErrorCode: "missing_probe_input"})
		return
	}
	models, err := d.fetchModels(r.Context(), request.BaseURL, request.APIKey)
	if err != nil {
		status := "insufficient"
		code := "upstream_unavailable"
		if errors.Is(err, errUnauthorized) {
			code = "upstream_unauthorized"
		}
		writeJSON(w, http.StatusOK, detectResponse{Status: status, DetectorVer: d.version, ErrorCode: code})
		return
	}
	for _, candidate := range models {
		if candidate == model {
			writeJSON(w, http.StatusOK, detectResponse{Status: "normal", JuiceStatus: "match", DetectorVer: d.version, Fingerprint: candidate})
			return
		}
	}
	writeJSON(w, http.StatusOK, detectResponse{Status: "abnormal", JuiceStatus: "mismatch", DetectorVer: d.version, ErrorCode: "model_not_advertised"})
}

var errUnauthorized = errors.New("upstream unauthorized")

func (d *detector) fetchModels(ctx context.Context, rawBaseURL, apiKey string) ([]string, error) {
	endpoint, err := modelsEndpoint(rawBaseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" && len(id) <= 128 {
			models = append(models, id)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("upstream model catalog empty")
	}
	return models, nil
}

func modelsEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", errors.New("invalid upstream base URL")
	}
	u.RawQuery, u.Fragment = "", ""
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		u.Path = path + "/models"
	} else {
		u.Path = path + "/v1/models"
	}
	return u.String(), nil
}

func (d *detector) authorized(r *http.Request) bool {
	if d.token == "" {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return len(provided) == len(d.token) && subtle.ConstantTimeCompare([]byte(provided), []byte(d.token)) == 1
}

func parseModels(raw string) map[string]struct{} {
	if strings.TrimSpace(raw) == "" {
		raw = defaultCatalog
	}
	models := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" && len(item) <= 128 {
			models[item] = struct{}{}
		}
	}
	return models
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
