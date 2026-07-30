package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	AccountMultiplierMeasurementExtraKey = "account_monitor_multiplier_measurement"
	AccountMultiplierMeasurementVersion  = 1
	AccountMultiplierMeasurementTTL      = 24 * time.Hour
	accountMultiplierFailureRetryTTL     = 15 * time.Minute
	accountMultiplierMeasurementSamples  = 3
	accountMultiplierMaxRelativeSpread   = 0.15
	accountMultiplierRequestTimeout      = 30 * time.Second
	accountMultiplierMaxBodyBytes        = 128 * 1024
	accountMultiplierQuotaPollAttempts   = 20
	accountMultiplierQuotaPollDelay      = 250 * time.Millisecond
	accountMultiplierQuotaPollMaxDelay   = 5 * time.Second

	AccountMonitorMultiplierSourceDeclared = "declared"
	AccountMonitorMultiplierSourceMeasured = "measured"

	AccountMonitorMultiplierStatusOK          = "ok"
	AccountMonitorMultiplierStatusStale       = "stale"
	AccountMonitorMultiplierStatusUnsupported = "unsupported"
	AccountMonitorMultiplierStatusFailed      = "failed"
	AccountMonitorMultiplierStatusUnavailable = "unavailable"
)

type AccountMultiplierMeasurementSnapshot struct {
	Version        int        `json:"version"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	Value          *float64   `json:"value,omitempty"`
	ModelID        string     `json:"model_id,omitempty"`
	SampleCount    int        `json:"sample_count,omitempty"`
	RelativeSpread *float64   `json:"relative_spread,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
	FreshUntil     *time.Time `json:"fresh_until,omitempty"`
	LastAttemptAt  time.Time  `json:"last_attempt_at"`
	FailureCode    string     `json:"failure_code,omitempty"`
}

type AccountMultiplierService struct {
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	billingService     *BillingService
	declarationProbe   accountMultiplierDeclarationProbe
	now                func() time.Time
	wait               func(context.Context, time.Duration) error
}

type accountMultiplierDeclarationProbe interface {
	ProbeAccount(context.Context, int64) (*UpstreamBillingProbeSnapshot, error)
}

func NewAccountMultiplierService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	billingService *BillingService,
) *AccountMultiplierService {
	return &AccountMultiplierService{
		accountRepo:        accountRepo,
		accountTestService: accountTestService,
		billingService:     billingService,
		now:                time.Now,
		wait:               waitForAccountMultiplierDelay,
	}
}

func (s *AccountMultiplierService) SetDeclarationProbe(probe accountMultiplierDeclarationProbe) {
	if s != nil {
		s.declarationProbe = probe
	}
}

type accountMultiplierMeasurementWriter interface {
	UpdateAccountMultiplierMeasurement(context.Context, *Account, *AccountMultiplierMeasurementSnapshot) error
}

type newAPIQuotaStatus struct {
	QuotaPerUnit float64
}

type newAPICompletionUsage struct {
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
}

func (s *AccountMultiplierService) Refresh(ctx context.Context, account *Account, force bool) error {
	if account == nil {
		return ErrAccountNilInput
	}
	declaration := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if force && s.declarationProbe != nil {
		probed, err := s.declarationProbe.ProbeAccount(ctx, account.ID)
		if err != nil {
			return err
		}
		if probed != nil {
			declaration = probed
		}
	}
	if declaration == nil || declaration.Status != UpstreamBillingProbeStatusUnsupported {
		return nil
	}
	now := s.currentTime().UTC()
	measurement := decodeAccountMultiplierMeasurementSnapshot(account.Extra)
	if !force && measurement != nil {
		if measurement.Status == AccountMonitorMultiplierStatusOK &&
			measurement.FreshUntil != nil && now.Before(*measurement.FreshUntil) {
			return nil
		}
		retryTTL := AccountMultiplierMeasurementTTL
		if measurement.Status == AccountMonitorMultiplierStatusFailed {
			retryTTL = accountMultiplierFailureRetryTTL
		}
		if !measurement.LastAttemptAt.IsZero() &&
			now.Sub(measurement.LastAttemptAt) >= 0 &&
			now.Sub(measurement.LastAttemptAt) < retryTTL {
			return nil
		}
	}
	if s.accountRepo == nil || s.accountTestService == nil ||
		s.accountTestService.httpUpstream == nil || s.billingService == nil {
		return ErrUpstreamBillingProbeUnavailable
	}

	snapshot, measureErr := s.measure(ctx, account, now)
	if snapshot == nil {
		snapshot = &AccountMultiplierMeasurementSnapshot{
			Version:       AccountMultiplierMeasurementVersion,
			Status:        AccountMonitorMultiplierStatusFailed,
			Source:        AccountMonitorMultiplierSourceMeasured,
			LastAttemptAt: now,
			FailureCode:   accountMultiplierFailureCode(measureErr),
		}
	}
	if persistErr := s.persistMeasurement(ctx, account, snapshot); persistErr != nil {
		return persistErr
	}
	return measureErr
}

