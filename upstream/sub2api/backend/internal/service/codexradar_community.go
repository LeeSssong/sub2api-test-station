package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	codexRadarSoftwareMetricsURL = "https://codexradar.com/api/intelligence-efficiency-metrics"
	codexRadarVisualReasoningURL = "https://codexradar.com/api/visual-spatial-reasoning"
	codexRadarCommunityMaxPoints = 128
)

var ErrCodexRadarCommunityUnavailable = errors.New("codexradar community unavailable")

type CodexRadarCommunityKey string

const (
	CodexRadarCommunityComprehensive CodexRadarCommunityKey = "comprehensive"
	CodexRadarCommunitySoftware      CodexRadarCommunityKey = "software"
	CodexRadarCommunityVisual        CodexRadarCommunityKey = "visual"
)

type CodexRadarCommunityPoint struct {
	Model                  string   `json:"model"`
	Effort                 string   `json:"effort"`
	Samples                int      `json:"samples"`
	IQ                     float64  `json:"iq"`
	AverageCostUSD         *float64 `json:"average_cost_usd"`
	AverageDurationMinutes *float64 `json:"average_duration_minutes"`
	SoftwareSamples        int      `json:"software_samples,omitempty"`
	VisualSamples          int      `json:"visual_samples,omitempty"`
	SoftwareIQ             float64  `json:"software_iq,omitempty"`
	VisualIQ               float64  `json:"visual_iq,omitempty"`
}

type CodexRadarCommunityTab struct {
	Key             CodexRadarCommunityKey     `json:"key"`
	SourceUpdatedAt string                     `json:"source_updated_at"`
	Points          []CodexRadarCommunityPoint `json:"points"`
}

type CodexRadarCommunity struct {
	GeneratedAt string                   `json:"generated_at"`
	Tabs        []CodexRadarCommunityTab `json:"tabs"`
}

type codexRadarSoftwareWire struct {
	Schema          int                       `json:"schema"`
	Mode            string                    `json:"mode"`
	SourceUpdatedAt string                    `json:"source_updated_at"`
	Points          []codexRadarSoftwarePoint `json:"points"`
}

type codexRadarSoftwarePoint struct {
	Model           string   `json:"model"`
	Effort          string   `json:"effort"`
	Passed          float64  `json:"passed"`
	Total           int      `json:"total"`
	IQ              float64  `json:"iq"`
	AveragePriceUSD *float64 `json:"average_price_usd"`
	AverageMinutes  *float64 `json:"average_minutes"`
}

type codexRadarVisualWire struct {
	Schema          int                     `json:"schema"`
	Type            string                  `json:"type"`
	Mode            string                  `json:"mode"`
	SourceUpdatedAt string                  `json:"source_updated_at"`
	Points          []codexRadarVisualPoint `json:"points"`
}

type codexRadarVisualPoint struct {
	Model           string   `json:"model"`
	Effort          string   `json:"effort"`
	Passed          float64  `json:"passed"`
	ValidTasks      int      `json:"valid_tasks"`
	BenchmarkTasks  int      `json:"benchmark_tasks"`
	IQ              float64  `json:"iq"`
	AveragePriceUSD *float64 `json:"average_price_usd"`
	PriceSamples    int      `json:"price_samples"`
	AverageMinutes  *float64 `json:"average_minutes"`
	DurationSamples int      `json:"duration_samples"`
}

type CodexRadarCommunityService struct {
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	cached    *CodexRadarCommunity
	fetchedAt time.Time
}

func NewCodexRadarCommunityService(client *http.Client, now func() time.Time) *CodexRadarCommunityService {
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
	return &CodexRadarCommunityService{client: client, now: now}
}

func ProvideCodexRadarCommunityService() *CodexRadarCommunityService {
	return NewCodexRadarCommunityService(nil, nil)
}

func (s *CodexRadarCommunityService) Get(ctx context.Context) (CodexRadarCommunity, bool, error) {
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
	return CodexRadarCommunity{}, false, fmt.Errorf("%w: %v", ErrCodexRadarCommunityUnavailable, err)
}

