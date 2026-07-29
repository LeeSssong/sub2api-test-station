package pricingevents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreampricing"
)

type AccountReader interface {
	ListAccounts(context.Context) ([]sub2api.Account, error)
}

type MultiplierSource interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
}

type BaselineRepository interface {
	GetOperationalBaseline(context.Context, string) (store.Baseline, bool, error)
	PutOperationalBaseline(context.Context, store.Baseline) error
}

type PricingResolver interface {
	Resolve(context.Context, string) (upstreampricing.Resolution, bool)
}

type EventSender interface {
	SendOneShot(context.Context, notify.OneShotIdentity, notify.FeishuMessage) error
}

type DecisionRecorder interface {
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type Service struct {
	Accounts    AccountReader
	Multipliers MultiplierSource
	Baselines   BaselineRepository
	Resolver    PricingResolver
	Notifier    EventSender
	Decisions   DecisionRecorder
	Policy      notificationpolicy.Policy
	Now         func() time.Time
}

func (service Service) Run(ctx context.Context) error {
	if service.Policy.Version <= 0 ||
		!service.Policy.Enabled(notificationpolicy.FamilyPricingNotice) {
		return nil
	}
	if service.Accounts == nil || service.Multipliers == nil ||
		service.Baselines == nil || service.Decisions == nil {
		return fmt.Errorf("pricing event dependencies are incomplete")
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list accounts for pricing events: %w", err)
	}
	projection, err := service.Multipliers.ListAccountMonitors(ctx)
	if err != nil || projection.Stale {
		return nil
	}
	active := make(map[int64]sub2api.Account, len(accounts))
	for _, account := range accounts {
		if account.Status == "active" && account.Schedulable {
			active[account.ID] = account
		}
	}
	now := time.Now().UTC()
	if service.Now != nil {
		now = service.Now().UTC()
	}
	for _, observed := range projection.Accounts {
		account, exists := active[observed.AccountID]
		if !exists {
			continue
		}
		value, trustworthy := trustworthyMultiplier(observed.Multiplier)
		if !trustworthy {
			continue
		}
		if err := service.evaluate(ctx, account, value, now); err != nil {
			return err
		}
	}
	return nil
}

func (service Service) evaluate(
	ctx context.Context,
	account sub2api.Account,
	current float64,
	observedAt time.Time,
) error {
	baselineKey := "multiplier:account:" + strconv.FormatInt(account.ID, 10)
	currentValue := multiplierValue(current)
	currentHash := hashParts(strconv.FormatInt(account.ID, 10), currentValue)
	baseline, found, err := service.Baselines.GetOperationalBaseline(ctx, baselineKey)
	if err != nil {
		return fmt.Errorf("read multiplier baseline: %w", err)
	}
	next := store.Baseline{
		Key: baselineKey, CurrentValue: currentValue,
		EvidenceHash: currentHash, UpdatedAt: observedAt,
	}
	if !found {
		return service.Baselines.PutOperationalBaseline(ctx, next)
	}
	if baseline.CurrentValue == currentValue {
		return nil
	}

	eventKey := "pricing:account:" + strconv.FormatInt(account.ID, 10) + ":" +
		hashParts(strconv.FormatInt(account.ID, 10), baseline.CurrentValue, currentValue)
	if service.Resolver != nil {
		if resolution, covered := service.Resolver.Resolve(ctx, account.Name); covered {
			if err := service.recordDecision(
				ctx, eventKey, "suppressed", "covered_by_production_pricing",
				account, baseline.CurrentValue, currentValue, resolution.PricingURL, observedAt,
			); err != nil {
				return err
			}
			return service.Baselines.PutOperationalBaseline(ctx, next)
		}
	}

	switch service.Policy.Mode {
	case notificationpolicy.ModeShadow:
		if err := service.recordDecision(
			ctx, eventKey, "shadow_would_deliver", eventKey,
			account, baseline.CurrentValue, currentValue, "", observedAt,
		); err != nil {
			return err
		}
		return service.Baselines.PutOperationalBaseline(ctx, next)
	case notificationpolicy.ModeEnabled:
		if service.Notifier == nil {
			return fmt.Errorf("pricing event notifier is required")
		}
	default:
		if err := service.recordDecision(
			ctx, eventKey, "suppressed", "delivery_mode_disabled",
			account, baseline.CurrentValue, currentValue, "", observedAt,
		); err != nil {
			return err
		}
		return service.Baselines.PutOperationalBaseline(ctx, next)
	}

	identity := notify.OneShotIdentity{
		Key: eventKey, Family: string(notificationpolicy.FamilyPricingNotice),
		PolicyVersion: service.Policy.Version, SourceKind: "account_multiplier",
	}
	message := notify.RenderPricingNotice(notify.PricingNoticeView{
		Upstream: accountDisplayName(account.Name),
		Change: "账号 " + accountDisplayName(account.Name) + " 的计费倍率由 " +
			multiplierDisplay(baseline.CurrentValue) + " 变为 " +
			multiplierDisplay(currentValue) + "。",
		Review:     "核对该账号对应的公开价格与当前毛利是否仍符合预期。",
		ObservedAt: observedAt,
	})
	if err := service.Notifier.SendOneShot(ctx, identity, message); err != nil {
		return err
	}
	if err := service.recordDecision(
		ctx, eventKey, "delivered", eventKey,
		account, baseline.CurrentValue, currentValue, "", observedAt,
	); err != nil {
		return err
	}
	return service.Baselines.PutOperationalBaseline(ctx, next)
}

func (service Service) recordDecision(
	ctx context.Context,
	key string,
	decision string,
	reason string,
	account sub2api.Account,
	before string,
	after string,
	pricingURL string,
	observedAt time.Time,
) error {
	details, _ := json.Marshal(struct {
		AccountID  int64  `json:"account_id"`
		Account    string `json:"account"`
		Before     string `json:"before"`
		After      string `json:"after"`
		PricingURL string `json:"pricing_url,omitempty"`
	}{
		AccountID: account.ID, Account: accountDisplayName(account.Name),
		Before: before, After: after, PricingURL: pricingURL,
	})
	return service.Decisions.RecordNotificationDecision(ctx, store.DecisionRecord{
		DecisionKey: key, Family: string(notificationpolicy.FamilyPricingNotice),
		PolicyVersion: service.Policy.Version, SourceKind: "account_multiplier",
		Decision: decision, Reason: reason, Details: details, ObservedAt: observedAt,
	})
}

func trustworthyMultiplier(value sub2api.AccountMonitorMultiplier) (float64, bool) {
	if value.Status != "ok" || value.Value == nil || *value.Value <= 0 ||
		math.IsNaN(*value.Value) || math.IsInf(*value.Value, 0) {
		return 0, false
	}
	return *value.Value, true
}

func multiplierValue(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func multiplierDisplay(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "x"))
	if value == "" {
		return "未记录"
	}
	return value + "x"
}

func accountDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") {
		return "生产账号"
	}
	return value
}

func hashParts(values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(sum[:])
}
