package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexRadarRoundTripper func(*http.Request) (*http.Response, error)

func (fn codexRadarRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

const validCodexRadarFixture = `{
  "schema":1,"generated_at":"2026-08-19T01:59:16Z","source_updated_at":"2026-08-19T01:54:42Z",
  "recommendations":[
    {"key":"daily_development","title":"日常开发","rule":"daily rule","items":[{"model":"gpt-5.6-sol","effort":"medium","iq":90.12,"average_duration_minutes":17.53,"average_cost_usd":3.267475,"rule":"daily rule"}]},
    {"key":"hard_problems","title":"难题攻坚","rule":"hard rule","items":[{"model":"gpt-5.6-sol","effort":"ultra","iq":104.61,"average_duration_minutes":49.95,"average_cost_usd":22.362359,"rule":"hard rule"}]},
    {"key":"background_automation","title":"后台自动化","rule":"automation rule","items":[{"model":"gpt-5.6-luna","effort":"xhigh","iq":84.59,"average_duration_minutes":26.58,"average_cost_usd":0.318406,"rule":"automation rule"}]},
    {"key":"lobster_tasks","title":"跑龙虾类任务","rule":"lobster rule","items":[{"model":"gpt-5.6-terra","effort":"low","iq":56.39,"average_duration_minutes":7.86,"average_cost_usd":0.461982,"rule":"lobster rule"}]}
  ]
}`

const codexRadarEmptyPrimaryRecommendationsFixture = `{
  "schema":1,"generated_at":"2026-09-06T10:30:41Z","source_updated_at":"2026-09-06T06:19:46Z",
  "comprehensive_points":[
    {"model":"gpt-5.6-sol","effort":"medium","iq":93.86},
    {"model":"gpt-5.6-sol","effort":"ultra","iq":105.07},
    {"model":"gpt-5.6-terra","effort":"ultra","iq":99.96},
    {"model":"gpt-5.6-luna","effort":"max","iq":95.85},
    {"model":"gpt-6-astra","effort":"max","iq":113.37}
  ],
  "recommendations":[
    {"key":"daily_development","title":"日常开发","rule":"daily rule","items":[]},
    {"key":"hard_problems","title":"难题攻坚","rule":"hard rule","items":[]},
    {"key":"background_automation","title":"后台自动化","rule":"automation rule","items":[{"model":"gpt-5.6-luna","effort":"high","iq":80.02,"average_duration_minutes":23.68,"average_cost_usd":0.204769,"rule":"automation rule"}]},
    {"key":"lobster_tasks","title":"跑龙虾类任务","rule":"lobster rule","items":[{"model":"gpt-5.6-terra","effort":"low","iq":61.59,"average_duration_minutes":8.33,"average_cost_usd":0.435755,"rule":"lobster rule"}]}
  ]
}`

const codexRadarMetricsFixture = `{
  "schema":3,"mode":"equal_latest_3","source_updated_at":"2026-09-06T10:30:27Z",
  "points":[
    {"model":"gpt-5.6-sol","effort":"medium","average_price_usd":2.94,"average_minutes":16.09,"combined_cost_index":1255.31},
    {"model":"gpt-5.6-sol","effort":"ultra","average_price_usd":20.01,"average_minutes":41.0,"combined_cost_index":9000.0},
    {"model":"gpt-5.6-terra","effort":"ultra","average_price_usd":11.11,"average_minutes":37.0,"combined_cost_index":7000.0},
    {"model":"gpt-5.6-luna","effort":"max","average_price_usd":0.62,"average_minutes":55.0,"combined_cost_index":500.0},
    {"model":"gpt-6-astra","effort":"max","average_price_usd":5.19,"average_minutes":25.08,"combined_cost_index":8595.36}
  ]
}`

func TestCodexRadarInsightsFixedTargetAndCache(t *testing.T) {
	now := time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "https", req.URL.Scheme)
		require.Equal(t, "codexradar.com", req.URL.Host)
		require.Equal(t, "/api/radar-insights", req.URL.Path)
		require.Empty(t, req.URL.RawQuery)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarFixture)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
	})}
	svc := NewCodexRadarInsightsService(client, func() time.Time { return now })
	first, stale, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, stale)
	require.Len(t, first.Recommendations, 4)
	_, _, err = svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(1), calls.Load())
}

