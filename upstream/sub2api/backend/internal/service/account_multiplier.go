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
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	accountMultiplierRequestTimeout = 30 * time.Second
	accountMultiplierMaxBodyBytes   = 128 * 1024

	AccountMonitorMultiplierSourceDeclared = "declared"
	AccountMonitorMultiplierSourceManual   = "manual"

	AccountMonitorMultiplierStatusOK          = "ok"
	AccountMonitorMultiplierStatusStale       = "stale"
	AccountMonitorMultiplierStatusUnsupported = "unsupported"
	AccountMonitorMultiplierStatusFailed      = "failed"
	AccountMonitorMultiplierStatusUnavailable = "unavailable"
)

type AccountMultiplierService struct {
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	declarationProbe   accountMultiplierDeclarationProbe
	now                func() time.Time
}

type accountMultiplierDeclarationProbe interface {
	ProbeAccount(context.Context, int64) (*UpstreamBillingProbeSnapshot, error)
}

func NewAccountMultiplierService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	_ *BillingService,
) *AccountMultiplierService {
	return &AccountMultiplierService{
		accountRepo:        accountRepo,
		accountTestService: accountTestService,
		now:                time.Now,
	}
}

func (s *AccountMultiplierService) SetDeclarationProbe(probe accountMultiplierDeclarationProbe) {
	if s != nil {
		s.declarationProbe = probe
	}
}

func (s *AccountMultiplierService) Refresh(ctx context.Context, account *Account, options AccountMonitorRefreshOptions) error {
	if account == nil {
		return ErrAccountNilInput
	}
	declaration := decodeUpstreamBillingProbeSnapshot(account.Extra)
	var refreshErrors []error
	if options.RefreshDeclaration && s.declarationProbe != nil {
		probed, err := s.declarationProbe.ProbeAccount(ctx, account.ID)
		if err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("refresh billing declaration: %w", err))
		} else if probed != nil {
			declaration = probed
			if account.Extra == nil {
				account.Extra = make(map[string]any)
			}
			account.Extra[UpstreamBillingProbeExtraKey] = probed
		}
	}
	if options.RefreshBalance {
		if err := s.refreshBalanceForDeclaration(ctx, account, declaration); err != nil {
			refreshErrors = append(refreshErrors, err)
		}
	}
	return errors.Join(refreshErrors...)
}

type newAPIQuotaStatus struct {
	QuotaPerUnit float64
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
		return nil, errors.New("account monitor upstream request failed")
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("account monitor upstream response is empty")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, accountMultiplierMaxBodyBytes+1))
	if err != nil {
		return nil, errors.New("account monitor upstream response read failed")
	}
	if len(body) > accountMultiplierMaxBodyBytes {
		return nil, errors.New("account monitor upstream response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("account monitor upstream returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func (s *AccountMultiplierService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func accountMultiplierProxyURL(account *Account) (string, error) {
	if account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy == nil {
		return "", errors.New("account monitor proxy is unavailable")
	}
	if account.Proxy.ID != *account.ProxyID {
		return "", ErrUpstreamBillingProbeIdentityChanged
	}
	return account.Proxy.URL(), nil
}

func buildNewAPIEndpointURL(baseURL string, endpoint string) string {
	return buildNewAPIEndpointURLWithNativePrefix(baseURL, endpoint, false)
}

func buildNewAPIEndpointURLWithNativePrefix(baseURL string, endpoint string, stripNativeAPIPrefix bool) string {
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
	if stripNativeAPIPrefix && path == "/api" {
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

func (s *AccountMultiplierService) Resolve(account *Account, now time.Time) AccountMonitorMultiplier {
	if account == nil {
		return unavailableAccountMultiplier()
	}
	source := AccountMonitorMultiplierSourceManual
	if syncEnabled, _ := account.Extra[UpstreamBillingRateSyncEnabledExtraKey].(bool); syncEnabled {
		source = AccountMonitorMultiplierSourceDeclared
	}
	value := account.BillingRateMultiplier()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return AccountMonitorMultiplier{Source: source, Status: AccountMonitorMultiplierStatusFailed}
	}
	if source == AccountMonitorMultiplierSourceDeclared {
		if now.IsZero() {
			now = s.currentTime()
		}
		snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
		status := AccountMonitorMultiplierStatusUnavailable
		var observedAt *time.Time
		if snapshot != nil {
			observedAt = snapshot.ReceivedAt
			switch snapshot.Status {
			case UpstreamBillingProbeStatusOK:
				status = AccountMonitorMultiplierStatusOK
				if snapshot.FreshUntil != nil && !snapshot.FreshUntil.After(now) {
					status = AccountMonitorMultiplierStatusStale
				}
			case UpstreamBillingProbeStatusUnsupported:
				status = AccountMonitorMultiplierStatusUnsupported
			case UpstreamBillingProbeStatusFailed:
				status = AccountMonitorMultiplierStatusFailed
			}
		}
		return AccountMonitorMultiplier{Value: float64PointerCopy(value), Source: source, Status: status, ObservedAt: observedAt}
	}
	return AccountMonitorMultiplier{
		Value:       float64PointerCopy(value),
		Source:      source,
		Status:      AccountMonitorMultiplierStatusOK,
		ObservedAt:  accountUpdatedAt(account),
		SampleCount: 0,
	}
}

func accountUpdatedAt(account *Account) *time.Time {
	if account == nil || account.UpdatedAt.IsZero() {
		return nil
	}
	value := account.UpdatedAt
	return &value
}

func unavailableAccountMultiplier() AccountMonitorMultiplier {
	return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusUnavailable}
}

func float64PointerCopy(value float64) *float64 {
	return &value
}
