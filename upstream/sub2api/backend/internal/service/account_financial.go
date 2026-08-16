package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrFinancialInvalidAmount     = errors.New("financial amount must be finite and non-negative")
	ErrFinancialNotToday          = errors.New("financial value may only be written for today")
	ErrFinancialOAuthType         = errors.New("daily oauth cost requires literal oauth account type")
	ErrFinancialReviewNotEligible = errors.New("usage is not a pending financial exception")
	ErrFinancialAuditRequired     = errors.New("financial audit dependency is required")
	ErrFinancialUsageReader       = errors.New("account financial usage reader is required")
	ErrFinancialUsageSnapshot     = errors.New("account financial usage snapshot is unavailable")
)

type AccountFinancialRange string

const (
	AccountFinancialRangeToday AccountFinancialRange = "today"
	AccountFinancialRange24H   AccountFinancialRange = "24h"
	AccountFinancialRange7D    AccountFinancialRange = "7d"
	AccountFinancialRange31D   AccountFinancialRange = "31d"
)

type FinancialAmounts struct {
	Requests        int64            `json:"requests"`
	Tokens          int64            `json:"tokens"`
	Cost            float64          `json:"cost"`
	UserCost        float64          `json:"user_cost"`
	Profit          float64          `json:"profit"`
	Margin          *float64         `json:"margin"`
	Revenue         float64          `json:"revenue"`
	Expense         float64          `json:"expense"`
	ProbeRequests   *int64           `json:"probe_requests"`
	ProbeTokens     *int64           `json:"probe_tokens"`
	ProbeCost       *decimal.Decimal `json:"probe_cost"`
	ProbeCostStatus *string          `json:"probe_cost_status"`
}

type AccountFinancialReport struct {
	GeneratedAt    time.Time                        `json:"generated_at"`
	Range          AccountFinancialRange            `json:"range"`
	Currency       string                           `json:"currency"`
	Summary        FinancialAmounts                 `json:"summary"`
	Accounts       []*AccountFinancialAccountReport `json:"accounts"`
	Groups         []*AccountFinancialGroupReport   `json:"groups"`
	UserBalanceCNY float64                          `json:"user_unconsumed_balance_cny"`
	ProbeDataError bool                             `json:"probe_data_error"`
	ProbeErrorCode *string                          `json:"probe_error_code"`
}

type AccountFinancialAccountReport struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Platform   string           `json:"platform"`
	Historical bool             `json:"historical"`
	Amounts    FinancialAmounts `json:"amounts"`
}

type AccountFinancialGroupReport struct {
	ID         int64                            `json:"id"`
	Name       string                           `json:"name"`
	Unassigned bool                             `json:"unassigned"`
	Historical bool                             `json:"historical"`
	Amounts    FinancialAmounts                 `json:"amounts"`
	Accounts   []*AccountFinancialAccountReport `json:"accounts"`
}

type AccountFinancialSnapshotQuery struct{ GeneratedAt, From, To time.Time }

// AccountFinancialUsageReader exposes the native usage aggregation used by the
// account-financial report without coupling it to the historical evidence and
// mutation repository contract.
type AccountFinancialUsageReader interface {
	ReadAccountFinancialUsage(ctx context.Context, from, to time.Time) (*AccountFinancialUsageSnapshot, error)
}

type AccountFinancialUsageSnapshot struct {
	Accounts       []AccountFinancialUsageAccount
	Groups         []AccountFinancialUsageGroup
	Rows           []AccountFinancialUsageRow
	ProbeRows      []AccountProbeCostAggregate
	ProbeDataError bool
	ProbeErrorCode *string
	UserBalanceCNY float64
}

type AccountFinancialUsageAccount struct {
	ID       int64
	Name     string
	Type     string
	Platform string
	Active   bool
}

type AccountFinancialUsageGroup struct {
	ID     int64
	Name   string
	Active bool
}

