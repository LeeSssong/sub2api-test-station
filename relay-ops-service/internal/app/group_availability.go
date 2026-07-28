package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type groupMonitorReader interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
	ListAccountMonitorHistory(context.Context, int64, int) ([]sub2api.AccountMonitorHistoryEntry, error)
}

type groupIncidentObserver interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type groupAlertSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

// runGroupAvailability is the body of the group-availability job. It lives as
// a named function (not an anonymous closure in New) so tests can reach the
// alerting logic directly.
func runGroupAvailability(
	ctx context.Context,
	reader groupMonitorReader,
	machine groupIncidentObserver,
	sender groupAlertSender,
	_ *time.Location,
	now time.Time,
) error {
	projection, readErr := reader.ListAccountMonitors(ctx)
	if readErr != nil {
		// fail-safe：监控自身故障不得伪装成业务故障，静默跳过本轮
		log.Printf("group availability: skip round, monitor read failed: %v", readErr)
		return nil
	}
	// 判定口径是最近 1 小时滚动窗，只拉窗口所需的少量 history（300 秒间隔
	// 是 18 条），不是 48 小时口径的 HistoryLimitFor（692 条，5 分钟一轮会
	// 白拉 40 倍数据）。
	limit := accounthealth.RollingWindowLimitFor(projection.Settings.IntervalSeconds)
	histories := make(map[int64][]sub2api.AccountMonitorHistoryEntry, len(projection.Accounts))
	for _, account := range projection.Accounts {
		entries, historyErr := reader.ListAccountMonitorHistory(ctx, account.AccountID, limit)
		if historyErr != nil {
			// 单账号历史失败退回投影口径（BuildGroupAvailability 的空窗回退），
			// 不得挡住其余账号的判定
			log.Printf("group availability: history read failed for account %d: %v", account.AccountID, historyErr)
			continue
		}
		histories[account.AccountID] = entries
	}
	var failures []error
	for _, item := range dailyreport.BuildGroupAvailability(projection, histories, now) {
		alert := item.Alert
		key := "group:" + alert.GroupName + ":availability"
		// State evidence describes only the observed capacity. Event occurrence
		// is a separate identity maintained by incidents.Machine, so an identical
		// later outage is deliverable without manufacturing hourly new evidence.
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%d",
			alert.GroupName, alert.Available, alert.Total)))
		evidence := hex.EncodeToString(hash[:])
		severity := "P1"
		confirmationWindows := 2
		if alert.Total > 0 && alert.Available == 0 {
			severity = "P0"
			confirmationWindows = 1
		}
		alert.Severity = severity
		// 健康分组同样要 Observe（Failing=false），否则状态机永远走不到恢复
		// 分支，分组恢复后 incident 卡在 confirmed。
		transition, observeErr := machine.Observe(ctx, incidents.Observation{
			Key:                 key,
			Severity:            severity,
			Failing:             item.Alerting,
			EvidenceHash:        evidence,
			CurrentValue:        fmt.Sprintf("可用 %d / 共 %d", alert.Available, alert.Total),
			ConfirmationWindows: confirmationWindows,
		})
		if observeErr != nil || !transition.Notify || sender == nil {
			continue
		}
		alert.Recovery = !item.Alerting
		message := notify.WithDeliveryIdentity(
			notify.RenderGroupAlert(alert), transition.OccurrenceNo, transition.Kind,
		)
		// 单个分组投递失败不得挡住其余分组的告警
		if sendErr := sender.SendIncident(ctx, key, evidence, message); sendErr != nil {
			failures = append(failures, sendErr)
		}
	}
	return errors.Join(failures...)
}
