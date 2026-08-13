package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type usageCostEvidenceRepoStub struct {
	created  *UsageCostEvidence
	inserted bool
	err      error
	calls    int
}

type usageCostEvidenceActivationStub struct {
	enabledAt *time.Time
	err       error
}

func (s usageCostEvidenceActivationStub) EnabledAt(context.Context) (*time.Time, error) {
	return s.enabledAt, s.err
}

func enabledUsageCostEvidenceRegistrar(usageRepo UsageLogRepository, evidenceRepo UsageCostEvidenceRepository) *UsageCostEvidenceRegistrar {
	enabledAt := time.Unix(0, 0).UTC()
	return NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo, usageCostEvidenceActivationStub{enabledAt: &enabledAt})
}

func (s *usageCostEvidenceRepoStub) CreateOnce(_ context.Context, evidence *UsageCostEvidence) (bool, error) {
	s.calls++
	s.created = evidence
	return s.inserted, s.err
}

func TestUsageCostEvidenceRegistrarRegistersConfirmedAndConfirmedZero(t *testing.T) {
	for _, tc := range []struct {
		name       string
		actualCost string
		status     UsageCostEvidenceStatus
	}{
		{name: "nonzero", actualCost: "0.004", status: UsageCostEvidenceStatusConfirmed},
		{name: "zero", actualCost: "0", status: UsageCostEvidenceStatusConfirmedZero},
		{name: "blank", actualCost: `""`, status: UsageCostEvidenceStatusConfirmedZero},
		{name: "null", actualCost: "null", status: UsageCostEvidenceStatusConfirmedZero},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstreamID := "provider-usage-1"
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte(`{"data":[{"request_id":"local-usage-1","upstream_request_id":"provider-usage-1","actual_cost":` + tc.actualCost + `}]}`))
			}))
			defer server.Close()

			usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
				ID: 91, RequestID: "local-usage-1", UpstreamRequestID: &upstreamID, ActualCost: 0.006,
				CreatedAt: time.Now(), Account: &Account{ID: 7, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}},
			}}
			evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}
			registrar := enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo)

			require.NoError(t, registrar.RegisterOnce(context.Background(), 91))
			require.Equal(t, 1, requests)
			require.Equal(t, tc.status, evidenceRepo.created.Status)
			require.Equal(t, UsageCostEvidenceSourceSub, evidenceRepo.created.Source)
			require.Equal(t, int64(91), evidenceRepo.created.UsageLogID)
		})
	}
}

func TestUsageCostEvidenceRegistrarStoresUnavailableWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	upstreamID := "missing-provider-usage"
	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 92, RequestID: "local-usage-2", UpstreamRequestID: &upstreamID, ActualCost: 0.006,
		CreatedAt: time.Now(), Account: &Account{ID: 8, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 92))
	require.Equal(t, 1, requests)
	require.Equal(t, UsageCostEvidenceStatusUnavailable, evidenceRepo.created.Status)
	require.Equal(t, "record_not_found", evidenceRepo.created.ReasonCode)
}

func TestUsageCostEvidenceRegistrarRejectsLocalRequestIDFallback(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":[{"request_id":"local-usage-3","actual_cost":0.004}]}`))
	}))
	defer server.Close()

	upstreamID := "provider-usage-3"
	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 93, RequestID: "local-usage-3", UpstreamRequestID: &upstreamID, ActualCost: 0.006,
		CreatedAt: time.Now(), Account: &Account{ID: 9, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 93))
	require.Equal(t, 1, requests)
	require.Equal(t, UsageCostEvidenceStatusUnavailable, evidenceRepo.created.Status)
	require.Equal(t, "record_not_found", evidenceRepo.created.ReasonCode)
}

func TestUsageCostEvidenceRegistrarStoresUnavailableWithoutRequestIDOrHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 95, RequestID: "local-only", CreatedAt: time.Now(),
		Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 95))
	require.Zero(t, requests)
	require.Equal(t, UsageCostEvidenceStatusUnavailable, evidenceRepo.created.Status)
	require.Equal(t, "request_id_missing", evidenceRepo.created.ReasonCode)
}

