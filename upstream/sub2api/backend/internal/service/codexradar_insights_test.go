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