type AccountFinancialUsageRow struct {
	GroupID         *int64
	GroupName       string
	AccountID       int64
	AccountName     string
	AccountType     string
	AccountPlatform string
	Requests        int64
	Tokens          int64
	Cost            float64
	UserCost        float64
}

type AccountFinancialSnapshot struct {
	GeneratedAt    time.Time
	EnabledAt      time.Time
	Accounts       []AccountFinancialSnapshotAccount
	Groups         []AccountFinancialSnapshotGroup
	Entries        []AccountFinancialSnapshotEntry
	DailyValues    []AccountFinancialDailyValue
	UserBalanceCNY float64
}
type AccountFinancialSnapshotGroup struct {
	ID   int64
	Name string
}
type AccountFinancialSnapshotAccount struct {
	ID                   int64
	Name, Type, Platform string
}
type AccountFinancialSnapshotEntry struct {
	UsageLogID, AccountID                                  int64
	GroupID                                                *int64
	GroupName                                              string
	CreatedAt                                              time.Time
	BusinessDate                                           string
	RevenueCNY                                             float64
	EvidenceID                                             *int64
	EvidenceStatus                                         string
	EvidenceCostCNY                                        *float64
	ReviewID                                               *int64
	ReviewCostCNY                                          *float64
	RequestID, Model, AccountName, AccountType, ReasonCode string
	Source                                                 string
	UpstreamRequestID                                      *string
	UpstreamBillingTime                                    *time.Time
	UpstreamModel                                          *string
	SubActualCost, NewAPIQuota, NewAPIQuotaPerUnit         *float64
}
type AccountFinancialDailyValue struct {
	AccountID                                      int64
	BusinessDate                                   string
	OAuthCostCNY                                   *float64
	RevenueOverrideCNY                             *float64
	RevenueOverrideAt                              *time.Time
	RevenueEvidenceCutoffID, RevenueReviewCutoffID *int64
	CostOverrideCNY                                *float64
	CostOverrideAt                                 *time.Time
	CostEvidenceCutoffID, CostReviewCutoffID       *int64
}
type UsageFinancialEvidence struct {
	UsageLogID, AccountID      int64
	RequestID, AccountName     string
	AccountType, Source        string
	UpstreamRequestID          *string
	UpstreamBillingTime        *time.Time
	UpstreamModel              *string
	EvidenceStatus, ReasonCode string
	SubActualCost              *float64
	NewAPIQuota                *float64
	NewAPIQuotaPerUnit         *float64
	NormalizedCostCNY          *float64
	ReviewID                   *int64
	ReviewCostCNY              *float64
}
type AccountFinancialException struct {
	UsageLogID, AccountID                              int64
	RequestID, Model, AccountName, AccountType, Source string
	UpstreamRequestID                                  *string
	UpstreamBillingTime                                *time.Time
	UpstreamModel                                      *string
	CreatedAt                                          time.Time
	RevenueCNY                                         float64
	EvidenceStatus, ReasonCode, ReviewStatus           string
	CostTrace                                          AccountFinancialCostTrace
}
type AccountFinancialCostTrace struct{ SubActualCost, NewAPIQuota, NewAPIQuotaPerUnit, NormalizedCostCNY *float64 }
type AccountFinancialExceptionList struct {
	GeneratedAt    time.Time
	Items          []AccountFinancialException
	Total          int
	Page, PageSize int
}
type UsageCostReviewInput struct {
	UsageLogID    int64
	ManualCostCNY *float64
	ReviewedBy    int64
	ReviewedAt    time.Time
	RequestID     string
}
type UsageCostReviewResult struct {
	Created                        bool
	UsageLogID                     int64
	AccountID                      int64
	BusinessDate                   string
	OldManualCostCNY               *float64
	ManualCostCNY, ManualProfitCNY float64
}
type ReviewFilter struct {
	AccountID                            *int64
	From, To                             *time.Time
	Page, PageSize                       int
	Search, EvidenceStatus, ReviewStatus string
}
type ReviewFilteredInput struct {
	Filter                    ReviewFilter
	MaxUsageLogID, ReviewedBy int64
	ReviewedAt                time.Time
	ManualCostCNY             *float64
	RequestID                 string
}
type ReviewFilteredResult struct {
	Cutoff, MaxUsageLogID     int64
	Matched, Updated, Skipped int
	Reviews                   []UsageCostReviewResult
}
type UsageCostReviewBatchResult = ReviewFilteredResult
type OAuthDailyCostInput struct {
	AccountID    int64
	BusinessDate string
	CostCNY      *float64
	ActorUserID  int64
	RequestID    string
}
type TodayOverrideInput struct {
	AccountID           int64
	BusinessDate        string
	RevenueCNY, CostCNY *float64
	ActorUserID         int64
	RequestID           string
}
type FinancialMutationResult struct {
	AccountID                        int64
	BusinessDate                     string
	OldValue, NewValue               *float64
	CutoffEvidenceID, CutoffReviewID int64
	MutationKind                     string
}
type GetUsageEvidenceInput struct{ UsageLogID int64 }

