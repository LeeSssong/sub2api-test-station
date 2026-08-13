package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrFinancialInvalidAmount = errors.New("financial amount must be finite and non-negative")
	ErrFinancialNotToday      = errors.New("financial value may only be written for today")
	ErrFinancialOAuthType     = errors.New("daily oauth cost requires literal oauth account type")
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
}

type AccountFinancialAccountReport struct {
	ID                 int64
	Name               string
	Type               string
	Platform           string
	GeneratedAt        time.Time
	Complete           bool
	Amounts            FinancialAmounts
	ExceptionCount     int
	AffectedRevenueCNY float64
}

type AccountFinancialSnapshotQuery struct{ GeneratedAt, From, To time.Time }
type AccountFinancialSnapshot struct {
	GeneratedAt    time.Time
	EnabledAt      time.Time
	Accounts       []AccountFinancialSnapshotAccount
	Entries        []AccountFinancialSnapshotEntry
	DailyValues    []AccountFinancialDailyValue
	UserBalanceCNY float64
}
type AccountFinancialSnapshotAccount struct {
	ID                   int64
	Name, Type, Platform string
}
type AccountFinancialSnapshotEntry struct {
	UsageLogID, AccountID int64
	CreatedAt             time.Time
	BusinessDate          string
	RevenueCNY            float64
	EvidenceID            *int64
	EvidenceStatus        string
	EvidenceCostCNY       *float64
	ReviewID              *int64
	ReviewCostCNY         *float64
}
type AccountFinancialDailyValue struct {
	AccountID                                      int64
	BusinessDate                                   string
	OAuthCostCNY                                   *float64
	RevenueOverrideCNY                             *float64
	RevenueEvidenceCutoffID, RevenueReviewCutoffID *int64
	CostOverrideCNY                                *float64
	CostEvidenceCutoffID, CostReviewCutoffID       *int64
}
type UsageFinancialEvidence struct {
	UsageLogID                 int64
	EvidenceStatus, ReasonCode string
	NormalizedCostCNY          *float64
	ReviewID                   *int64
	ReviewCostCNY              *float64
}
type AccountFinancialException struct {
	UsageLogID, AccountID      int64
	CreatedAt                  time.Time
	RevenueCNY                 float64
	EvidenceStatus, ReasonCode string
}
type AccountFinancialExceptionList struct {
	GeneratedAt time.Time
	Items       []AccountFinancialException
	Total       int
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
	ManualCostCNY, ManualProfitCNY float64
}
type ReviewFilter struct {
	AccountID *int64
	From, To  *time.Time
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
	repo AccountFinancialRepository
	now  func() time.Time
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
	start := localNow.Truncate(24 * time.Hour)
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
	for _, e := range snap.Entries {
		if !entryInRange(e, r, now, localNow) {
			continue
		}
		a := byID[e.AccountID]
		if a == nil {
			continue
		}
		if a.Type == "oauth" {
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
			}
			continue
		}
		a.Amounts.RevenueCNY += e.RevenueCNY
		a.Amounts.CostCNY += cost
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
		}
	}
	for _, a := range report.Accounts {
		if a.Type == "oauth" {
			if r == AccountFinancialRangeToday {
				d := dailyFor(snap.DailyValues, a.ID, localNow.Format("2006-01-02"))
				if d == nil || d.OAuthCostCNY == nil {
					a.Complete = false
					a.Amounts = FinancialAmounts{}
				} else {
					a.Amounts.CostCNY = *d.OAuthCostCNY
				}
			} else if r == AccountFinancialRange7D || r == AccountFinancialRange31D {
				a.Amounts = FinancialAmounts{}
				seen := map[string]bool{}
				for _, e := range snap.Entries {
					if e.AccountID != a.ID || !entryInRange(e, r, now, localNow) {
						continue
					}
					d := dailyFor(snap.DailyValues, a.ID, e.BusinessDate)
					if d == nil || d.OAuthCostCNY == nil {
						a.Complete = false
						continue
					}
					a.Amounts.RevenueCNY += e.RevenueCNY
					if !seen[e.BusinessDate] {
						a.Amounts.CostCNY += *d.OAuthCostCNY
						seen[e.BusinessDate] = true
					}
				}
			} else if r == AccountFinancialRange24H {
				a.Complete = false
			}
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
	if report.Summary.RevenueCNY != 0 {
		v := report.Summary.ProfitCNY / report.Summary.RevenueCNY
		report.Summary.Margin = &v
	}
	return report, nil
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
	if err := validateMoney(in.ManualCostCNY); err != nil {
		return nil, err
	}
	return s.repo.CreateReview(ctx, in)
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
	types := map[int64]string{}
	for _, a := range snap.Accounts {
		types[a.ID] = a.Type
	}
	out := &AccountFinancialExceptionList{GeneratedAt: snap.GeneratedAt}
	for _, e := range snap.Entries {
		if filter.AccountID != nil && e.AccountID != *filter.AccountID {
			continue
		}
		if types[e.AccountID] == "oauth" || e.ReviewID != nil || e.EvidenceStatus == "confirmed" {
			continue
		}
		reason := ""
		if e.EvidenceID == nil {
			reason = "evidence_not_registered"
		}
		out.Items = append(out.Items, AccountFinancialException{UsageLogID: e.UsageLogID, AccountID: e.AccountID, CreatedAt: e.CreatedAt, RevenueCNY: e.RevenueCNY, EvidenceStatus: e.EvidenceStatus, ReasonCode: reason})
	}
	out.Total = len(out.Items)
	return out, nil
}
func (s *AccountFinancialService) ReviewSelected(ctx context.Context, in []UsageCostReviewInput) ([]UsageCostReviewResult, error) {
	out := make([]UsageCostReviewResult, 0, len(in))
	for _, x := range in {
		r, e := s.ReviewOne(ctx, x)
		if e != nil {
			return nil, e
		}
		out = append(out, *r)
	}
	return out, nil
}
func (s *AccountFinancialService) ReviewFiltered(ctx context.Context, in ReviewFilteredInput) (*ReviewFilteredResult, error) {
	if err := validateMoney(in.ManualCostCNY); err != nil {
		return nil, err
	}
	if in.MaxUsageLogID == 0 {
		cutoff, err := s.repo.FreezeReviewFilter(ctx, in.Filter)
		if err != nil {
			return nil, err
		}
		in.MaxUsageLogID = cutoff
	}
	return s.repo.ReviewFiltered(ctx, in)
}
func (s *AccountFinancialService) SetOAuthDailyCost(ctx context.Context, in OAuthDailyCostInput) (*FinancialMutationResult, error) {
	if err := validateMoney(in.CostCNY); err != nil {
		return nil, err
	}
	return s.repo.SetOAuthDailyCost(ctx, in)
}
func (s *AccountFinancialService) SetTodayOverride(ctx context.Context, in TodayOverrideInput) (*FinancialMutationResult, error) {
	if err := validateMoney(in.RevenueCNY); err != nil {
		return nil, err
	}
	if err := validateMoney(in.CostCNY); err != nil {
		return nil, err
	}
	return s.repo.SetTodayOverride(ctx, in)
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
