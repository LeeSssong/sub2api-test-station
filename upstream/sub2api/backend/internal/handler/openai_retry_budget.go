package handler

import (
	"context"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	openAIMaxAccountSwitches            = 2
	openAIRetryReasonAttemptLimit       = "attempt_limit"
	openAIRetryReasonAccountSwitchLimit = "account_switch_limit"
	openAIRetryReasonFailureDomainLimit = "failure_domain_limit"
	openAIRetryReasonRetryDeadline      = "retry_deadline"
	openAIRetryReasonUnsafeToReplay     = "unsafe_to_replay"
)

func openAISameAccountRetryLimit(statusCode, configuredLimit int) int {
	if configuredLimit < 0 {
		configuredLimit = 0
	}
	// T82's automatic 502/503 recovery is deliberately bounded to one
	// same-account replay even when pool-mode configuration asks for more.
	if statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable {
		if configuredLimit > 1 {
			return 1
		}
	}
	return configuredLimit
}

type openAIRetryBudgetConfig struct {
	MaxAttempts        int
	MaxAccountSwitches int
	MaxFailureDomains  int
	Total              time.Duration
	BackoffInitial     time.Duration
	BackoffMax         time.Duration
}

type openAIRetryBudget struct {
	cfg           openAIRetryBudgetConfig
	now           func() time.Time
	deadline      time.Time
	attempts      int
	switches      int
	lastAccountID int64
	domains       map[string]struct{}
	observed      []service.OpenAIFailureDomain
	unknownSeen   bool
	reason        string
	degraded      bool
	maxSwitches   int
}

func openAIRetryBudgetConfigFromConfig(cfg *config.Config) openAIRetryBudgetConfig {
	shared := config.DefaultGatewayOpenAISharedHealthConfig()
	if cfg != nil {
		candidate := cfg.Gateway.OpenAISharedHealth
		if candidate != (config.GatewayOpenAISharedHealthConfig{}) {
			if candidate.MaxAttempts > 0 {
				shared.MaxAttempts = candidate.MaxAttempts
			}
			if candidate.MaxAccountSwitches >= 0 {
				shared.MaxAccountSwitches = candidate.MaxAccountSwitches
			}
			if candidate.MaxFailureDomains > 0 {
				shared.MaxFailureDomains = candidate.MaxFailureDomains
			}
			if candidate.TotalRetryBudgetMS > 0 {
				shared.TotalRetryBudgetMS = candidate.TotalRetryBudgetMS
			}
			if candidate.BackoffInitialMS >= 0 {
				shared.BackoffInitialMS = candidate.BackoffInitialMS
			}
			if candidate.BackoffMaxMS > 0 {
				shared.BackoffMaxMS = candidate.BackoffMaxMS
			}
		}
	}
	if shared.MaxAttempts > 4 {
		shared.MaxAttempts = 4
	}
	if shared.MaxAccountSwitches > openAIMaxAccountSwitches {
		shared.MaxAccountSwitches = openAIMaxAccountSwitches
	}
	if shared.MaxFailureDomains > 2 {
		shared.MaxFailureDomains = 2
	}
	if shared.TotalRetryBudgetMS > 5000 {
		shared.TotalRetryBudgetMS = 5000
	}
	if shared.BackoffInitialMS < 0 {
		shared.BackoffInitialMS = 0
	}
	if shared.BackoffMaxMS < shared.BackoffInitialMS || shared.BackoffMaxMS > 2000 {
		shared.BackoffMaxMS = 2000
	}
	return openAIRetryBudgetConfig{
		MaxAttempts: shared.MaxAttempts, MaxAccountSwitches: shared.MaxAccountSwitches,
		MaxFailureDomains: shared.MaxFailureDomains, Total: time.Duration(shared.TotalRetryBudgetMS) * time.Millisecond,
		BackoffInitial: time.Duration(shared.BackoffInitialMS) * time.Millisecond,
		BackoffMax:     time.Duration(shared.BackoffMaxMS) * time.Millisecond,
	}
}

