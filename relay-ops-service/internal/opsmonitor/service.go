// Package opsmonitor evaluates native site-runtime and account-quality evidence.
package opsmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/sub2api"
)

const (
	minimumRequests      = 20
	errorRateThreshold   = 0.05
	ttftAbsoluteMS       = 3000
	ttftDegradationRatio = 1.30
	qualityStaleAfter    = 20 * time.Minute
)

type QualitySource interface {
	Read(time.Time) (accountquality.Result, error)
}

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

// MultiplierSource provides the schema v2 account-monitors projection that
// carries the real, changing multiplier. It is deliberately a narrow interface
// instead of a widened opsmetrics.Reader so other Reader implementers are not
// affected; *sub2api.HTTPReader satisfies both.
type MultiplierSource interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
}

type Service struct {
	Reader      opsmetrics.Reader
	Quality     QualitySource
	Multipliers MultiplierSource
	Incidents   *incidents.Machine
	Notifier    MessageSender
	Now         func() time.Time
}

func (s Service) Run(ctx context.Context) error {
	if s.Reader == nil {
		return fmt.Errorf("site monitor reader is required")
	}
	if s.Incidents == nil || s.Incidents.Repository == nil {
		return fmt.Errorf("site monitor incident machine is required")
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	groups, err := s.Reader.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("list public groups: %w", err)
	}
	accounts, err := s.Reader.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	sortAccounts(accounts)

	for _, group := range groups {
		if !group.CustomerVisible() {
			continue
		}
		window, baseline, available := s.snapshots(ctx, sub2api.OpsQuery{GroupID: group.ID})
		if !available {
			continue
		}
		if err := s.evaluateRuntime(ctx, object{kind: "group", id: group.ID}, window, baseline, now); err != nil {
			return err
		}
	}

	active := make(map[int64]struct{}, len(accounts))
	activeIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account.Status != "active" {
			continue
		}
		if err := s.observe(ctx, object{kind: "account", id: account.ID}, "paused", !account.Schedulable, "active && schedulable", schedulingValue(account), 1, now); err != nil {
			return err
		}
		if !account.Schedulable {
			continue
		}
		active[account.ID] = struct{}{}
		activeIDs = append(activeIDs, account.ID)
		window, baseline, available := s.snapshots(ctx, sub2api.OpsQuery{AccountID: account.ID})
		if !available {
			continue
		}
		if err := s.evaluateRuntime(ctx, object{kind: "account", id: account.ID}, window, baseline, now); err != nil {
			return err
		}
	}

	if err := s.evaluateMultipliers(ctx, active, now); err != nil {
		return err
	}

	if s.Quality == nil {
		return nil
	}
	quality, err := s.Quality.Read(now)
	if err != nil {
		return nil // Missing or stale quality evidence is surfaced in /ops, not guessed as an incident.
	}
	if quality.ObservedAt.IsZero() || now.Sub(quality.ObservedAt) > qualityStaleAfter {
		return nil
	}
	accountSetSHA256, hashErr := accountquality.CanonicalAccountSetSHA256(activeIDs)
	if hashErr != nil || quality.AccountSetSHA256 != accountSetSHA256 {
		return nil
	}
	for _, account := range quality.Accounts {
		if _, ok := active[account.AccountID]; !ok {
			continue
		}
		object := object{kind: "account", id: account.AccountID}
		if account.LastResult != "passed" && account.LastResult != "balance_exhausted" {
			// A failed or indeterminate patrol is not evidence that an account
			// recovered from an explicit balance exhaustion incident.
			continue
		}
		if err := s.observe(ctx, object, "balance_exhausted", account.LastResult == "balance_exhausted", "not_balance_exhausted", account.LastResult, 1, account.LastObservedAt); err != nil {
			return err
		}
	}
	return nil
}

type object struct {
	kind string
	id   int64
}

func (o object) key(metric string) string {
	return "site:" + o.kind + ":" + strconv.FormatInt(o.id, 10) + ":" + metric
}

func (s Service) snapshots(ctx context.Context, query sub2api.OpsQuery) (sub2api.OpsOverview, sub2api.OpsOverview, bool) {
	query.TimeRange = "15m"
	window, err := s.Reader.GetOpsSnapshot(ctx, query)
	if err != nil {
		return sub2api.OpsOverview{}, sub2api.OpsOverview{}, false
	}
	query.TimeRange = "24h"
	baseline, err := s.Reader.GetOpsSnapshot(ctx, query)
	if err != nil {
		return sub2api.OpsOverview{}, sub2api.OpsOverview{}, false
	}
	return window.Overview, baseline.Overview, true
}

func (s Service) evaluateRuntime(ctx context.Context, item object, window, baseline sub2api.OpsOverview, observedAt time.Time) error {
	completeFailure := window.RequestCountTotal > 0 && window.SuccessCount == 0
	if window.RequestCountTotal < minimumRequests && !completeFailure {
		return nil
	}
	if err := s.observe(ctx, item, "availability", completeFailure, "successful requests", fmt.Sprintf("%d/%d successful", window.SuccessCount, window.RequestCountTotal), 1, observedAt); err != nil {
		return err
	}
	if err := s.observe(ctx, item, "error_rate", window.ErrorRate >= errorRateThreshold, "<5.00%", percent(window.ErrorRate), 2, observedAt); err != nil {
		return err
	}
	ttftWorse := baseline.TTFT.P95MS > 0 && window.TTFT.P95MS > ttftAbsoluteMS && window.TTFT.P95MS >= baseline.TTFT.P95MS*ttftDegradationRatio
	return s.observe(ctx, item, "ttft_p95", ttftWorse, milliseconds(baseline.TTFT.P95MS), milliseconds(window.TTFT.P95MS), 2, observedAt)
}

