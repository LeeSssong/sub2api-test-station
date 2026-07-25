package service

import (
	"context"
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
	"github.com/stretchr/testify/require"
)

type upstreamMultiplierHTTPResponse struct {
	status int
	body   string
}

type upstreamMultiplierHTTPStub struct {
	mu        sync.Mutex
	responses []upstreamMultiplierHTTPResponse
	requests  []*http.Request
	proxies   []string
	onRequest func(int)
}

func (s *upstreamMultiplierHTTPStub) Do(
	req *http.Request,
	proxyURL string,
	_ int64,
	_ int,
) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, 0, 0, nil)
}

func (s *upstreamMultiplierHTTPStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	_ int64,
	_ int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req.Clone(req.Context()))
	s.proxies = append(s.proxies, proxyURL)
	if s.onRequest != nil {
		s.onRequest(len(s.requests))
	}
	if len(s.responses) == 0 {
		return nil, context.Canceled
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return &http.Response{
		StatusCode: response.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}

func (r *upstreamBillingProbeAccountRepo) UpdateUpstreamMultiplierMeasurementSnapshot(
	_ context.Context,
	expected *Account,
	snapshot *UpstreamMultiplierMeasurementSnapshot,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil || account.Platform != expected.Platform || account.Type != expected.Type ||
		account.Status != expected.Status || account.Schedulable != expected.Schedulable ||
		!reflect.DeepEqual(account.Credentials, expected.Credentials) ||
		!reflect.DeepEqual(account.ProxyID, expected.ProxyID) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	if !reflect.DeepEqual(
		account.Extra[UpstreamMultiplierMeasurementExtraKey],
		expected.Extra[UpstreamMultiplierMeasurementExtraKey],
	) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	account.Extra[UpstreamMultiplierMeasurementExtraKey] = snapshot
	return nil
}

func TestParseNewAPIQuotaUsage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want float64
	}{
		{
			name: "nested total usage",
			body: `{"success":true,"data":{"object":"list","total_usage":125000}}`,
			want: 125000,
		},
		{
			name: "nested used quota",
			body: `{"success":true,"data":{"used_quota":250000}}`,
			want: 250000,
		},
		{
			name: "top level total usage",
			body: `{"success":true,"total_usage":375000}`,
			want: 375000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNewAPIQuotaUsage([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseNewAPIQuotaUsageRejectsUntrustworthyResponses(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"data":{"total_usage":125000}}`,
		`{"success":true,"data":{"total_usage":-1}}`,
		`{"success":true,"data":{}}`,
		`{"success":true,"data":{"total_usage":"not-a-number"}}`,
		`not-json`,
	} {
		_, err := parseNewAPIQuotaUsage([]byte(body))
		require.Error(t, err, body)
	}
}

func TestParseNewAPIQuotaPerUnit(t *testing.T) {
	for _, body := range []string{
		`{"success":true,"data":{"quota_per_unit":500000}}`,
		`{"success":true,"quota_per_unit":500000}`,
	} {
		got, err := parseNewAPIQuotaPerUnit([]byte(body))
		require.NoError(t, err)
		require.Equal(t, 500000.0, got)
	}
}

func TestParseNewAPIQuotaPerUnitRejectsInvalidValues(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"data":{"quota_per_unit":500000}}`,
		`{"success":true,"data":{"quota_per_unit":0}}`,
		`{"success":true,"data":{"quota_per_unit":-1}}`,
		`{"success":true,"data":{}}`,
	} {
		_, err := parseNewAPIQuotaPerUnit([]byte(body))
		require.Error(t, err, body)
	}
}

func TestOfficialMeasurementCostUsesModelPricingAtOneTimes(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)

	got, err := officialMeasurementCost(billing, "gpt-5.4", UsageTokens{
		InputTokens:  1000,
		OutputTokens: 500,
	})

	require.NoError(t, err)
	require.Greater(t, got, 0.0)
	breakdown, err := billing.CalculateCost("gpt-5.4", UsageTokens{
		InputTokens:  1000,
		OutputTokens: 500,
	}, 1)
	require.NoError(t, err)
	require.Equal(t, breakdown.TotalCost, got)
}

func TestOfficialMeasurementCostRejectsMissingEvidence(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	_, err := officialMeasurementCost(billing, "gpt-5.4", UsageTokens{})
	require.Error(t, err)
	_, err = officialMeasurementCost(nil, "gpt-5.4", UsageTokens{InputTokens: 1})
	require.Error(t, err)
	_, err = officialMeasurementCost(billing, "unknown-unpriceable-model", UsageTokens{InputTokens: 1})
	require.Error(t, err)
}

func TestParseMeasurementCompletionUsageSeparatesCachedInput(t *testing.T) {
	tokens, err := parseMeasurementCompletionUsage([]byte(`{
		"usage":{
			"prompt_tokens":1000,
			"completion_tokens":500,
			"prompt_tokens_details":{
				"cached_tokens":200,
				"cache_creation_tokens":100
			}
		}
	}`))

	require.NoError(t, err)
	require.Equal(t, 700, tokens.InputTokens)
	require.Equal(t, 500, tokens.OutputTokens)
	require.Equal(t, 200, tokens.CacheReadTokens)
	require.Equal(t, 100, tokens.CacheCreationTokens)
}

func TestCalculateMeasuredMultiplierConvertsQuotaDeltaToUSD(t *testing.T) {
	got, err := calculateMeasuredMultiplier(1000, 1250, 500000, 0.005)
	require.NoError(t, err)
	require.InDelta(t, 0.1, got, 1e-12)
}

func TestCalculateMeasuredMultiplierRejectsInvalidInputs(t *testing.T) {
	tests := [][4]float64{
		{1000, 1000, 500000, 0.005},
		{1000, 999, 500000, 0.005},
		{1000, 1250, 0, 0.005},
		{1000, 1250, 500000, 0},
		{1000, math.Inf(1), 500000, 0.005},
	}
	for _, input := range tests {
		_, err := calculateMeasuredMultiplier(input[0], input[1], input[2], input[3])
		require.Error(t, err, "%v", input)
	}
}

func TestSummarizeMeasuredMultipliersUsesMedianAndSpread(t *testing.T) {
	median, spread, err := summarizeMeasuredMultipliers([]float64{0.101, 0.099, 0.100}, 0.05)
	require.NoError(t, err)
	require.InDelta(t, 0.1, median, 1e-12)
	require.InDelta(t, 0.02, spread, 1e-12)
}

func TestSummarizeMeasuredMultipliersRejectsIncompleteOrUnstableSamples(t *testing.T) {
	for _, samples := range [][]float64{
		{0.1, 0.1},
		{0.1, 0.1, 0.2},
		{0.1, 0, 0.1},
		{0.1, math.NaN(), 0.1},
	} {
		_, _, err := summarizeMeasuredMultipliers(samples, 0.05)
		require.Error(t, err, "%v", samples)
	}
}

func TestUpstreamMultiplierMeasurementUsesBoundedNewAPIFlow(t *testing.T) {
	upstream := &upstreamMultiplierHTTPStub{responses: []upstreamMultiplierHTTPResponse{
		{status: http.StatusOK, body: `{"success":true,"data":{"quota_per_unit":500000}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1000}}`},
		{status: http.StatusOK, body: `{"model":"upstream-gpt","usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1100}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1100}}`},
		{status: http.StatusOK, body: `{"model":"upstream-gpt","usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1200}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1200}}`},
		{status: http.StatusOK, body: `{"model":"upstream-gpt","usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1300}}`},
	}}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
	accountTestService := &AccountTestService{
		httpUpstream: upstream,
		cfg:          cfg,
	}
	service := NewUpstreamMultiplierMeasurementService(
		nil,
		accountTestService,
		NewBillingService(cfg, nil),
	)
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	proxyID := int64(9)
	account := &Account{
		ID:          17,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":                 "sk-secret",
			"base_url":                "https://newapi.example/v1",
			"header_override_enabled": true,
			"model_mapping": map[string]any{
				"gpt-5.4": "upstream-gpt",
			},
			"header_overrides": map[string]any{
				"X-Route": "measurement",
			},
		},
		ProxyID: &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     3128,
			Status:   StatusActive,
		},
	}
	require.Equal(t, "measurement", account.GetHeaderOverrides()["x-route"])

	snapshot, err := service.measureLoadedAccount(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, UpstreamMultiplierMeasurementSchemaVersion, snapshot.SchemaVersion)
	require.Equal(t, AccountMonitorMultiplierStatusOK, snapshot.Status)
	require.NotNil(t, snapshot.Multiplier)
	require.Greater(t, *snapshot.Multiplier, 0.0)
	require.Equal(t, "gpt-5.4", snapshot.Model)
	require.Equal(t, 3, snapshot.SampleCount)
	require.Equal(t, now, snapshot.ObservedAt)
	require.Equal(t, now.Add(24*time.Hour), snapshot.FreshUntil)
	require.Len(t, upstream.requests, 10)

	wantPaths := []string{
		"/api/status",
		"/api/usage/token/", "/v1/chat/completions", "/api/usage/token/",
		"/api/usage/token/", "/v1/chat/completions", "/api/usage/token/",
		"/api/usage/token/", "/v1/chat/completions", "/api/usage/token/",
	}
	for i, request := range upstream.requests {
		require.Equal(t, wantPaths[i], request.URL.Path, "request %d", i)
		require.Equal(t, "Bearer sk-secret", request.Header.Get("Authorization"))
		require.Equal(t, "measurement", headerValueEqualFold(request.Header, "X-Route"))
		require.Equal(t, "http://127.0.0.1:3128", upstream.proxies[i])
	}
	for _, index := range []int{2, 5, 8} {
		request := upstream.requests[index]
		require.Equal(t, http.MethodPost, request.Method)
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		text := string(body)
		require.Contains(t, text, `"model":"upstream-gpt"`)
		require.Contains(t, text, `"stream":false`)
		require.Contains(t, text, `"max_completion_tokens":8`)
		require.Greater(t, len(text), 1000)
		require.NotContains(t, text, "sk-secret")
	}
}

