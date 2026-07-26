package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type groupMonitorReader interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
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
	var failures []error
	for _, item := range dailyreport.BuildGroupAvailability(projection) {
		alert := item.Alert
		key := "group:" + alert.GroupName + ":availability"
		// 投递证据必须带日期。notification_deliveries.dedup_key 有 UNIQUE 约束
		// 且成功投递的记录永不复用，而可用比会重复出现（单账号分组每次故障
		// 都是 0/1）。不带日期的话，同一分组第二次故障会撞上第一次的
		// dedup_key 被静默丢弃——告警只会成功发出一次。dailyreport 的
		// summaryHash 用的就是这个做法。
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%d:%s",
			alert.GroupName, alert.Available, alert.Total, now.In(loc).Format("2006-01-02"))))
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