// evaluateMultipliers watches the trustworthy schema v2 multiplier. The old
// implementation watched accounts.rate_multiplier, which is fixed at 1 in
// production — 22 baselines, zero changes in four days. An upstream price
// change never reached this tripwire.
func (s Service) evaluateMultipliers(ctx context.Context, active map[int64]struct{}, now time.Time) error {
	if s.Multipliers == nil {
		return nil
	}
	projection, err := s.Multipliers.ListAccountMonitors(ctx)
	if err != nil {
		// 读不到倍率证据不是变更事件，静默跳过本轮
		return nil
	}
	for _, account := range projection.Accounts {
		if _, ok := active[account.AccountID]; !ok {
			continue
		}
		value := trustworthyMultiplier(account.Multiplier)
		if value == nil {
			continue
		}
		item := object{kind: "account", id: account.AccountID}
		if err := s.evaluateMultiplier(ctx, item, *value, now); err != nil {
			return err
		}
	}
	return nil
}

// trustworthyMultiplier mirrors the daily report's rule: status must be ok,
// the value present and positive. Sub2API only rejects value < 0, so a zero
// can reach us, and a zero would look like a price change to zero.
func trustworthyMultiplier(m sub2api.AccountMonitorMultiplier) *float64 {
	if m.Status != "ok" || m.Value == nil || *m.Value <= 0 {
		return nil
	}
	return m.Value
}

func (s Service) evaluateMultiplier(ctx context.Context, item object, current float64, observedAt time.Time) error {
	baselineKey := item.key("multiplier_baseline")
	currentValue := multiplier(current)
	record, found, err := s.Incidents.Repository.Get(ctx, baselineKey)
	if err != nil {
		return fmt.Errorf("read multiplier baseline: %w", err)
	}
	if !found {
		return s.Incidents.Repository.Put(ctx, incidents.Record{Key: baselineKey, Severity: "P2", State: "muted", SampleCount: 1, CurrentValue: currentValue, EvidenceHash: evidenceHash(item, "multiplier_baseline", currentValue, "")})
	}
	changed := record.CurrentValue != currentValue
	if err := s.observe(ctx, item, "multiplier", changed, record.CurrentValue, currentValue, 1, observedAt); err != nil {
		return err
	}
	if record.CurrentValue == currentValue {
		return nil
	}
	record.CurrentValue = currentValue
	record.SampleCount++
	record.EvidenceHash = evidenceHash(item, "multiplier_baseline", currentValue, "")
	return s.Incidents.Repository.Put(ctx, record)
}

func (s Service) observe(ctx context.Context, item object, metric string, failing bool, baseline, current string, windows int, observedAt time.Time) error {
	key := item.key(metric)
	transition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: failing, EvidenceHash: evidenceHash(item, metric, current, baseline),
		CurrentValue: current, ConfirmationWindows: windows,
	})
	if err != nil {
		return err
	}
	if !transition.Notify || s.Notifier == nil {
		return nil
	}
	return s.Notifier.SendIncident(ctx, key, transition.Kind+":"+evidenceHash(item, metric, current, baseline), render(item, metric, current, baseline, windows, observedAt, transition.Kind))
}

func render(item object, metric, current, baseline string, windows int, observedAt time.Time, transition string) notify.FeishuMessage {
	label := item.kind + " #" + strconv.FormatInt(item.id, 10)
	isRecovery := transition == "recovered"
	domain, source, actions, focus := renderDomain(metric)
	title := domain + "告警：" + label
	if isRecovery {
		title = domain + "已恢复：" + label
	}
	results := []string{
		"数据来源：" + source,
		"对象：" + label,
		"指标：" + metric,
		"当前值：" + current,
		"基线：" + baseline,
		"证据时间：" + observedAt.UTC().Format(time.RFC3339),
	}
	if isRecovery {
		results = append(results, "恢复窗口：1 个完整健康窗口")
	} else {
		results = append(results, "连续窗口："+strconv.Itoa(windows))
	}
	return notify.RenderFeishu(notify.IncidentView{
		Title: title, Severity: "P1", Recovery: isRecovery, Current: current, Baseline: baseline,
		WhatWasDone: actions,
		Results:     results,
		Change:      "指标状态已更新", Focus: focus, Links: []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
}

func renderDomain(metric string) (string, string, []string, string) {
	if metric == "multiplier" || metric == "balance_exhausted" {
		return "上游账号质量", "账号质量定时巡检结果（不发起新探测）",
			[]string{"读取账号质量定时巡检结果", "按固定质量规则评估，不执行路由或账号写入"},
			"在运维后台查看上游账号质量证据"
	}
	return "站内运行", "Sub2API 原生站内运行快照（15 分钟与 24 小时）",
		[]string{"读取 Sub2API 原生站内运行快照", "按固定窗口规则评估，不执行路由或账号写入"},
		"在运维后台查看站内运行证据"
}

func schedulingValue(account sub2api.Account) string {
	if account.Schedulable {
		return "active && schedulable"
	}
	return "active but paused"
}

func percent(value float64) string      { return fmt.Sprintf("%.2f%%", value*100) }
func milliseconds(value float64) string { return fmt.Sprintf("%.0fms", value) }
func multiplier(value float64) string   { return fmt.Sprintf("%.6gx", value) }

func evidenceHash(item object, metric, current, baseline string) string {
	payload := item.kind + "|" + strconv.FormatInt(item.id, 10) + "|" + metric + "|" + current + "|" + baseline
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func sortAccounts(accounts []sub2api.Account) {
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
}
