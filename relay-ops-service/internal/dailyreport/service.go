package dailyreport

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/cachepolicy"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type DailyNotificationSummary = store.DailyNotificationSummary

type DigestSummaryReader interface {
	ReadDailyNotificationSummary(
		context.Context,
		time.Time,
		time.Time,
	) (DailyNotificationSummary, error)
}

type ReconciliationSummaryReader interface {
	ReadReconciliationSummary(context.Context, int64, time.Time, time.Time, string) (reconciliation.Summary, error)
}

type EventSender interface {
	SendOneShot(context.Context, notify.OneShotIdentity, notify.FeishuMessage) error
}

type DecisionRecorder interface {
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type Result struct {
	ReportDate   string `json:"report_date"`
	Groups       int    `json:"groups"`
	Notification string `json:"notification"`
}

type Service struct {
	Reader    opsmetrics.Reader
	Summary   DigestSummaryReader
	Reconciliation ReconciliationSummaryReader
	Notifier  EventSender
	Decisions DecisionRecorder
	Policy    notificationpolicy.Policy
	Timezone  *time.Location
	Now       func() time.Time
	// Fallback resolves an account's multiplier from upstream public pricing
	// when Sub2API's own schema v2 multiplier is unusable. nil disables the
	// fallback; every failure must return nil, never a guessed value.
	Fallback func(context.Context, string) *float64
}

func (s Service) Run(ctx context.Context) (Result, error) {
	if s.Reader == nil {
		return Result{}, fmt.Errorf("daily report reader is required")
	}
	if s.Summary == nil {
		return Result{}, fmt.Errorf("daily notification summary reader is required")
	}
	location := s.Timezone
	if location == nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	date := now.In(location).Format("2006-01-02")
	localNow := now.In(location)
	summaryTo := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		0, 0, 0, 0, location,
	)
	summaryFrom := summaryTo.AddDate(0, 0, -1)
	runtime, err := opsmetrics.Collect(ctx, s.Reader, now)
	if err != nil {
		return Result{}, fmt.Errorf("collect site runtime: %w", err)
	}
	summary, err := s.Summary.ReadDailyNotificationSummary(
		ctx,
		summaryFrom.UTC(),
		summaryTo.UTC(),
	)
	if err != nil {
		return Result{}, fmt.Errorf("read daily notification summary: %w", err)
	}
	publicGroups := len(runtime.Groups)
	view := notify.HealthDigestView{
		Date: date, PublicGroups: publicGroups,
		Summary: digestSummary(summary),
		Quality: notify.QualityLine{
			DataUnavailable:       true,
			DataUnavailableReason: "账号监控数据不可用",
		},
	}
	if monitorReader, ok := s.Reader.(sub2api.AccountMonitorReader); ok {
		if projection, readErr := monitorReader.ListAccountMonitors(ctx); readErr == nil {
			limit := accounthealth.HistoryLimitFor(projection.Settings.IntervalSeconds)
			histories := make(map[int64][]sub2api.AccountMonitorHistoryEntry, len(projection.Accounts))
			for _, account := range projection.Accounts {
				entries, historyErr := monitorReader.ListAccountMonitorHistory(ctx, account.AccountID, limit)
				if historyErr != nil {
					// 单账号历史失败只影响该账号的同比，其余账号照常出数
					continue
				}
				histories[account.AccountID] = entries
			}
			var fallback func(string) *float64
			if s.Fallback != nil {
				fallback = func(name string) *float64 { return s.Fallback(ctx, name) }
			}
			view = BuildHealthDigestWithFallback(projection, histories, location, now, fallback)
		}
	}
	view.Date = date
	view.PublicGroups = publicGroups
	view.Summary = digestSummary(summary)
	if s.Reconciliation != nil {
		if ledger, ledgerErr := s.Reconciliation.ReadReconciliationSummary(ctx, 0, summaryFrom.UTC(), summaryTo.UTC(), "USD"); ledgerErr == nil {
			coverage, _ := ledger.CoverageRatio.Float64()
			upstream, _ := ledger.UpstreamCost.Float64()
			userCharge, _ := ledger.UserCharge.Float64()
			profit, _ := ledger.PaperProfit.Float64()
			view.Reconciliation = &notify.ReconciliationLine{CoverageRatio: coverage, PendingCount: ledger.PendingAttempts,
				UpstreamCost: upstream, UserCharge: userCharge, PaperProfit: profit, Currency: ledger.Currency}
		}
	}
	message := notify.RenderHealthDigest(view)
	result := Result{
		ReportDate: date, Groups: publicGroups, Notification: "not_configured",
	}
	key := "daily-digest:" + date
	details, _ := json.Marshal(struct {
		Date    string                   `json:"date"`
		Groups  int                      `json:"groups"`
		Summary DailyNotificationSummary `json:"summary"`
	}{Date: date, Groups: publicGroups, Summary: summary})
	if s.Policy.Version <= 0 {
		result.Notification = "suppressed"
		return result, nil
	}
	if s.Decisions == nil {
		return Result{}, fmt.Errorf("daily digest decision recorder is required")
	}
	if !s.Policy.Enabled(notificationpolicy.FamilyDailyDigest) {
		result.Notification = "suppressed"
		return result, s.recordDecision(
			ctx, key, "suppressed", "policy_disabled", details, now,
		)
	}
	switch s.Policy.Mode {
	case notificationpolicy.ModeShadow:
		result.Notification = "shadow"
		return result, s.recordDecision(
			ctx, key, "shadow_would_deliver", key, details, now,
		)
	case notificationpolicy.ModeEnabled:
	default:
		result.Notification = "suppressed"
		return result, s.recordDecision(
			ctx, key, "suppressed", "delivery_mode_disabled", details, now,
		)
	}
	if s.Notifier == nil {
		result.Notification = "not_configured"
		return result, s.recordDecision(
			ctx, key, "suppressed", "transport_unavailable", details, now,
		)
	}
	if err := s.Notifier.SendOneShot(ctx, notify.OneShotIdentity{
		Key: key, Family: string(notificationpolicy.FamilyDailyDigest),
		PolicyVersion: s.Policy.Version, SourceKind: "daily_report",
	}, message); err != nil {
		result.Notification = "failed"
		_ = s.recordDecision(ctx, key, "failed", "delivery_failed", details, now)
		return result, err
	}
	result.Notification = "delivered"
	return result, s.recordDecision(ctx, key, "delivered", key, details, now)
}

