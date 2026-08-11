package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type subUpstreamCostUsageRepoStub struct {
	UsageLogRepository
	record *UsageLog
	err    error
}

func (s *subUpstreamCostUsageRepoStub) GetByID(context.Context, int64) (*UsageLog, error) {
	return s.record, s.err
}

func TestSubUpstreamCostServiceConfirmsMatchedActualCost(t *testing.T) {
	createdAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
	upstreamID := "upstream-req-456"
	var gotAuth string
	var gotPath string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"request_id":          "local-req-123",
				"upstream_request_id": upstreamID,
				"actual_cost":         0.004,
			}},
			"has_more": false,
		})
	}))
	defer server.Close()

	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID:                42,
		RequestID:         "local-req-123",
		UpstreamRequestID: &upstreamID,
		ActualCost:        0.00688,
		CreatedAt:         createdAt,
		Account: &Account{Credentials: map[string]any{
			"base_url": server.URL + "/v1",
			"api_key":  "stored-upstream-key",
		}},
	}}
	svc := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil))

	detail, err := svc.GetByUsageID(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, int64(42), detail.UsageID)
	require.Equal(t, "local-req-123", detail.LocalRequestID)
	require.Equal(t, upstreamID, *detail.UpstreamRequestID)
	require.Equal(t, "confirmed", detail.Status)
	require.NotNil(t, detail.UpstreamActualCost)
	require.InDelta(t, 0.004, *detail.UpstreamActualCost, 1e-9)
	require.NotNil(t, detail.Profit)
	require.InDelta(t, 0.00288, *detail.Profit, 1e-9)
	require.Equal(t, "Bearer stored-upstream-key", gotAuth)
	require.Equal(t, "/v1/usage/records", gotPath)
	require.Equal(t, "1000", gotQuery.Get("limit"))
	start, err := time.Parse(time.RFC3339Nano, gotQuery.Get("start_time"))
	require.NoError(t, err)
	end, err := time.Parse(time.RFC3339Nano, gotQuery.Get("end_time"))
	require.NoError(t, err)
	require.Equal(t, createdAt.Add(-10*time.Minute), start)
	require.Equal(t, createdAt.Add(10*time.Minute), end)
}

func TestSubUpstreamCostServiceConfirmsNewAPIQuotaCostForExactRequestMatch(t *testing.T) {
	createdAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
	upstreamID := "provider-req-456"
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		require.Equal(t, "Bearer stored-upstream-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/log/token":
			require.Equal(t, "1000", r.URL.Query().Get("limit"))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"type":                2,
				"quota":               125000,
				"request_id":          "local-req-123",
				"upstream_request_id": upstreamID,
			}}})
		case "/api/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"quota_per_unit": 500000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID:                43,
		RequestID:         "local-req-123",
		UpstreamRequestID: &upstreamID,
		ActualCost:        0.00688,
		CreatedAt:         createdAt,
		Account: &Account{Credentials: map[string]any{
			"base_url": server.URL + "/v1",
			"api_key":  "stored-upstream-key",
		}, Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		}},
	}}

	detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 43)

	require.NoError(t, err)
	require.Equal(t, "confirmed", detail.Status)
	require.NotNil(t, detail.UpstreamActualCost)
	require.InDelta(t, 0.25, *detail.UpstreamActualCost, 1e-9)
	require.NotNil(t, detail.Profit)
	require.InDelta(t, -0.24312, *detail.Profit, 1e-9)
	require.Equal(t, []string{"/api/log/token", "/api/status"}, gotPaths)
}

func TestSubUpstreamCostServiceConfirmsNewAPIQuotaCostForAPIInferenceBase(t *testing.T) {
	createdAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
	upstreamID := "provider-api-base-456"
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		require.Equal(t, "Bearer stored-upstream-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/log/token":
			require.Equal(t, "1000", r.URL.Query().Get("limit"))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"type": 2, "quota": 125000, "request_id": "local-api-base", "upstream_request_id": upstreamID,
			}}})
		case "/api/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"quota_per_unit": 500000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID:                45,
		RequestID:         "local-api-base",
		UpstreamRequestID: &upstreamID,
		ActualCost:        0.00688,
		CreatedAt:         createdAt,
		Account: &Account{Credentials: map[string]any{
			"base_url": server.URL + "/api/v1",
			"api_key":  "stored-upstream-key",
		}, Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		}},
	}}

	detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 45)

	require.NoError(t, err)
	require.Equal(t, "confirmed", detail.Status)
	require.Equal(t, "local-api-base", detail.LocalRequestID)
	require.Equal(t, upstreamID, *detail.UpstreamRequestID)
	require.NotNil(t, detail.UpstreamActualCost)
	require.InDelta(t, 0.25, *detail.UpstreamActualCost, 1e-9)
	require.NotNil(t, detail.Profit)
	require.InDelta(t, -0.24312, *detail.Profit, 1e-9)
	require.Equal(t, []string{"/api/log/token", "/api/status"}, gotPaths)
}