func (s *AccountMultiplierService) measure(
	ctx context.Context,
	account *Account,
	now time.Time,
) (*AccountMultiplierMeasurementSnapshot, error) {
	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return nil, errors.New("account multiplier measurement requires an API key")
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.accountTestService.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, errors.New("account multiplier measurement base URL is invalid")
	}
	proxyURL, err := accountMultiplierProxyURL(account)
	if err != nil {
		return nil, err
	}
	status, err := s.readNewAPIQuotaStatus(ctx, account, normalizedBaseURL, apiKey, proxyURL)
	if err != nil {
		return nil, err
	}
	modelID := monitorModelForAccount(account)
	upstreamModelID := account.GetMappedModel(modelID)
	before, err := s.readNewAPIQuotaUsage(ctx, account, normalizedBaseURL, apiKey, proxyURL)
	if err != nil {
		return nil, err
	}
	var officialCost float64
	for range accountMultiplierMeasurementSamples {
		usage, completionErr := s.runNewAPICompletion(ctx, account, normalizedBaseURL, apiKey, proxyURL, upstreamModelID)
		if completionErr != nil {
			return nil, completionErr
		}
		cost, costErr := s.billingService.CalculateCost(modelID, UsageTokens{
			InputTokens:     usage.InputTokens - usage.CacheReadTokens,
			OutputTokens:    usage.OutputTokens,
			CacheReadTokens: usage.CacheReadTokens,
		}, 1)
		if costErr != nil || cost == nil {
			return nil, errors.New("official model pricing is unavailable")
		}
		officialCost += cost.TotalCost
	}
	after, err := s.waitForNewAPIQuotaUsage(
		ctx,
		account,
		normalizedBaseURL,
		apiKey,
		proxyURL,
		before,
	)
	if err != nil {
		return nil, err
	}
	value, err := calculateAccountMultiplierSample(before, after, status.QuotaPerUnit, officialCost)
	if err != nil {
		return nil, err
	}
	observedAt := now
	freshUntil := now.Add(AccountMultiplierMeasurementTTL)
	return &AccountMultiplierMeasurementSnapshot{
		Version:       AccountMultiplierMeasurementVersion,
		Status:        AccountMonitorMultiplierStatusOK,
		Source:        AccountMonitorMultiplierSourceMeasured,
		Value:         float64PointerCopy(value),
		ModelID:       modelID,
		SampleCount:   accountMultiplierMeasurementSamples,
		ObservedAt:    &observedAt,
		FreshUntil:    &freshUntil,
		LastAttemptAt: now,
	}, nil
}

func (s *AccountMultiplierService) readNewAPIQuotaStatus(
	ctx context.Context,
	account *Account,
	baseURL string,
	apiKey string,
	proxyURL string,
) (newAPIQuotaStatus, error) {
	body, err := s.doJSONRequest(ctx, account, http.MethodGet, buildNewAPIEndpointURL(baseURL, "/api/status"), apiKey, proxyURL, nil)
	if err != nil {
		return newAPIQuotaStatus{}, err
	}
	value, err := decodeNewAPINumber(body, "quota_per_unit")
	if err != nil || value <= 0 {
		return newAPIQuotaStatus{}, errors.New("New API quota_per_unit is unavailable")
	}
	return newAPIQuotaStatus{QuotaPerUnit: value}, nil
}