func (s Service) recordDecision(
	ctx context.Context,
	key string,
	decision string,
	reason string,
	details []byte,
	observedAt time.Time,
) error {
	return s.Decisions.RecordNotificationDecision(ctx, store.DecisionRecord{
		DecisionKey: key, Family: string(notificationpolicy.FamilyDailyDigest),
		PolicyVersion: s.Policy.Version, SourceKind: "daily_report",
		Decision: decision, Reason: reason, Details: details, ObservedAt: observedAt,
	})
}

func digestSummary(summary DailyNotificationSummary) notify.DigestNotificationSummary {
	return notify.DigestNotificationSummary{
		ActiveP0: summary.ActiveP0, ActiveP1: summary.ActiveP1,
		Recovered: summary.Recovered, PricingEvents: summary.PricingEvents,
		TrackedPublicGroups:   summary.PublicGroups,
		FreshCapacityGroups:   summary.FreshCapacityGroups,
		PricingSources:        summary.PricingSources,
		TrackedPricingSources: summary.TrackedPricingSources,
	}
}

func cacheReportLine(usage sub2api.UsageStats, ready bool, eligible, discounted int, blockers []string) string {
	summary := cachepolicy.Summarize(usage)
	parts := make([]string, 0, 2)
	if summary.Confirmed {
		parts = append(parts, fmt.Sprintf("缓存读取 %s，写入 %s，命中率 %.2f%%",
			formatTokens(summary.CacheReadTokens), formatTokens(summary.CacheCreationTokens), summary.HitRatePercent))
	} else {
		parts = append(parts, "缓存统计不可确认")
	}
	if ready {
		if eligible == 0 {
			parts = append(parts, "缓存优惠无适用模型")
		} else {
			parts = append(parts, fmt.Sprintf("缓存优惠 %d/%d 模型", discounted, eligible))
		}
	} else {
		codes := make([]string, 0, len(blockers))
		seen := make(map[string]struct{})
		for _, blocker := range blockers {
			code := strings.SplitN(blocker, ":", 2)[0]
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
		parts = append(parts, "缓存优惠未就绪（"+strings.Join(codes, "、")+"）")
	}
	return strings.Join(parts, "，")
}

func formatTokens(value int64) string {
	switch {
	case value >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(value)/1_000_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.2fK", float64(value)/1_000)
	default:
		return strconv.FormatInt(value, 10)
	}
}