func TestSubUpstreamCostServiceFallsBackToNewAPILedgerWhenProbeMetadataIsUnresolved(t *testing.T) {
	for _, probeStatus := range []string{"missing", UpstreamBillingProbeStatusFailed, "stale"} {
		t.Run(probeStatus, func(t *testing.T) {
			createdAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
			upstreamID := "provider-req-fallback"
			var gotPaths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				switch r.URL.Path {
				case "/v1/usage/records":
					http.NotFound(w, r)
				case "/api/log/token":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
						"type": 2, "quota": 125000, "request_id": "local-fallback", "upstream_request_id": upstreamID,
					}}})
				case "/api/status":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"quota_per_unit": 500000}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			extra := map[string]any{}
			if probeStatus != "missing" {
				extra[UpstreamBillingProbeExtraKey] = UpstreamBillingProbeSnapshot{Status: probeStatus}
			}
			repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
				ID: 44, RequestID: "local-fallback", UpstreamRequestID: &upstreamID,
				ActualCost: 0.00688, CreatedAt: createdAt,
				Account: &Account{Credentials: map[string]any{"base_url": server.URL + "/v1", "api_key": "stored-upstream-key"}, Extra: extra},
			}}

			detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 44)
			require.NoError(t, err)
			require.Equal(t, "confirmed", detail.Status, "detail=%#v paths=%#v", detail, gotPaths)
			require.NotNil(t, detail.UpstreamActualCost)
			require.InDelta(t, 0.25, *detail.UpstreamActualCost, 1e-9)
			require.Equal(t, []string{"/v1/usage/records", "/api/log/token", "/api/status"}, gotPaths)
		})
	}
}

func TestSubUpstreamCostServiceUsesExactMatchingFallbackOrder(t *testing.T) {
	localUpstreamID := "provider-id"
	cases := []struct {
		name string
		row  map[string]any
	}{
		{name: "remote upstream id", row: map[string]any{"request_id": "other", "upstream_request_id": localUpstreamID, "actual_cost": 0}},
		{name: "remote local id", row: map[string]any{"request_id": localUpstreamID, "upstream_request_id": "other", "actual_cost": 0.004}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{tc.row}})
			}))
			defer server.Close()
			repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
				ID: 1, RequestID: "local-req", UpstreamRequestID: &localUpstreamID,
				ActualCost: 0.006, CreatedAt: time.Now(),
				Account: &Account{Credentials: map[string]any{"base_url": server.URL, "api_key": "k"}},
			}}
			detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 1)
			require.NoError(t, err)
			require.Equal(t, "confirmed", detail.Status)
		})
	}
}

func TestSubUpstreamCostServicePrefersStrongerMatchAcrossRows(t *testing.T) {
	localUpstreamID := "provider-id"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"request_id": "local-req", "upstream_request_id": "other", "actual_cost": 0.009},
			{"request_id": "other", "upstream_request_id": localUpstreamID, "actual_cost": 0.004},
		}})
	}))
	defer server.Close()
	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 1, RequestID: "local-req", UpstreamRequestID: &localUpstreamID,
		ActualCost: 0.006, CreatedAt: time.Now(),
		Account: &Account{Credentials: map[string]any{"base_url": server.URL, "api_key": "k"}},
	}}
	detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "confirmed", detail.Status)
	require.InDelta(t, 0.004, *detail.UpstreamActualCost, 1e-9)
}

func TestSubUpstreamCostServiceReturnsUnavailableWithoutCredentials(t *testing.T) {
	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 1, RequestID: "local", ActualCost: 0.1, Account: &Account{}}}
	detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "unavailable", detail.Status)
	require.Equal(t, "credentials_unavailable", detail.ReasonCode)
	require.Nil(t, detail.UpstreamActualCost)
	require.Nil(t, detail.Profit)
	require.NotEmpty(t, detail.Reason)
	require.NotContains(t, strings.ToLower(detail.Reason), "api")
}

func TestSubUpstreamCostServiceClassifiesUnsupportedUsageEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	upstreamID := "provider-id"
	repo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 1, RequestID: "local-req", UpstreamRequestID: &upstreamID,
		ActualCost: 0.006, CreatedAt: time.Now(),
		Account: &Account{Credentials: map[string]any{"base_url": server.URL, "api_key": "k"}},
	}}

	detail, err := NewSubUpstreamCostService(NewUsageService(repo, nil, nil, nil)).GetByUsageID(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, "unavailable", detail.Status)
	require.Equal(t, "endpoint_unsupported", detail.ReasonCode)
	require.Nil(t, detail.UpstreamActualCost)
	require.Nil(t, detail.Profit)
}
