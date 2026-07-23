package credits

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
)

type Service struct {
	Store               *store.Store
	Provider            sub2api.Client
	Timezone            *time.Location
	TotalBudget         domain.MicroUSD
	DailyLoginCredit    domain.MicroUSD
	CostMultiplierBPS   int64
	CostPolicyID        string
	CostPolicyQualified bool
	Mode                string
	mu                  sync.Mutex
}
type GrantResult struct {
	UserID         int64
	Kind           string
	Amount         domain.MicroUSD
	AlreadyApplied bool
	IdempotencyKey string
}
type UsageSyncResult struct {
	UserID          int64
	Records         int
	Successful      int
	ReferralRewards int
	LastUsageID     int64
}
type Reconciliation struct {
	UserID   int64
	Expected domain.MicroUSD
	Provider domain.MicroUSD
	Match    bool
}
type BudgetSnapshot struct {
	ActualProviderCost          domain.MicroUSD
	CurrentBalances             domain.MicroUSD
	PendingReferralReservations int
	Occupancy                   domain.MicroUSD
	Remaining                   domain.MicroUSD
}

func (s *Service) CanReserveReferral(ctx context.Context) (bool, error) {
	if err := s.requireQualifiedCostPolicy(); err != nil {
		return false, err
	}
	budget, err := s.Budget(ctx)
	if err != nil {
		return false, err
	}
	return budget.Occupancy+s.multiplierCost(domain.ReferralGrant) <= s.TotalBudget, nil
}

func (s *Service) CanGrantDailyLogin(ctx context.Context) (bool, error) {
	if err := s.requireQualifiedCostPolicy(); err != nil {
		return false, err
	}
	budget, err := s.Budget(ctx)
	if err != nil {
		return false, err
	}
	amount := s.DailyLoginCredit
	if amount == 0 {
		amount = domain.DailyLoginCredit
	}
	return budget.Occupancy+s.multiplierCost(amount) <= s.TotalBudget, nil
}

func (s *Service) multiplierCost(amount domain.MicroUSD) domain.MicroUSD {
	return domain.MicroUSD(int64(amount) * s.CostMultiplierBPS / 10_000)
}
func (s *Service) writable(ctx context.Context) error {
	if s.Mode != "write" {
		return errors.New("internal test credit writes are disabled")
	}
	if err := s.requireQualifiedCostPolicy(); err != nil {
		return err
	}
	if reason, _ := s.Store.GetReadOnlyReason(ctx); reason != "" {
		return fmt.Errorf("read-only: %s", reason)
	}
	return nil
}

func (s *Service) requireQualifiedCostPolicy() error {
	if !s.CostPolicyQualified || s.CostPolicyID == "" || s.CostMultiplierBPS <= 0 {
		return errors.New("qualified cost policy is required")
	}
	return nil
}

func (s *Service) CheckIn(ctx context.Context, userID int64, now time.Time) (GrantResult, error) {
	return s.grantDaily(
		ctx,
		userID,
		now,
		domain.GrantCheckin,
		domain.CheckinGrant,
		"d04-checkin",
		"D04 daily check-in",
		"uncertain check-in write",
	)
}

func (s *Service) GrantDailyLogin(ctx context.Context, userID int64, now time.Time) (GrantResult, error) {
	amount := s.DailyLoginCredit
	if amount == 0 {
		amount = domain.DailyLoginCredit
	}
	return s.grantDaily(
		ctx,
		userID,
		now,
		domain.GrantDailyLogin,
		amount,
		"d04-login",
		"D04 daily login credit",
		"uncertain daily login write",
	)
}

