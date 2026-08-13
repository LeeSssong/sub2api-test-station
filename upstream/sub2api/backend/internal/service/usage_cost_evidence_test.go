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
			registrar := NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo)

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

	require.NoError(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 92))
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

	require.NoError(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 93))
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

	require.NoError(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 95))
	require.Zero(t, requests)
	require.Equal(t, UsageCostEvidenceStatusUnavailable, evidenceRepo.created.Status)
	require.Equal(t, "request_id_unavailable", evidenceRepo.created.ReasonCode)
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

	require.NoError(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 96))
	require.Equal(t, 2, requests)
	require.Equal(t, UsageCostEvidenceSourceNewAPI, evidenceRepo.created.Source)
	require.Equal(t, UsageCostEvidenceStatusConfirmed, evidenceRepo.created.Status)
	require.InDelta(t, 125000, *evidenceRepo.created.NewAPIQuota, 1e-9)
	require.InDelta(t, 500000, *evidenceRepo.created.NewAPIQuotaPerUnit, 1e-9)
	require.InDelta(t, 0.25, *evidenceRepo.created.NormalizedCostCNY, 1e-9)
}

func TestUsageCostEvidenceRegistrarSkipsOAuthWithoutHTTPOrEvidence(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 93, Account: &Account{ID: 9, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 93))
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

	require.ErrorIs(t, NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 94), wantErr)
}