func (s *AccountMultiplierService) readNewAPIQuotaUsage(
	ctx context.Context,
	account *Account,
	baseURL string,
	apiKey string,
	proxyURL string,
) (float64, error) {
	body, err := s.doJSONRequest(ctx, account, http.MethodGet, buildNewAPIEndpointURL(baseURL, "/api/usage/token/"), apiKey, proxyURL, nil)
	if err != nil {
		return 0, err
	}
	value, err := decodeNewAPINumber(body, "total_used")
	if err != nil || value < 0 {
		return 0, errors.New("New API total_used is unavailable")
	}
	return value, nil
}

func (s *AccountMultiplierService) waitForNewAPIQuotaUsage(
	ctx context.Context,
	account *Account,
	baseURL string,
	apiKey string,
	proxyURL string,
	before float64,
) (float64, error) {
	delay := accountMultiplierQuotaPollDelay
	for attempt := 0; attempt < accountMultiplierQuotaPollAttempts; attempt++ {
		after, err := s.readNewAPIQuotaUsage(ctx, account, baseURL, apiKey, proxyURL)
		if err != nil {
			return 0, err
		}
		if after > before {
			return after, nil
		}
		if attempt == accountMultiplierQuotaPollAttempts-1 {
			break
		}
		wait := s.wait
		if wait == nil {
			wait = waitForAccountMultiplierDelay
		}
		if err := wait(ctx, delay); err != nil {
			return 0, err
		}
		delay *= 2
		if delay > accountMultiplierQuotaPollMaxDelay {
			delay = accountMultiplierQuotaPollMaxDelay
		}
	}
	return 0, errors.New("quota delta was not observed")
}

func (s *AccountMultiplierService) runNewAPICompletion(
	ctx context.Context,
	account *Account,
	baseURL string,
	apiKey string,
	proxyURL string,
	modelID string,
) (newAPICompletionUsage, error) {
	payload := map[string]any{
		"model": modelID,
		"messages": []map[string]any{{
			"role":    "user",
			"content": strings.Repeat("multiplier probe ", 512) + "Reply with OK.",
		}},
		"temperature": 0,
		"max_tokens":  64,
		"stream":      false,
	}
	body, err := s.doJSONRequest(
		ctx,
		account,
		http.MethodPost,
		buildOpenAIChatCompletionsURL(baseURL),
		apiKey,
		proxyURL,
		payload,
	)
	if err != nil {
		return newAPICompletionUsage{}, err
	}
	return decodeNewAPICompletionUsage(body)
}