func (s *CodexRadarCommunityService) fetch(ctx context.Context) (CodexRadarCommunity, error) {
	var software codexRadarSoftwareWire
	if err := s.fetchJSON(ctx, codexRadarSoftwareMetricsURL, &software); err != nil {
		return CodexRadarCommunity{}, err
	}
	if err := validateCodexRadarSoftware(software); err != nil {
		return CodexRadarCommunity{}, err
	}
	var visual codexRadarVisualWire
	if err := s.fetchJSON(ctx, codexRadarVisualReasoningURL, &visual); err != nil {
		return CodexRadarCommunity{}, err
	}
	if err := validateCodexRadarVisual(visual); err != nil {
		return CodexRadarCommunity{}, err
	}

	softwarePoints := make([]CodexRadarCommunityPoint, 0, len(software.Points))
	for _, point := range software.Points {
		softwarePoints = append(softwarePoints, CodexRadarCommunityPoint{
			Model: point.Model, Effort: point.Effort, Samples: point.Total, IQ: point.IQ,
			AverageCostUSD: point.AveragePriceUSD, AverageDurationMinutes: point.AverageMinutes,
		})
	}
	visualPoints := make([]CodexRadarCommunityPoint, 0, len(visual.Points))
	for _, point := range visual.Points {
		visualPoints = append(visualPoints, CodexRadarCommunityPoint{
			Model: point.Model, Effort: point.Effort, Samples: point.ValidTasks, IQ: point.IQ,
			AverageCostUSD: point.AveragePriceUSD, AverageDurationMinutes: point.AverageMinutes,
		})
	}
	comprehensive := compositeCodexRadarPoints(software.Points, visual.Points)
	if len(comprehensive) == 0 {
		return CodexRadarCommunity{}, errors.New("no shared model efforts")
	}
	combinedUpdatedAt, err := olderCodexRadarTime(software.SourceUpdatedAt, visual.SourceUpdatedAt)
	if err != nil {
		return CodexRadarCommunity{}, err
	}
	return CodexRadarCommunity{
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
		Tabs: []CodexRadarCommunityTab{
			{Key: CodexRadarCommunityComprehensive, SourceUpdatedAt: combinedUpdatedAt, Points: comprehensive},
			{Key: CodexRadarCommunitySoftware, SourceUpdatedAt: software.SourceUpdatedAt, Points: softwarePoints},
			{Key: CodexRadarCommunityVisual, SourceUpdatedAt: visual.SourceUpdatedAt, Points: visualPoints},
		},
	}, nil
}

func (s *CodexRadarCommunityService) fetchJSON(ctx context.Context, target string, destination any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, codexRadarMaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > codexRadarMaxResponseBytes {
		return errors.New("response too large")
	}
	return json.Unmarshal(body, destination)
}

func validateCodexRadarSoftware(wire codexRadarSoftwareWire) error {
	if wire.Schema != 3 || wire.Mode != "equal_latest_3" {
		return errors.New("unsupported software schema")
	}
	if _, err := time.Parse(time.RFC3339, wire.SourceUpdatedAt); err != nil {
		return errors.New("invalid software source_updated_at")
	}
	if len(wire.Points) < 1 || len(wire.Points) > codexRadarCommunityMaxPoints {
		return errors.New("invalid software point count")
	}
	seen := make(map[string]struct{}, len(wire.Points))
	for _, point := range wire.Points {
		if !validCodexRadarText(point.Model, 128) || !validCodexRadarText(point.Effort, 64) {
			return errors.New("invalid software point text")
		}
		key := point.Model + "\x00" + point.Effort
		if _, exists := seen[key]; exists {
			return errors.New("duplicate software point")
		}
		seen[key] = struct{}{}
		if point.Total < 1 || point.Passed < 0 || point.Passed > float64(point.Total) || !validCommunityMetric(point.IQ, 150) || !validOptionalCodexRadarNumber(point.AveragePriceUSD) || !validOptionalCodexRadarNumber(point.AverageMinutes) {
			return errors.New("invalid software point number")
		}
	}
	return nil
}

