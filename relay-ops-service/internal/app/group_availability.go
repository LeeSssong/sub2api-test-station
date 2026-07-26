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
	loc *time.Location,
	now time.Time,
) error {
	projection, readErr := reader.ListAccountMonitors(ctx)
	if readErr != nil {
		// fail-safe：监控自身故障不得伪装成业务故障，静默跳过本轮
		log.Printf("group availability: skip round, monitor read failed: %v", readErr)
		return nil
	}
	if loc == nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
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
		// 投递证据必须带时间桶。notification_deliveries.dedup_key 有 UNIQUE
		// 约束且成功投递的记录永不复用，而可用比会重复出现（单账号分组每次
		// 故障都是 0/1）。不带时间桶的话，同一分组第二次故障会撞上第一次的
		// dedup_key 被静默丢弃——告警只会成功发出一次。
		//
		// 已知残留：桶内「故障→恢复→再故障」的第二次故障与其恢复卡仍会被
		// 投递层吞掉，静默窗口 <= 1 小时。根因是同一个 evidenceHash 被两个
		// 目的复用——状态机用它判断「取值是否变化」，投递层用它做「事件轮次
		// 幂等键」，二者语义冲突。彻底修法是为投递另行派生带事件轮次判别量
		// 的证据（需改 incidents 包暴露轮次），本次先用小时桶把窗口压到 1
		// 小时。不要把这个残留描述成「有意的防刷屏」——防重复由状态机的
		// 转移判定和 ConfirmationWindows 负责，时间桶抑制的是全新事件。
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%d:%s",
			alert.GroupName, alert.Available, alert.Total, now.In(loc).Format("2006-01-02T15"))))
		evidence := hex.EncodeToString(hash[:])
		// 健康分组同样要 Observe（Failing=false），否则状态机永远走不到恢复
		// 分支，分组恢复后 incident 卡在 confirmed。
		transition, observeErr := machine.Observe(ctx, incidents.Observation{
			Key:                 key,
			Severity:            "P1",
			Failing:             item.Alerting,
			EvidenceHash:        evidence,
			CurrentValue:        fmt.Sprintf("可用 %d / 共 %d", alert.Available, alert.Total),
			ConfirmationWindows: 2,
		})
		if observeErr != nil || !transition.Notify || sender == nil {
			continue
		}
		alert.Recovery = !item.Alerting
		// 单个分组投递失败不得挡住其余分组的告警
		if sendErr := sender.SendIncident(ctx, key, evidence, notify.RenderGroupAlert(alert)); sendErr != nil {
			failures = append(failures, sendErr)
		}
	}
	return errors.Join(failures...)
}
