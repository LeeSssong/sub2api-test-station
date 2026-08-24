package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	businessOverviewRecordRecharge = "recharge"
	businessOverviewRecordUsage    = "usage_consumption"

	BusinessOverviewRevenueConfirmed    = "confirmed"
	BusinessOverviewRevenuePendingSplit = "pending_split"
	BusinessOverviewRevenuePending      = "pending"
	BusinessOverviewRevenueUnavailable  = "unavailable"

	BusinessOverviewBalanceBalanced   = "balanced"
	BusinessOverviewBalanceUnbalanced = "unbalanced"
	BusinessOverviewBalancePending    = "pending"
)

var ErrBusinessOverviewInvalidRange = errors.New("business overview range must have end after start")

type BusinessOverviewRange string

const (
	BusinessOverviewRangeToday         BusinessOverviewRange = "today"
	BusinessOverviewRange7D            BusinessOverviewRange = "7d"
	BusinessOverviewRange30D           BusinessOverviewRange = "30d"
	BusinessOverviewRangeMonth         BusinessOverviewRange = "month"
	BusinessOverviewRangePreviousMonth BusinessOverviewRange = "previous_month"
	BusinessOverviewRangeCustom        BusinessOverviewRange = "custom"
)

type BusinessOverviewQuery struct {
	Range    BusinessOverviewRange
	Start    time.Time
	End      time.Time
	GroupID  *int64
	Timezone *time.Location
}

type BusinessOverviewReport struct {
	GeneratedAt    time.Time                   `json:"generated_at"`
	Timezone       string                      `json:"timezone"`
	StartDate      string                      `json:"start_date"`
	EndDate        string                      `json:"end_date"`
	Currency       string                      `json:"currency"`
	QuotaUnit      string                      `json:"quota_unit"`
	QuotaUnitLabel string                      `json:"quota_unit_label"`
	RevenueStatus  string                      `json:"revenue_status"`
	Summary        BusinessOverviewSummary     `json:"summary"`
	CashAndBalance BusinessOverviewCashBalance `json:"cash_and_balance"`
	Trend          []BusinessOverviewTrend     `json:"trend"`
	Groups         []BusinessOverviewGroup     `json:"groups"`
}

type BusinessOverviewSummary struct {
	RevenueStatus       string   `json:"revenue_status"`
	RevenueCNY          *float64 `json:"revenue_cny"`
	UpstreamCostCNY     *float64 `json:"upstream_cost_cny"`
	GrossProfitCNY      *float64 `json:"gross_profit_cny"`
	GrossMargin         *float64 `json:"gross_margin"`
	PaidConsumptionQ    *float64 `json:"paid_consumption_q"`
	GiftConsumptionQ    *float64 `json:"gift_consumption_q"`
	GiftUpstreamCostCNY *float64 `json:"gift_upstream_cost_cny"`
	PendingSplitCount   int      `json:"pending_split_count"`
	PendingCostCount    int      `json:"pending_cost_count"`
}

type BusinessOverviewCashBalance struct {
	CashRechargeCNY       *float64                       `json:"cash_recharge_cny"`
	OpeningPaidBalanceCNY *float64                       `json:"opening_paid_balance_cny"`
	PaidQuotaIssuedCNY    *float64                       `json:"paid_quota_issued_cny"`
	PaidConsumptionCNY    *float64                       `json:"paid_consumption_cny"`
	ClosingPaidBalanceCNY *float64                       `json:"closing_paid_balance_cny"`
	OpeningGiftBalanceQ   *float64                       `json:"opening_gift_balance_q"`
	ClosingGiftBalanceQ   *float64                       `json:"closing_gift_balance_q"`
	NetSettlementCNY      *float64                       `json:"net_settlement_cny"`
	Reconciliation        BusinessOverviewReconciliation `json:"balance_reconciliation"`
}

type BusinessOverviewReconciliation struct {
	Status        string   `json:"status"`
	DifferenceCNY float64  `json:"difference_cny"`
	Adjustments   []string `json:"adjustments"`
}