func newOpenAIRetryBudget(cfg openAIRetryBudgetConfig, now func() time.Time) *openAIRetryBudget {
	if now == nil {
		now = time.Now
	}
	if cfg.MaxAttempts <= 0 || cfg.MaxAttempts > 4 {
		cfg.MaxAttempts = 4
	}
	if cfg.MaxAccountSwitches < 0 || cfg.MaxAccountSwitches > openAIMaxAccountSwitches {
		cfg.MaxAccountSwitches = openAIMaxAccountSwitches
	}
	if cfg.MaxFailureDomains <= 0 || cfg.MaxFailureDomains > 2 {
		cfg.MaxFailureDomains = 2
	}
	if cfg.Total <= 0 || cfg.Total > 5*time.Second {
		cfg.Total = 5 * time.Second
	}
	if cfg.BackoffInitial < 0 {
		cfg.BackoffInitial = 0
	}
	if cfg.BackoffInitial == 0 {
		cfg.BackoffInitial = 120 * time.Millisecond
	}
	if cfg.BackoffMax < cfg.BackoffInitial || cfg.BackoffMax > 2*time.Second {
		cfg.BackoffMax = 2 * time.Second
	}
	startedAt := now()
	return &openAIRetryBudget{
		cfg: cfg, now: now, deadline: startedAt.Add(cfg.Total), domains: make(map[string]struct{}),
		maxSwitches: cfg.MaxAccountSwitches,
	}
}

func (b *openAIRetryBudget) ConsumeAttempt(accountID int64) bool {
	if b == nil || accountID <= 0 {
		return false
	}
	if b.DeadlineReached() {
		b.reason = openAIRetryReasonRetryDeadline
		return false
	}
	if b.attempts >= b.cfg.MaxAttempts {
		b.reason = openAIRetryReasonAttemptLimit
		return false
	}
	if b.lastAccountID > 0 && accountID != b.lastAccountID {
		if b.switches >= b.maxSwitches {
			b.reason = openAIRetryReasonAccountSwitchLimit
			return false
		}
		b.switches++
	}
	b.attempts++
	b.lastAccountID = accountID
	return true
}

func (b *openAIRetryBudget) consumeAccountAttempt(account *service.Account) bool {
	if account == nil {
		return false
	}
	return b.ConsumeAttempt(account.ID)
}

func openAIRetryFailureDomains(account *service.Account, channelID int64) []service.OpenAIFailureDomain {
	domains := service.DeriveOpenAIFailureDomains(account, channelID)
	if len(domains) > 0 {
		return domains
	}
	return []service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainUnknown, ID: "unknown"}}
}

func (b *openAIRetryBudget) CanSwitch(nextAccountID int64, outputStarted, hasSideEffect bool) bool {
	if b == nil {
		return false
	}
	if outputStarted || hasSideEffect {
		b.reason = openAIRetryReasonUnsafeToReplay
		return false
	}
	if b.DeadlineReached() {
		b.reason = openAIRetryReasonRetryDeadline
		return false
	}
	if b.attempts >= b.cfg.MaxAttempts {
		b.reason = openAIRetryReasonAttemptLimit
		return false
	}
	if b.lastAccountID != 0 && nextAccountID != b.lastAccountID && b.switches >= b.maxSwitches {
		b.reason = openAIRetryReasonAccountSwitchLimit
		return false
	}
	return true
}

func (b *openAIRetryBudget) ObserveDomain(domains []service.OpenAIFailureDomain) bool {
	if b == nil {
		return false
	}
	newKeys := make([]string, 0, len(domains))
	unknown := false
	for _, domain := range domains {
		if domain.Type == service.OpenAIFailureDomainUnknown || strings.TrimSpace(domain.ID) == "" {
			unknown = true
			continue
		}
		key := string(domain.Type) + ":" + strings.TrimSpace(domain.ID)
		if _, exists := b.domains[key]; !exists {
			newKeys = append(newKeys, key)
		}
	}
	additional := len(newKeys)
	if unknown && !b.unknownSeen {
		additional++
	}
	if len(b.domains)+boolIntHandler(b.unknownSeen)+additional > b.cfg.MaxFailureDomains {
		b.reason = openAIRetryReasonFailureDomainLimit
		return false
	}
	for _, key := range newKeys {
		b.domains[key] = struct{}{}
		parts := strings.SplitN(key, ":", 2)
		b.observed = append(b.observed, service.OpenAIFailureDomain{Type: service.OpenAIFailureDomainType(parts[0]), ID: parts[1]})
	}
	if unknown {
		if !b.unknownSeen {
			b.observed = append(b.observed, service.OpenAIFailureDomain{Type: service.OpenAIFailureDomainUnknown, ID: "unknown"})
		}
		b.unknownSeen = true
	}
	return true
}

func (b *openAIRetryBudget) ObservedDomains() []service.OpenAIFailureDomain {
	if b == nil || len(b.observed) == 0 {
		return nil
	}
	return append([]service.OpenAIFailureDomain(nil), b.observed...)
}

