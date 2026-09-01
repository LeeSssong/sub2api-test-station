package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func TestDecodeSub2APIBalanceUSDReadsBalanceOrRemaining(t *testing.T) {
	tests := []struct {
		name string
		body string
		want float64
	}{
		{name: "balance", body: `{"balance":12.5}`, want: 12.5},
		{name: "nested remaining", body: `{"data":{"remaining":9.75}}`, want: 9.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSub2APIBalanceUSD([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("decodeSub2APIBalanceUSD() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeNewAPIBalanceUSDNormalizesQuotaUnits(t *testing.T) {
	got, err := decodeNewAPIBalanceUSD([]byte(`{"data":{"total_available":600000}}`), 500000)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-1.2) > 1e-9 {
		t.Fatalf("decodeNewAPIBalanceUSD() = %v, want 1.2", got)
	}
}

func TestDecodeBalanceRejectsInvalidPayload(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"balance":-1}`,
		`{"data":{"total_available":100}}`,
	} {
		if _, err := decodeSub2APIBalanceUSD([]byte(body)); err == nil {
			t.Fatalf("decodeSub2APIBalanceUSD(%s) unexpectedly succeeded", body)
		}
	}
	if _, err := decodeNewAPIBalanceUSD([]byte(`{"data":{"total_available":100}}`), 0); err == nil {
		t.Fatal("zero quota_per_unit unexpectedly succeeded")
	}
}

func TestBalanceFailureRetainsLastGoodValue(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	value := 12.5
	observedAt := now.Add(-time.Hour)
	previous := &AccountMonitorBalance{
		Version:       AccountMonitorBalanceVersion,
		ValueUSD:      &value,
		Source:        AccountMonitorBalanceSourceNewAPI,
		Status:        AccountMonitorBalanceStatusOK,
		ObservedAt:    &observedAt,
		LastAttemptAt: &observedAt,
	}

	got := failedAccountMonitorBalance(previous, "upstream_http_error", now)
	if got.Status != AccountMonitorBalanceStatusFailed || got.ValueUSD == nil || *got.ValueUSD != 12.5 {
		t.Fatalf("failed balance snapshot = %#v, want retained value", got)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(observedAt) || got.LastAttemptAt == nil || !got.LastAttemptAt.Equal(now) {
		t.Fatalf("failure timestamps = %#v", got)
	}
}

func TestAccountMonitorBalanceFailureCodeClassifiesHTTPFailures(t *testing.T) {
	if got := accountMonitorBalanceFailureCode(errors.New("account multiplier upstream returned HTTP 502")); got != "upstream_http_error" {
		t.Fatalf("accountMonitorBalanceFailureCode() = %q, want upstream_http_error", got)
	}
	if got := accountMonitorBalanceFailureCode(errors.New("account monitor upstream response is empty")); got == "balance_unavailable" {
		t.Fatalf("empty response must not be classified as balance exhaustion: %q", got)
	}
	if got := accountMonitorBalanceFailureCode(&accountMonitorHTTPError{statusCode: http.StatusBadGateway, body: `{"error":"insufficient quota"}`}); got != "balance_unavailable" {
		t.Fatalf("explicit quota error = %q, want balance_unavailable", got)
	}
	if got := accountMonitorBalanceFailureCode(&accountMonitorHTTPError{statusCode: http.StatusPaymentRequired}); got != "upstream_http_error" {
		t.Fatalf("HTTP 402 without explicit balance evidence = %q, want upstream_http_error", got)
	}
}

func TestDecodeBalanceOnlyVetoesExplicitExhaustionEvidence(t *testing.T) {
	if _, err := decodeSub2APIBalanceUSD([]byte(`{"error":"insufficient balance"}`)); !errors.Is(err, errExplicitBalanceUnavailable) {
		t.Fatalf("explicit balance exhaustion error = %v", err)
	}
	if _, err := decodeSub2APIBalanceUSD([]byte(`{"error":"temporary upstream failure"}`)); errors.Is(err, errExplicitBalanceUnavailable) {
		t.Fatal("generic upstream payload must not be classified as balance exhaustion")
	}
}

func TestResolveBalanceOnlyProjectsOpenAIAPIKey(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	value := 12.5
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountMonitorBalanceExtraKey: AccountMonitorBalance{
			Version: AccountMonitorBalanceVersion, ValueUSD: &value,
			Source: AccountMonitorBalanceSourceSub2API, Status: AccountMonitorBalanceStatusOK,
			ObservedAt: probeTimePtr(now), LastAttemptAt: probeTimePtr(now),
		},
	}}
	got := NewAccountMultiplierService(nil, nil, nil).ResolveBalance(account, now)
	if got.ValueUSD == nil || *got.ValueUSD != value || got.Status != AccountMonitorBalanceStatusOK {
		t.Fatalf("ResolveBalance() = %#v", got)
	}
	if got := NewAccountMultiplierService(nil, nil, nil).ResolveBalance(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, now); got.Status != AccountMonitorBalanceStatusUnavailable || got.ValueUSD != nil {
		t.Fatalf("non API-key balance = %#v", got)
	}
}

func TestRefreshBalanceSelectsExactlyOneSourceFromDeclaration(t *testing.T) {
	tests := []struct {
		name        string
		declaration *UpstreamBillingProbeSnapshot
		baseURL     string
		responses   map[string]string
		wantSource  string
		wantPaths   []string
	}{
		{
			name:        "supported declaration uses Sub2API usage only",
			declaration: &UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusOK},
			responses:   map[string]string{"/v1/usage": `{"balance":12.5}`},
			wantSource:  AccountMonitorBalanceSourceSub2API,
			wantPaths:   []string{"/v1/usage"},
		},
		{
			name:        "Sub2API API-prefixed base preserves usage path",
			declaration: &UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusOK},
			baseURL:     "http://balance.example/api",
			responses:   map[string]string{"/api/v1/usage": `{"balance":12.5}`},
			wantSource:  AccountMonitorBalanceSourceSub2API,
			wantPaths:   []string{"/api/v1/usage"},
		},
		{
			name:        "explicitly unsupported declaration uses New API only",
			declaration: &UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
			responses: map[string]string{
				"/api/status":       `{"data":{"quota_per_unit":500000}}`,
				"/api/usage/token/": `{"data":{"total_available":600000}}`,
			},
			wantSource: AccountMonitorBalanceSourceNewAPI,
			wantPaths:  []string{"/api/status", "/api/usage/token/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
			baseURL := tt.baseURL
			if baseURL == "" {
				baseURL = "http://balance.example"
			}
			account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL},
				Extra:       map[string]any{UpstreamBillingProbeExtraKey: *tt.declaration},
			}
			baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			repo := &accountMultiplierRepoStub{upstreamBillingProbeAccountRepo: baseRepo}
			upstream := &accountMonitorBalanceHTTPStub{responses: tt.responses}
			svc := NewAccountMultiplierService(repo, &AccountTestService{
				accountRepo: repo, httpUpstream: upstream,
				cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
					Enabled: false, AllowInsecureHTTP: true,
				}}},
			}, nil)
			svc.now = func() time.Time { return now }

			if err := svc.Refresh(context.Background(), account, AccountMonitorRefreshOptions{RefreshBalance: true}); err != nil {
				t.Fatal(err)
			}
			got := decodeAccountMonitorBalance(account.Extra)
			if got == nil || got.Source != tt.wantSource || got.Status != AccountMonitorBalanceStatusOK {
				t.Fatalf("stored balance = %#v", got)
			}
			expectedHash := sha256.Sum256([]byte("sk-test"))
			if got.CredentialFingerprint != hex.EncodeToString(expectedHash[:]) {
				t.Fatalf("credential fingerprint = %q", got.CredentialFingerprint)
			}
			if !reflect.DeepEqual(upstream.paths, tt.wantPaths) {
				t.Fatalf("balance paths = %#v, want %#v", upstream.paths, tt.wantPaths)
			}
		})
	}
}

type accountMonitorBalanceHTTPStub struct {
	paths     []string
	responses map[string]string
}

func (s *accountMonitorBalanceHTTPStub) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (s *accountMonitorBalanceHTTPStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if req.Header.Get("Authorization") != "Bearer sk-test" {
		return nil, errorsForAccountMultiplierTest("missing account authorization")
	}
	s.paths = append(s.paths, req.URL.Path)
	body, ok := s.responses[req.URL.Path]
	if !ok {
		return accountMultiplierJSONResponse(http.StatusNotFound, `{}`), nil
	}
	return accountMultiplierJSONResponse(http.StatusOK, body), nil
}
