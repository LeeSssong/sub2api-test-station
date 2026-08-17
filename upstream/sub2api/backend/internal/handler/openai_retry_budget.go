package handler

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type openAIRetryBudgetConfig struct {
	MaxAttempts        int
	MaxAccountSwitches int
	MaxFailureDomains  int
	Total              time.Duration
}

type openAIRetryBudget struct {
	cfg           openAIRetryBudgetConfig
	now           func() time.Time
	deadline      time.Time
	attempts      int
	switches      int
	lastAccountID int64
	domains       map[string]struct{}
	unknownSeen   bool
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
		}
	}
	if shared.MaxAttempts > 4 {
		shared.MaxAttempts = 4
	}
	if shared.MaxAccountSwitches > 3 {
		shared.MaxAccountSwitches = 3
	}
	if shared.MaxFailureDomains > 2 {
		shared.MaxFailureDomains = 2
	}
	if shared.TotalRetryBudgetMS > 5000 {
		shared.TotalRetryBudgetMS = 5000
	}
	return openAIRetryBudgetConfig{
		MaxAttempts: shared.MaxAttempts, MaxAccountSwitches: shared.MaxAccountSwitches,
		MaxFailureDomains: shared.MaxFailureDomains, Total: time.Duration(shared.TotalRetryBudgetMS) * time.Millisecond,
	}
}

func newOpenAIRetryBudget(cfg openAIRetryBudgetConfig, now func() time.Time) *openAIRetryBudget {
	if now == nil {
		now = time.Now
	}
	if cfg.MaxAttempts <= 0 || cfg.MaxAttempts > 4 {
		cfg.MaxAttempts = 4
	}
	if cfg.MaxAccountSwitches < 0 || cfg.MaxAccountSwitches > 3 {
		cfg.MaxAccountSwitches = 3
	}
	if cfg.MaxFailureDomains <= 0 || cfg.MaxFailureDomains > 2 {
		cfg.MaxFailureDomains = 2
	}
	if cfg.Total <= 0 || cfg.Total > 5*time.Second {
		cfg.Total = 5 * time.Second
	}
	startedAt := now()
	return &openAIRetryBudget{cfg: cfg, now: now, deadline: startedAt.Add(cfg.Total), domains: make(map[string]struct{})}
}

func (b *openAIRetryBudget) ConsumeAttempt(accountID int64) bool {
	if b == nil || accountID <= 0 || b.DeadlineReached() || b.attempts >= b.cfg.MaxAttempts {
		return false
	}
	if b.lastAccountID > 0 && accountID != b.lastAccountID {
		if b.switches >= b.cfg.MaxAccountSwitches {
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
	for _, wanted := range []service.OpenAIFailureDomainType{service.OpenAIFailureDomainProviderChannel, service.OpenAIFailureDomainQuotaPool} {
		for _, domain := range domains {
			if domain.Type == wanted {
				return []service.OpenAIFailureDomain{domain}
			}
		}
	}
	return []service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainUnknown, ID: "unknown"}}
}

func (b *openAIRetryBudget) CanSwitch(nextAccountID int64, outputStarted, hasSideEffect bool) bool {
	if b == nil || outputStarted || hasSideEffect || b.DeadlineReached() || b.attempts >= b.cfg.MaxAttempts {
		return false
	}
	return b.lastAccountID == 0 || nextAccountID == b.lastAccountID || b.switches < b.cfg.MaxAccountSwitches
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
		return false
	}
	for _, key := range newKeys {
		b.domains[key] = struct{}{}
	}
	if unknown {
		b.unknownSeen = true
	}
	return true
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

func boolIntHandler(value bool) int {
	if value {
		return 1
	}
	return 0
}
