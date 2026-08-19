package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	codexRadarInsightsURL      = "https://codexradar.com/api/radar-insights"
	codexRadarMaxResponseBytes = 512 << 10
	codexRadarCacheTTL         = 60 * time.Second
	codexRadarTimeout          = 3 * time.Second
)

var ErrCodexRadarUnavailable = errors.New("codexradar insights unavailable")

type CodexRadarRecommendationItem struct {
	Model                  string  `json:"model"`
	Effort                 string  `json:"effort"`
	IQ                     float64 `json:"iq"`
	AverageDurationMinutes float64 `json:"average_duration_minutes"`
	AverageCostUSD         float64 `json:"average_cost_usd"`
	Rule                   string  `json:"rule"`
}

type CodexRadarRecommendation struct {
	Key   string                         `json:"key"`
	Title string                         `json:"title"`
	Rule  string                         `json:"rule"`
	Items []CodexRadarRecommendationItem `json:"items"`
}

type CodexRadarInsights struct {
	GeneratedAt     string                     `json:"generated_at"`
	SourceUpdatedAt string                     `json:"source_updated_at"`
	Recommendations []CodexRadarRecommendation `json:"recommendations"`
}

type codexRadarWireResponse struct {
	Schema          int                        `json:"schema"`
	GeneratedAt     string                     `json:"generated_at"`
	SourceUpdatedAt string                     `json:"source_updated_at"`
	Recommendations []CodexRadarRecommendation `json:"recommendations"`
}

type CodexRadarInsightsService struct {
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	cached    *CodexRadarInsights
	fetchedAt time.Time
}

func NewCodexRadarInsightsService(client *http.Client, now func() time.Time) *CodexRadarInsightsService {
	if client == nil {
		client = &http.Client{Timeout: codexRadarTimeout}
	} else if client.Timeout == 0 {
		copyClient := *client
		copyClient.Timeout = codexRadarTimeout
		client = &copyClient
	}
	if now == nil {
		now = time.Now
	}
	return &CodexRadarInsightsService{client: client, now: now}
}

func ProvideCodexRadarInsightsService() *CodexRadarInsightsService {
	return NewCodexRadarInsightsService(nil, nil)
}

func (s *CodexRadarInsightsService) Get(ctx context.Context) (CodexRadarInsights, bool, error) {
	now := s.now()
	s.mu.Lock()
	if s.cached != nil && now.Sub(s.fetchedAt) < codexRadarCacheTTL {
		value := *s.cached
		s.mu.Unlock()
		return value, false, nil
	}
	s.mu.Unlock()

	value, err := s.fetch(ctx)
	if err == nil {
		s.mu.Lock()
		s.cached = &value
		s.fetchedAt = now
		s.mu.Unlock()
		return value, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil {
		return *s.cached, true, nil
	}
	return CodexRadarInsights{}, false, fmt.Errorf("%w: %v", ErrCodexRadarUnavailable, err)
}

func (s *CodexRadarInsightsService) fetch(ctx context.Context) (CodexRadarInsights, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexRadarInsightsURL, nil)
	if err != nil {
		return CodexRadarInsights{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return CodexRadarInsights{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CodexRadarInsights{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, codexRadarMaxResponseBytes+1))
	if err != nil {
		return CodexRadarInsights{}, err
	}
	if len(body) > codexRadarMaxResponseBytes {
		return CodexRadarInsights{}, errors.New("response too large")
	}
	var wire codexRadarWireResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return CodexRadarInsights{}, err
	}
	if err := validateCodexRadarWire(wire); err != nil {
		return CodexRadarInsights{}, err
	}
	return CodexRadarInsights{
		GeneratedAt: wire.GeneratedAt, SourceUpdatedAt: wire.SourceUpdatedAt,
		Recommendations: wire.Recommendations,
	}, nil
}

func validateCodexRadarWire(wire codexRadarWireResponse) error {
	if wire.Schema != 1 {
		return errors.New("unsupported schema")
	}
	if _, err := time.Parse(time.RFC3339, wire.GeneratedAt); err != nil {
		return errors.New("invalid generated_at")
	}
	if _, err := time.Parse(time.RFC3339, wire.SourceUpdatedAt); err != nil {
		return errors.New("invalid source_updated_at")
	}
	expected := []string{"daily_development", "hard_problems", "background_automation", "lobster_tasks"}
	if len(wire.Recommendations) != len(expected) {
		return errors.New("recommendation category count mismatch")
	}
	seen := make(map[string]CodexRadarRecommendation, len(expected))
	for _, recommendation := range wire.Recommendations {
		if _, exists := seen[recommendation.Key]; exists {
			return errors.New("duplicate recommendation category")
		}
		if !validCodexRadarText(recommendation.Key, 64) || !validCodexRadarText(recommendation.Title, 128) || !validCodexRadarText(recommendation.Rule, 2048) {
			return errors.New("invalid recommendation text")
		}
		if len(recommendation.Items) < 1 || len(recommendation.Items) > 2 {
			return errors.New("invalid recommendation item count")
		}
		for _, item := range recommendation.Items {
			if !validCodexRadarText(item.Model, 128) || !validCodexRadarText(item.Effort, 64) || !validCodexRadarText(item.Rule, 2048) {
				return errors.New("invalid recommendation item text")
			}
			if !validCodexRadarNumber(item.IQ) || !validCodexRadarNumber(item.AverageDurationMinutes) || !validCodexRadarNumber(item.AverageCostUSD) {
				return errors.New("invalid recommendation number")
			}
		}
		seen[recommendation.Key] = recommendation
	}
	ordered := make([]CodexRadarRecommendation, 0, len(expected))
	for _, key := range expected {
		value, ok := seen[key]
		if !ok {
			return errors.New("missing recommendation category")
		}
		ordered = append(ordered, value)
	}
	wire.Recommendations = ordered
	return nil
}

func validCodexRadarText(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]rune(value)) <= maximum
}

func validCodexRadarNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
