package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func TestAccountMultiplierResolveDeclaredUsesFreshEffectiveRate(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{
		RateMultiplier: float64Pointer(1),
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data: map[string]any{
					"billing_scope":             "token",
					"resolved_rate_multiplier":  0.17,
					"peak_rate_enabled":         false,
					"effective_rate_multiplier": 0.17,
				},
				ReceivedAt: probeTimePtr(now.Add(-time.Minute)),
				FreshUntil: probeTimePtr(now.Add(time.Hour)),
			},
		},
	}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceDeclared {
		t.Fatalf("Resolve() = %#v", got)
	}
	if got.Value == nil || math.Abs(*got.Value-0.17) > 1e-9 {
		t.Fatalf("Resolve().Value = %#v, want 0.17", got.Value)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("Resolve().ObservedAt = %#v", got.ObservedAt)
	}
}

func TestAccountMultiplierResolveDoesNotUseLocalBillingMultiplier(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{RateMultiplier: float64Pointer(0.08)}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)

	if got.Value != nil || got.Status != AccountMonitorMultiplierStatusUnavailable {
		t.Fatalf("Resolve() = %#v, want unavailable without value", got)
	}
	encoded, err := json.Marshal(AccountMonitorAccount{Multiplier: got})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == `{"multiplier":0.08}` {
		t.Fatalf("projection leaked local billing multiplier: %s", encoded)
	}
}

func TestAccountMultiplierResolveRejectsExpiredAndNonFiniteDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		value  float64
		expiry time.Time
		status string
	}{
		{name: "expired", value: 0.1, expiry: now.Add(-time.Second), status: AccountMonitorMultiplierStatusStale},
		{name: "nan", value: math.NaN(), expiry: now.Add(time.Hour), status: AccountMonitorMultiplierStatusFailed},
		{name: "infinite", value: math.Inf(1), expiry: now.Add(time.Hour), status: AccountMonitorMultiplierStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Extra: map[string]any{
				UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
					Status: UpstreamBillingProbeStatusOK,
					Data: map[string]any{
						"billing_scope":             "token",
						"resolved_rate_multiplier":  tt.value,
						"peak_rate_enabled":         false,
						"effective_rate_multiplier": tt.value,
					},
					ReceivedAt: probeTimePtr(now.Add(-time.Minute)),
					FreshUntil: probeTimePtr(tt.expiry),
				},
			}}

			got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)
			if got.Value != nil || got.Status != tt.status {
				t.Fatalf("Resolve() = %#v, want status %q without value", got, tt.status)
			}
		})
	}
}

func TestCalculateAccountMultiplierSampleConvertsQuotaDeltaToOfficialRate(t *testing.T) {
	got, err := calculateAccountMultiplierSample(100, 250, 500_000, 0.0012)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("calculateAccountMultiplierSample() = %v, want 0.25", got)
	}
}

func TestCalculateAccountMultiplierSampleRejectsInvalidEvidence(t *testing.T) {
	tests := []struct {
		name         string
		before       float64
		after        float64
		quotaPerUnit float64
		officialCost float64
	}{
		{name: "zero delta", before: 100, after: 100, quotaPerUnit: 500_000, officialCost: 0.001},
		{name: "negative delta", before: 101, after: 100, quotaPerUnit: 500_000, officialCost: 0.001},
		{name: "zero quota unit", before: 100, after: 101, quotaPerUnit: 0, officialCost: 0.001},
		{name: "zero official cost", before: 100, after: 101, quotaPerUnit: 500_000, officialCost: 0},
		{name: "non finite", before: 100, after: math.Inf(1), quotaPerUnit: 500_000, officialCost: 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := calculateAccountMultiplierSample(tt.before, tt.after, tt.quotaPerUnit, tt.officialCost); err == nil {
				t.Fatal("expected invalid evidence error")
			}
		})
	}
}

func TestSummarizeAccountMultiplierSamplesUsesMedianWithinVarianceGate(t *testing.T) {
	value, spread, err := summarizeAccountMultiplierSamples([]float64{0.26, 0.24, 0.25})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(value-0.25) > 1e-9 {
		t.Fatalf("median = %v, want 0.25", value)
	}
	if math.Abs(spread-0.08) > 1e-9 {
		t.Fatalf("relative spread = %v, want 0.08", spread)
	}
}

func TestSummarizeAccountMultiplierSamplesRejectsHighVariance(t *testing.T) {
	if _, _, err := summarizeAccountMultiplierSamples([]float64{0.20, 0.25, 0.40}); err == nil {
		t.Fatal("expected high variance error")
	}
}

