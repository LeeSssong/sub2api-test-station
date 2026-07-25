package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	UpstreamMultiplierMeasurementExtraKey      = "upstream_multiplier_measurement_v1"
	UpstreamMultiplierMeasurementSchemaVersion = 1

	upstreamMultiplierMeasurementFreshFor       = 24 * time.Hour
	upstreamMultiplierMeasurementRequestTimeout = 15 * time.Second
	upstreamMultiplierMeasurementMaxBodyBytes   = 64 * 1024
	upstreamMultiplierMeasurementMaxSpread      = 0.05
)

var upstreamMultiplierMeasurementPrompt = strings.Repeat(
	"Account multiplier measurement. Use this fixed text only. ",
	128,
)

type UpstreamMultiplierMeasurementSnapshot struct {
	SchemaVersion  int       `json:"schema_version"`
	Status         string    `json:"status"`
	Multiplier     *float64  `json:"multiplier,omitempty"`
	Model          string    `json:"model,omitempty"`
	SampleCount    int       `json:"sample_count,omitempty"`
	RelativeSpread *float64  `json:"relative_spread,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
	FreshUntil     time.Time `json:"fresh_until,omitempty"`
}

type UpstreamMultiplierMeasurementService struct {
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	billingService     *BillingService
	now                func() time.Time
}

type upstreamMultiplierMeasurementSnapshotWriter interface {
	UpdateUpstreamMultiplierMeasurementSnapshot(
		context.Context,
		*Account,
		*UpstreamMultiplierMeasurementSnapshot,
	) error
}

func NewUpstreamMultiplierMeasurementService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	billingService *BillingService,
) *UpstreamMultiplierMeasurementService {
	return &UpstreamMultiplierMeasurementService{
		accountRepo:        accountRepo,
		accountTestService: accountTestService,
		billingService:     billingService,
		now:                time.Now,
	}
}

func (s *UpstreamMultiplierMeasurementService) RefreshAccount(
	ctx context.Context,
	accountID int64,
	force bool,
) (*UpstreamMultiplierMeasurementSnapshot, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("upstream multiplier measurement is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	now := s.currentTime().UTC()
	existing := decodeUpstreamMultiplierMeasurementSnapshot(account.Extra)
	if !force && existing != nil {
		if existing.Status == AccountMonitorMultiplierStatusOK &&
			existing.Multiplier != nil &&
			!existing.FreshUntil.IsZero() &&
			now.Before(existing.FreshUntil) {
			return existing, nil
		}
		if !existing.ObservedAt.IsZero() &&
			now.Before(existing.ObservedAt.Add(upstreamMultiplierMeasurementFreshFor)) {
			return existing, nil
		}
	}

	snapshot, measurementErr := s.measureLoadedAccount(ctx, account)
	if measurementErr != nil {
		snapshot = &UpstreamMultiplierMeasurementSnapshot{
			SchemaVersion: UpstreamMultiplierMeasurementSchemaVersion,
			Status:        AccountMonitorMultiplierStatusFailed,
			ObservedAt:    now,
		}
	}
	writer, ok := s.accountRepo.(upstreamMultiplierMeasurementSnapshotWriter)
	if !ok {
		return nil, fmt.Errorf("upstream multiplier snapshot writer is unavailable")
	}
	if err := writer.UpdateUpstreamMultiplierMeasurementSnapshot(ctx, account, snapshot); err != nil {
		return nil, err
	}
	return snapshot, measurementErr
}

func (s *UpstreamMultiplierMeasurementService) measureLoadedAccount(
	ctx context.Context,
	account *Account,
) (*UpstreamMultiplierMeasurementSnapshot, error) {
	if s == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil ||
		s.billingService == nil {
		return nil, fmt.Errorf("upstream multiplier measurement is unavailable")
	}
	if !isUpstreamBillingProbeAccount(account) {
		return nil, ErrUpstreamBillingProbeAccountInvalid
	}
	apiKey := account.GetOpenAIApiKey()
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("missing API key")
	}
	baseURL := account.GetOpenAIBaseURL()
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("missing upstream Base URL")
	}
	normalizedBaseURL, err := s.accountTestService.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("validate upstream Base URL: %w", err)
	}
	proxyURL, err := upstreamMultiplierProxyURL(account)
	if err != nil {
		return nil, err
	}
	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}

	statusBody, err := s.doJSON(
		ctx,
		account,
		http.MethodGet,
		buildNewAPIRootEndpointURL(normalizedBaseURL, "/api/status"),
		apiKey,
		proxyURL,
		tlsProfile,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("read New API status: %w", err)
	}
	quotaPerUnit, err := parseNewAPIQuotaPerUnit(statusBody)
	if err != nil {
		return nil, err
	}

	model := monitorModelForAccount(account)
	upstreamModel := account.GetMappedModel(model)
	if !isAccountMonitorTextModel(model) || !isAccountMonitorTextModel(upstreamModel) {
		return nil, fmt.Errorf("no measurable text model")
	}
	samples := make([]float64, 0, 3)
	for range 3 {
		sample, sampleErr := s.measureSample(
			ctx,
			account,
			normalizedBaseURL,
			apiKey,
			proxyURL,
			tlsProfile,
			model,
			upstreamModel,
			quotaPerUnit,
		)
		if sampleErr != nil {
			return nil, sampleErr
		}
		samples = append(samples, sample)
	}
	multiplier, spread, err := summarizeMeasuredMultipliers(
		samples,
		upstreamMultiplierMeasurementMaxSpread,
	)
	if err != nil {
		return nil, err
	}
	now := s.currentTime().UTC()
	return &UpstreamMultiplierMeasurementSnapshot{
		SchemaVersion:  UpstreamMultiplierMeasurementSchemaVersion,
		Status:         AccountMonitorMultiplierStatusOK,
		Multiplier:     &multiplier,
		Model:          model,
		SampleCount:    len(samples),
		RelativeSpread: &spread,
		ObservedAt:     now,
		FreshUntil:     now.Add(upstreamMultiplierMeasurementFreshFor),
	}, nil
}

func (s *UpstreamMultiplierMeasurementService) measureSample(
	ctx context.Context,
	account *Account,
	normalizedBaseURL string,
	apiKey string,
	proxyURL string,
	tlsProfile *tlsfingerprint.Profile,
	model string,
	upstreamModel string,
	quotaPerUnit float64,
) (float64, error) {
	beforeBody, err := s.doJSON(
		ctx,
		account,
		http.MethodGet,
		buildNewAPIRootEndpointURL(normalizedBaseURL, "/api/usage/token/"),
		apiKey,
		proxyURL,
		tlsProfile,
		nil,
	)
	if err != nil {
		return 0, fmt.Errorf("read quota before measurement: %w", err)
	}
	beforeQuota, err := parseNewAPIQuotaUsage(beforeBody)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(map[string]any{
		"model": upstreamModel,
		"messages": []map[string]string{{
			"role":    "user",
			"content": upstreamMultiplierMeasurementPrompt + " Reply with OK.",
		}},
		"stream":                false,
		"max_completion_tokens": 8,
	})
	if err != nil {
		return 0, err
	}
	completionBody, err := s.doJSON(
		ctx,
		account,
		http.MethodPost,
		buildOpenAIChatCompletionsURL(normalizedBaseURL),
		apiKey,
		proxyURL,
		tlsProfile,
		payload,
	)
	if err != nil {
		return 0, fmt.Errorf("run multiplier measurement completion: %w", err)
	}
	tokens, err := parseMeasurementCompletionUsage(completionBody)
	if err != nil {
		return 0, err
	}
	officialCost, err := officialMeasurementCost(s.billingService, model, tokens)
	if err != nil {
		return 0, err
	}
	afterBody, err := s.doJSON(
		ctx,
		account,
		http.MethodGet,
		buildNewAPIRootEndpointURL(normalizedBaseURL, "/api/usage/token/"),
		apiKey,
		proxyURL,
		tlsProfile,
		nil,
	)
	if err != nil {
		return 0, fmt.Errorf("read quota after measurement: %w", err)
	}
	afterQuota, err := parseNewAPIQuotaUsage(afterBody)
	if err != nil {
		return 0, err
	}
	return calculateMeasuredMultiplier(beforeQuota, afterQuota, quotaPerUnit, officialCost)
}

func (s *UpstreamMultiplierMeasurementService) doJSON(
	ctx context.Context,
	account *Account,
	method string,
	endpoint string,
	apiKey string,
	proxyURL string,
	tlsProfile *tlsfingerprint.Profile,
	payload []byte,
) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, upstreamMultiplierMeasurementRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request = request.WithContext(WithHTTPUpstreamRedirectsDisabled(
		WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI),
	))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if len(payload) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	account.ApplyHeaderOverrides(request.Header)
	response, err := s.accountTestService.httpUpstream.DoWithTLS(
		request,
		proxyURL,
		account.ID,
		account.Concurrency,
		tlsProfile,
	)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("empty upstream response")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, upstreamMultiplierMeasurementMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > upstreamMultiplierMeasurementMaxBodyBytes {
		return nil, fmt.Errorf("upstream response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	return body, nil
}

func (s *UpstreamMultiplierMeasurementService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func upstreamMultiplierProxyURL(account *Account) (string, error) {
	if account == nil || account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy == nil {
		return "", fmt.Errorf("configured proxy is unavailable")
	}
	if account.Proxy.ID != *account.ProxyID {
		return "", ErrUpstreamBillingProbeIdentityChanged
	}
	return account.Proxy.URL(), nil
}

func buildNewAPIRootEndpointURL(baseURL string, endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimRight(strings.TrimSuffix(strings.TrimSpace(baseURL), "/v1"), "/") +
			"/" + strings.TrimLeft(endpoint, "/")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	parsed.Path = strings.TrimRight(path, "/") + "/" + strings.TrimLeft(endpoint, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func parseMeasurementCompletionUsage(body []byte) (UsageTokens, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response map[string]any
	if err := decoder.Decode(&response); err != nil {
		return UsageTokens{}, fmt.Errorf("decode measurement completion: %w", err)
	}
	usage, ok := response["usage"].(map[string]any)
	if !ok {
		return UsageTokens{}, fmt.Errorf("measurement completion did not include usage")
	}
	input, inputOK := measurementTokenCount(usage, "prompt_tokens", "input_tokens")
	output, outputOK := measurementTokenCount(usage, "completion_tokens", "output_tokens")
	if !inputOK || !outputOK || input+output <= 0 {
		return UsageTokens{}, fmt.Errorf("measurement completion included invalid usage")
	}
	cacheRead := measurementNestedTokenCount(
		usage,
		[]string{"prompt_tokens_details", "input_tokens_details"},
		[]string{"cached_tokens"},
	)
	cacheCreation := measurementNestedTokenCount(
		usage,
		[]string{"prompt_tokens_details", "input_tokens_details"},
		[]string{"cache_creation_tokens", "cache_write_tokens"},
	)
	if cacheRead == 0 {
		cacheRead, _ = measurementOptionalTokenCount(
			usage,
			"cache_read_input_tokens",
			"cache_read_tokens",
			"cached_tokens",
		)
	}
	if cacheCreation == 0 {
		cacheCreation, _ = measurementOptionalTokenCount(
			usage,
			"cache_creation_input_tokens",
			"cache_write_input_tokens",
			"cache_creation_tokens",
			"cache_write_tokens",
		)
	}
	uncachedInput := input - cacheRead - cacheCreation
	if uncachedInput < 0 {
		return UsageTokens{}, fmt.Errorf("measurement completion included inconsistent cache usage")
	}
	return UsageTokens{
		InputTokens:         uncachedInput,
		OutputTokens:        output,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
	}, nil
}

func measurementTokenCount(usage map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := usage[key]
		if !ok {
			continue
		}
		number, ok := value.(json.Number)
		if !ok {
			return 0, false
		}
		parsed, err := number.Int64()
		if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	}
	return 0, false
}

func measurementOptionalTokenCount(usage map[string]any, keys ...string) (int, bool) {
	value, ok := measurementTokenCount(usage, keys...)
	if !ok {
		return 0, false
	}
	return value, true
}

func measurementNestedTokenCount(
	usage map[string]any,
	containers []string,
	keys []string,
) int {
	for _, container := range containers {
		details, ok := usage[container].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := measurementOptionalTokenCount(details, keys...); ok {
			return value
		}
	}
	return 0
}

func decodeUpstreamMultiplierMeasurementSnapshot(extra map[string]any) *UpstreamMultiplierMeasurementSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[UpstreamMultiplierMeasurementExtraKey]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot UpstreamMultiplierMeasurementSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil ||
		snapshot.SchemaVersion != UpstreamMultiplierMeasurementSchemaVersion ||
		!validAccountMonitorMultiplierStatus(snapshot.Status) {
		return nil
	}
	if snapshot.Multiplier != nil &&
		(*snapshot.Multiplier < 0 || math.IsNaN(*snapshot.Multiplier) || math.IsInf(*snapshot.Multiplier, 0)) {
		return nil
	}
	return &snapshot
}

func validAccountMonitorMultiplierStatus(status string) bool {
	switch status {
	case AccountMonitorMultiplierStatusOK,
		AccountMonitorMultiplierStatusStale,
		AccountMonitorMultiplierStatusUnsupported,
		AccountMonitorMultiplierStatusFailed,
		AccountMonitorMultiplierStatusUnavailable:
		return true
	default:
		return false
	}
}

func parseNewAPIQuotaUsage(body []byte) (float64, error) {
	document, err := decodeNewAPIResponse(body)
	if err != nil {
		return 0, err
	}
	value, ok := nestedNewAPINumber(document, "total_usage")
	if !ok {
		value, ok = nestedNewAPINumber(document, "used_quota")
	}
	if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid New API quota usage")
	}
	return value, nil
}

func parseNewAPIQuotaPerUnit(body []byte) (float64, error) {
	document, err := decodeNewAPIResponse(body)
	if err != nil {
		return 0, err
	}
	value, ok := nestedNewAPINumber(document, "quota_per_unit")
	if !ok || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid New API quota_per_unit")
	}
	return value, nil
}

func decodeNewAPIResponse(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode New API response: %w", err)
	}
	success, ok := document["success"].(bool)
	if !ok || !success {
		return nil, fmt.Errorf("New API response was not successful")
	}
	return document, nil
}

func nestedNewAPINumber(document map[string]any, key string) (float64, bool) {
	if value, ok := finiteJSONNumber(document[key]); ok {
		return value, true
	}
	data, ok := document["data"].(map[string]any)
	if !ok {
		return 0, false
	}
	return finiteJSONNumber(data[key])
}

func finiteJSONNumber(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func officialMeasurementCost(
	billing *BillingService,
	model string,
	tokens UsageTokens,
) (float64, error) {
	if billing == nil || strings.TrimSpace(model) == "" ||
		tokens.InputTokens < 0 || tokens.OutputTokens < 0 ||
		tokens.InputTokens+tokens.OutputTokens <= 0 {
		return 0, fmt.Errorf("missing official pricing evidence")
	}
	breakdown, err := billing.CalculateCost(model, tokens, 1)
	if err != nil {
		return 0, fmt.Errorf("calculate official model cost: %w", err)
	}
	if breakdown == nil || breakdown.TotalCost <= 0 ||
		math.IsNaN(breakdown.TotalCost) || math.IsInf(breakdown.TotalCost, 0) {
		return 0, fmt.Errorf("invalid official model cost")
	}
	return breakdown.TotalCost, nil
}

func calculateMeasuredMultiplier(
	beforeQuota float64,
	afterQuota float64,
	quotaPerUnit float64,
	officialCost float64,
) (float64, error) {
	for _, value := range []float64{beforeQuota, afterQuota, quotaPerUnit, officialCost} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("non-finite measurement input")
		}
	}
	delta := afterQuota - beforeQuota
	if delta <= 0 || quotaPerUnit <= 0 || officialCost <= 0 {
		return 0, fmt.Errorf("invalid measurement input")
	}
	multiplier := (delta / quotaPerUnit) / officialCost
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("invalid measured multiplier")
	}
	return multiplier, nil
}

func summarizeMeasuredMultipliers(samples []float64, maxRelativeSpread float64) (float64, float64, error) {
	if len(samples) != 3 || maxRelativeSpread <= 0 ||
		math.IsNaN(maxRelativeSpread) || math.IsInf(maxRelativeSpread, 0) {
		return 0, 0, fmt.Errorf("three valid samples are required")
	}
	sorted := append([]float64(nil), samples...)
	for _, sample := range sorted {
		if sample <= 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
			return 0, 0, fmt.Errorf("invalid multiplier sample")
		}
	}
	sort.Float64s(sorted)
	median := sorted[1]
	spread := (sorted[2] - sorted[0]) / median
	if spread > maxRelativeSpread {
		return 0, spread, fmt.Errorf("multiplier samples are unstable")
	}
	return median, spread, nil
}