type AccountFinancialRepository interface {
	ReadSnapshot(context.Context, AccountFinancialSnapshotQuery) (*AccountFinancialSnapshot, error)
	CreateReview(context.Context, UsageCostReviewInput) (*UsageCostReviewResult, error)
	FreezeReviewFilter(context.Context, ReviewFilter) (int64, error)
	ReviewFiltered(context.Context, ReviewFilteredInput) (*ReviewFilteredResult, error)
	SetOAuthDailyCost(context.Context, OAuthDailyCostInput) (*FinancialMutationResult, error)
	SetTodayOverride(context.Context, TodayOverrideInput) (*FinancialMutationResult, error)
	GetUsageEvidence(context.Context, int64) (*UsageFinancialEvidence, error)
}

type AccountFinancialService struct {
	repo        AccountFinancialRepository
	usageReader AccountFinancialUsageReader
	now         func() time.Time
	audit       *AccountFinancialAudit
}

func NewAccountFinancialServiceWithAudit(repo AccountFinancialRepository, usageReader AccountFinancialUsageReader, now func() time.Time, audit *AccountFinancialAudit) *AccountFinancialService {
	if now == nil {
		now = time.Now
	}
	return &AccountFinancialService{repo: repo, usageReader: usageReader, now: now, audit: audit}
}

func NewAccountFinancialService(repo AccountFinancialRepository, usageReader AccountFinancialUsageReader, now func() time.Time) *AccountFinancialService {
	return NewAccountFinancialServiceWithAudit(repo, usageReader, now, nil)
}

func finalizeFinancialAmounts(v *FinancialAmounts) {
	v.Profit = v.UserCost - v.Cost
	v.Revenue = v.UserCost
	v.Expense = v.Cost
	if v.UserCost == 0 {
		v.Margin = nil
		return
	}
	margin := v.Profit / v.UserCost
	v.Margin = &margin
}

