package dailyreport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/accountrecommendation"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

// windowSample builds the classifier input for one account from an aggregated
// time window. Two fallbacks keep accounts without window data judged instead
// of silently dropped:
//
//   - ErrorCode: an empty (or all-success) window carries no error evidence,
//     so the projection's live error code fills in — a freshly exhausted
//     account must classify Unavailable even before a probe lands in the
//     window.
//   - Rates: with zero window samples the projection's cumulative figures are
//     used, so the account is judged exactly as before this window existed.
//     Judging it Unknown instead would remove it from group capacity (Total),
//     silently loosening the alert threshold, and would drop it from the
//     daily three-tier counts.
func windowSample(account sub2api.AccountMonitorAccount, slice accounthealth.DaySlice) accounthealth.AccountSample {
	sample := accounthealth.AccountSampleFrom(slice, account.AccountID, account.Name, account.GroupNames)
	if sample.ErrorCode == "" {
		sample.ErrorCode = account.ErrorCode
	}
	if slice.SampleCount == 0 {
		sample.SuccessRate = account.SuccessRate
		sample.SampleCount = account.SampleCount
		sample.TTFTP95MS = account.TTFTP95MS
	}
	return sample
}

// trustworthyMultiplier returns the schema v2 multiplier only when it is
// usable. The deprecated accounts.rate_multiplier must never be substituted.
//
// A non-positive multiplier is treated as unusable even when the upstream
// reports status=ok: Sub2API only rejects value < 0, so a zero can reach us,
// and a zero multiplier would report 100% margin on a real cost. Production
// multipliers sit in 0.05x-0.25x, so non-positive means bad data.
func trustworthyMultiplier(account sub2api.AccountMonitorAccount) *float64 {
	if account.Multiplier.Status != "ok" || account.Multiplier.Value == nil {
		return nil
	}
	if *account.Multiplier.Value <= 0 {
		return nil
	}
	return account.Multiplier.Value
}

// unsupportedMeasurement reports whether the upstream simply cannot be measured:
// it declared no billing, the probe fell back to quota measurement, and that
// failed. shuaiapi settles token usage asynchronously, so the measurement reads
// an unchanged counter every time. Nothing on our side can fix it while the
// official Sub2API image owns the measurement, so it must not sit in the daily
// action list forever.
func unsupportedMeasurement(m sub2api.AccountMonitorMultiplier) bool {
	return m.Source == "measured" && m.Status == "failed"
}

// resolvedMultiplier is the multiplier chosen for profit accounting plus its
// provenance, so the detail line can label upstream-pricing values instead of
// passing them off as Sub2API measurements.
type resolvedMultiplier struct {
	value        *float64
	fromUpstream bool
}

// resolveMultiplier centralizes the priority chain declared > measured >
// upstream_pricing. Sub2API itself already prefers declared over measured in
// the schema v2 projection, so any trustworthy value from it wins; the
// upstream-pricing fallback is consulted only when Sub2API has nothing usable
// and must never override a trustworthy value. A non-positive fallback value
// is rejected for the same reason as in trustworthyMultiplier.
func resolveMultiplier(account sub2api.AccountMonitorAccount, fallback func(string) *float64) resolvedMultiplier {
	if value := trustworthyMultiplier(account); value != nil {
		return resolvedMultiplier{value: value}
	}
	if fallback == nil {
		return resolvedMultiplier{}
	}
	value := fallback(account.Name)
	if value == nil || *value <= 0 {
		return resolvedMultiplier{}
	}
	return resolvedMultiplier{value: value, fromUpstream: true}
}

func BuildHealthDigest(
	projection sub2api.AccountMonitorProjection,
	histories map[int64][]sub2api.AccountMonitorHistoryEntry,
	loc *time.Location,
	now time.Time,
) notify.HealthDigestView {
	return BuildHealthDigestWithFallback(projection, histories, loc, now, nil)
}

