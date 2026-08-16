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

type newAPIRateRefreshRepoStub struct {
	claimed   bool
	completed []NewAPIRateRefreshCompletion
}

func (s *newAPIRateRefreshRepoStub) ClaimNewAPIRateRefresh(context.Context, int64, string, string, time.Time) (bool, error) {
	return s.claimed, nil
}

func (s *newAPIRateRefreshRepoStub) CompleteNewAPIRateRefresh(_ context.Context, input NewAPIRateRefreshCompletion) error {
	s.completed = append(s.completed, input)
	return nil
}

func (s *newAPIRateRefreshRepoStub) ReleaseNewAPIRateRefresh(context.Context, int64, string) error {
	return nil
}

func (s usageCostEvidenceActivationStub) EnabledAt(context.Context) (*time.Time, error) {
	return s.enabledAt, s.err
}

func enabledUsageCostEvidenceRegistrar(usageRepo UsageLogRepository, evidenceRepo UsageCostEvidenceRepository) *UsageCostEvidenceRegistrar {
	enabledAt := time.Unix(0, 0).UTC()
	return NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo, usageCostEvidenceActivationStub{enabledAt: &enabledAt})
}

func subLedgerEvidenceExtra() map[string]any {
	return map[string]any{UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusOK}}
}

func newAPILedgerEvidenceExtra() map[string]any {
	return map[string]any{AccountMonitorBalanceExtraKey: AccountMonitorBalance{
		Version: AccountMonitorBalanceVersion,
		Source:  AccountMonitorBalanceSourceNewAPI,
		Status:  AccountMonitorBalanceStatusOK,
	}}
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
				CreatedAt: time.Now(), Account: &Account{ID: 7, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()},
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
		CreatedAt: time.Now(), Account: &Account{ID: 8, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()},
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
		CreatedAt: time.Now(), Account: &Account{ID: 9, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()},
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
		Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()},
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
		Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: newAPILedgerEvidenceExtra()},
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

func TestUsageCostEvidenceRegistrarReusesNewAPILogForRateRegistration(t *testing.T) {
	upstreamID := "provider-newapi-rate-1"
	logRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/log/token":
			logRequests++
			_, _ = w.Write([]byte(`{"data":[{"type":2,"quota":125000,"request_id":"local-newapi-rate-1","upstream_request_id":"provider-newapi-rate-1","other":"{\"group_ratio\":0.17}"}]}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":500000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 196, RequestID: "local-newapi-rate-1", UpstreamRequestID: &upstreamID, ActualCost: 0.3, CreatedAt: time.Now(),
		Account: &Account{ID: 196, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		}},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}
	rateRepo := &newAPIRateRefreshRepoStub{claimed: true}
	registrar := enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo)
	registrar.SetNewAPIRateRefreshRepository(rateRepo)

	require.NoError(t, registrar.RegisterOnce(context.Background(), 196))
	require.Equal(t, 1, logRequests)
	require.Len(t, rateRepo.completed, 1)
	require.InDelta(t, 0.17, rateRepo.completed[0].GroupRatio, 1e-9)
}

func TestUsageCostEvidenceRegistrarPreservesNewAPILookupFailureReasonWithoutSecondLookup(t *testing.T) {
	upstreamID := "provider-newapi-failure"
	logRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/log/token" {
			http.NotFound(w, r)
			return
		}
		logRequests++
		if r.URL.Query().Get("cursor") == "next" {
			_, _ = w.Write([]byte(`{"data":[],"next_cursor":"next-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"next_cursor":"next"}`))
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{
		ID: 197, RequestID: "local-newapi-failure", UpstreamRequestID: &upstreamID, ActualCost: 0.3, CreatedAt: time.Now(),
		Account: &Account{ID: 197, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: newAPILedgerEvidenceExtra()},
	}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}
	rateRepo := &newAPIRateRefreshRepoStub{claimed: true}
	registrar := enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo)
	registrar.SetNewAPIRateRefreshRepository(rateRepo)

	require.NoError(t, registrar.RegisterOnce(context.Background(), 197))
	require.Equal(t, 10, logRequests)
	require.Equal(t, UsageCostEvidenceStatusUnavailable, evidenceRepo.created.Status)
	require.Equal(t, "response_unavailable", evidenceRepo.created.ReasonCode)
	require.Empty(t, rateRepo.completed)
}

func TestBeijingRefreshDateUsesExplicitAsiaShanghaiLocation(t *testing.T) {
	date, err := beijingRefreshDate(time.Date(2026, 8, 15, 16, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "2026-08-16", date)
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

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 97, RequestID: "local-page-2", UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 97))
	require.Equal(t, 2, requests)
	require.Equal(t, UsageCostEvidenceStatusConfirmed, evidenceRepo.created.Status)
}