func (s *AccountFinancialService) GetReport(ctx context.Context, r AccountFinancialRange) (*AccountFinancialReport, error) {
	if s.usageReader == nil {
		return nil, ErrFinancialUsageReader
	}
	now := s.now()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(loc)
	from := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	switch r {
	case AccountFinancialRange24H:
		from = now.Add(-24 * time.Hour)
	case AccountFinancialRange7D:
		from = time.Date(localNow.Year(), localNow.Month(), localNow.Day()-6, 0, 0, 0, 0, loc)
	case AccountFinancialRange31D:
		from = time.Date(localNow.Year(), localNow.Month(), localNow.Day()-30, 0, 0, 0, 0, loc)
	default:
		r = AccountFinancialRangeToday
	}
	snap, err := s.usageReader.ReadAccountFinancialUsage(ctx, from, now)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, ErrFinancialUsageSnapshot
	}
	report := &AccountFinancialReport{
		GeneratedAt: now, Range: r, Currency: "USD", UserBalanceCNY: snap.UserBalanceCNY,
		ProbeDataError: snap.ProbeDataError, ProbeErrorCode: snap.ProbeErrorCode,
	}
	accountMeta := make(map[int64]AccountFinancialUsageAccount, len(snap.Accounts))
	accountByID := make(map[int64]*AccountFinancialAccountReport, len(snap.Accounts))
	for _, meta := range snap.Accounts {
		accountMeta[meta.ID] = meta
		if !meta.Active {
			continue
		}
		account := &AccountFinancialAccountReport{ID: meta.ID, Name: financialAccountName(meta.Name, meta.ID), Type: meta.Type, Platform: meta.Platform}
		accountByID[meta.ID] = account
		report.Accounts = append(report.Accounts, account)
	}
	accountForRow := func(row AccountFinancialUsageRow) *AccountFinancialAccountReport {
		if account := accountByID[row.AccountID]; account != nil {
			return account
		}
		meta := accountMeta[row.AccountID]
		account := &AccountFinancialAccountReport{
			ID: row.AccountID, Name: financialAccountName(financialFirstNonEmpty(meta.Name, row.AccountName), row.AccountID),
			Type: financialFirstNonEmpty(meta.Type, row.AccountType), Platform: financialFirstNonEmpty(meta.Platform, row.AccountPlatform), Historical: !meta.Active,
		}
		accountByID[row.AccountID] = account
		report.Accounts = append(report.Accounts, account)
		return account
	}
	type groupAccumulator struct {
		report   *AccountFinancialGroupReport
		accounts map[int64]*AccountFinancialAccountReport
	}
	groupMeta := make(map[int64]AccountFinancialUsageGroup, len(snap.Groups))
	groupByID := make(map[int64]*groupAccumulator, len(snap.Groups)+1)
	for _, meta := range snap.Groups {
		groupMeta[meta.ID] = meta
		if !meta.Active {
			continue
		}
		group := &AccountFinancialGroupReport{ID: meta.ID, Name: financialGroupName(meta.Name, meta.ID)}
		groupByID[meta.ID] = &groupAccumulator{report: group, accounts: make(map[int64]*AccountFinancialAccountReport)}
		report.Groups = append(report.Groups, group)
	}
	var historicalGroups []*AccountFinancialGroupReport
	var unassigned *groupAccumulator
	groupForRow := func(row AccountFinancialUsageRow) *groupAccumulator {
		if row.GroupID == nil {
			if unassigned == nil {
				unassigned = &groupAccumulator{report: &AccountFinancialGroupReport{ID: 0, Name: "未归属", Unassigned: true}, accounts: make(map[int64]*AccountFinancialAccountReport)}
			}
			return unassigned
		}
		id := *row.GroupID
		if group := groupByID[id]; group != nil {
			return group
		}
		meta := groupMeta[id]
		group := &AccountFinancialGroupReport{ID: id, Name: financialGroupName(financialFirstNonEmpty(meta.Name, row.GroupName), id), Historical: !meta.Active}
		accumulator := &groupAccumulator{report: group, accounts: make(map[int64]*AccountFinancialAccountReport)}
		groupByID[id] = accumulator
		historicalGroups = append(historicalGroups, group)
		return accumulator
	}
	add := func(dst *FinancialAmounts, row AccountFinancialUsageRow) {
		dst.Requests += row.Requests
		dst.Tokens += row.Tokens
		dst.Cost += row.Cost
		dst.UserCost += row.UserCost
	}
	for _, row := range snap.Rows {
		account := accountForRow(row)
		group := groupForRow(row)
		groupAccount := group.accounts[row.AccountID]
		if groupAccount == nil {
			groupAccount = &AccountFinancialAccountReport{ID: account.ID, Name: account.Name, Type: account.Type, Platform: account.Platform, Historical: account.Historical}
			group.accounts[row.AccountID] = groupAccount
			group.report.Accounts = append(group.report.Accounts, groupAccount)
		}
		add(&report.Summary, row)
		add(&account.Amounts, row)
		add(&group.report.Amounts, row)
		add(&groupAccount.Amounts, row)
	}
	if !snap.ProbeDataError {
		initializeProbeAmounts(&report.Summary)
		for _, account := range report.Accounts {
			initializeProbeAmounts(&account.Amounts)
		}
		for _, group := range report.Groups {
			initializeProbeAmounts(&group.Amounts)
			for _, account := range group.Accounts {
				initializeProbeAmounts(&account.Amounts)
			}
		}
		for _, row := range snap.ProbeRows {
			identityRow := AccountFinancialUsageRow{GroupID: row.GroupID, AccountID: row.AccountID}
			account := accountForRow(identityRow)
			group := groupForRow(identityRow)
			groupAccount := group.accounts[row.AccountID]
			if groupAccount == nil {
				groupAccount = &AccountFinancialAccountReport{ID: account.ID, Name: account.Name, Type: account.Type, Platform: account.Platform, Historical: account.Historical}
				initializeProbeAmounts(&groupAccount.Amounts)
				group.accounts[row.AccountID] = groupAccount
				group.report.Accounts = append(group.report.Accounts, groupAccount)
			}
			initializeProbeAmountsIfUnset(&account.Amounts)
			initializeProbeAmountsIfUnset(&group.report.Amounts)
			addProbeAmounts(&report.Summary, row)
			addProbeAmounts(&account.Amounts, row)
			addProbeAmounts(&group.report.Amounts, row)
			addProbeAmounts(&groupAccount.Amounts, row)
		}
	}
	for _, account := range report.Accounts {
		finalizeFinancialAmounts(&account.Amounts)
	}
	report.Groups = append(report.Groups, historicalGroups...)
	if unassigned != nil {
		report.Groups = append(report.Groups, unassigned.report)
	}
	if !snap.ProbeDataError {
		for _, account := range report.Accounts {
			initializeProbeAmountsIfUnset(&account.Amounts)
		}
		for _, group := range report.Groups {
			initializeProbeAmountsIfUnset(&group.Amounts)
			for _, account := range group.Accounts {
				initializeProbeAmountsIfUnset(&account.Amounts)
			}
		}
	}
	for _, group := range report.Groups {
		for _, account := range group.Accounts {
			finalizeFinancialAmounts(&account.Amounts)
		}
		finalizeFinancialAmounts(&group.Amounts)
	}
	finalizeFinancialAmounts(&report.Summary)
	return report, nil
}