func TestUpstreamMultiplierMeasurementRejectsCompletionWithoutUsage(t *testing.T) {
	upstream := &upstreamMultiplierHTTPStub{responses: []upstreamMultiplierHTTPResponse{
		{status: http.StatusOK, body: `{"success":true,"data":{"quota_per_unit":500000}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1000}}`},
		{status: http.StatusOK, body: `{"choices":[{"message":{"content":"OK"}}]}`},
	}}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
	service := NewUpstreamMultiplierMeasurementService(
		nil,
		&AccountTestService{httpUpstream: upstream, cfg: cfg},
		NewBillingService(cfg, nil),
	)
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-secret",
			"base_url": "https://newapi.example/v1",
			"model_mapping": map[string]any{
				"gpt-5.4": "upstream-gpt",
			},
		},
	}

	_, err := service.measureLoadedAccount(context.Background(), account)

	require.Error(t, err)
}

func TestUpstreamMultiplierMeasurementRefreshReusesFreshSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	existing := &UpstreamMultiplierMeasurementSnapshot{
		SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
		Status:        AccountMonitorMultiplierStatusOK,
		Multiplier:    numberPointer(0.1),
		Model:         "gpt-5.4",
		SampleCount:   3,
		ObservedAt:    now.Add(-time.Hour),
		FreshUntil:    now.Add(23 * time.Hour),
	}
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-secret",
			"base_url": "https://newapi.example/v1",
		},
		Extra: map[string]any{UpstreamMultiplierMeasurementExtraKey: existing},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	upstream := &upstreamMultiplierHTTPStub{}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: false,
	}}}
	service := NewUpstreamMultiplierMeasurementService(
		repo,
		&AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: cfg},
		NewBillingService(cfg, nil),
	)
	service.now = func() time.Time { return now }

	got, err := service.RefreshAccount(context.Background(), 17, false)

	require.NoError(t, err)
	require.Equal(t, existing, got)
	require.Empty(t, upstream.requests)
}

