package main

import (
	"bytes"
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
	Status                string         `json:"status"`
	JuiceStatus           string         `json:"juice_status,omitempty"`
	JuiceSummary          map[string]any `json:"juice_summary,omitempty"`
	DetectorVer           string         `json:"detector_version,omitempty"`
	Fingerprint           string         `json:"fingerprint_candidate,omitempty"`
	FingerprintSimilarity map[string]any `json:"fingerprint_similarity,omitempty"`
	ErrorCode             string         `json:"error_code,omitempty"`
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
	type catalogResult struct {
		models []string
		err    error
	}
	type activeResult struct {
		model string
		err   error
	}
	catalogCh := make(chan catalogResult, 1)
	activeCh := make(chan activeResult, 1)
	go func() {
		models, err := d.fetchModels(r.Context(), request.BaseURL, request.APIKey)
		catalogCh <- catalogResult{models: models, err: err}
	}()
	go func() {
		returnedModel, err := d.fetchResponseModel(r.Context(), request.BaseURL, request.APIKey, model)
		activeCh <- activeResult{model: returnedModel, err: err}
	}()
	catalog := <-catalogCh
	active := <-activeCh

	evidence := detectionEvidence{
		RequestedModel: model,
		Catalog: catalogEvidence{
			Status:         evidenceUnavailable,
			ReturnedCount:  len(catalog.models),
			ReturnedModels: catalog.models,
		},
		ActiveResponse: activeResponseEvidence{Status: evidenceUnavailable},
		Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
	}
	if catalog.err == nil {
		evidence.Catalog.Status = evidenceMissing
		for _, candidate := range catalog.models {
			if candidate == model {
				evidence.Catalog.Status = evidenceMatch
				break
			}
		}
	}
	if active.err == nil {
		evidence.ActiveResponse.ReturnedModel = boundedEvidenceModelID(active.model)
		switch {
		case evidence.ActiveResponse.ReturnedModel == "":
			evidence.ActiveResponse.Status = evidenceMissing
		case evidence.ActiveResponse.ReturnedModel == model:
			evidence.ActiveResponse.Status = evidenceMatch
		default:
			evidence.ActiveResponse.Status = evidenceMismatch
		}
	}
	evidence.Verdict = classifyEvidence(evidence)
	response := detectResponse{
		Status:       statusForVerdict(evidence.Verdict),
		JuiceStatus:  evidence.Verdict,
		JuiceSummary: evidenceSummary(evidence),
		DetectorVer:  d.version,
		ErrorCode:    errorCodeForEvidence(evidence, catalog.err, active.err),
	}
	writeJSON(w, http.StatusOK, response)
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

func (d *detector) fetchResponseModel(ctx context.Context, rawBaseURL, apiKey, model string) (string, error) {
	endpoint, err := responsesEndpoint(rawBaseURL)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"model":             model,
		"input":             "Reply with exactly OK.",
		"max_output_tokens": 8,
		"stream":            false,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", errUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&payload); err != nil {
		return "", err
	}
	return boundedEvidenceModelID(payload.Model), nil
}

func modelsEndpoint(raw string) (string, error) {
	return upstreamEndpoint(raw, "/models")
}

func responsesEndpoint(raw string) (string, error) {
	return upstreamEndpoint(raw, "/responses")
}

func upstreamEndpoint(raw, suffix string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", errors.New("invalid upstream base URL")
	}
	u.RawQuery, u.Fragment = "", ""
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		u.Path = path + suffix
	} else {
		u.Path = path + "/v1" + suffix
	}
	return u.String(), nil
}

func statusForVerdict(verdict string) string {
	switch verdict {
	case verdictVerified:
		return "normal"
	case verdictSuspectedMapping, verdictSuspectedReplacement, verdictHighRiskInconsistent:
		return "abnormal"
	default:
		return "insufficient"
	}
}

func errorCodeForEvidence(evidence detectionEvidence, catalogErr, activeErr error) string {
	switch evidence.Verdict {
	case verdictVerified:
		return ""
	case verdictSuspectedMapping:
		return "model_not_advertised"
	case verdictSuspectedReplacement:
		return "response_or_fingerprint_mismatch"
	case verdictHighRiskInconsistent:
		return "evidence_inconsistent"
	}
	if errors.Is(catalogErr, errUnauthorized) || errors.Is(activeErr, errUnauthorized) {
		return "upstream_unauthorized"
	}
	if evidence.ActiveResponse.Status == evidenceMissing {
		return "response_model_missing"
	}
	return "evidence_insufficient"
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
