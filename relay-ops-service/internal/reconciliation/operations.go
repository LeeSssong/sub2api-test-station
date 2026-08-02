package reconciliation

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type OperationsScope struct {
	GroupID   *int64    `json:"group_id,omitempty"`
	AccountID *int64    `json:"account_id,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Currency  string    `json:"currency"`
	Timezone  string    `json:"timezone"`
}

type OperationsSummary struct {
	Scope                    OperationsScope  `json:"scope"`
	TotalAttempts            int64            `json:"total_attempts"`
	MatchedAttempts          int64            `json:"matched_attempts"`
	PendingAttempts          int64            `json:"pending_attempts"`
	ExceptionAttempts        int64            `json:"exception_attempts"`
	ConflictAttempts         int64            `json:"conflict_attempts"`
	CoverageKnown            bool             `json:"coverage_known"`
	CoverageRatio            decimal.Decimal  `json:"coverage_ratio"`
	UpstreamCost             decimal.Decimal  `json:"upstream_cost"`
	UserCharge               decimal.Decimal  `json:"user_charge"`
	PaperProfit              decimal.Decimal  `json:"paper_profit"`
	ProfitMargin             *decimal.Decimal `json:"profit_margin"`
	UnattributedAttempts     int64            `json:"unattributed_attempts"`
	UnattributedUserCharge   decimal.Decimal  `json:"unattributed_user_charge"`
	UnattributedUpstreamCost decimal.Decimal  `json:"unattributed_upstream_cost"`
	Currency                 string           `json:"currency"`
	ObservedAt               time.Time        `json:"observed_at"`
}

type OperationsDailyRow struct {
	Day               string           `json:"day"`
	TotalAttempts     int64            `json:"total_attempts"`
	MatchedAttempts   int64            `json:"matched_attempts"`
	PendingAttempts   int64            `json:"pending_attempts"`
	ExceptionAttempts int64            `json:"exception_attempts"`
	ConflictAttempts  int64            `json:"conflict_attempts"`
	CoverageKnown     bool             `json:"coverage_known"`
	CoverageRatio     decimal.Decimal  `json:"coverage_ratio"`
	UpstreamCost      decimal.Decimal  `json:"upstream_cost"`
	UserCharge        decimal.Decimal  `json:"user_charge"`
	PaperProfit       decimal.Decimal  `json:"paper_profit"`
	ProfitMargin      *decimal.Decimal `json:"profit_margin"`
	Currency          string           `json:"currency"`
}

func ValidateOperationsScope(scope OperationsScope) (OperationsScope, error) {
	if scope.GroupID != nil && *scope.GroupID <= 0 {
		return OperationsScope{}, fmt.Errorf("group_id must be positive")
	}
	if scope.AccountID != nil && *scope.AccountID <= 0 {
		return OperationsScope{}, fmt.Errorf("account_id must be positive")
	}
	if scope.Start.IsZero() || scope.End.IsZero() || !scope.Start.Before(scope.End) {
		return OperationsScope{}, fmt.Errorf("time window is invalid")
	}
	scope.Currency = strings.ToUpper(strings.TrimSpace(scope.Currency))
	if len(scope.Currency) != 3 {
		return OperationsScope{}, fmt.Errorf("currency must be a three-letter code")
	}
	scope.Timezone = strings.TrimSpace(scope.Timezone)
	if scope.Timezone == "" {
		scope.Timezone = "UTC"
	}
	location, err := time.LoadLocation(scope.Timezone)
	if err != nil {
		return OperationsScope{}, fmt.Errorf("timezone is invalid")
	}
	scope.Timezone = location.String()
	scope.Start = scope.Start.UTC().Truncate(time.Microsecond)
	scope.End = scope.End.UTC().Truncate(time.Microsecond)
	return scope, nil
}