type BusinessOverviewTrend struct {
	Date               string  `json:"date"`
	CashRechargeCNY    float64 `json:"cash_recharge_cny"`
	PaidConsumptionCNY float64 `json:"paid_consumption_cny"`
	NetSettlementCNY   float64 `json:"net_settlement_cny"`
}

type BusinessOverviewGroup struct {
	GroupID                     *int64   `json:"group_id"`
	GroupName                   string   `json:"group_name"`
	Unassigned                  bool     `json:"unassigned"`
	ModelCount                  int64    `json:"model_count"`
	RequestCount                int64    `json:"request_count"`
	ConfiguredMultiplier        *float64 `json:"configured_multiplier"`
	PresetUpstreamMultiplier    *float64 `json:"preset_upstream_multiplier"`
	PresetMargin                *float64 `json:"preset_margin"`
	PresetStatus                string   `json:"preset_status"`
	EffectiveUpstreamMultiplier *float64 `json:"effective_upstream_multiplier"`
	RevenueCNY                  *float64 `json:"revenue_cny"`
	UpstreamCostCNY             *float64 `json:"upstream_cost_cny"`
	GrossProfitCNY              *float64 `json:"gross_profit_cny"`
	GrossMargin                 *float64 `json:"gross_margin"`
	RevenueStatus               string   `json:"revenue_status"`
}

type BusinessOverviewService interface {
	GetReport(context.Context, BusinessOverviewQuery) (*BusinessOverviewReport, error)
}

type businessOverviewService struct {
	db  *sql.DB
	now func() time.Time
}

func NewBusinessOverviewService(db *sql.DB) BusinessOverviewService {
	return &businessOverviewService{db: db, now: time.Now}
}

func (s *businessOverviewService) GetReport(ctx context.Context, q BusinessOverviewQuery) (*BusinessOverviewReport, error) {
	return s.getReport(ctx, q)
}

type businessOverviewUsageRow struct {
	ID        int64
	CreatedAt time.Time
	GroupID   sql.NullInt64
	GroupName string
	Model     string
	Cost      sql.NullFloat64
}

type businessOverviewLedgerRow struct {
	ReferenceID string
	CreatedAt   time.Time
	PaidDelta   sql.NullFloat64
	GiftDelta   sql.NullFloat64
}

type businessOverviewLedgerEvent struct {
	RecordType string
	CreatedAt  time.Time
	CashDelta  sql.NullFloat64
	PaidDelta  sql.NullFloat64
	GiftDelta  sql.NullFloat64
}

type businessOverviewWalletSnapshot struct {
	CashCNY float64
	PaidQ   float64
	GiftQ   float64
}