func TestAccountMultiplierResolveUsesFreshMeasurementOnlyAfterUnsupportedDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	value := 0.25
	account := &Account{Extra: map[string]any{
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
			Status:     UpstreamBillingProbeStatusUnsupported,
			HTTPStatus: 404,
		},
		AccountMultiplierMeasurementExtraKey: AccountMultiplierMeasurementSnapshot{
			Version:     1,
			Status:      AccountMonitorMultiplierStatusOK,
			Source:      AccountMonitorMultiplierSourceMeasured,
			Value:       &value,
			ModelID:     "gpt-4o-mini",
			SampleCount: 3,
			ObservedAt:  probeTimePtr(now.Add(-time.Hour)),
			FreshUntil:  probeTimePtr(now.Add(23 * time.Hour)),
		},
	}}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)
	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceMeasured {
		t.Fatalf("Resolve() = %#v", got)
	}
	if got.Value == nil || math.Abs(*got.Value-value) > 1e-9 {
		t.Fatalf("Resolve().Value = %#v", got.Value)
	}

	account.Extra[UpstreamBillingProbeExtraKey] = UpstreamBillingProbeSnapshot{
		Status: UpstreamBillingProbeStatusFailed,
	}
	got = NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)
	if got.Status != AccountMonitorMultiplierStatusFailed || got.Value != nil {
		t.Fatalf("failed native declaration must not silently use measurement: %#v", got)
	}
}

type accountMultiplierRepoStub struct {
	*upstreamBillingProbeAccountRepo
}

type accountMultiplierDeclarationProbeStub struct {
	calls    []int64
	snapshot *UpstreamBillingProbeSnapshot
	err      error
}

func (s *accountMultiplierDeclarationProbeStub) ProbeAccount(_ context.Context, accountID int64) (*UpstreamBillingProbeSnapshot, error) {
	s.calls = append(s.calls, accountID)
	return s.snapshot, s.err
}

func (r *accountMultiplierRepoStub) UpdateAccountMultiplierMeasurement(
	_ context.Context,
	expected *Account,
	snapshot *AccountMultiplierMeasurementSnapshot,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil ||
		account.Platform != expected.Platform ||
		account.Type != expected.Type ||
		!reflect.DeepEqual(account.Credentials, expected.Credentials) ||
		!reflect.DeepEqual(account.ProxyID, expected.ProxyID) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[AccountMultiplierMeasurementExtraKey] = snapshot
	return nil
}

type accountMultiplierHTTPStub struct {
	mu               sync.Mutex
	usageValues      []float64
	usageIndex       int
	completionCalls  int
	expectedAPIKey   string
	expectedModel    string
	expectedOverride string
	paths            []string
	lastError        string
}

func (s *accountMultiplierHTTPStub) Do(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *accountMultiplierHTTPStub) DoWithTLS(
	req *http.Request,
	_ string,
	_ int64,
	_ int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, req.URL.Path)
	if req.Header.Get("Authorization") != "Bearer "+s.expectedAPIKey {
		s.lastError = "missing account authorization"
		return nil, errorsForAccountMultiplierTest("missing account authorization")
	}
	if s.expectedOverride != "" && accountMultiplierTestHeader(req.Header, "X-Monitor-Test") != s.expectedOverride {
		s.lastError = "missing account header override"
		return nil, errorsForAccountMultiplierTest("missing account header override")
	}
	switch req.URL.Path {
	case "/api/status":
		return accountMultiplierJSONResponse(http.StatusOK, `{"success":true,"data":{"quota_per_unit":500000}}`), nil
	case "/api/usage/token/":
		if s.usageIndex >= len(s.usageValues) {
			s.lastError = "unexpected quota read"
			return nil, errorsForAccountMultiplierTest("unexpected quota read")
		}
		value := s.usageValues[s.usageIndex]
		s.usageIndex++
		body, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"total_used": value}})
		return accountMultiplierJSONResponse(http.StatusOK, string(body)), nil
	case "/v1/chat/completions":
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload["model"] != s.expectedModel || payload["stream"] != false || payload["temperature"] != float64(0) {
			s.lastError = "unexpected completion payload"
			return nil, errorsForAccountMultiplierTest("unexpected completion payload")
		}
		s.completionCalls++
		return accountMultiplierJSONResponse(http.StatusOK, `{
			"id":"chatcmpl-monitor",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],
			"usage":{"prompt_tokens":1000,"completion_tokens":100,"total_tokens":1100}
		}`), nil
	default:
		return accountMultiplierJSONResponse(http.StatusNotFound, `{}`), nil
	}
}

