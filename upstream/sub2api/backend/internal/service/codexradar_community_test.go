package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const validCodexRadarSoftwareFixture = `{"schema":3,"mode":"equal_latest_3","source_updated_at":"2026-08-19T04:22:43Z","points":[{"model":"gpt-5.6-sol","effort":"low","passed":175,"total":336,"iq":78.12,"average_price_usd":2,"average_minutes":12,"runs_24h":4},{"model":"software-only","effort":"high","passed":10,"total":20,"iq":75,"average_price_usd":null,"average_minutes":10,"runs_24h":1}]}`
const validCodexRadarVisualFixture = `{"schema":1,"type":"visual_spatial_reasoning_summary","mode":"latest_valid_per_task","source_updated_at":"2026-08-19T04:45:30Z","points":[{"model":"gpt-5.6-sol","effort":"low","passed":44.98,"valid_tasks":86,"benchmark_tasks":86,"iq":78.45522732558143,"average_price_usd":1,"price_samples":86,"average_minutes":12.5,"duration_samples":86},{"model":"visual-only","effort":"medium","passed":40,"valid_tasks":80,"benchmark_tasks":86,"iq":75,"average_price_usd":1,"price_samples":80,"average_minutes":10,"duration_samples":80}]}`

func TestCodexRadarCommunityFixedTargetsCompositeAndCache(t *testing.T) {
	now := time.Date(2026, 8, 19, 5, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "https", req.URL.Scheme)
		require.Equal(t, "codexradar.com", req.URL.Host)
		require.Empty(t, req.URL.RawQuery)
		switch req.URL.Path {
		case "/api/intelligence-efficiency-metrics":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarSoftwareFixture))}, nil
		case "/api/visual-spatial-reasoning":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarVisualFixture))}, nil
		default:
			t.Fatalf("unexpected target %s", req.URL.Path)
			return nil, nil
		}
	})}

	svc := NewCodexRadarCommunityService(client, func() time.Time { return now })
	value, stale, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, stale)
	require.Equal(t, []CodexRadarCommunityKey{"comprehensive", "software", "visual"}, []CodexRadarCommunityKey{value.Tabs[0].Key, value.Tabs[1].Key, value.Tabs[2].Key})
	require.Len(t, value.Tabs[0].Points, 1)
	point := value.Tabs[0].Points[0]
	require.Equal(t, 422, point.Samples)
	require.Equal(t, 336, point.SoftwareSamples)
	require.Equal(t, 86, point.VisualSamples)
	require.InDelta(t, math.Sqrt(78.12*78.45522732558143), point.IQ, 1e-9)
	require.NotNil(t, point.AverageCostUSD)
	require.NotNil(t, point.AverageDurationMinutes)
	require.InDelta(t, (2*336+1*86)/422.0, *point.AverageCostUSD, 1e-9)
	require.InDelta(t, (12*336+12.5*86)/422.0, *point.AverageDurationMinutes, 1e-9)
	require.Equal(t, "2026-08-19T04:22:43Z", value.Tabs[0].SourceUpdatedAt)
	require.Nil(t, value.Tabs[1].Points[1].AverageCostUSD)

	_, _, err = svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
}

func TestCodexRadarCommunityFallsBackToRecentSuccess(t *testing.T) {
	now := time.Date(2026, 8, 19, 5, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		if call > 2 {
			return nil, context.DeadlineExceeded
		}
		body := validCodexRadarSoftwareFixture
		if req.URL.Path == "/api/visual-spatial-reasoning" {
			body = validCodexRadarVisualFixture
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	svc := NewCodexRadarCommunityService(client, func() time.Time { return now })
	first, _, err := svc.Get(context.Background())
	require.NoError(t, err)
	now = now.Add(61 * time.Second)
	fallback, stale, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, stale)
	require.Equal(t, first, fallback)
}

func TestCodexRadarCommunityDoesNotRejectParseable2xxPayloadByLocalBusinessRules(t *testing.T) {
	tests := []struct{ name, software, visual string }{
		{"software schema", strings.Replace(validCodexRadarSoftwareFixture, `"schema":3`, `"schema":2`, 1), validCodexRadarVisualFixture},
		{"visual schema", validCodexRadarSoftwareFixture, strings.Replace(validCodexRadarVisualFixture, `"schema":1`, `"schema":2`, 1)},
		{"zero sample", strings.Replace(validCodexRadarSoftwareFixture, `"passed":175,"total":336,"iq":78.12`, `"passed":0,"total":0,"iq":null`, 1), validCodexRadarVisualFixture},
		{"duplicate point", strings.Replace(validCodexRadarSoftwareFixture, `]}`, `,{"model":"gpt-5.6-sol","effort":"low","passed":1,"total":1,"iq":1,"average_price_usd":1,"average_minutes":1}]}`, 1), validCodexRadarVisualFixture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
				body := tt.software
				if req.URL.Path == "/api/visual-spatial-reasoning" {
					body = tt.visual
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			value, stale, err := NewCodexRadarCommunityService(client, time.Now).Get(context.Background())
			require.NoError(t, err)
			require.False(t, stale)
			require.Equal(t, "fresh", value.SourceStatus)
			require.Equal(t, "fresh", value.Tabs[0].Status)
			require.Equal(t, "fresh", value.Tabs[1].Status)
			require.Equal(t, "fresh", value.Tabs[2].Status)
		})
	}
}

func TestCodexRadarCommunityRejectsWhenBothSourcesFail(t *testing.T) {
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("source down")
	})}
	_, _, err := NewCodexRadarCommunityService(client, time.Now).Get(context.Background())
	require.ErrorIs(t, err, ErrCodexRadarCommunityUnavailable)
}

func TestCodexRadarCommunityReturnsPartialWhenOneSourceFails(t *testing.T) {
	client := &http.Client{Transport: codexRadarRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/visual-spatial-reasoning" {
			return nil, errors.New("visual source down")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validCodexRadarSoftwareFixture))}, nil
	})}
	value, stale, err := NewCodexRadarCommunityService(client, time.Now).Get(context.Background())
	require.NoError(t, err)
	require.False(t, stale)
	require.Equal(t, "partial", value.SourceStatus)
	require.Equal(t, "unavailable", value.Tabs[0].Status)
	require.Equal(t, "fresh", value.Tabs[1].Status)
	require.Equal(t, "unavailable", value.Tabs[2].Status)
	require.NotEmpty(t, value.Tabs[1].Points)
}

func TestCodexRadarCommunityRejectsOversizedAndRemoteFailure(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", codexRadarMaxResponseBytes+1), ""} {
		client := &http.Client{Transport: codexRadarRoundTripper(func(_ *http.Request) (*http.Response, error) {
			if body == "" {
				return nil, errors.New("network down")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		_, _, err := NewCodexRadarCommunityService(client, time.Now).Get(context.Background())
		require.ErrorIs(t, err, ErrCodexRadarCommunityUnavailable)
	}
}
