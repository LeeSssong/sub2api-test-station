package dailyreport

import (
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/accountrecommendation"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func classify(projection sub2api.AccountMonitorProjection) []accounthealth.AccountVerdict {
	verdicts := make([]accounthealth.AccountVerdict, 0, len(projection.Accounts))
	for _, account := range projection.Accounts {
		verdicts = append(verdicts, accounthealth.ClassifyAccount(accounthealth.AccountSample{
			AccountID:   account.AccountID,
			Name:        account.Name,
			GroupNames:  account.GroupNames,
			SuccessRate: account.SuccessRate,
			SampleCount: account.SampleCount,
			TTFTP95MS:   account.TTFTP95MS,
			ErrorCode:   account.ErrorCode,
		}))
	}
	return verdicts
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

func BuildHealthDigest(
	projection sub2api.AccountMonitorProjection,
	histories map[int64][]sub2api.AccountMonitorHistoryEntry,
	loc *time.Location,
	now time.Time,
) notify.HealthDigestView {
	verdicts := classify(projection)

	view := notify.HealthDigestView{Date: now.In(loc).Format("2006-01-02")}
	profitInputs := make([]accounthealth.ProfitInput, 0, len(projection.Accounts))
	todayHealthy, yesterdayHealthy := 0, 0
	hasTraffic := false

	for index, account := range projection.Accounts {
		verdict := verdicts[index]
		switch verdict.Tier {
		case accounthealth.TierHealthy:
			view.Quality.Healthy++
			todayHealthy++
		case accounthealth.TierDegraded:
			view.Quality.Degraded++
		case accounthealth.TierUnavailable:
			view.Quality.Unavailable++
		}
		if verdict.Slow {
			view.Quality.Slow++
		}

		multiplier := trustworthyMultiplier(account)
		standardCost, userCost := 0.0, 0.0
		if account.TodayStats != nil {
			standardCost, userCost = account.TodayStats.StandardCost, account.TodayStats.UserCost
		}
		input := accounthealth.ProfitInput{StandardCost: standardCost, UserCost: userCost, Multiplier: multiplier}
		profitInputs = append(profitInputs, input)
		if standardCost > 0 || userCost > 0 {
			hasTraffic = true
		}

		view.Accounts = append(view.Accounts, notify.AccountDetailLine{
			Name:              account.Name,
			SuccessRate:       fmt.Sprintf("%.1f%%", account.SuccessRate*100),
			TTFTP50:           millis(account.TTFTP50MS),
			LatencyP95:        millis(account.LatencyP95MS),
			Multiplier:        multiplierLabel(account.Multiplier),
			GrossContribution: grossLabel(accounthealth.ComputeProfit(input)),
		})

		if item, ok := pendingFor(account, verdict); ok {
			view.Pending = append(view.Pending, item)
		}

		if entries := histories[account.AccountID]; len(entries) > 0 {
			_, yesterday := accounthealth.SliceByDay(toHistoryEntries(entries), loc, now)
			if yesterday.SampleCount > 0 && yesterday.SuccessRate >= accounthealth.HealthyMinSuccessRate {
				yesterdayHealthy++
			}
		}
	}

	view.Recommendations = buildRecommendations(projection)

	if len(histories) > 0 {
		delta := todayHealthy - yesterdayHealthy
		view.Quality.HealthyDelta = &delta
	}
	view.Quality.TTFTMedianMS = medianTTFT(projection.Accounts)

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
		NoTraffic: !hasTraffic,
	}
	view.Traffic = notify.TrafficLine{HasTraffic: hasTraffic}
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

func BuildGroupAlerts(projection sub2api.AccountMonitorProjection, loc *time.Location, now time.Time) []notify.GroupAlertView {
	alerts := []notify.GroupAlertView{}
	for _, group := range accounthealth.GroupAvailabilities(classify(projection)) {
		if !group.Alerting {
			continue
		}
		alert := notify.GroupAlertView{GroupName: group.GroupName, Available: group.Available, Total: group.Total}
		for _, down := range group.Down {
			alert.Down = append(alert.Down, notify.GroupAlertAccount{
				Name:      down.Name,
				ErrorCode: errorCodeOf(projection, down.AccountID),
				Duration:  "",
			})
		}
		alerts = append(alerts, alert)
	}
	return alerts
}

func errorCodeOf(projection sub2api.AccountMonitorProjection, accountID int64) string {
	for _, account := range projection.Accounts {
		if account.AccountID == accountID {
			return account.ErrorCode
		}
	}
	return ""
}

func pendingFor(account sub2api.AccountMonitorAccount, verdict accounthealth.AccountVerdict) (notify.PendingItem, bool) {
	switch {
	case verdict.Tier == accounthealth.TierUnavailable:
		return notify.PendingItem{AccountName: account.Name, Problem: problemLabel(account.ErrorCode), Detail: ""}, true
	case verdict.Tier == accounthealth.TierDegraded:
		return notify.PendingItem{
			AccountName: account.Name,
			Problem:     fmt.Sprintf("成功率 %.0f%% ↓", account.SuccessRate*100),
			Detail:      account.ErrorCode,
		}, true
	case trustworthyMultiplier(account) == nil:
		return notify.PendingItem{AccountName: account.Name, Problem: "倍率测不出", Detail: "利润无法核算"}, true
	}
	return notify.PendingItem{}, false
}

func problemLabel(errorCode string) string {
	if errorCode == accounthealth.ErrorCodeBalanceExhausted {
		return "余额耗尽"
	}
	if errorCode == "" {
		return "不可用"
	}
	return errorCode
}

func multiplierLabel(multiplier sub2api.AccountMonitorMultiplier) string {
	if multiplier.Status != "ok" || multiplier.Value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2fx", *multiplier.Value)
}

func grossLabel(profit accounthealth.Profit) string {
	if !profit.Computable {
		return "—"
	}
	return fmt.Sprintf("$%.2f", profit.Gross)
}

func millis(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.0fms", *value)
}

func medianTTFT(accounts []sub2api.AccountMonitorAccount) *float64 {
	values := make([]float64, 0, len(accounts))
	for _, account := range accounts {
		if account.TTFTP95MS != nil {
			values = append(values, *account.TTFTP95MS)
		}
	}
	if len(values) == 0 {
		return nil
	}
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
	median := values[len(values)/2]
	return &median
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