func (s *AccountMultiplierService) doJSONRequest(
	ctx context.Context,
	account *Account,
	method string,
	endpoint string,
	apiKey string,
	proxyURL string,
	payload any,
) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(body)
	}
	requestCtx, cancel := context.WithTimeout(ctx, accountMultiplierRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(
		WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI),
	))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	account.ApplyHeaderOverrides(req.Header)
	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := s.accountTestService.httpUpstream.DoWithTLS(
		req,
		proxyURL,
		account.ID,
		account.Concurrency,
		tlsProfile,
	)
	if err != nil {
		return nil, errors.New("account multiplier upstream request failed")
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("account multiplier upstream response is empty")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, accountMultiplierMaxBodyBytes+1))
	if err != nil {
		return nil, errors.New("account multiplier upstream response read failed")
	}
	if len(body) > accountMultiplierMaxBodyBytes {
		return nil, errors.New("account multiplier upstream response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("account multiplier upstream returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func (s *AccountMultiplierService) persistMeasurement(
	ctx context.Context,
	account *Account,
	snapshot *AccountMultiplierMeasurementSnapshot,
) error {
	writer, ok := s.accountRepo.(accountMultiplierMeasurementWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	if err := writer.UpdateAccountMultiplierMeasurement(ctx, account, snapshot); err != nil {
		return err
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[AccountMultiplierMeasurementExtraKey] = snapshot
	return nil
}

func (s *AccountMultiplierService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func waitForAccountMultiplierDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func accountMultiplierFailureCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	case strings.Contains(err.Error(), "quota delta"):
		return "quota_delta_not_observed"
	case strings.Contains(err.Error(), "pricing"):
		return "pricing_unavailable"
	case strings.Contains(err.Error(), "completion usage"):
		return "completion_usage_unavailable"
	case strings.Contains(err.Error(), "HTTP "):
		return "upstream_http_error"
	default:
		return "measurement_failed"
	}
}

func accountMultiplierProxyURL(account *Account) (string, error) {
	if account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy == nil {
		return "", errors.New("account multiplier proxy is unavailable")
	}
	if account.Proxy.ID != *account.ProxyID {
		return "", ErrUpstreamBillingProbeIdentityChanged
	}
	return account.Proxy.URL(), nil
}

func buildNewAPIEndpointURL(baseURL string, endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if slash := strings.LastIndex(path, "/"); slash >= 0 && isOpenAIAPIVersionSegment(path[slash+1:]) {
		path = path[:slash]
	} else if isOpenAIAPIVersionSegment(strings.TrimLeft(path, "/")) {
		path = ""
	}
	parsed.Path = strings.TrimRight(path, "/") + "/" + strings.TrimLeft(endpoint, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func decodeNewAPINumber(body []byte, field string) (float64, error) {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, err
	}
	if data, ok := envelope["data"].(map[string]any); ok {
		envelope = data
	}
	value, ok := resolveAccountExtraNumber(envelope, field)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s is invalid", field)
	}
	return value, nil
}

func decodeNewAPICompletionUsage(body []byte) (newAPICompletionUsage, error) {
	var response struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return newAPICompletionUsage{}, err
	}
	if response.Usage.PromptTokens <= 0 || response.Usage.CompletionTokens < 0 ||
		response.Usage.PromptTokensDetails.CachedTokens < 0 ||
		response.Usage.PromptTokensDetails.CachedTokens > response.Usage.PromptTokens {
		return newAPICompletionUsage{}, errors.New("completion usage is unavailable")
	}
	return newAPICompletionUsage{
		InputTokens:     response.Usage.PromptTokens,
		OutputTokens:    response.Usage.CompletionTokens,
		CacheReadTokens: response.Usage.PromptTokensDetails.CachedTokens,
	}, nil
}

func (s *AccountMultiplierService) Resolve(account *Account, now time.Time) AccountMonitorMultiplier {
	if account == nil {
		return unavailableAccountMultiplier()
	}
	snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if snapshot == nil {
		if account.Extra != nil {
			if _, exists := account.Extra[UpstreamBillingProbeExtraKey]; exists {
				return AccountMonitorMultiplier{
					Source: AccountMonitorMultiplierSourceDeclared,
					Status: AccountMonitorMultiplierStatusFailed,
				}
			}
		}
		return unavailableAccountMultiplier()
	}
	switch snapshot.Status {
	case UpstreamBillingProbeStatusUnsupported:
		return resolveMeasuredAccountMultiplier(account.Extra, now)
	case UpstreamBillingProbeStatusFailed:
		return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusFailed}
	case UpstreamBillingProbeStatusOK:
	default:
		return unavailableAccountMultiplier()
	}
	if snapshot.FreshUntil == nil || !now.Before(*snapshot.FreshUntil) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceDeclared,
			Status:     AccountMonitorMultiplierStatusStale,
			ObservedAt: accountMultiplierObservedAt(snapshot),
		}
	}
	value, ok := upstreamBillingRateAt(snapshot.Data, now)
	if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceDeclared,
			Status:     AccountMonitorMultiplierStatusFailed,
			ObservedAt: accountMultiplierObservedAt(snapshot),
		}
	}
	return AccountMonitorMultiplier{
		Value:      float64PointerCopy(value),
		Source:     AccountMonitorMultiplierSourceDeclared,
		Status:     AccountMonitorMultiplierStatusOK,
		ObservedAt: accountMultiplierObservedAt(snapshot),
	}
}