func accountMultiplierTestHeader(header http.Header, name string) string {
	for key, values := range header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func accountMultiplierJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type accountMultiplierTestError string

func (e accountMultiplierTestError) Error() string { return string(e) }

func errorsForAccountMultiplierTest(message string) error {
	return accountMultiplierTestError(message)
}

func TestAccountMultiplierRefreshMeasuresNewAPIThreeTimesAndPersistsSanitizedSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	billing := NewBillingService(nil, nil)
	cost, err := billing.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 1000, OutputTokens: 100}, 1)
	if err != nil {
		t.Fatal(err)
	}
	const quotaPerUnit = 500_000.0
	samples := []float64{0.24, 0.25, 0.26}
	usageValues := make([]float64, 0, len(samples)*2)
	totalUsed := 10_000.0
	for _, sample := range samples {
		usageValues = append(usageValues, totalUsed)
		totalUsed += cost.TotalCost * quotaPerUnit * sample
		usageValues = append(usageValues, totalUsed)
	}
	account := &Account{
		ID:          21,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":                 "sk-sensitive",
			"base_url":                "http://new-api.example",
			"header_override_enabled": true,
			"model_mapping": map[string]any{
				"gpt-5.4": "upstream-mini",
			},
			"header_overrides": map[string]any{
				"X-Monitor-Test": "mapped",
			},
		},
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status:     UpstreamBillingProbeStatusUnsupported,
				HTTPStatus: http.StatusNotFound,
			},
		},
	}
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	repo := &accountMultiplierRepoStub{upstreamBillingProbeAccountRepo: baseRepo}
	upstream := &accountMultiplierHTTPStub{
		usageValues:      usageValues,
		expectedAPIKey:   "sk-sensitive",
		expectedModel:    "upstream-mini",
		expectedOverride: "mapped",
	}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
	testService := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: cfg}
	svc := NewAccountMultiplierService(repo, testService, billing)
	svc.now = func() time.Time { return now }
	if got := account.GetHeaderOverrides(); got["x-monitor-test"] != "mapped" {
		t.Fatalf("header overrides = %#v", got)
	}

	if err := svc.Refresh(context.Background(), account, true); err != nil {
		t.Fatalf("%v (paths=%v stub_error=%s)", err, upstream.paths, upstream.lastError)
	}

	persisted := decodeAccountMultiplierMeasurementSnapshot(account.Extra)
	if persisted == nil || persisted.Value == nil {
		t.Fatalf("persisted snapshot = %#v", persisted)
	}
	if math.Abs(*persisted.Value-0.25) > 1e-9 || persisted.SampleCount != 3 {
		t.Fatalf("persisted snapshot = %#v", persisted)
	}
	if persisted.FreshUntil == nil || !persisted.FreshUntil.Equal(now.Add(AccountMultiplierMeasurementTTL)) {
		t.Fatalf("FreshUntil = %#v", persisted.FreshUntil)
	}
	if upstream.completionCalls != 3 || upstream.usageIndex != 6 {
		t.Fatalf("upstream calls: completions=%d usage=%d", upstream.completionCalls, upstream.usageIndex)
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"sk-sensitive", "new-api.example", "total_used", "quota_per_unit", "upstream-mini"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("snapshot contains sensitive/raw evidence %q: %s", forbidden, encoded)
		}
	}
}

func TestAccountMultiplierRefreshReusesFreshMeasurementUnlessForced(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	value := 0.25
	account := &Account{Extra: map[string]any{
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		AccountMultiplierMeasurementExtraKey: AccountMultiplierMeasurementSnapshot{
			Version:     AccountMultiplierMeasurementVersion,
			Status:      AccountMonitorMultiplierStatusOK,
			Source:      AccountMonitorMultiplierSourceMeasured,
			Value:       &value,
			SampleCount: 3,
			ObservedAt:  probeTimePtr(now.Add(-time.Hour)),
			FreshUntil:  probeTimePtr(now.Add(time.Hour)),
		},
	}}
	svc := NewAccountMultiplierService(nil, nil, nil)
	svc.now = func() time.Time { return now }

	if err := svc.Refresh(context.Background(), account, false); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMultiplierRefreshDoesNotRetryRecentFailureAutomatically(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{Extra: map[string]any{
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
		AccountMultiplierMeasurementExtraKey: AccountMultiplierMeasurementSnapshot{
			Version:       AccountMultiplierMeasurementVersion,
			Status:        AccountMonitorMultiplierStatusFailed,
			Source:        AccountMonitorMultiplierSourceMeasured,
			LastAttemptAt: now.Add(-time.Hour),
		},
	}}
	svc := NewAccountMultiplierService(nil, nil, nil)
	svc.now = func() time.Time { return now }

	if err := svc.Refresh(context.Background(), account, false); err != nil {
		t.Fatalf("recent automatic failure must be throttled: %v", err)
	}
}

func TestAccountMultiplierRefreshForceReprobesNativeDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 30, 0, 0, time.UTC)
	account := &Account{
		ID: 24,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data: map[string]any{
					"billing_scope":            "token",
					"resolved_rate_multiplier": 0.16,
					"peak_rate_enabled":        false,
				},
				FreshUntil: probeTimePtr(now.Add(time.Hour)),
			},
		},
	}
	probe := &accountMultiplierDeclarationProbeStub{snapshot: &UpstreamBillingProbeSnapshot{
		Status: UpstreamBillingProbeStatusOK,
	}}
	svc := NewAccountMultiplierService(nil, nil, nil)
	svc.SetDeclarationProbe(probe)

	if err := svc.Refresh(context.Background(), account, true); err != nil {
		t.Fatal(err)
	}
	if len(probe.calls) != 1 || probe.calls[0] != account.ID {
		t.Fatalf("declaration probe calls = %#v", probe.calls)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