func (b *openAIRetryBudget) NarrowForSharedHealthDegraded() {
	if b == nil || b.degraded {
		return
	}
	b.degraded = true
	limit := b.switches + 1
	if limit < b.maxSwitches {
		b.maxSwitches = limit
	}
}

func (b *openAIRetryBudget) Reason() string {
	if b == nil {
		return ""
	}
	return b.reason
}

func (b *openAIRetryBudget) Remaining() time.Duration {
	if b == nil {
		return 0
	}
	remaining := b.deadline.Sub(b.now())
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (b *openAIRetryBudget) DeadlineReached() bool {
	return b == nil || b.Remaining() <= 0
}

func (b *openAIRetryBudget) hasAdditionalSwitchAttemptCapacity() bool {
	if b == nil || b.DeadlineReached() || b.attempts >= b.cfg.MaxAttempts {
		return false
	}
	return b.lastAccountID == 0 || b.switches < b.maxSwitches
}

func openAITTFTReportEligible(enabled, outputStarted, hasSideEffects bool, eligibleCount int, budget *openAIRetryBudget) bool {
	return enabled && !outputStarted && !hasSideEffects && eligibleCount > 1 && budget.hasAdditionalSwitchAttemptCapacity()
}

func openAIRetryBudgetExhausted(reason string) bool {
	switch reason {
	case openAIRetryReasonAttemptLimit, openAIRetryReasonAccountSwitchLimit, openAIRetryReasonFailureDomainLimit, openAIRetryReasonRetryDeadline:
		return true
	default:
		return false
	}
}

func (b *openAIRetryBudget) RetryDelay(failure *service.UpstreamFailoverError, retryCount int) (time.Duration, bool) {
	if b == nil {
		return 0, false
	}
	if b.DeadlineReached() {
		b.reason = openAIRetryReasonRetryDeadline
		return 0, false
	}
	if failure != nil && failure.StatusCode == http.StatusTooManyRequests {
		if delay, ok := parseOpenAIRetryAfter(failure.ResponseHeaders, b.now()); ok {
			if delay > b.Remaining() {
				b.reason = openAIRetryReasonRetryDeadline
				return delay, false
			}
			return delay, true
		}
	}
	if retryCount < 1 {
		retryCount = 1
	}
	delay := b.cfg.BackoffInitial
	for attempt := 1; attempt < retryCount && delay < b.cfg.BackoffMax; attempt++ {
		delay *= 2
		if delay > b.cfg.BackoffMax {
			delay = b.cfg.BackoffMax
		}
	}
	delay = stableOpenAIRetryJitter(delay, b.cfg.BackoffMax, failure, retryCount)
	if delay > b.Remaining() {
		b.reason = openAIRetryReasonRetryDeadline
		return delay, false
	}
	return delay, true
}

func stableOpenAIRetryJitter(base, maximum time.Duration, failure *service.UpstreamFailoverError, retryCount int) time.Duration {
	if base <= 0 || maximum <= base {
		return base
	}
	upper := base + base/5
	if upper > maximum {
		upper = maximum
	}
	span := upper - base
	if span <= 0 {
		return base
	}
	h := fnv.New64a()
	if failure != nil {
		_, _ = h.Write([]byte(failure.AttemptID))
		_, _ = h.Write([]byte("|" + strconv.Itoa(failure.StatusCode)))
	}
	_, _ = h.Write([]byte("|" + strconv.Itoa(retryCount)))
	return base + time.Duration(h.Sum64()%uint64(span+1))
}

func parseOpenAIRetryAfter(headers http.Header, now time.Time) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(raw, 10, 31); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	retryAt, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	delay := retryAt.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func boolIntHandler(value bool) int {
	if value {
		return 1
	}
	return 0
}

func recordOpenAIAttemptSharedSuccess(
	ctx context.Context,
	gateway *service.OpenAIGatewayService,
	account *service.Account,
	canonicalModel string,
	channelID int64,
	eventID string,
	platform string,
	result *service.OpenAIForwardResult,
) {
	if gateway == nil || account == nil {
		return
	}
	if strings.TrimSpace(eventID) != "" {
		eventID += ":success"
	}
	var ttft time.Duration
	if result != nil && result.FirstTokenMs != nil && *result.FirstTokenMs > 0 {
		ttft = time.Duration(*result.FirstTokenMs) * time.Millisecond
	}
	gateway.RecordOpenAIAccountModelSuccess(ctx, service.OpenAIAccountModelSuccessEvent{
		EventID: eventID, AccountID: account.ID, CanonicalModel: canonicalModel,
		Domains: service.DeriveOpenAIFailureDomains(account, channelID), TTFT: ttft,
		Platform: platform,
	})
}