// BuildHealthDigestWithFallback additionally consults fallback (an
// upstream-pricing lookup keyed by account name) for accounts whose Sub2API
// multiplier is unusable. A nil fallback reproduces BuildHealthDigest exactly.
func BuildHealthDigestWithFallback(
	projection sub2api.AccountMonitorProjection,
	histories map[int64][]sub2api.AccountMonitorHistoryEntry,
	loc *time.Location,
	now time.Time,
	fallback func(string) *float64,
) notify.HealthDigestView {
	view := notify.HealthDigestView{Date: now.In(loc).Format("2006-01-02")}
	totalAccounts := len(projection.Accounts)
	pricedAccounts := 0
	upstreamPricedAccounts := 0
	profitInputs := make([]accounthealth.ProfitInput, 0, len(projection.Accounts))
	ttfts := make([]float64, 0, len(projection.Accounts))
	comparableAccounts, todayHealthyComparable, yesterdayHealthy := 0, 0, 0
	hasTraffic := false
	totalRequests := int64(0)
	unsupported := 0

	for _, account := range projection.Accounts {
		// 判定必须走当日切片而不是 projection 的 7 天累计口径：账号上午硬挂
		// 后，7 天成功率一整天都还停在 90%+，三档计数、待处理与同比会集体
		// 把「今天刚挂」报成「基本健康」。
		todaySlice, yesterdaySlice := accounthealth.SliceByDay(toHistoryEntries(histories[account.AccountID]), loc, now)
		sample := windowSample(account, todaySlice)
		verdict := accounthealth.ClassifyAccount(sample)
		switch verdict.Tier {
		case accounthealth.TierHealthy:
			view.Quality.Healthy++
		case accounthealth.TierDegraded:
			view.Quality.Degraded++
		case accounthealth.TierUnavailable:
			view.Quality.Unavailable++
		}
		if verdict.Slow {
			view.Quality.Slow++
		}

		multiplier := resolveMultiplier(account, fallback)
		if multiplier.value != nil {
			pricedAccounts++
		}
		if multiplier.fromUpstream {
			upstreamPricedAccounts++
		}
		// 兜底成功的账号利润已经算得出来，不再是「上游不支持自动测算」。
		if unsupportedMeasurement(account.Multiplier) && multiplier.value == nil {
			unsupported++
		}
		standardCost, userCost := 0.0, 0.0
		if account.TodayStats != nil {
			standardCost, userCost = account.TodayStats.StandardCost, account.TodayStats.UserCost
			totalRequests += account.TodayStats.Requests
		}
		input := accounthealth.ProfitInput{StandardCost: standardCost, UserCost: userCost, Multiplier: multiplier.value}
		profitInputs = append(profitInputs, input)
		if standardCost > 0 || userCost > 0 {
			hasTraffic = true
		}

		if sample.TTFTP95MS != nil {
			ttfts = append(ttfts, *sample.TTFTP95MS)
		}

		if item, ok := pendingFor(account, sample, verdict, multiplier); ok {
			view.Pending = append(view.Pending, item)
		}

		// 同比两侧必须走同一套判定规则（ClassifyAccount）：今天用当日切片、
		// 昨天用昨日切片。此前昨天只比裸成功率、今天走完整 tier 规则，喂入
		// 完全相同的两日历史也会算出非零 delta。
		if todaySlice.SampleCount > 0 && yesterdaySlice.SampleCount > 0 {
			comparableAccounts++
			yesterdayVerdict := accounthealth.ClassifyAccount(accounthealth.AccountSampleFrom(
				yesterdaySlice, account.AccountID, account.Name, account.GroupNames))
			if yesterdayVerdict.Tier == accounthealth.TierHealthy {
				yesterdayHealthy++
			}
			if verdict.Tier == accounthealth.TierHealthy {
				todayHealthyComparable++
			}
		}
	}

	view.Recommendations = buildRecommendations(projection)

	// 同比只能在「今天和昨天都有样本」的账号子集上计算。若拿全部账号的今日
	// 健康数去减「仅有历史记录的账号」的昨日健康数，两个总体不一致，delta 会
	// 系统性偏高；历史为空时更会凭空得出「较昨日 ↑N」。宁可不给同比。
	if comparableAccounts > 0 {
		delta := todayHealthyComparable - yesterdayHealthy
		view.Quality.HealthyDelta = &delta
	}
	view.Quality.TTFTP95MedianMS = accounthealth.Percentile(ttfts, 0.5)

	total, excluded := accounthealth.SumProfit(profitInputs)
	computable := total.Computable
	if hasTraffic && total.Revenue == 0 && total.UpstreamCost == 0 {
		// Every account that carried traffic today was excluded for an unusable
		// multiplier; the only computable inputs were zero-traffic. Reporting
		// "revenue 0, cost 0" as computable would fabricate a 100%-margin day,
		// so the profit line degrades to not-computable instead.
		computable = false
	}
	view.Profit = notify.ProfitLine{
		Revenue: total.Revenue, UpstreamCost: total.UpstreamCost, Gross: total.Gross,
		Margin: total.Margin, Computable: computable, ExcludedAccounts: excluded,
		UnsupportedAccounts:    unsupported,
		TotalAccounts:          totalAccounts,
		PricedAccounts:         pricedAccounts,
		UpstreamPricedAccounts: upstreamPricedAccounts,
		NoTraffic:              !hasTraffic,
	}
	view.Traffic = notify.TrafficLine{HasTraffic: hasTraffic, Requests: totalRequests}
	return view
}

// buildRecommendations surfaces only actionable switches: a group is listed
// solely when the analyzer concluded the candidate is better and named it.
func buildRecommendations(projection sub2api.AccountMonitorProjection) []notify.RecommendationLine {
	recommendations := []notify.RecommendationLine{}
	for _, group := range accountrecommendation.Analyze(projection).Groups {
		if group.Decision != "candidate_better" || group.CandidateAccountID == 0 {
			continue
		}
		if group.Current.Name == "" || group.Candidate.Name == "" {
			continue
		}
		recommendations = append(recommendations, notify.RecommendationLine{
			GroupName:     group.GroupName,
			CurrentName:   group.Current.Name,
			CandidateName: group.Candidate.Name,
			Reason:        strings.Join(group.Reasons, "、"),
		})
	}
	return recommendations
}

