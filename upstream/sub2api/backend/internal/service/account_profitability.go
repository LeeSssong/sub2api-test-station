package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	AccountProfitabilitySourceSub2API       = "sub2api"
	AccountProfitabilitySourceNewAPI        = "newapi"
	AccountProfitabilitySourceSelfPurchased = "self_purchased"
	AccountProfitabilitySourcePending       = "pending"

	AccountProfitabilityExpenseAvailable = "available"
	AccountProfitabilityExpensePending   = "pending"

	AccountProfitabilityCostBasisUsage       = "usage_account_cost"
	AccountProfitabilityCostBasisProcurement = "allocated_procurement"
	AccountProfitabilityCostBasisMissing     = "missing_cost"
)

var ErrInvalidAccountProfitabilityRange = errors.New("account profitability range must have end after start")

// AccountProfitabilitySummary is the account-level operating result for a range.
// Expense-derived totals are null when any account in the report has unknown cost,
// so an incomplete total cannot be mistaken for zero cost.
type AccountProfitabilitySummary struct {
	Revenue      float64  `json:"revenue"`
	Expense      *float64 `json:"expense"`
	Profit       *float64 `json:"profit"`
	Margin       *float64 `json:"margin"`
	AccountCount int      `json:"account_count"`
	PendingCount int      `json:"pending_count"`
}

type AccountProfitabilityRow struct {
	AccountID             int64    `json:"account_id"`
	Name                  string   `json:"name"`
	Platform              string   `json:"platform"`
	AccountType           string   `json:"account_type"`
	Source                string   `json:"source"`
	Status                string   `json:"status"`
	Revenue               float64  `json:"revenue"`
	Expense               *float64 `json:"expense"`
	ExpenseCurrency       string   `json:"expense_currency,omitempty"`
	ProcurementExpenseCNY *float64 `json:"procurement_expense_cny,omitempty"`
	Profit                *float64 `json:"profit"`
	Margin                *float64 `json:"margin"`
	ExpenseStatus         string   `json:"expense_status"`
	RequestCount          int64    `json:"request_count"`
	Tokens                int64    `json:"tokens"`
	CostBasis             string   `json:"cost_basis"`
}

type AccountProfitabilityReport struct {
	StartDate   string                      `json:"start_date"`
	EndDate     string                      `json:"end_date"`
	GeneratedAt time.Time                   `json:"generated_at"`
	Summary     AccountProfitabilitySummary `json:"summary"`
	Rows        []AccountProfitabilityRow   `json:"rows"`
}

type accountProfitabilityAggregate struct {
	AccountID                  int64
	Name                       string
	Platform                   string
	AccountType                string
	Status                     string
	Extra                      map[string]any
	ProcurementCostCNY         *float64
	ProcurementCostEffectiveAt *time.Time
	ExpiresAt                  *time.Time
	Revenue                    float64
	RelayExpense               float64
	RequestCount               int64
	Tokens                     int64
}

// AccountProfitabilityService aggregates billed revenue and defensible account
// costs for the administrator operations page.
type AccountProfitabilityService struct {
	db  *sql.DB
	now func() time.Time
}

func NewAccountProfitabilityService(db *sql.DB) *AccountProfitabilityService {
	return &AccountProfitabilityService{db: db, now: time.Now}
}

// GetReport expects a half-open interval [start, end). The HTTP handler turns
// inclusive calendar dates into this interval in the requested timezone.
func (s *AccountProfitabilityService) GetReport(ctx context.Context, start, end time.Time) (*AccountProfitabilityReport, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("account profitability database is unavailable")
	}
	if !end.After(start) {
		return nil, ErrInvalidAccountProfitabilityRange
	}

	aggregates, err := s.queryAggregates(ctx, start, end)
	if err != nil {
		return nil, err
	}

	report := &AccountProfitabilityReport{
		StartDate:   start.Format("2006-01-02"),
		EndDate:     end.Add(-time.Nanosecond).In(start.Location()).Format("2006-01-02"),
		GeneratedAt: s.now().UTC(),
		Rows:        make([]AccountProfitabilityRow, 0, len(aggregates)),
	}
	knownExpense := 0.0
	for _, aggregate := range aggregates {
		row := buildAccountProfitabilityRow(aggregate, start, end)
		report.Rows = append(report.Rows, row)
		report.Summary.Revenue += row.Revenue
		if row.Expense == nil || row.Profit == nil {
			report.Summary.PendingCount++
			continue
		}
		knownExpense += *row.Expense
	}
	report.Summary.AccountCount = len(report.Rows)
	if report.Summary.PendingCount == 0 {
		expense := knownExpense
		profit := report.Summary.Revenue - expense
		margin := accountProfitabilityMargin(profit, report.Summary.Revenue)
		report.Summary.Expense = &expense
		report.Summary.Profit = &profit
		report.Summary.Margin = &margin
	}
	return report, nil
}