func TestCodexRadarInsightsFallsBackToRecentSuccess(t *testing.T) {
	now := time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	client := &http.Client{Transport: codexRadarRoundTripper(func(_ *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarFixture)), Header: http.Header{}}, nil
		}
		return nil, context.DeadlineExceeded
	})}
	svc := NewCodexRadarInsightsService(client, func() time.Time { return now })
	first, _, err := svc.Get(context.Background())
	require.NoError(t, err)
	now = now.Add(61 * time.Second)
	fallback, stale, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, stale)
	require.Equal(t, first, fallback)
}

func TestCodexRadarInsightsSupplementsEmptyPrimaryRecommendationsFromMetrics(t *testing.T) {
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Path {
		case "/api/radar-insights":
			body = codexRadarEmptyPrimaryRecommendationsFixture
		case "/api/intelligence-efficiency-metrics":
			body = codexRadarMetricsFixture
		default:
			t.Fatalf("unexpected CodexRadar path %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}

	value, stale, err := NewCodexRadarInsightsService(client, time.Now).Get(context.Background())
	require.NoError(t, err)
	require.False(t, stale)
	require.Equal(t, []CodexRadarRecommendationItem{
		{Model: "gpt-5.6-sol", Effort: "medium", IQ: 93.86, AverageDurationMinutes: 16.09, AverageCostUSD: 2.94, Rule: "daily rule"},
		{Model: "gpt-5.6-terra", Effort: "ultra", IQ: 99.96, AverageDurationMinutes: 37, AverageCostUSD: 11.11, Rule: "daily rule"},
	}, value.Recommendations[0].Items)
	require.Equal(t, []CodexRadarRecommendationItem{
		{Model: "gpt-5.6-sol", Effort: "ultra", IQ: 105.07, AverageDurationMinutes: 41, AverageCostUSD: 20.01, Rule: "hard rule"},
		{Model: "gpt-5.6-terra", Effort: "ultra", IQ: 99.96, AverageDurationMinutes: 37, AverageCostUSD: 11.11, Rule: "hard rule"},
	}, value.Recommendations[1].Items)
	require.Equal(t, "fresh", value.Recommendations[0].Status)
	require.Equal(t, "fresh", value.Recommendations[1].Status)
}

func TestCodexRadarInsightsKeepsExistingRecommendationsWithoutMetricsRequest(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, "/api/radar-insights", req.URL.Path)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarFixture)), Header: http.Header{}}, nil
	})}

	value, _, err := NewCodexRadarInsightsService(client, time.Now).Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, "gpt-5.6-sol", value.Recommendations[0].Items[0].Model)
	require.Equal(t, 90.12, value.Recommendations[0].Items[0].IQ)
}

func TestCodexRadarInsightsRejectsInvalidWithoutSnapshot(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"bad json", `{`},
		{"wrong schema", strings.Replace(validCodexRadarFixture, `"schema":1`, `"schema":2`, 1)},
		{"extra category", strings.Replace(validCodexRadarFixture, `]}`, `,{"key":"extra","title":"x","rule":"x","items":[]}]}`, 1)},
		{"non finite", strings.Replace(validCodexRadarFixture, `90.12`, `-1`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: codexRadarRoundTripper(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tt.body)), Header: http.Header{}}, nil
			})}
			svc := NewCodexRadarInsightsService(client, time.Now)
			_, _, err := svc.Get(context.Background())
			require.ErrorIs(t, err, ErrCodexRadarUnavailable)
		})
	}
}

func TestCodexRadarInsightsRemoteFailureWithoutSnapshot(t *testing.T) {
	client := &http.Client{Transport: codexRadarRoundTripper(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	_, _, err := NewCodexRadarInsightsService(client, time.Now).Get(context.Background())
	require.ErrorIs(t, err, ErrCodexRadarUnavailable)
}