func TestUsageCostEvidenceRegistrarBoundsKnownNewLedgerAndUnit(t *testing.T) {
	upstreamID := "provider-known-new"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/log/token":
			_, _ = w.Write([]byte(`{"data":[{"type":2,"quota":1000,"request_id":"local-known-new","upstream_request_id":"provider-known-new"}]}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":1000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 98, RequestID: "local-known-new", UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: newAPILedgerEvidenceExtra()}}}
	evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

	require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 98))
	require.Equal(t, 2, requests)
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

	usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 94, CreatedAt: time.Now(), Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()}}}
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
			usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 101, RequestID: "local-boundary", UpstreamRequestID: &upstreamID, CreatedAt: tc.createdAt, Account: &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: subLedgerEvidenceExtra()}}}
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

func TestUsageCostEvidenceRegistrarRequiresKnownNativeLedger(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		extra    map[string]any
	}{
		{name: "official OpenAI", platform: PlatformOpenAI},
		{name: "official Anthropic", platform: PlatformAnthropic},
		{name: "official Gemini", platform: PlatformGemini},
		{name: "official Grok", platform: PlatformGrok},
		{name: "unsupported probe only", platform: PlatformOpenAI, extra: map[string]any{UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported}}},
		{name: "unknown metadata", platform: PlatformOpenAI, extra: map[string]any{"unrelated": "metadata"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
			defer server.Close()
			upstreamID := "provider-excluded"
			usageRepo := &subUpstreamCostUsageRepoStub{record: &UsageLog{ID: 103, UpstreamRequestID: &upstreamID, CreatedAt: time.Now(), Account: &Account{
				Platform: tc.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "secret"}, Extra: tc.extra,
			}}}
			evidenceRepo := &usageCostEvidenceRepoStub{inserted: true}

			require.NoError(t, enabledUsageCostEvidenceRegistrar(usageRepo, evidenceRepo).RegisterOnce(context.Background(), 103))
			require.Zero(t, requests)
			require.Zero(t, evidenceRepo.calls)
		})
	}
}

func TestUsageCostLedgerForAccountUsesPositiveNativeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  usageCostLedgerIdentity
	}{
		{name: "successful probe is Sub", extra: subLedgerEvidenceExtra(), want: usageCostLedgerSub},
		{name: "Sub balance is Sub", extra: map[string]any{AccountMonitorBalanceExtraKey: AccountMonitorBalance{Version: AccountMonitorBalanceVersion, Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK}}, want: usageCostLedgerSub},
		{name: "New balance is New", extra: newAPILedgerEvidenceExtra(), want: usageCostLedgerNewAPI},
		{name: "New balance wins over successful Sub probe", extra: map[string]any{
			AccountMonitorBalanceExtraKey: AccountMonitorBalance{Version: AccountMonitorBalanceVersion, Source: AccountMonitorBalanceSourceNewAPI, Status: AccountMonitorBalanceStatusOK},
			UpstreamBillingProbeExtraKey:  UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusOK},
		}, want: usageCostLedgerNewAPI},
		{name: "unsupported probe remains unknown generic ledger", extra: map[string]any{UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported}}, want: usageCostLedgerUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Type: AccountTypeAPIKey, Extra: tc.extra}
			require.Equal(t, tc.want, usageCostLedgerForAccount(account))
			require.Equal(t, tc.want != usageCostLedgerUnknown, isUsageCostEvidenceEligibleAccount(account))
			require.Equal(t, tc.want == usageCostLedgerNewAPI, evidenceSourceForAccount(account) == UsageCostEvidenceSourceNewAPI)
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