func initializeProbeAmounts(amounts *FinancialAmounts) {
	zero := int64(0)
	zeroCost := decimal.Zero
	status := "unavailable"
	amounts.ProbeRequests = &zero
	amounts.ProbeTokens = &zero
	amounts.ProbeCost = &zeroCost
	amounts.ProbeCostStatus = &status
}

func initializeProbeAmountsIfUnset(amounts *FinancialAmounts) {
	if amounts.ProbeCostStatus == nil {
		initializeProbeAmounts(amounts)
	}
}

func addProbeAmounts(amounts *FinancialAmounts, row AccountProbeCostAggregate) {
	initializeProbeAmountsIfUnset(amounts)
	requests := *amounts.ProbeRequests + row.ProbeRequests
	tokens := *amounts.ProbeTokens + row.ProbeTokens
	amounts.ProbeRequests = &requests
	amounts.ProbeTokens = &tokens
	if *amounts.ProbeCostStatus == "incomplete" || row.HasIncompleteCost || row.ProbeCost == nil {
		status := "incomplete"
		amounts.ProbeCost = nil
		amounts.ProbeCostStatus = &status
		return
	}
	cost := amounts.ProbeCost.Add(*row.ProbeCost)
	status := "confirmed"
	amounts.ProbeCost = &cost
	amounts.ProbeCostStatus = &status
}

func financialFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func financialAccountName(name string, id int64) string {
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("账号 #%d", id)
	}
	return name
}

func financialGroupName(name string, id int64) string {
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("分组 #%d", id)
	}
	return name
}