func (s *businessOverviewService) getReport(ctx context.Context, q BusinessOverviewQuery) (*BusinessOverviewReport, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("business overview database is unavailable")
	}
	if !q.End.After(q.Start) {
		return nil, ErrBusinessOverviewInvalidRange
	}
	loc := q.Start.Location()
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ul.id, ul.created_at, ul.group_id, COALESCE(g.name, ''), COALESCE(ul.model, ''),
			COALESCE(ul.account_cost, COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1))
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
		  AND ($3::bigint IS NULL OR ul.group_id = $3)
		ORDER BY ul.created_at, ul.id`, q.Start, q.End, q.GroupID)
	if err != nil {
		return nil, fmt.Errorf("query business overview usage: %w", err)
	}
	defer rows.Close()
	usage := make([]businessOverviewUsageRow, 0)
	for rows.Next() {
		var row businessOverviewUsageRow
		if err := rows.Scan(&row.ID, &row.CreatedAt, &row.GroupID, &row.GroupName, &row.Model, &row.Cost); err != nil {
			return nil, fmt.Errorf("scan business overview usage: %w", err)
		}
		usage = append(usage, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate business overview usage: %w", err)
	}

	ledger, ledgerErr := s.readLedger(ctx, q)
	ledgerAvailable := ledgerErr == nil
	if ledgerErr != nil && !isBusinessOverviewMissingLedger(ledgerErr) {
		return nil, ledgerErr
	}
	events, eventsErr := s.readLedgerEvents(ctx, q)
	if eventsErr != nil && !isBusinessOverviewMissingLedger(eventsErr) {
		return nil, eventsErr
	}
	if eventsErr != nil {
		ledgerAvailable = false
	}
	wallet, walletErr := s.readWalletSnapshot(ctx)
	if walletErr != nil && !isBusinessOverviewMissingLedger(walletErr) {
		return nil, walletErr
	}
	if walletErr != nil {
		ledgerAvailable = false
	}
	byReference := make(map[string]businessOverviewLedgerRow, len(ledger))
	for _, item := range ledger {
		byReference[item.ReferenceID] = item
	}

	var cost float64
	pendingCost := 0
	groups := make(map[string]*BusinessOverviewGroup)
	var paidQ, giftQ float64
	pendingSplit := 0
	for _, row := range usage {
		if !row.Cost.Valid || math.IsNaN(row.Cost.Float64) || math.IsInf(row.Cost.Float64, 0) {
			pendingCost++
		} else {
			cost += row.Cost.Float64
		}
		key := "unassigned"
		if row.GroupID.Valid {
			key = fmt.Sprintf("group:%d", row.GroupID.Int64)
		}
		group := groups[key]
		if group == nil {
			id := (*int64)(nil)
			if row.GroupID.Valid {
				value := row.GroupID.Int64
				id = &value
			}
			group = &BusinessOverviewGroup{GroupID: id, GroupName: row.GroupName, Unassigned: !row.GroupID.Valid, PresetStatus: BusinessOverviewRevenueUnavailable, RevenueStatus: BusinessOverviewRevenuePending}
			if group.Unassigned {
				group.GroupName = "未归组"
			}
			groups[key] = group
		}
		group.RequestCount++
		group.UpstreamCostCNY = addPointerFloat(group.UpstreamCostCNY, row.Cost)
		if row.Model != "" {
			group.ModelCount++
		}
		if !ledgerAvailable {
			continue
		}
		item, ok := byReference[fmt.Sprintf("%d", row.ID)]
		if !ok {
			pendingSplit++
			continue
		}
		paid := -item.PaidDelta.Float64
		gift := -item.GiftDelta.Float64
		if !item.PaidDelta.Valid || !item.GiftDelta.Valid || paid < 0 || gift < 0 {
			pendingSplit++
			continue
		}
		paidQ += paid
		giftQ += gift
		groupRevenue := paid
		group.RevenueCNY = addPointerFloat(group.RevenueCNY, sql.NullFloat64{Float64: groupRevenue, Valid: true})
		group.RevenueStatus = BusinessOverviewRevenueConfirmed
	}

	status := BusinessOverviewRevenueConfirmed
	if !ledgerAvailable {
		status = BusinessOverviewRevenuePending
	} else if pendingSplit > 0 {
		status = BusinessOverviewRevenuePendingSplit
	}
	revenue := paidQ
	if status != BusinessOverviewRevenueConfirmed {
		revenuePtr := (*float64)(nil)
		summary := BusinessOverviewSummary{RevenueStatus: status, RevenueCNY: revenuePtr, UpstreamCostCNY: floatPointer(cost), PaidConsumptionQ: floatPointer(paidQ), GiftConsumptionQ: floatPointer(giftQ), PendingSplitCount: pendingSplit, PendingCostCount: pendingCost}
		if pendingCost > 0 {
			summary.UpstreamCostCNY = nil
		}
		return s.buildReport(loc, q, summary, groups, ledgerAvailable, events, wallet)
	}
	summary := BusinessOverviewSummary{RevenueStatus: status, RevenueCNY: floatPointer(revenue), UpstreamCostCNY: floatPointer(cost), PaidConsumptionQ: floatPointer(paidQ), GiftConsumptionQ: floatPointer(giftQ), PendingSplitCount: pendingSplit, PendingCostCount: pendingCost}
	if pendingCost > 0 {
		summary.UpstreamCostCNY = nil
	} else {
		finalizeBusinessOverviewSummary(&summary)
	}
	return s.buildReport(loc, q, summary, groups, ledgerAvailable, events, wallet)
}

func (s *businessOverviewService) readLedger(ctx context.Context, q BusinessOverviewQuery) ([]businessOverviewLedgerRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(reference_id, ''), created_at, paid_quota_delta_usd, gift_quota_delta_usd FROM user_quota_ledger_entries WHERE record_type='usage_consumption' AND created_at >= $1 AND created_at < $2`, q.Start, q.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]businessOverviewLedgerRow, 0)
	for rows.Next() {
		var item businessOverviewLedgerRow
		if err := rows.Scan(&item.ReferenceID, &item.CreatedAt, &item.PaidDelta, &item.GiftDelta); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *businessOverviewService) readLedgerEvents(ctx context.Context, q BusinessOverviewQuery) ([]businessOverviewLedgerEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT record_type, created_at, cash_delta_cny, paid_quota_delta_usd, gift_quota_delta_usd FROM user_quota_ledger_entries WHERE created_at >= $1 AND created_at < $2 ORDER BY created_at`, q.Start, q.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]businessOverviewLedgerEvent, 0)
	for rows.Next() {
		var event businessOverviewLedgerEvent
		if err := rows.Scan(&event.RecordType, &event.CreatedAt, &event.CashDelta, &event.PaidDelta, &event.GiftDelta); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *businessOverviewService) readWalletSnapshot(ctx context.Context) (businessOverviewWalletSnapshot, error) {
	var snapshot businessOverviewWalletSnapshot
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cash_balance_cny), 0), COALESCE(SUM(paid_quota_balance_usd), 0), COALESCE(SUM(gift_quota_balance_usd), 0) FROM user_wallets`).Scan(&snapshot.CashCNY, &snapshot.PaidQ, &snapshot.GiftQ)
	return snapshot, err
}