func (s *Service) grantDaily(ctx context.Context, userID int64, now time.Time, kind string, amount domain.MicroUSD, keyPrefix, note, uncertainReason string) (GrantResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	date := domain.ShanghaiDate(now, s.Timezone)
	key := fmt.Sprintf("%s-%d-%s", keyPrefix, userID, date)
	existing, err := s.Store.FindGrantByIdempotencyKey(ctx, key)
	if err == nil {
		switch existing.Status {
		case domain.TaskSucceeded:
			return GrantResult{UserID: userID, Kind: existing.Kind, Amount: existing.Amount, AlreadyApplied: true, IdempotencyKey: key}, nil
		case domain.TaskPending, domain.TaskUncertain:
			if s.confirmHistory(ctx, userID, key) {
				if err := s.Store.MarkGrantApplied(ctx, key, now); err != nil {
					return GrantResult{}, err
				}
				return GrantResult{UserID: userID, Kind: existing.Kind, Amount: existing.Amount, AlreadyApplied: true, IdempotencyKey: key}, nil
			}
			return GrantResult{}, fmt.Errorf("daily grant requires reconciliation")
		default:
			return GrantResult{}, fmt.Errorf("daily grant has unexpected status")
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return GrantResult{}, err
	}
	if err := s.writable(ctx); err != nil {
		return GrantResult{}, err
	}
	budget, err := s.Budget(ctx)
	if err != nil {
		return GrantResult{}, err
	}
	if budget.Occupancy+s.multiplierCost(amount) > s.TotalBudget {
		return GrantResult{}, errors.New("internal test budget full")
	}
	ok, err := s.Store.CreateGrant(ctx, store.Grant{UserID: userID, Kind: kind, Amount: amount, GrantDate: sql.NullString{String: date, Valid: true}, IdempotencyKey: key, Status: domain.TaskPending, CreatedAt: now})
	if err != nil {
		return GrantResult{}, err
	}
	if !ok {
		return GrantResult{}, fmt.Errorf("daily grant reservation requires reconciliation")
	}
	if err := s.Provider.AddBalance(ctx, userID, amount, key, note); err != nil {
		if s.confirmHistory(ctx, userID, key) {
			_ = s.Store.MarkGrantApplied(ctx, key, now)
			return GrantResult{UserID: userID, Kind: kind, Amount: amount, IdempotencyKey: key}, nil
		}
		_ = s.Store.SetReadOnlyReason(ctx, uncertainReason)
		return GrantResult{}, err
	}
	if err := s.Store.MarkGrantApplied(ctx, key, now); err != nil {
		return GrantResult{}, err
	}
	return GrantResult{UserID: userID, Kind: kind, Amount: amount, IdempotencyKey: key}, nil
}

func (s *Service) ProcessUsage(ctx context.Context, userID int64) (UsageSyncResult, error) {
	readOnly := s.Mode == "read_only"
	if !readOnly {
		if err := s.writable(ctx); err != nil {
			return UsageSyncResult{}, err
		}
	}
	after, err := s.Store.GetUsageCursor(ctx, userID)
	if err != nil {
		return UsageSyncResult{}, err
	}
	items, err := s.Provider.ListUsage(ctx, userID, after)
	if err != nil {
		return UsageSyncResult{}, err
	}
	result := UsageSyncResult{UserID: userID, LastUsageID: after}
	for _, item := range items {
		amount, parseErr := domain.ParseMicroUSD(item.AmountUSD)
		if parseErr != nil {
			return result, parseErr
		}
		result.Records++
		if item.Successful {
			result.Successful++
		}
		if item.ID > result.LastUsageID {
			result.LastUsageID = item.ID
		}
		if readOnly {
			continue
		}
		if err := s.Store.RecordUsage(ctx, store.UsageRecord{UsageID: item.ID, UserID: userID, Amount: amount, Successful: item.Successful, RecordedAt: time.Now()}); err != nil {
			return result, err
		}
		if item.Successful {
			if err := s.Store.MarkFirstUsage(ctx, userID, time.Now()); err != nil {
				return result, err
			}
		}
		if err := s.Store.SetUsageCursor(ctx, userID, item.ID, time.Now()); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) rewardReferral(ctx context.Context, inviteeID int64) (bool, error) {
	reservation, err := s.Store.FindReferralReservation(ctx, inviteeID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if reservation.Status == domain.TaskSucceeded {
		return false, nil
	}
	if !reservation.InviteeUserID.Valid {
		return false, nil
	}
	key := reservation.IdempotencyKey
	if s.confirmHistory(ctx, reservation.UserID, key) {
		if err := s.Store.MarkGrantApplied(ctx, key, time.Now()); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := s.Provider.AddBalance(ctx, reservation.UserID, domain.ReferralGrant, key, "D04 referral reward"); err != nil {
		if !s.confirmHistory(ctx, reservation.UserID, key) {
			_ = s.Store.SetReadOnlyReason(ctx, "uncertain referral write")
			return false, err
		}
	}
	if err := s.Store.MarkGrantApplied(ctx, key, time.Now()); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) confirmHistory(ctx context.Context, userID int64, key string) bool {
	b, err := s.Provider.GetBalance(ctx, userID)
	if err != nil {
		return false
	}
	for _, h := range b.History {
		if h.IdempotencyKey == key || strings.Contains(h.Notes, "idempotency_key="+key) {
			return true
		}
	}
	return false
}

func (s *Service) ReconcileUser(ctx context.Context, userID int64) (Reconciliation, error) {
	snap, err := s.Store.GetUserBalanceSnapshot(ctx, userID)
	if err != nil {
		return Reconciliation{}, err
	}
	provider, err := s.Provider.GetBalance(ctx, userID)
	if err != nil {
		return Reconciliation{}, err
	}
	expected := snap.GrantTotal - snap.UsageTotal
	result := Reconciliation{UserID: userID, Expected: expected, Provider: provider.Balance, Match: expected == provider.Balance}
	if !result.Match && s.Mode == "write" {
		_ = s.Store.SetReadOnlyReason(ctx, fmt.Sprintf("balance drift for user %d", userID))
	}
	return result, nil
}

func (s *Service) Budget(ctx context.Context) (BudgetSnapshot, error) {
	users, err := s.Store.ListInternalUsers(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	var balances domain.MicroUSD
	for _, u := range users {
		b, e := s.Provider.GetBalance(ctx, u.UserID)
		if e != nil {
			return BudgetSnapshot{}, e
		}
		balances += b.Balance
	}
	usage, err := s.Store.SumSuccessfulUsage(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	pending, err := s.Store.ListPendingReferralReservations(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	actual := s.multiplierCost(usage)
	occupancy := actual + s.multiplierCost(balances)
	remaining := s.TotalBudget - occupancy
	if remaining < 0 {
		remaining = 0
	}
	return BudgetSnapshot{ActualProviderCost: actual, CurrentBalances: balances, PendingReferralReservations: len(pending), Occupancy: occupancy, Remaining: remaining}, nil
}
