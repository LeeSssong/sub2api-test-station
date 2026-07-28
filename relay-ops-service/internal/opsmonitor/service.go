// Package opsmonitor temporarily retains only the trustworthy account
// multiplier watcher. Public-group runtime and account-state incidents are
// owned by groupimpact; pricingevents replaces this watcher in the next step.
package opsmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type MultiplierSource interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
}

type Service struct {
	Reader      opsmetrics.Reader
	Multipliers MultiplierSource
	Incidents   *incidents.Machine
	Notifier    MessageSender
	Policy      notificationpolicy.Policy
	Now         func() time.Time
	Fallback    func(context.Context, string) *float64
}

func (service Service) Run(ctx context.Context) error {
	if service.Policy.Version <= 0 ||
		!service.Policy.Enabled(notificationpolicy.FamilyPricingNotice) {
		return nil
	}
	if service.Reader == nil {
		return fmt.Errorf("multiplier watcher reader is required")
	}
	if service.Incidents == nil || service.Incidents.Repository == nil {
		return fmt.Errorf("multiplier watcher incident machine is required")
	}
	now := time.Now().UTC()
	if service.Now != nil {
		now = service.Now().UTC()
	}
	accounts, err := service.Reader.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list accounts for multiplier watcher: %w", err)
	}
	active := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		if account.Status == "active" && account.Schedulable {
			active[account.ID] = account.Name
		}
	}
	return service.evaluateMultipliers(ctx, active, now)
}

func (service Service) evaluateMultipliers(
	ctx context.Context,
	active map[int64]string,
	now time.Time,
) error {
	if service.Multipliers == nil {
		return nil
	}
	projection, err := service.Multipliers.ListAccountMonitors(ctx)
	if err != nil {
		return nil
	}
	for _, account := range projection.Accounts {
		displayName, ok := active[account.AccountID]
		if !ok {
			continue
		}
		value := trustworthyMultiplier(account.Multiplier)
		if value == nil && service.Fallback != nil {
			fallback := service.Fallback(ctx, account.Name)
			if fallback != nil && *fallback > 0 {
				value = fallback
			}
		}
		if value == nil {
			continue
		}
		if err := service.evaluateMultiplier(
			ctx,
			account.AccountID,
			displayName,
			*value,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func trustworthyMultiplier(multiplier sub2api.AccountMonitorMultiplier) *float64 {
	if multiplier.Status != "ok" || multiplier.Value == nil || *multiplier.Value <= 0 {
		return nil
	}
	return multiplier.Value
}

func (service Service) evaluateMultiplier(
	ctx context.Context,
	accountID int64,
	accountName string,
	current float64,
	observedAt time.Time,
) error {
	baselineKey := multiplierKey(accountID, "multiplier_baseline")
	currentValue := fmt.Sprintf("%.6gx", current)
	record, found, err := service.Incidents.Repository.Get(ctx, baselineKey)
	if err != nil {
		return fmt.Errorf("read multiplier baseline: %w", err)
	}
	if !found {
		return service.Incidents.Repository.Put(ctx, incidents.Record{
			Key: baselineKey, Severity: "P2", State: "muted", SampleCount: 1,
			CurrentValue: currentValue,
			EvidenceHash: multiplierEvidence(accountID, currentValue, ""),
		})
	}
	if record.CurrentValue == currentValue {
		return nil
	}
	key := multiplierKey(accountID, "multiplier")
	evidence := multiplierEvidence(accountID, currentValue, record.CurrentValue)
	transition, err := service.Incidents.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P2", Failing: true,
		EvidenceHash: evidence, CurrentValue: currentValue, ConfirmationWindows: 1,
	})
	if err != nil {
		return err
	}
	if transition.Notify && service.Notifier != nil &&
		service.Policy.ShouldDeliver(notificationpolicy.FamilyPricingNotice) {
		message := notify.WithDeliveryIdentity(notify.RenderFeishu(notify.IncidentView{
			Title:    "价格变更｜" + multiplierAccountName(accountName) + " 计费倍率发生变化",
			Severity: "P2", CurrentLabel: "当前倍率", BaselineLabel: "上次记录",
			Current: currentValue, Baseline: record.CurrentValue,
			Change:  "计费倍率已更新",
			Focus:   "只读核对公开定价和毛利；系统未修改价格、倍率或路由。",
			Results: []string{"证据时间：" + observedAt.Format(time.RFC3339)},
		}), transition.OccurrenceNo, transition.Kind)
		if err := service.Notifier.SendIncident(ctx, key, evidence, message); err != nil {
			return err
		}
	}
	record.CurrentValue = currentValue
	record.SampleCount++
	record.EvidenceHash = multiplierEvidence(accountID, currentValue, "")
	return service.Incidents.Repository.Put(ctx, record)
}

func multiplierKey(accountID int64, suffix string) string {
	return "site:account:" + strconv.FormatInt(accountID, 10) + ":" + suffix
}

func multiplierEvidence(accountID int64, current string, baseline string) string {
	sum := sha256.Sum256([]byte(
		strconv.FormatInt(accountID, 10) + "\x00" + current + "\x00" + baseline,
	))
	return hex.EncodeToString(sum[:])
}

func multiplierAccountName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") {
		return "生产账号"
	}
	return value
}