func resolveMeasuredAccountMultiplier(extra map[string]any, now time.Time) AccountMonitorMultiplier {
	snapshot := decodeAccountMultiplierMeasurementSnapshot(extra)
	if snapshot == nil {
		return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusUnsupported}
	}
	observedAt := accountMultiplierMeasurementObservedAt(snapshot)
	if snapshot.Status != AccountMonitorMultiplierStatusOK {
		status := snapshot.Status
		if status != AccountMonitorMultiplierStatusFailed &&
			status != AccountMonitorMultiplierStatusUnavailable &&
			status != AccountMonitorMultiplierStatusUnsupported {
			status = AccountMonitorMultiplierStatusFailed
		}
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceMeasured,
			Status:     status,
			ObservedAt: observedAt,
		}
	}
	if snapshot.FreshUntil == nil || !now.Before(*snapshot.FreshUntil) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceMeasured,
			Status:     AccountMonitorMultiplierStatusStale,
			ObservedAt: observedAt,
		}
	}
	if snapshot.Value == nil || *snapshot.Value <= 0 ||
		math.IsNaN(*snapshot.Value) || math.IsInf(*snapshot.Value, 0) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceMeasured,
			Status:     AccountMonitorMultiplierStatusFailed,
			ObservedAt: observedAt,
		}
	}
	return AccountMonitorMultiplier{
		Value:      float64PointerCopy(*snapshot.Value),
		Source:     AccountMonitorMultiplierSourceMeasured,
		Status:     AccountMonitorMultiplierStatusOK,
		ObservedAt: observedAt,
	}
}

func decodeAccountMultiplierMeasurementSnapshot(extra map[string]any) *AccountMultiplierMeasurementSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[AccountMultiplierMeasurementExtraKey]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot AccountMultiplierMeasurementSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil ||
		snapshot.Version != AccountMultiplierMeasurementVersion ||
		snapshot.Source != AccountMonitorMultiplierSourceMeasured ||
		snapshot.Status == "" {
		return nil
	}
	return &snapshot
}

func accountMultiplierMeasurementObservedAt(snapshot *AccountMultiplierMeasurementSnapshot) *time.Time {
	if snapshot == nil || snapshot.ObservedAt == nil {
		return nil
	}
	observedAt := snapshot.ObservedAt.UTC()
	return &observedAt
}

func calculateAccountMultiplierSample(before, after, quotaPerUnit, officialCost float64) (float64, error) {
	values := []float64{before, after, quotaPerUnit, officialCost}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.New("non-finite multiplier evidence")
		}
	}
	delta := after - before
	if delta <= 0 {
		return 0, errors.New("quota delta must be positive")
	}
	if quotaPerUnit <= 0 {
		return 0, errors.New("quota_per_unit must be positive")
	}
	if officialCost <= 0 {
		return 0, errors.New("official cost must be positive")
	}
	value := (delta / quotaPerUnit) / officialCost
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("measured multiplier is invalid")
	}
	return value, nil
}

func summarizeAccountMultiplierSamples(samples []float64) (float64, float64, error) {
	if len(samples) != accountMultiplierMeasurementSamples {
		return 0, 0, errors.New("exactly three multiplier samples are required")
	}
	sorted := append([]float64(nil), samples...)
	for _, sample := range sorted {
		if sample <= 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
			return 0, 0, errors.New("multiplier sample is invalid")
		}
	}
	sort.Float64s(sorted)
	median := sorted[1]
	spread := (sorted[len(sorted)-1] - sorted[0]) / median
	if spread > accountMultiplierMaxRelativeSpread {
		return 0, spread, errors.New("multiplier samples are unstable")
	}
	return median, spread, nil
}

func unavailableAccountMultiplier() AccountMonitorMultiplier {
	return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusUnavailable}
}

func accountMultiplierObservedAt(snapshot *UpstreamBillingProbeSnapshot) *time.Time {
	if snapshot == nil || snapshot.ReceivedAt == nil {
		return nil
	}
	observedAt := snapshot.ReceivedAt.UTC()
	return &observedAt
}

func float64PointerCopy(value float64) *float64 {
	return &value
}
