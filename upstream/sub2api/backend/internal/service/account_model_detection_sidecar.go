package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	accountModelDetectionSidecarBodyLimit = 64 << 10
	accountModelDetectionSummaryLimit     = 8 << 10
	accountModelDetectionStringLimit      = 256
)

type HTTPAccountModelDetectionSidecar struct {
	baseURL string
	token   string
	client  *http.Client
}

var (
	ErrAccountModelDetectorNotConfigured = errors.New("detector_not_configured")
	ErrAccountModelDetectorUnavailable   = errors.New("detector_unavailable")
)

func NewHTTPAccountModelDetectionSidecar(baseURL, token string, client *http.Client) *HTTPAccountModelDetectionSidecar {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &HTTPAccountModelDetectionSidecar{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: strings.TrimSpace(token), client: client}
}

func (s *HTTPAccountModelDetectionSidecar) Catalog(ctx context.Context) (AccountModelDetectionSidecarCatalog, error) {
	if s == nil || s.baseURL == "" {
		return AccountModelDetectionSidecarCatalog{}, ErrAccountModelDetectorNotConfigured
	}
	var payload struct {
		Version string `json:"version"`
		Models  []struct {
			ID        string `json:"id"`
			Supported bool   `json:"supported"`
		} `json:"models"`
	}
	if err := s.doJSON(ctx, http.MethodGet, "/v1/catalog", nil, &payload); err != nil {
		return AccountModelDetectionSidecarCatalog{}, err
	}
	models := make([]string, 0, len(payload.Models))
	seen := map[string]bool{}
	for _, model := range payload.Models {
		id := boundedString(model.ID, 128)
		if id == "" || !model.Supported || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	return AccountModelDetectionSidecarCatalog{Version: boundedString(payload.Version, 64), Models: models, State: AccountModelDetectorStateReady}, nil
}

func (s *HTTPAccountModelDetectionSidecar) Detect(ctx context.Context, request AccountModelDetectionRequest) (AccountModelDetectionResponse, error) {
	if s == nil || s.baseURL == "" {
		return AccountModelDetectionResponse{}, ErrAccountModelDetectorNotConfigured
	}
	var payload AccountModelDetectionResponse
	if err := s.doJSON(ctx, http.MethodPost, "/v1/detect", request, &payload); err != nil {
		return AccountModelDetectionResponse{}, err
	}
	if !validDetectionStatus(payload.Status) {
		return AccountModelDetectionResponse{}, errors.New("detector_invalid_response")
	}
	payload.JuiceStatus = boundedString(payload.JuiceStatus, 64)
	payload.FingerprintCandidate = boundedString(payload.FingerprintCandidate, 128)
	payload.DetectorVersion = boundedString(payload.DetectorVersion, 64)
	payload.ErrorCode = boundedString(payload.ErrorCode, 64)
	payload.JuiceSummary = boundedSummary(payload.JuiceSummary)
	payload.FingerprintSimilarity = boundedSummary(payload.FingerprintSimilarity)
	return payload, nil
}

func (s *HTTPAccountModelDetectionSidecar) doJSON(ctx context.Context, method, path string, request any, response any) error {
	if s == nil || s.baseURL == "" {
		return ErrAccountModelDetectorNotConfigured
	}
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			return errors.New("detector_request_invalid")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return errors.New("detector_unavailable")
	}
	req.Header.Set("Accept", "application/json")
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return ErrAccountModelDetectorUnavailable
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, accountModelDetectionSidecarBodyLimit+1))
	if err != nil || len(data) > accountModelDetectionSidecarBodyLimit {
		return ErrAccountModelDetectorUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("detector_http_%d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, response); err != nil {
		return errors.New("detector_invalid_response")
	}
	return nil
}

func validDetectionStatus(status string) bool {
	switch status {
	case AccountModelDetectionStatusNormal, AccountModelDetectionStatusAbnormal, AccountModelDetectionStatusInsufficient:
		return true
	default:
		return false
	}
}

func boundedString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func boundedSummary(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	sanitized := sanitizeSummaryMap(value, 0)
	encoded, err := json.Marshal(sanitized)
	if err != nil || len(encoded) > accountModelDetectionSummaryLimit {
		return nil
	}
	return sanitized
}

func sanitizeSummaryMap(value map[string]any, depth int) map[string]any {
	if depth >= 4 {
		return nil
	}
	result := make(map[string]any)
	count := 0
	for key, raw := range value {
		key = boundedString(key, 64)
		if key == "" || sensitiveSummaryKey(key) || count >= 32 {
			continue
		}
		if sanitized, ok := sanitizeSummaryValue(raw, depth+1); ok {
			result[key] = sanitized
			count++
		}
	}
	return result
}

func sanitizeSummaryValue(value any, depth int) (any, bool) {
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, true
	case string:
		return boundedString(typed, accountModelDetectionStringLimit), true
	case map[string]any:
		if depth >= 4 {
			return nil, false
		}
		return sanitizeSummaryMap(typed, depth), true
	case []any:
		if depth >= 4 {
			return nil, false
		}
		result := make([]any, 0, min(len(typed), 32))
		for _, item := range typed {
			if len(result) >= 32 {
				break
			}
			if sanitized, ok := sanitizeSummaryValue(item, depth+1); ok {
				result = append(result, sanitized)
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func sensitiveSummaryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	switch normalized {
	case "api_key", "apikey", "authorization", "base_url", "prompt", "full_prompt", "output", "full_output", "request", "response", "token", "secret", "credentials":
		return true
	default:
		return false
	}
}
