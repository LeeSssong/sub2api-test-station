package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	AccountMonitorBalanceExtraKey = "account_monitor_balance"
	AccountMonitorBalanceVersion  = 1

	AccountMonitorBalanceSourceSub2API = "sub2api"
	AccountMonitorBalanceSourceNewAPI  = "newapi"

	AccountMonitorBalanceStatusOK          = "ok"
	AccountMonitorBalanceStatusStale       = "stale"
	AccountMonitorBalanceStatusFailed      = "failed"
	AccountMonitorBalanceStatusUnsupported = "unsupported"
	AccountMonitorBalanceStatusUnavailable = "unavailable"
)

var errExplicitBalanceUnavailable = errors.New("explicit upstream balance unavailable")

// accountMonitorHTTPError preserves the small amount of upstream evidence
// needed to distinguish an explicit billing/credit exhaustion response from
// an otherwise generic HTTP failure. The body is never returned to callers.
type accountMonitorHTTPError struct {
	statusCode int
	body       string
}

func (e *accountMonitorHTTPError) Error() string {
	if e == nil {
		return "account monitor upstream returned HTTP error"
	}
	return fmt.Sprintf("account monitor upstream returned HTTP %d", e.statusCode)
}

// AccountMonitorBalance is display-only upstream balance evidence. It is
// deliberately separate from cost scoring and account scheduling state.
type AccountMonitorBalance struct {
	Version       int        `json:"version"`
	ValueUSD      *float64   `json:"value_usd,omitempty"`
	Source        string     `json:"source,omitempty"`
	Status        string     `json:"status"`
	ObservedAt    *time.Time `json:"observed_at,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	FailureCode   string     `json:"failure_code,omitempty"`
}

type accountMonitorBalanceWriter interface {
	UpdateAccountMonitorBalance(context.Context, *Account, *AccountMonitorBalance) error
}

// ResolveBalance returns a stored balance snapshot only for OpenAI API-key
// accounts. Invalid or missing historical values stay unavailable rather than
// being presented as a zero balance.
func (s *AccountMultiplierService) ResolveBalance(account *Account, _ time.Time) AccountMonitorBalance {
	if !isAccountMonitorBalanceEligible(account) {
		return unavailableAccountMonitorBalance()
	}
	snapshot := decodeAccountMonitorBalance(account.Extra)
	if snapshot == nil {
		return unavailableAccountMonitorBalance()
	}
	return *snapshot
}

func (s *AccountMultiplierService) refreshBalance(
	ctx context.Context,
	account *Account,
	declaration *UpstreamBillingProbeSnapshot,
) error {
	if !isAccountMonitorBalanceEligible(account) {
		return nil
	}
	now := s.currentTime().UTC()
	previous := decodeAccountMonitorBalance(account.Extra)
	var (
		snapshot *AccountMonitorBalance
		err      error
	)
	switch {
	case declaration != nil && declaration.Status == UpstreamBillingProbeStatusOK:
		snapshot, err = s.readSub2APIBalance(ctx, account, now)
	case declaration != nil && declaration.Status == UpstreamBillingProbeStatusUnsupported:
		snapshot, err = s.readNewAPIBalance(ctx, account, now)
	default:
		err = errors.New("balance source is unavailable until billing declaration is resolved")
	}
	if err != nil {
		snapshot = failedAccountMonitorBalance(previous, accountMonitorBalanceFailureCode(err), now)
	}
	if persistErr := s.persistBalance(ctx, account, snapshot); persistErr != nil {
		return persistErr
	}
	return err
}

func (s *AccountMultiplierService) readSub2APIBalance(
	ctx context.Context,
	account *Account,
	now time.Time,
) (*AccountMonitorBalance, error) {
	baseURL, apiKey, proxyURL, err := s.accountMonitorBalanceRequestIdentity(account)
	if err != nil {
		return nil, err
	}
	body, err := s.doJSONRequest(ctx, account, http.MethodGet, buildNewAPIEndpointURL(baseURL, "/v1/usage"), apiKey, proxyURL, nil)
	if err != nil {
		return nil, err
	}
	value, err := decodeSub2APIBalanceUSD(body)
	if err != nil {
		return nil, err
	}
	return successfulAccountMonitorBalance(value, AccountMonitorBalanceSourceSub2API, now), nil
}

func (s *AccountMultiplierService) readNewAPIBalance(
	ctx context.Context,
	account *Account,
	now time.Time,
) (*AccountMonitorBalance, error) {
	baseURL, apiKey, proxyURL, err := s.accountMonitorBalanceRequestIdentity(account)
	if err != nil {
		return nil, err
	}
	status, err := s.readNewAPIQuotaStatus(ctx, account, baseURL, apiKey, proxyURL)
	if err != nil {
		return nil, err
	}
	body, err := s.doJSONRequest(ctx, account, http.MethodGet, buildNewAPIEndpointURL(baseURL, "/api/usage/token/"), apiKey, proxyURL, nil)
	if err != nil {
		return nil, err
	}
	value, err := decodeNewAPIBalanceUSD(body, status.QuotaPerUnit)
	if err != nil {
		return nil, err
	}
	return successfulAccountMonitorBalance(value, AccountMonitorBalanceSourceNewAPI, now), nil
}

func (s *AccountMultiplierService) accountMonitorBalanceRequestIdentity(account *Account) (string, string, string, error) {
	if s == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return "", "", "", ErrUpstreamBillingProbeUnavailable
	}
	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return "", "", "", errors.New("account balance requires an API key")
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.accountTestService.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", "", "", errors.New("account balance base URL is invalid")
	}
	proxyURL, err := accountMultiplierProxyURL(account)
	if err != nil {
		return "", "", "", err
	}
	return normalizedBaseURL, apiKey, proxyURL, nil
}

func (s *AccountMultiplierService) persistBalance(
	ctx context.Context,
	account *Account,
	snapshot *AccountMonitorBalance,
) error {
	writer, ok := s.accountRepo.(accountMonitorBalanceWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	if err := writer.UpdateAccountMonitorBalance(ctx, account, snapshot); err != nil {
		return err
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[AccountMonitorBalanceExtraKey] = snapshot
	return nil
}

func decodeSub2APIBalanceUSD(body []byte) (float64, error) {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, err
	}
	if data, ok := envelope["data"].(map[string]any); ok {
		envelope = data
	}
	for _, field := range []string{"balance", "remaining"} {
		value, ok := resolveAccountExtraNumber(envelope, field)
		if ok && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value, nil
		}
	}
	if explicitBalanceUnavailableBody(body) {
		return 0, errExplicitBalanceUnavailable
	}
	return 0, errors.New("Sub2API balance is unavailable")
}

func decodeNewAPIBalanceUSD(body []byte, quotaPerUnit float64) (float64, error) {
	if quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return 0, errors.New("New API quota_per_unit is invalid")
	}
	totalAvailable, err := decodeNewAPINumber(body, "total_available")
	if err != nil || totalAvailable < 0 {
		if explicitBalanceUnavailableBody(body) {
			return 0, errExplicitBalanceUnavailable
		}
		return 0, errors.New("New API total_available is unavailable")
	}
	value := totalAvailable / quotaPerUnit
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, errors.New("New API balance is invalid")
	}
	return value, nil
}

func decodeAccountMonitorBalance(extra map[string]any) *AccountMonitorBalance {
	if extra == nil {
		return nil
	}
	value, ok := extra[AccountMonitorBalanceExtraKey]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot AccountMonitorBalance
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.Version != AccountMonitorBalanceVersion ||
		snapshot.Status == "" || !validAccountMonitorBalanceStatus(snapshot.Status) ||
		(snapshot.Source != "" && snapshot.Source != AccountMonitorBalanceSourceSub2API && snapshot.Source != AccountMonitorBalanceSourceNewAPI) {
		return nil
	}
	if snapshot.ValueUSD != nil && (*snapshot.ValueUSD < 0 || math.IsNaN(*snapshot.ValueUSD) || math.IsInf(*snapshot.ValueUSD, 0)) {
		return nil
	}
	return &snapshot
}

func successfulAccountMonitorBalance(value float64, source string, now time.Time) *AccountMonitorBalance {
	valueCopy := value
	observedAt := now.UTC()
	return &AccountMonitorBalance{
		Version:       AccountMonitorBalanceVersion,
		ValueUSD:      &valueCopy,
		Source:        source,
		Status:        AccountMonitorBalanceStatusOK,
		ObservedAt:    &observedAt,
		LastAttemptAt: &observedAt,
	}
}

func failedAccountMonitorBalance(previous *AccountMonitorBalance, failureCode string, now time.Time) *AccountMonitorBalance {
	attemptedAt := now.UTC()
	result := &AccountMonitorBalance{
		Version:       AccountMonitorBalanceVersion,
		Status:        AccountMonitorBalanceStatusFailed,
		LastAttemptAt: &attemptedAt,
		FailureCode:   failureCode,
	}
	if previous == nil {
		return result
	}
	result.Source = previous.Source
	if previous.ValueUSD != nil {
		value := *previous.ValueUSD
		result.ValueUSD = &value
	}
	if previous.ObservedAt != nil {
		observedAt := previous.ObservedAt.UTC()
		result.ObservedAt = &observedAt
	}
	return result
}

func unavailableAccountMonitorBalance() AccountMonitorBalance {
	return AccountMonitorBalance{Status: AccountMonitorBalanceStatusUnavailable}
}

func isAccountMonitorBalanceEligible(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey
}

// accountBalanceVetoesScheduling is intentionally narrow: only an explicit
// upstream "balance unavailable" result is strong enough to remove an
// account from the scheduling pool. Unknown, stale, timeout, and transport
// failures must not turn into an accidental account outage.
func accountBalanceVetoesScheduling(account *Account) bool {
	if !isAccountMonitorBalanceEligible(account) {
		return false
	}
	snapshot := decodeAccountMonitorBalance(account.Extra)
	return snapshot != nil && snapshot.Status == AccountMonitorBalanceStatusFailed && snapshot.FailureCode == "balance_unavailable"
}

func validAccountMonitorBalanceStatus(status string) bool {
	switch status {
	case AccountMonitorBalanceStatusOK, AccountMonitorBalanceStatusStale, AccountMonitorBalanceStatusFailed,
		AccountMonitorBalanceStatusUnsupported, AccountMonitorBalanceStatusUnavailable:
		return true
	default:
		return false
	}
}

func accountMonitorBalanceFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	if errors.Is(err, errExplicitBalanceUnavailable) {
		return "balance_unavailable"
	}
	var httpErr *accountMonitorHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.statusCode == http.StatusPaymentRequired || explicitBalanceUnavailableBody([]byte(httpErr.body)) {
			return "balance_unavailable"
		}
		return "upstream_http_error"
	}
	if errors.Is(err, ErrUpstreamBillingProbeUnavailable) {
		return "upstream_unavailable"
	}
	// Preserve the historical classification for wrapped/string-only HTTP
	// errors emitted by callers outside doJSONRequest; these are not balance
	// evidence and therefore remain non-vetoing.
	if strings.Contains(err.Error(), "HTTP ") {
		return "upstream_http_error"
	}
	return "unknown"
}

func explicitBalanceUnavailableBody(body []byte) bool {
	text := strings.ToLower(string(body))
	for _, marker := range []string{
		"insufficient balance",
		"insufficient quota",
		"quota exceeded",
		"balance exhausted",
		"run out of credits",
		"billing hard limit",
		"payment required",
		"余额不足",
		"额度不足",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (s *AccountMultiplierService) refreshBalanceForDeclaration(
	ctx context.Context,
	account *Account,
	declaration *UpstreamBillingProbeSnapshot,
) error {
	if err := s.refreshBalance(ctx, account, declaration); err != nil {
		return fmt.Errorf("refresh account balance: %w", err)
	}
	return nil
}
