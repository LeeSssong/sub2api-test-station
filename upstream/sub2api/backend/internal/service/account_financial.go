package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrFinancialInvalidAmount     = errors.New("financial amount must be finite and non-negative")
	ErrFinancialNotToday          = errors.New("financial value may only be written for today")
	ErrFinancialOAuthType         = errors.New("daily oauth cost requires literal oauth account type")
	ErrFinancialReviewNotEligible = errors.New("usage is not a pending financial exception")
	ErrFinancialAuditRequired     = errors.New("financial audit dependency is required")
)

type AccountFinancialRange string

const (
	AccountFinancialRangeToday AccountFinancialRange = "today"
	AccountFinancialRange24H   AccountFinancialRange = "24h"
	AccountFinancialRange7D    AccountFinancialRange = "7d"
	AccountFinancialRange31D   AccountFinancialRange = "31d"
)

type FinancialAmounts struct {
	RevenueCNY         float64
	CostCNY            float64
	ProfitCNY          float64
	Margin             *float64
	ExceptionCount     int
	AffectedRevenueCNY float64
}

type AccountFinancialReport struct {
	GeneratedAt        time.Time
	Range              AccountFinancialRange
	Summary            FinancialAmounts
	Accounts           []*AccountFinancialAccountReport
	ExceptionCount     int
	AffectedRevenueCNY float64
	UserBalanceCNY     float64
	Groups             []*AccountFinancialGroupReport
}

type AccountFinancialAccountReport struct {
	ID                        int64
	Name                      string
	Type                      string
	Platform                  string
	GeneratedAt               time.Time
	Complete                  bool
	Amounts                   FinancialAmounts
	ExceptionCount            int
	AffectedRevenueCNY        float64
	HasUnallocatedAdjustments bool
}

type AccountFinancialGroupReport struct {
	ID                        int64
	Name                      string
	Unassigned                bool
	Complete                  bool
	HasUnallocatedAdjustments bool
	Amounts                   FinancialAmounts
	Accounts                  []*AccountFinancialAccountReport
	ExceptionCount            int
	AffectedRevenueCNY        float64
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
	repo  AccountFinancialRepository
	now   func() time.Time
	audit *AccountFinancialAudit
}

func NewAccountFinancialServiceWithAudit(repo AccountFinancialRepository, now func() time.Time, audit *AccountFinancialAudit) *AccountFinancialService {
	if now == nil {
		now = time.Now
	}
	return &AccountFinancialService{repo: repo, now: now, audit: audit}
}

func NewAccountFinancialService(repo AccountFinancialRepository, now func() time.Time) *AccountFinancialService {
	if now == nil {
		now = time.Now
	}
	return &AccountFinancialService{repo: repo, now: now}
}