// GroupAvailabilityView pairs a renderable alert with the alerting flag.
//
// Every group must be reported, not just the alerting ones: the incident state
// machine only emits a recovery when it observes Failing=false. Returning just
// the alerting groups would leave a recovered group stuck in `confirmed`
// forever, so its next real outage carries an unchanged evidence hash and is
// silently suppressed — the alert would fire exactly once, ever.
type GroupAvailabilityView struct {
	Alert    notify.GroupAlertView
	Alerting bool
}

// BuildGroupAvailability judges every account over the trailing one-hour
// window [now-1h, now) so a hard failure surfaces within the confirmation
// windows of the 5-minute job, not after the 7-day cumulative rate finally
// decays below the tier thresholds (a matter of days). Accounts without any
// window history fall back to the projection's cumulative figures (see
// windowSample) so a single history-read failure degrades to the old caliber
// instead of shrinking group capacity.
func BuildGroupAvailability(
	projection sub2api.AccountMonitorProjection,
	histories map[int64][]sub2api.AccountMonitorHistoryEntry,
	now time.Time,
) []GroupAvailabilityView {
	verdicts := make([]accounthealth.AccountVerdict, 0, len(projection.Accounts))
	problems := make(map[int64]string, len(projection.Accounts))
	from := now.Add(-time.Hour)
	for _, account := range projection.Accounts {
		slice := accounthealth.Aggregate(toHistoryEntries(histories[account.AccountID]), from, now)
		sample := windowSample(account, slice)
		verdicts = append(verdicts, accounthealth.ClassifyAccount(sample))
		// 与日报待处理层统一走 problemLabel：同一个运维群不该一边收到
		// 「余额耗尽」、一边收到裸的 balance_exhausted。
		problems[account.AccountID] = problemLabel(sample.ErrorCode)
	}
	views := []GroupAvailabilityView{}
	seen := map[string]bool{}
	for _, group := range accounthealth.GroupAvailabilities(verdicts) {
		alert := notify.GroupAlertView{GroupName: group.GroupName, Available: group.Available, Total: group.Total}
		for _, down := range group.Down {
			alert.Down = append(alert.Down, notify.GroupAlertAccount{
				Name:      down.Name,
				ErrorCode: problems[down.AccountID],
			})
		}
		seen[group.GroupName] = true
		views = append(views, GroupAvailabilityView{Alert: alert, Alerting: group.Alerting})
	}
	// GroupAvailabilities 会跳过 TierUnknown 账号，因此某个分组的账号全部失去
	// 样本时，该分组会整个从结果里消失，于是也不再被 Observe，incident 卡在
	// confirmed —— 与「告警只发一次」同源。这里把缺席的分组补回来，以
	// Alerting=false 观测，让状态机能够走完恢复路径。
	for _, name := range groupNamesIn(projection) {
		if !seen[name] {
			views = append(views, GroupAvailabilityView{
				Alert: notify.GroupAlertView{GroupName: name},
			})
		}
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Alert.GroupName < views[j].Alert.GroupName })
	return views
}

func groupNamesIn(projection sub2api.AccountMonitorProjection) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, account := range projection.Accounts {
		for _, name := range account.GroupNames {
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func pendingFor(
	account sub2api.AccountMonitorAccount,
	sample accounthealth.AccountSample,
	verdict accounthealth.AccountVerdict,
	resolved resolvedMultiplier,
) (notify.PendingItem, bool) {
	switch {
	case verdict.Tier == accounthealth.TierUnavailable:
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     problemLabel(sample.ErrorCode),
			Severity:    notify.PendingCritical,
		}, true
	case verdict.Tier == accounthealth.TierDegraded:
		detail := ""
		if sample.ErrorCode != "" {
			detail = problemLabel(sample.ErrorCode)
		}
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     fmt.Sprintf("成功率 %.0f%% ↓", sample.SuccessRate*100),
			Detail:      detail,
			Severity:    notify.PendingWarning,
		}, true
	case resolved.value == nil:
		if unsupportedMeasurement(account.Multiplier) {
			return notify.PendingItem{}, false
		}
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     "倍率不可用",
			Detail:      "利润未核算",
			Severity:    notify.PendingAccounting,
		}, true
	}
	return notify.PendingItem{}, false
}

func problemLabel(errorCode string) string {
	switch errorCode {
	case accounthealth.ErrorCodeBalanceExhausted:
		return "余额耗尽"
	case "http_error":
		return "HTTP 错误"
	case "timeout":
		return "请求超时"
	case "malformed_stream":
		return "响应格式异常"
	case "model_unavailable":
		return "模型不可用"
	case "account_test_error":
		return "账号测试失败"
	case "":
		return "不可用"
	default:
		return "账号异常"
	}
}

func toHistoryEntries(entries []sub2api.AccountMonitorHistoryEntry) []accounthealth.HistoryEntry {
	converted := make([]accounthealth.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		converted = append(converted, accounthealth.HistoryEntry{
			CheckedAt: entry.CheckedAt, Status: entry.Status,
			ErrorCode: entry.ErrorCode, TTFTMS: entry.TTFTMS,
		})
	}
	return converted
}