func (s *businessOverviewService) buildReport(loc *time.Location, q BusinessOverviewQuery, summary BusinessOverviewSummary, groups map[string]*BusinessOverviewGroup, ledgerAvailable bool, events []businessOverviewLedgerEvent, wallet businessOverviewWalletSnapshot) (*BusinessOverviewReport, error) {
	endDate := q.End.Add(-time.Nanosecond).In(loc).Format("2006-01-02")
	report := &BusinessOverviewReport{GeneratedAt: s.now().UTC(), Timezone: loc.String(), StartDate: q.Start.In(loc).Format("2006-01-02"), EndDate: endDate, Currency: "CNY", QuotaUnit: "Q", QuotaUnitLabel: "内部记账额度，不是美元", RevenueStatus: summary.RevenueStatus, Summary: summary, Trend: []BusinessOverviewTrend{}, Groups: make([]BusinessOverviewGroup, 0, len(groups))}
	if ledgerAvailable {
		var recharge, consumed, paidIssued, giftIssued float64
		for _, event := range events {
			if event.RecordType == businessOverviewRecordRecharge && event.CashDelta.Valid {
				recharge += event.CashDelta.Float64
			}
			if event.RecordType == businessOverviewRecordUsage && event.PaidDelta.Valid {
				consumed += -event.PaidDelta.Float64
			}
			if event.RecordType == businessOverviewRecordRecharge && event.PaidDelta.Valid {
				paidIssued += event.PaidDelta.Float64
			}
			if event.RecordType == businessOverviewRecordRecharge && event.GiftDelta.Valid {
				giftIssued += event.GiftDelta.Float64
			}
		}
		report.CashAndBalance.CashRechargeCNY = &recharge
		report.CashAndBalance.PaidQuotaIssuedCNY = &paidIssued
		report.CashAndBalance.PaidConsumptionCNY = &consumed
		closingPaid := wallet.PaidQ
		openingPaid := closingPaid - paidIssued + consumed
		report.CashAndBalance.ClosingPaidBalanceCNY = &closingPaid
		report.CashAndBalance.OpeningPaidBalanceCNY = &openingPaid
		closingGift := wallet.GiftQ
		openingGift := closingGift - giftIssued
		report.CashAndBalance.ClosingGiftBalanceQ = &closingGift
		report.CashAndBalance.OpeningGiftBalanceQ = &openingGift
		net := recharge - consumed
		report.CashAndBalance.NetSettlementCNY = &net
		report.CashAndBalance.Reconciliation = BusinessOverviewReconciliation{Status: BusinessOverviewBalancePending, DifferenceCNY: 0, Adjustments: []string{"历史余额边界待钱包快照"}}
	} else {
		report.CashAndBalance.Reconciliation = BusinessOverviewReconciliation{Status: BusinessOverviewBalancePending, DifferenceCNY: 0, Adjustments: []string{"T55 额度账本不可用"}}
	}
	for _, group := range groups {
		if group.RevenueCNY != nil && group.UpstreamCostCNY != nil {
			profit := *group.RevenueCNY - *group.UpstreamCostCNY
			group.GrossProfitCNY = &profit
			if *group.RevenueCNY != 0 {
				margin := profit / *group.RevenueCNY
				group.GrossMargin = &margin
			}
		}
		if !ledgerAvailable {
			group.RevenueStatus = BusinessOverviewRevenuePending
		}
		report.Groups = append(report.Groups, *group)
	}
	if ledgerAvailable {
		trendByDate := make(map[string]*BusinessOverviewTrend)
		for _, event := range events {
			date := event.CreatedAt.In(loc).Format("2006-01-02")
			item := trendByDate[date]
			if item == nil {
				item = &BusinessOverviewTrend{Date: date}
				trendByDate[date] = item
			}
			if event.RecordType == businessOverviewRecordRecharge && event.CashDelta.Valid {
				item.CashRechargeCNY += event.CashDelta.Float64
			}
			if event.RecordType == businessOverviewRecordUsage && event.PaidDelta.Valid {
				item.PaidConsumptionCNY += -event.PaidDelta.Float64
			}
			item.NetSettlementCNY = item.CashRechargeCNY - item.PaidConsumptionCNY
		}
		for day := q.Start.In(loc); day.Before(q.End); day = day.AddDate(0, 0, 1) {
			date := day.Format("2006-01-02")
			if item := trendByDate[date]; item != nil {
				report.Trend = append(report.Trend, *item)
			} else {
				report.Trend = append(report.Trend, BusinessOverviewTrend{Date: date})
			}
		}
	}
	return report, nil
}