func (s *AccountFinancialService) GetReport(ctx context.Context, r AccountFinancialRange) (*AccountFinancialReport, error) {
	now := s.now()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(loc)
	q := AccountFinancialSnapshotQuery{GeneratedAt: now}
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	switch r {
	case AccountFinancialRange24H:
		q.From = now.Add(-24 * time.Hour)
		q.To = now
	case AccountFinancialRange7D:
		start = time.Date(localNow.Year(), localNow.Month(), localNow.Day()-6, 0, 0, 0, 0, loc)
		q.From, q.To = start, now
	case AccountFinancialRange31D:
		start = time.Date(localNow.Year(), localNow.Month(), localNow.Day()-30, 0, 0, 0, 0, loc)
		q.From, q.To = start, now
	default:
		q.From, q.To = start, now
		r = AccountFinancialRangeToday
	}
	snap, err := s.repo.ReadSnapshot(ctx, q)
	if err != nil {
		return nil, err
	}
	report := &AccountFinancialReport{GeneratedAt: snap.GeneratedAt, Range: r, UserBalanceCNY: snap.UserBalanceCNY}
	byID := make(map[int64]*AccountFinancialAccountReport, len(snap.Accounts))
	for _, a := range snap.Accounts {
		x := &AccountFinancialAccountReport{ID: a.ID, Name: a.Name, Type: a.Type, Platform: a.Platform, GeneratedAt: snap.GeneratedAt, Complete: true}
		byID[a.ID] = x
		report.Accounts = append(report.Accounts, x)
	}
	type groupAccumulator struct {
		report   *AccountFinancialGroupReport
		accounts map[int64]*AccountFinancialAccountReport
	}
	groupByID := make(map[int64]*groupAccumulator, len(snap.Groups))
	for _, group := range snap.Groups {
		g := &AccountFinancialGroupReport{ID: group.ID, Name: group.Name, Complete: true}
		groupByID[group.ID] = &groupAccumulator{report: g, accounts: make(map[int64]*AccountFinancialAccountReport)}
		report.Groups = append(report.Groups, g)
	}
	var historicalGroups []*AccountFinancialGroupReport
	var unassignedGroup *AccountFinancialGroupReport
	ensureGroupAccount := func(e AccountFinancialSnapshotEntry, account *AccountFinancialAccountReport) (*AccountFinancialGroupReport, *AccountFinancialAccountReport) {
		var accumulator *groupAccumulator
		if e.GroupID == nil {
			accumulator = groupByID[0]
			if accumulator == nil {
				unassignedGroup = &AccountFinancialGroupReport{Unassigned: true, Complete: true}
				accumulator = &groupAccumulator{report: unassignedGroup, accounts: make(map[int64]*AccountFinancialAccountReport)}
				groupByID[0] = accumulator
			}
		} else {
			accumulator = groupByID[*e.GroupID]
			if accumulator == nil {
				group := &AccountFinancialGroupReport{ID: *e.GroupID, Name: e.GroupName, Complete: true}
				accumulator = &groupAccumulator{report: group, accounts: make(map[int64]*AccountFinancialAccountReport)}
				groupByID[*e.GroupID] = accumulator
				historicalGroups = append(historicalGroups, group)
			}
		}
		groupAccount := accumulator.accounts[account.ID]
		if groupAccount == nil {
			groupAccount = &AccountFinancialAccountReport{ID: account.ID, Name: account.Name, Type: account.Type, Platform: account.Platform, GeneratedAt: snap.GeneratedAt, Complete: true}
			accumulator.accounts[account.ID] = groupAccount
			accumulator.report.Accounts = append(accumulator.report.Accounts, groupAccount)
		}
		return accumulator.report, groupAccount
	}
	for _, e := range snap.Entries {
		if !entryInRange(e, r, now, localNow) {
			continue
		}
		a := byID[e.AccountID]
		if a == nil {
			continue
		}
		group, groupAccount := ensureGroupAccount(e, a)
		if a.Type == "oauth" {
			group.Complete = false
			group.HasUnallocatedAdjustments = true
			groupAccount.Complete = false
			groupAccount.HasUnallocatedAdjustments = true
			continue
		}
		include, cost := includeEntry(e)
		if !include {
			if e.EvidenceStatus != "" && e.ReviewID == nil {
				a.Complete = false
				a.ExceptionCount++
				a.AffectedRevenueCNY += e.RevenueCNY
				a.Amounts.ExceptionCount++
				a.Amounts.AffectedRevenueCNY += e.RevenueCNY
				report.ExceptionCount++
				report.AffectedRevenueCNY += e.RevenueCNY
				report.Summary.ExceptionCount++
				report.Summary.AffectedRevenueCNY += e.RevenueCNY
				group.Complete = false
				group.ExceptionCount++
				group.AffectedRevenueCNY += e.RevenueCNY
				group.Amounts.ExceptionCount++
				group.Amounts.AffectedRevenueCNY += e.RevenueCNY
				groupAccount.Complete = false
				groupAccount.ExceptionCount++
				groupAccount.AffectedRevenueCNY += e.RevenueCNY
				groupAccount.Amounts.ExceptionCount++
				groupAccount.Amounts.AffectedRevenueCNY += e.RevenueCNY
			}
			continue
		}
		a.Amounts.RevenueCNY += e.RevenueCNY
		a.Amounts.CostCNY += cost
		group.Amounts.RevenueCNY += e.RevenueCNY
		group.Amounts.CostCNY += cost
		groupAccount.Amounts.RevenueCNY += e.RevenueCNY
		groupAccount.Amounts.CostCNY += cost
	}
	if r != AccountFinancialRange24H {
		for _, d := range snap.DailyValues {
			if !dailyValueInRange(d.BusinessDate, r, localNow) {
				continue
			}
			a := byID[d.AccountID]
			if a == nil {
				continue
			}
			applyDailyOverride(a, d, snap.Entries, r == AccountFinancialRangeToday)
			if d.OAuthCostCNY == nil && d.RevenueOverrideCNY == nil && d.CostOverrideCNY == nil {
				continue
			}
			for _, e := range snap.Entries {
				if e.AccountID != d.AccountID || e.BusinessDate != d.BusinessDate || !entryInRange(e, r, now, localNow) {
					continue
				}
				group, groupAccount := ensureGroupAccount(e, a)
				group.Complete = false
				group.HasUnallocatedAdjustments = true
				groupAccount.Complete = false
				groupAccount.HasUnallocatedAdjustments = true
			}
		}
	}
	for _, a := range report.Accounts {
		if a.Type == "oauth" {
			a.Amounts, a.Complete = aggregateOAuthAccount(a.ID, r, now, localNow, snap)
		}
		a.Amounts.ProfitCNY = a.Amounts.RevenueCNY - a.Amounts.CostCNY
		if a.Amounts.RevenueCNY != 0 {
			v := a.Amounts.ProfitCNY / a.Amounts.RevenueCNY
			a.Amounts.Margin = &v
		}
		report.Summary.RevenueCNY += a.Amounts.RevenueCNY
		report.Summary.CostCNY += a.Amounts.CostCNY
		report.Summary.ProfitCNY += a.Amounts.ProfitCNY
	}
	report.Groups = append(report.Groups, historicalGroups...)
	if unassignedGroup != nil {
		report.Groups = append(report.Groups, unassignedGroup)
	}
	for _, group := range report.Groups {
		for _, account := range group.Accounts {
			account.Amounts.ProfitCNY = account.Amounts.RevenueCNY - account.Amounts.CostCNY
			if account.Amounts.RevenueCNY != 0 {
				margin := account.Amounts.ProfitCNY / account.Amounts.RevenueCNY
				account.Amounts.Margin = &margin
			}
		}
		group.Amounts.ProfitCNY = group.Amounts.RevenueCNY - group.Amounts.CostCNY
		if group.Amounts.RevenueCNY != 0 {
			margin := group.Amounts.ProfitCNY / group.Amounts.RevenueCNY
			group.Amounts.Margin = &margin
		}
	}
	if report.Summary.RevenueCNY != 0 {
		v := report.Summary.ProfitCNY / report.Summary.RevenueCNY
		report.Summary.Margin = &v
	}
	return report, nil
}
func aggregateOAuthAccount(id int64, r AccountFinancialRange, now, localNow time.Time, snap *AccountFinancialSnapshot) (FinancialAmounts, bool) {
	if r == AccountFinancialRange24H {
		return FinancialAmounts{}, false
	}
	revenueByDay := map[string]float64{}
	for _, e := range snap.Entries {
		if e.AccountID == id && entryInRange(e, r, now, localNow) {
			revenueByDay[e.BusinessDate] += e.RevenueCNY
		}
	}
	out := FinancialAmounts{}
	complete := true
	for day, revenue := range revenueByDay {
		d := dailyFor(snap.DailyValues, id, day)
		if d == nil || d.OAuthCostCNY == nil {
			complete = false
			continue
		}
		cost := *d.OAuthCostCNY
		if d.RevenueOverrideCNY != nil {
			revenue = *d.RevenueOverrideCNY + sumOAuthRevenueAfter(snap.Entries, id, day, d.RevenueOverrideAt)
		}
		if d.CostOverrideCNY != nil {
			cost = *d.CostOverrideCNY
		}
		out.RevenueCNY += revenue
		out.CostCNY += cost
	}
	return out, complete
}
func sumOAuthRevenueAfter(entries []AccountFinancialSnapshotEntry, accountID int64, day string, cutoff *time.Time) float64 {
	if cutoff == nil {
		return 0
	}
	var total float64
	for _, e := range entries {
		if e.AccountID == accountID && e.BusinessDate == day && e.CreatedAt.After(*cutoff) {
			total += e.RevenueCNY
		}
	}
	return total
}
func dailyValueInRange(day string, r AccountFinancialRange, now time.Time) bool {
	parsed, err := time.ParseInLocation("2006-01-02", day, now.Location())
	if err != nil {
		return false
	}
	switch r {
	case AccountFinancialRangeToday:
		return day == now.Format("2006-01-02")
	case AccountFinancialRange7D:
		return !parsed.Before(time.Date(now.Year(), now.Month(), now.Day()-6, 0, 0, 0, 0, now.Location())) && !parsed.After(now)
	case AccountFinancialRange31D:
		return !parsed.Before(time.Date(now.Year(), now.Month(), now.Day()-30, 0, 0, 0, 0, now.Location())) && !parsed.After(now)
	default:
		return true
	}
}
func entryInRange(e AccountFinancialSnapshotEntry, r AccountFinancialRange, now time.Time, localNow time.Time) bool {
	switch r {
	case AccountFinancialRange24H:
		return !e.CreatedAt.IsZero() && !e.CreatedAt.Before(now.Add(-24*time.Hour)) && !e.CreatedAt.After(now)
	case AccountFinancialRangeToday:
		return e.BusinessDate == localNow.Format("2006-01-02")
	case AccountFinancialRange7D, AccountFinancialRange31D:
		start := 6
		if r == AccountFinancialRange31D {
			start = 30
		}
		first := time.Date(localNow.Year(), localNow.Month(), localNow.Day()-start, 0, 0, 0, 0, localNow.Location())
		day, err := time.ParseInLocation("2006-01-02", e.BusinessDate, localNow.Location())
		return err == nil && !day.Before(first) && !day.After(localNow)
	default:
		return true
	}
}
func includeEntry(e AccountFinancialSnapshotEntry) (bool, float64) {
	if e.ReviewID != nil {
		if e.ReviewCostCNY == nil {
			return true, 0
		}
		return true, *e.ReviewCostCNY
	}
	if e.EvidenceStatus == "confirmed" && e.EvidenceCostCNY != nil {
		return true, *e.EvidenceCostCNY
	}
	return false, 0
}
func applyDailyOverride(a *AccountFinancialAccountReport, d AccountFinancialDailyValue, entries []AccountFinancialSnapshotEntry, today bool) {
	dayRevenue, dayCost := 0.0, 0.0
	for _, e := range entries {
		if e.AccountID != a.ID || e.BusinessDate != d.BusinessDate {
			continue
		}
		include, cost := includeEntry(e)
		if !include {
			continue
		}
		if d.RevenueReviewCutoffID != nil && e.ReviewID != nil && *e.ReviewID <= *d.RevenueReviewCutoffID {
			dayRevenue += e.RevenueCNY
		} else if e.ReviewID == nil && d.RevenueEvidenceCutoffID != nil && e.EvidenceID != nil && *e.EvidenceID <= *d.RevenueEvidenceCutoffID {
			dayRevenue += e.RevenueCNY
		} else if d.RevenueOverrideCNY == nil {
			dayRevenue += e.RevenueCNY
		}
		if d.CostReviewCutoffID != nil && e.ReviewID != nil && *e.ReviewID <= *d.CostReviewCutoffID {
			dayCost += cost
		} else if e.ReviewID == nil && d.CostEvidenceCutoffID != nil && e.EvidenceID != nil && *e.EvidenceID <= *d.CostEvidenceCutoffID {
			dayCost += cost
		} else if d.CostOverrideCNY == nil {
			dayCost += cost
		}
	}
	if d.RevenueOverrideCNY != nil {
		a.Amounts.RevenueCNY += *d.RevenueOverrideCNY - dayRevenue
	}
	if d.CostOverrideCNY != nil {
		a.Amounts.CostCNY += *d.CostOverrideCNY - dayCost
	}
	_ = today
}
func dailyFor(v []AccountFinancialDailyValue, id int64, day string) *AccountFinancialDailyValue {
	for i := range v {
		if v[i].AccountID == id && v[i].BusinessDate == day {
			return &v[i]
		}
	}
	return nil
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