func TestUsageCostEvidenceRegistrarRegistersNewAPIStructuredEvidence(t *testing.T) {
	upstreamID := "provider-newapi-1"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/log/token":
			_, _ = w.Write([]byte(`{"data":[{"type":2,"quota":125000,"request_id":"local-newapi-1","upstream_request_id":"provider-newapi-1"}]}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":500000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 96, RequestID: "local-newapi-1", UpstreamRequestID: &upstreamID, ActualCost: 0.3, CreatedAt: time.Now(),
		Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		}},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 96))
	require.Equal(t, 2, requests)
	require.Equal(t, UsageCostEvidenceSourceNewAPI, evidenceRepo.created.Source)
	require.Equal(t, UsageCostEvidenceStatusConfirmed, evidenceRepo.created.Status)
	require.InDelta(t, 125000, *evidenceRepo.created.NewAPIQuota, 1e-9)
	require.InDelta(t, 500000, *evidenceRepo.created.NewAPIQuotaPerUnit, 1e-9)
	require.InDelta(t, 0.25, *evidenceRepo.created.NormalizedCostCNY, 1e-9)
}

func TestUsageCostEvidenceRegistrarBoundsSubPaginationToExactMatch(t *testing.T) {
	upstreamID := "provider-page-2"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("cursor") == "next" {
			_, _ = w.Write([]byte(`{"data":[{"request_id":"local-page-2","upstream_request_id":"provider-page-2","actual_cost":0.004}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"request_id":"other","upstream_request_id":"other","actual_cost":99}],"has_more":true,"next_cursor":"next"}`))
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 97, RequestID: "local-page-2", UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 97))
	require.Equal(t, 2, requests)
	require.Equal(t, UsageCostEvidenceStatusConfirmed, evidenceRepo.created.Status)
}

func TestUsageCostEvidenceRegistrarBoundsSubFallbackToNewLedgerAndUnit(t *testing.T) {
	upstreamID := "provider-fallback-new"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/v1/usage/records":
			http.NotFound(w, r)
		case "/api/log/token":
			_, _ = w.Write([]byte(`{"data":[{"type":2,"quota":1000,"request_id":"local-fallback-new","upstream_request_id":"provider-fallback-new"}]}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":1000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 98, RequestID: "local-fallback-new", UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 98))
	require.Equal(t, 3, requests)
	require.Equal(t, UsageCostEvidenceStatusConfirmed, evidenceRepo.created.Status)
	require.InDelta(t, 1, *evidenceRepo.created.NormalizedCostCNY, 1e-9)
}

func TestUsageCostEvidenceRegistrarSkipsOAuthWithoutHTTPOrEvidence(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 93, Account: &Account{ID: 9, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 93))
	require.Zero(t, requests)
	require.Zero(t, evidenceRepo.calls)
}

func TestUsageCostEvidenceRegistrarReturnsRepositoryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 94, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
	wantErr := errors.New("evidence insert failed")
	evidenceRepo := &usageCostEvidenceRepoStub{err: wantErr}

	require.ErrorIs(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 94), wantErr)
}

func TestUsageCostEvidenceRegistrarActivationBoundary(t *testing.T) {
	activation := time.Date(2026, 8, 13, 2, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		enabledAt *time.Time
		createdAt time.Time
		wantCalls int
	}{
		{name: "setting missing", createdAt: activation.Add(time.Minute)},
		{name: "enabled at missing", createdAt: activation.Add(time.Minute)},
		{name: "pre enable", enabledAt: &activation, createdAt: activation.Add(-time.Nanosecond)},
		{name: "exactly at enable", enabledAt: &activation, createdAt: activation, wantCalls: 1},
		{name: "after enable", enabledAt: &activation, createdAt: activation.Add(time.Nanosecond), wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte(`{"data":[{"request_id":"local-boundary","upstream_request_id":"provider-boundary","actual_cost":0.004}]}`))
			}))
			defer server.Close()
			upstreamID := "provider-boundary"
			usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 101, RequestID: "local-boundary", UpstreamRequestID: &upstreamID, CreatedAt: tc.createdAt, Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
			evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}
			registrar := NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo, usageCostEvidenceActivationStub{enabledAt: tc.enabledAt})

			require.NoError(t, registrar.RegisterOnce(context.Background(), 101))
			require.Equal(t, tc.wantCalls, requests)
			require.Equal(t, tc.wantCalls, evidenceRepo.calls)
		})
	}
}

func TestUsageCostEvidenceRegistrarRejectsNonNativeAccountTypes(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeServiceAccount, AccountTypeBedrock, ""} {
		t.Run(accountType, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
			defer server.Close()
			upstreamID := "provider-excluded"
			usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 102, UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{Type: accountType, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
			evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

			require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 102))
			require.Zero(t, requests)
			require.Zero(t, evidenceRepo.calls)
		})
	}
}

func TestNormalizeUsageCostEvidenceReason(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"request_id_unavailable", "request_id_missing"},
		{"request_id_missing", "request_id_missing"},
		{"endpoint_unavailable", "endpoint_unsupported"},
		{"pagination_unavailable", "response_unavailable"},
		{"credentials_unavailable", "credentials_unavailable"},
		{"authentication_rejected", "authentication_rejected"},
		{"request_unavailable", "request_unavailable"},
		{"response_unavailable", "response_unavailable"},
		{"endpoint_unsupported", "endpoint_unsupported"},
		{"record_not_found", "record_not_found"},
		{"unknown_internal_reason", "response_unavailable"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeUsageCostEvidenceReason(tc.input))
		})
	}
}