func TestUpstreamMultiplierMeasurementRefreshBacksOffRecentFailure(t *testing.T) {
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	existing := &UpstreamMultiplierMeasurementSnapshot{
		SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
		Status:        AccountMonitorMultiplierStatusFailed,
		ObservedAt:    now.Add(-time.Hour),
	}
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{UpstreamMultiplierMeasurementExtraKey: existing},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	upstream := &upstreamMultiplierHTTPStub{}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: false,
	}}}
	service := NewUpstreamMultiplierMeasurementService(
		repo,
		&AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: cfg},
		NewBillingService(cfg, nil),
	)
	service.now = func() time.Time { return now }

	got, err := service.RefreshAccount(context.Background(), 17, false)

	require.NoError(t, err)
	require.Equal(t, existing, got)
	require.Empty(t, upstream.requests)
}

func TestUpstreamMultiplierMeasurementDiscardsResultAfterIdentityChange(t *testing.T) {
	upstream := &upstreamMultiplierHTTPStub{responses: []upstreamMultiplierHTTPResponse{
		{status: http.StatusOK, body: `{"success":true,"data":{"quota_per_unit":500000}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1000}}`},
		{status: http.StatusOK, body: `{"usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1100}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1100}}`},
		{status: http.StatusOK, body: `{"usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1200}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1200}}`},
		{status: http.StatusOK, body: `{"usage":{"prompt_tokens":1000,"completion_tokens":500}}`},
		{status: http.StatusOK, body: `{"success":true,"data":{"total_usage":1300}}`},
	}}
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-secret",
			"base_url": "https://newapi.example/v1",
			"model_mapping": map[string]any{
				"gpt-5.4": "upstream-gpt",
			},
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	upstream.onRequest = func(index int) {
		if index != 10 {
			return
		}
		repo.mu.Lock()
		repo.accounts[17].Credentials["api_key"] = "sk-replaced"
		repo.mu.Unlock()
	}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: false,
	}}}
	service := NewUpstreamMultiplierMeasurementService(
		repo,
		&AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: cfg},
		NewBillingService(cfg, nil),
	)

	_, err := service.RefreshAccount(context.Background(), 17, true)

	require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
	require.NotContains(t, repo.accounts[17].Extra, UpstreamMultiplierMeasurementExtraKey)
}

func headerValueEqualFold(header http.Header, name string) string {
	for key, values := range header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