func validateMoney(v *float64) error {
	if v == nil {
		return nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return ErrFinancialInvalidAmount
	}
	return nil
}
func ValidateFinancialAmount(v *float64) error { return validateMoney(v) }
func (s *AccountFinancialService) ReviewOne(ctx context.Context, in UsageCostReviewInput) (*UsageCostReviewResult, error) {
	if s.audit == nil {
		return nil, ErrFinancialAuditRequired
	}
	if err := validateMoney(in.ManualCostCNY); err != nil {
		s.auditMutation(ctx, "admin.account_financial.review", in.ReviewedBy, in.RequestID, 0, "", nil, in.ManualCostCNY, 0, map[string]int64{"failed": 1})
		return nil, err
	}
	res, err := s.repo.CreateReview(ctx, in)
	if err == nil {
		s.auditMutation(ctx, "admin.account_financial.review", in.ReviewedBy, in.RequestID, res.AccountID, res.BusinessDate, res.OldManualCostCNY, &res.ManualCostCNY, 0, map[string]int64{"updated": boolInt64(res.Created), "skipped": boolInt64(!res.Created)})
	} else {
		s.auditMutation(ctx, "admin.account_financial.review", in.ReviewedBy, in.RequestID, 0, "", nil, in.ManualCostCNY, 0, map[string]int64{"failed": 1})
	}
	return res, err
}
func (s *AccountFinancialService) ListExceptions(ctx context.Context, filter ReviewFilter) (*AccountFinancialExceptionList, error) {
	now := s.now()
	q := AccountFinancialSnapshotQuery{GeneratedAt: now}
	if filter.From != nil {
		q.From = *filter.From
	}
	if filter.To != nil {
		q.To = *filter.To
	}
	snap, err := s.repo.ReadSnapshot(ctx, q)
	if err != nil {
		return nil, err
	}
	accounts := map[int64]AccountFinancialSnapshotAccount{}
	for _, a := range snap.Accounts {
		accounts[a.ID] = a
	}
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	out := &AccountFinancialExceptionList{GeneratedAt: snap.GeneratedAt, Page: page, PageSize: pageSize}
	for _, e := range snap.Entries {
		if filter.AccountID != nil && e.AccountID != *filter.AccountID {
			continue
		}
		accountType := e.AccountType
		if a, ok := accounts[e.AccountID]; ok {
			accountType = a.Type
		}
		if accountType == "oauth" || e.EvidenceStatus == "confirmed" {
			continue
		}
		reason := e.ReasonCode
		if e.EvidenceID == nil {
			reason = "evidence_not_registered"
		}
		if filter.EvidenceStatus != "" && filter.EvidenceStatus != e.EvidenceStatus {
			continue
		}
		reviewStatus := "pending"
		if e.ReviewID != nil {
			reviewStatus = "reviewed"
		}
		if filter.ReviewStatus != "" && filter.ReviewStatus != reviewStatus {
			continue
		}
		if filter.Search != "" && !strings.Contains(strings.ToLower(e.RequestID+" "+e.Model), strings.ToLower(filter.Search)) {
			continue
		}
		accountName := e.AccountName
		if a, ok := accounts[e.AccountID]; ok {
			accountName = a.Name
			accountType = a.Type
		}
		out.Items = append(out.Items, AccountFinancialException{UsageLogID: e.UsageLogID, AccountID: e.AccountID, AccountName: accountName, AccountType: accountType, RequestID: e.RequestID, Model: e.Model, Source: e.Source, UpstreamRequestID: e.UpstreamRequestID, UpstreamBillingTime: e.UpstreamBillingTime, UpstreamModel: e.UpstreamModel, CreatedAt: e.CreatedAt, RevenueCNY: e.RevenueCNY, EvidenceStatus: e.EvidenceStatus, ReasonCode: reason, ReviewStatus: reviewStatus, CostTrace: AccountFinancialCostTrace{SubActualCost: e.SubActualCost, NewAPIQuota: e.NewAPIQuota, NewAPIQuotaPerUnit: e.NewAPIQuotaPerUnit, NormalizedCostCNY: e.EvidenceCostCNY}})
	}
	out.Total = len(out.Items)
	start := (page - 1) * pageSize
	if start >= len(out.Items) {
		out.Items = nil
	} else {
		end := start + pageSize
		if end > len(out.Items) {
			end = len(out.Items)
		}
		out.Items = out.Items[start:end]
	}
	return out, nil
}
func (s *AccountFinancialService) ReviewSelected(ctx context.Context, in []UsageCostReviewInput) ([]UsageCostReviewResult, error) {
	if s.audit == nil {
		return nil, ErrFinancialAuditRequired
	}
	out := make([]UsageCostReviewResult, 0, len(in))
	committedInputs := make([]UsageCostReviewInput, 0, len(in))
	for _, x := range in {
		if err := validateMoney(x.ManualCostCNY); err != nil {
			for i, committed := range out {
				input := committedInputs[i]
				s.auditMutation(ctx, "admin.account_financial.review_selected", input.ReviewedBy, input.RequestID, committed.AccountID, committed.BusinessDate, committed.OldManualCostCNY, &committed.ManualCostCNY, 0, map[string]int64{"updated": boolInt64(committed.Created), "skipped": boolInt64(!committed.Created)})
			}
			s.auditMutation(ctx, "admin.account_financial.review_selected", x.ReviewedBy, x.RequestID, 0, "", nil, x.ManualCostCNY, 0, map[string]int64{"failed": 1})
			return nil, err
		}
		r, e := s.repo.CreateReview(ctx, x)
		if e != nil {
			for i, committed := range out {
				input := committedInputs[i]
				s.auditMutation(ctx, "admin.account_financial.review_selected", input.ReviewedBy, input.RequestID, committed.AccountID, committed.BusinessDate, committed.OldManualCostCNY, &committed.ManualCostCNY, 0, map[string]int64{"updated": boolInt64(committed.Created), "skipped": boolInt64(!committed.Created)})
			}
			s.auditMutation(ctx, "admin.account_financial.review_selected", x.ReviewedBy, x.RequestID, 0, "", nil, x.ManualCostCNY, 0, map[string]int64{"failed": 1})
			return nil, e
		}
		out = append(out, *r)
		committedInputs = append(committedInputs, x)
	}
	if len(in) > 0 {
		for i, r := range out {
			input := committedInputs[i]
			s.auditMutation(ctx, "admin.account_financial.review_selected", input.ReviewedBy, input.RequestID, r.AccountID, r.BusinessDate, r.OldManualCostCNY, &r.ManualCostCNY, 0, map[string]int64{"updated": boolInt64(r.Created), "skipped": boolInt64(!r.Created)})
		}
	}
	return out, nil
}
func (s *AccountFinancialService) ReviewFiltered(ctx context.Context, in ReviewFilteredInput) (*ReviewFilteredResult, error) {
	if s.audit == nil {
		return nil, ErrFinancialAuditRequired
	}
	if err := validateMoney(in.ManualCostCNY); err != nil {
		s.auditMutation(ctx, "admin.account_financial.review_filtered", in.ReviewedBy, in.RequestID, 0, "", nil, in.ManualCostCNY, in.MaxUsageLogID, map[string]int64{"failed": 1})
		return nil, err
	}
	res, err := s.repo.ReviewFiltered(ctx, in)
	if err == nil {
		for _, r := range res.Reviews {
			s.auditMutation(ctx, "admin.account_financial.review_filtered", in.ReviewedBy, in.RequestID, r.AccountID, r.BusinessDate, r.OldManualCostCNY, &r.ManualCostCNY, res.Cutoff, map[string]int64{"updated": boolInt64(r.Created), "skipped": boolInt64(!r.Created)})
		}
	}
	if err != nil {
		s.auditMutation(ctx, "admin.account_financial.review_filtered", in.ReviewedBy, in.RequestID, 0, "", nil, in.ManualCostCNY, in.MaxUsageLogID, map[string]int64{"failed": 1})
	}
	return res, err
}
func (s *AccountFinancialService) SetOAuthDailyCost(ctx context.Context, in OAuthDailyCostInput) (*FinancialMutationResult, error) {
	if s.audit == nil {
		return nil, ErrFinancialAuditRequired
	}
	if err := validateMoney(in.CostCNY); err != nil {
		s.auditMutation(ctx, "admin.account_financial.oauth_cost", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, in.CostCNY, 0, map[string]int64{"failed": 1})
		return nil, err
	}
	res, err := s.repo.SetOAuthDailyCost(ctx, in)
	if err == nil {
		s.auditMutation(ctx, "admin.account_financial.oauth_cost", in.ActorUserID, in.RequestID, res.AccountID, res.BusinessDate, res.OldValue, res.NewValue, 0, nil)
	}
	if err != nil {
		s.auditMutation(ctx, "admin.account_financial.oauth_cost", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, in.CostCNY, 0, map[string]int64{"failed": 1})
	}
	return res, err
}
func (s *AccountFinancialService) SetTodayOverride(ctx context.Context, in TodayOverrideInput) (*FinancialMutationResult, error) {
	if s.audit == nil {
		return nil, ErrFinancialAuditRequired
	}
	if err := validateMoney(in.RevenueCNY); err != nil {
		s.auditMutation(ctx, "admin.account_financial.override", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, in.RevenueCNY, 0, map[string]int64{"failed": 1})
		return nil, err
	}
	if err := validateMoney(in.CostCNY); err != nil {
		s.auditMutation(ctx, "admin.account_financial.override", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, in.CostCNY, 0, map[string]int64{"failed": 1})
		return nil, err
	}
	if in.RevenueCNY != nil && in.CostCNY != nil {
		s.auditMutation(ctx, "admin.account_financial.override", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, nil, 0, map[string]int64{"failed": 1})
		return nil, errors.New("set one override dimension per mutation")
	}
	res, err := s.repo.SetTodayOverride(ctx, in)
	if err == nil {
		s.auditMutationWithKind(ctx, "admin.account_financial.override", in.ActorUserID, in.RequestID, res.AccountID, res.BusinessDate, res.OldValue, res.NewValue, res.CutoffEvidenceID, res.MutationKind, map[string]int64{"review_cutoff": res.CutoffReviewID})
	}
	if err != nil {
		value := in.RevenueCNY
		if in.CostCNY != nil {
			value = in.CostCNY
		}
		s.auditMutation(ctx, "admin.account_financial.override", in.ActorUserID, in.RequestID, in.AccountID, in.BusinessDate, nil, value, 0, map[string]int64{"failed": 1})
	}
	return res, err
}
func boolInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func (s *AccountFinancialService) auditMutation(ctx context.Context, action string, actor int64, request string, account int64, day string, old, new *float64, cutoff int64, result map[string]int64) {
	s.auditMutationWithKind(ctx, action, actor, request, account, day, old, new, cutoff, "", result)
}
func (s *AccountFinancialService) auditMutationWithKind(ctx context.Context, action string, actor int64, request string, account int64, day string, old, new *float64, cutoff int64, kind string, result map[string]int64) {
	if s.audit != nil {
		s.audit.Record(ctx, AccountFinancialAuditEvent{Action: action, ActorUserID: actor, RequestID: request, AccountID: account, BusinessDate: day, OldValue: old, NewValue: new, Cutoff: cutoff, MutationKind: kind, Result: result})
	}
}
func (s *AccountFinancialService) GetUsageEvidence(ctx context.Context, id int64) (*UsageFinancialEvidence, error) {
	return s.repo.GetUsageEvidence(ctx, id)
}
func validateToday(day string, now time.Time) error {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if day != now.In(loc).Format("2006-01-02") {
		return ErrFinancialNotToday
	}
	return nil
}
func ValidateFinancialToday(day string, now time.Time) error { return validateToday(day, now) }
func validateOAuthType(t string) error {
	if t != "oauth" {
		return ErrFinancialOAuthType
	}
	return nil
}
func ValidateFinancialOAuthType(t string) error { return validateOAuthType(t) }

var _ = fmt.Sprintf