func addPointerFloat(current *float64, value sql.NullFloat64) *float64 {
	if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
		return current
	}
	if current == nil {
		v := value.Float64
		return &v
	}
	*current += value.Float64
	return current
}

func floatPointer(value float64) *float64 { return &value }

func isBusinessOverviewMissingLedger(err error) bool {
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "does not exist") {
		return false
	}
	return strings.Contains(message, "user_quota_ledger_entries") || strings.Contains(message, "user_wallets")
}

func BusinessOverviewDateRange(startDate, endDate string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid start_date, use YYYY-MM-DD")
	}
	last, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid end_date, use YYYY-MM-DD")
	}
	end := last.AddDate(0, 0, 1)
	if !end.After(start) {
		return time.Time{}, time.Time{}, ErrBusinessOverviewInvalidRange
	}
	return start, end, nil
}

func finalizeBusinessOverviewSummary(summary *BusinessOverviewSummary) {
	if summary == nil || summary.RevenueCNY == nil || summary.UpstreamCostCNY == nil {
		return
	}
	profit := *summary.RevenueCNY - *summary.UpstreamCostCNY
	if *summary.RevenueCNY == 0 {
		return
	}
	summary.GrossProfitCNY = &profit
	margin := profit / *summary.RevenueCNY
	summary.GrossMargin = &margin
}

func markBusinessOverviewPendingSplit(summary *BusinessOverviewSummary) {
	if summary == nil {
		return
	}
	summary.RevenueStatus = BusinessOverviewRevenuePendingSplit
	summary.RevenueCNY = nil
	summary.GrossProfitCNY = nil
	summary.GrossMargin = nil
}

func reconcileBusinessOverview(opening, issued, adjustments, consumed, closing float64) BusinessOverviewReconciliation {
	difference := opening + issued + adjustments - consumed - closing
	if math.Abs(difference) < 0.00000001 {
		difference = 0
	}
	status := BusinessOverviewBalanceBalanced
	if difference != 0 {
		status = BusinessOverviewBalanceUnbalanced
	}
	return BusinessOverviewReconciliation{Status: status, DifferenceCNY: difference, Adjustments: []string{}}
}