func validateCodexRadarVisual(wire codexRadarVisualWire) error {
	if wire.Schema != 1 || wire.Type != "visual_spatial_reasoning_summary" || wire.Mode != "latest_valid_per_task" {
		return errors.New("unsupported visual schema")
	}
	if _, err := time.Parse(time.RFC3339, wire.SourceUpdatedAt); err != nil {
		return errors.New("invalid visual source_updated_at")
	}
	if len(wire.Points) < 1 || len(wire.Points) > codexRadarCommunityMaxPoints {
		return errors.New("invalid visual point count")
	}
	seen := make(map[string]struct{}, len(wire.Points))
	for _, point := range wire.Points {
		if !validCodexRadarText(point.Model, 128) || !validCodexRadarText(point.Effort, 64) {
			return errors.New("invalid visual point text")
		}
		key := point.Model + "\x00" + point.Effort
		if _, exists := seen[key]; exists {
			return errors.New("duplicate visual point")
		}
		seen[key] = struct{}{}
		if point.ValidTasks < 1 || point.BenchmarkTasks < point.ValidTasks || point.Passed < 0 || point.Passed > float64(point.ValidTasks) || !validCommunityMetric(point.IQ, 150) || !validOptionalCodexRadarNumber(point.AveragePriceUSD) || !validOptionalCodexRadarNumber(point.AverageMinutes) || point.PriceSamples < 0 || point.DurationSamples < 0 {
			return errors.New("invalid visual point number")
		}
	}
	return nil
}

func validCommunityMetric(value, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= maximum
}

func validOptionalCodexRadarNumber(value *float64) bool {
	return value == nil || validCodexRadarNumber(*value)
}

func compositeCodexRadarPoints(software []codexRadarSoftwarePoint, visual []codexRadarVisualPoint) []CodexRadarCommunityPoint {
	visualByKey := make(map[string]codexRadarVisualPoint, len(visual))
	for _, point := range visual {
		visualByKey[point.Model+"\x00"+point.Effort] = point
	}
	result := make([]CodexRadarCommunityPoint, 0, len(software))
	for _, left := range software {
		right, ok := visualByKey[left.Model+"\x00"+left.Effort]
		if !ok {
			continue
		}
		costRightWeight := right.PriceSamples
		if costRightWeight < 1 {
			costRightWeight = right.ValidTasks
		}
		durationRightWeight := right.DurationSamples
		if durationRightWeight < 1 {
			durationRightWeight = right.ValidTasks
		}
		result = append(result, CodexRadarCommunityPoint{
			Model: left.Model, Effort: left.Effort,
			Samples: left.Total + right.ValidTasks, IQ: math.Sqrt(left.IQ * right.IQ),
			AverageCostUSD:         weightedCodexRadarMetric(left.AveragePriceUSD, left.Total, right.AveragePriceUSD, costRightWeight),
			AverageDurationMinutes: weightedCodexRadarMetric(left.AverageMinutes, left.Total, right.AverageMinutes, durationRightWeight),
			SoftwareSamples:        left.Total, VisualSamples: right.ValidTasks, SoftwareIQ: left.IQ, VisualIQ: right.IQ,
		})
	}
	return result
}

func weightedCodexRadarMetric(left *float64, leftWeight int, right *float64, rightWeight int) *float64 {
	if left == nil || right == nil {
		return nil
	}
	weight := leftWeight + rightWeight
	if weight < 1 {
		return nil
	}
	value := (*left*float64(leftWeight) + *right*float64(rightWeight)) / float64(weight)
	return &value
}

func olderCodexRadarTime(left, right string) (string, error) {
	leftTime, err := time.Parse(time.RFC3339, left)
	if err != nil {
		return "", err
	}
	rightTime, err := time.Parse(time.RFC3339, right)
	if err != nil {
		return "", err
	}
	times := []time.Time{leftTime, rightTime}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return times[0].UTC().Format(time.RFC3339), nil
}