func (s *AccountProfitabilityService) queryAggregates(ctx context.Context, start, end time.Time) ([]accountProfitabilityAggregate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			a.id AS account_id,
			a.name,
			a.platform,
			a.type AS account_type,
			a.status,
			a.extra::text AS extra,
			a.procurement_cost_cny,
			a.procurement_cost_effective_at,
			a.expires_at,
			COALESCE(SUM(ul.actual_cost), 0)::double precision AS revenue,
			COALESCE(SUM(COALESCE(ul.account_cost, COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1))), 0)::double precision AS relay_expense,
			COUNT(ul.id)::bigint AS request_count,
			COALESCE(SUM(
				ul.input_tokens + ul.output_tokens +
				ul.cache_creation_tokens + ul.cache_read_tokens
			), 0)::bigint AS tokens
		FROM accounts AS a
		LEFT JOIN usage_logs AS ul
			ON ul.account_id = a.id
			AND ul.created_at >= $1
			AND ul.created_at < $2
		WHERE a.deleted_at IS NULL
		GROUP BY
			a.id, a.name, a.platform, a.type, a.status, a.extra,
			a.procurement_cost_cny, a.procurement_cost_effective_at, a.expires_at
		ORDER BY a.id`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query account profitability: %w", err)
	}
	defer rows.Close()

	result := make([]accountProfitabilityAggregate, 0)
	for rows.Next() {
		var (
			aggregate       accountProfitabilityAggregate
			extraRaw        string
			procurementCost sql.NullFloat64
			effectiveAt     sql.NullTime
			expiresAt       sql.NullTime
		)
		if err := rows.Scan(
			&aggregate.AccountID,
			&aggregate.Name,
			&aggregate.Platform,
			&aggregate.AccountType,
			&aggregate.Status,
			&extraRaw,
			&procurementCost,
			&effectiveAt,
			&expiresAt,
			&aggregate.Revenue,
			&aggregate.RelayExpense,
			&aggregate.RequestCount,
			&aggregate.Tokens,
		); err != nil {
			return nil, fmt.Errorf("scan account profitability: %w", err)
		}
		if extraRaw != "" {
			if err := json.Unmarshal([]byte(extraRaw), &aggregate.Extra); err != nil {
				return nil, fmt.Errorf("decode account %d profitability metadata: %w", aggregate.AccountID, err)
			}
		}
		if procurementCost.Valid {
			value := procurementCost.Float64
			aggregate.ProcurementCostCNY = &value
		}
		if effectiveAt.Valid {
			value := effectiveAt.Time
			aggregate.ProcurementCostEffectiveAt = &value
		}
		if expiresAt.Valid {
			value := expiresAt.Time
			aggregate.ExpiresAt = &value
		}
		result = append(result, aggregate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account profitability: %w", err)
	}
	return result, nil
}

func buildAccountProfitabilityRow(aggregate accountProfitabilityAggregate, start, end time.Time) AccountProfitabilityRow {
	row := AccountProfitabilityRow{
		AccountID:       aggregate.AccountID,
		Name:            aggregate.Name,
		Platform:        aggregate.Platform,
		AccountType:     aggregate.AccountType,
		Status:          aggregate.Status,
		Revenue:         aggregate.Revenue,
		RequestCount:    aggregate.RequestCount,
		Tokens:          aggregate.Tokens,
		ExpenseStatus:   AccountProfitabilityExpensePending,
		CostBasis:       AccountProfitabilityCostBasisMissing,
		ExpenseCurrency: "USD",
	}

	snapshot := decodeAccountMonitorBalance(aggregate.Extra)
	switch {
	case snapshot != nil && snapshot.Source == AccountMonitorBalanceSourceSub2API:
		row.Source = AccountProfitabilitySourceSub2API
		setAccountProfitabilityExpense(&row, aggregate.RelayExpense, AccountProfitabilityCostBasisUsage)
	case snapshot != nil && snapshot.Source == AccountMonitorBalanceSourceNewAPI:
		row.Source = AccountProfitabilitySourceNewAPI
		setAccountProfitabilityExpense(&row, aggregate.RelayExpense, AccountProfitabilityCostBasisUsage)
	case aggregate.ProcurementCostCNY != nil:
		row.Source = AccountProfitabilitySourceSelfPurchased
		if expense, ok := allocateAccountProcurementCost(
			*aggregate.ProcurementCostCNY,
			aggregate.ProcurementCostEffectiveAt,
			aggregate.ExpiresAt,
			start,
			end,
		); ok {
			// Procurement is recorded in CNY while usage revenue/cost is USD.
			// Keep the CNY expense visible, but do not calculate mixed-currency
			// profit or include it in the USD summary without an explicit FX rate.
			row.Expense = &expense
			row.ExpenseCurrency = "CNY"
			row.ProcurementExpenseCNY = &expense
			row.ExpenseStatus = AccountProfitabilityExpenseAvailable
			row.CostBasis = AccountProfitabilityCostBasisProcurement
		}
	default:
		row.Source = AccountProfitabilitySourcePending
	}
	return row
}

func setAccountProfitabilityExpense(row *AccountProfitabilityRow, expense float64, basis string) {
	profit := row.Revenue - expense
	margin := accountProfitabilityMargin(profit, row.Revenue)
	row.Expense = &expense
	row.Profit = &profit
	row.Margin = &margin
	row.ExpenseStatus = AccountProfitabilityExpenseAvailable
	row.CostBasis = basis
}

func accountProfitabilityMargin(profit, revenue float64) float64 {
	if revenue == 0 {
		return 0
	}
	return profit / revenue
}

func allocateAccountProcurementCost(cost float64, effectiveAt, expiresAt *time.Time, start, end time.Time) (float64, bool) {
	if effectiveAt == nil || expiresAt == nil || math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 || !expiresAt.After(*effectiveAt) {
		return 0, false
	}
	overlapStart := start
	if effectiveAt.After(overlapStart) {
		overlapStart = *effectiveAt
	}
	overlapEnd := end
	if expiresAt.Before(overlapEnd) {
		overlapEnd = *expiresAt
	}
	if !overlapEnd.After(overlapStart) {
		return 0, true
	}
	activeDuration := expiresAt.Sub(*effectiveAt)
	overlapDuration := overlapEnd.Sub(overlapStart)
	return cost * float64(overlapDuration) / float64(activeDuration), true
}
