//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type balanceUserRepoStub struct {
	*userRepoStub
	adjustErr error
	// changes 记录每次原子余额变更，顺序与调用顺序一致。
	changes []BalanceChange
}

func (s *balanceUserRepoStub) AdjustBalance(ctx context.Context, id int64, delta float64) (BalanceChange, error) {
	return s.apply(func(current float64) float64 { return current + delta })
}

func (s *balanceUserRepoStub) SetBalance(ctx context.Context, id int64, value float64) (BalanceChange, error) {
	return s.apply(func(float64) float64 { return value })
}

func (s *balanceUserRepoStub) apply(next func(current float64) float64) (BalanceChange, error) {
	if s.adjustErr != nil {
		return BalanceChange{}, s.adjustErr
	}
	if s.userRepoStub == nil || s.userRepoStub.user == nil {
		return BalanceChange{}, ErrUserNotFound
	}
	change := BalanceChange{Old: s.userRepoStub.user.Balance}
	change.New = next(change.Old)
	if change.New < 0 {
		return change, ErrBalanceNegative
	}
	s.userRepoStub.user.Balance = change.New
	s.changes = append(s.changes, change)
	return change, nil
}

type balanceRedeemRepoStub struct {
	*redeemRepoStub
	created []*RedeemCode
}

func (s *balanceRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	s.created = append(s.created, &clone)
	return nil
}

type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
}

type adminRechargeAffiliateAccruerStub struct {
	calls  []adminRechargeAffiliateAccrual
	rebate float64
	err    error
}

type adminRechargeAffiliateAccrual struct {
	userID int64
	amount float64
}

type adminGiftAdjusterStub struct {
	paid       float64
	gift       float64
	grantCall  []float64
	deductCall []float64
}

func (s *adminGiftAdjusterStub) GrantGift(_ context.Context, userID, operatorID int64, amount float64, idempotencyKey, note string) error {
	if userID != 7 || operatorID != 99 || idempotencyKey != "gift-key" || note != "gift" {
		return errors.New("unexpected grant input")
	}
	s.grantCall = append(s.grantCall, amount)
	s.gift += amount
	return nil
}

func (s *adminGiftAdjusterStub) DeductGift(_ context.Context, userID, operatorID int64, amount float64, idempotencyKey, note string) error {
	if userID != 7 || operatorID != 99 || idempotencyKey != "deduct-key" || note != "deduct" {
		return errors.New("unexpected deduction input")
	}
	if amount > s.gift {
		return ErrBalanceNegative
	}
	s.deductCall = append(s.deductCall, amount)
	s.gift -= amount
	return nil
}

func TestAdminService_UpdateUserBalance_UsesGiftQuotaOnly(t *testing.T) {
	adjuster := &adminGiftAdjusterStub{paid: 20, gift: 5}
	svc := &adminServiceImpl{
		userRepo:      &userRepoStub{user: &User{ID: 7, Balance: 25}},
		quotaAdjuster: adjuster,
	}

	user, err := svc.UpdateUserBalance(context.Background(), 7, 10, "add", "gift", 99, "gift-key")
	require.NoError(t, err)
	require.Equal(t, 25.0, user.Balance, "legacy user balance must not be used as the paid/gift source")
	require.Equal(t, []float64{10}, adjuster.grantCall)
	require.Empty(t, adjuster.deductCall)

	_, err = svc.UpdateUserBalance(context.Background(), 7, 6, "subtract", "deduct", 99, "deduct-key")
	require.NoError(t, err)
	require.Equal(t, []float64{6}, adjuster.deductCall)
	require.Equal(t, 20.0, adjuster.paid, "admin deduction must not change paid quota")
	require.Equal(t, 9.0, adjuster.gift)
}

func TestAdminService_UpdateUserBalance_RejectsGiftDeductionShortfall(t *testing.T) {
	adjuster := &adminGiftAdjusterStub{paid: 20, gift: 5}
	svc := &adminServiceImpl{
		userRepo:      &userRepoStub{user: &User{ID: 7, Balance: 25}},
		quotaAdjuster: adjuster,
	}

	_, err := svc.UpdateUserBalance(context.Background(), 7, 6, "subtract", "deduct", 99, "deduct-key")
	require.ErrorIs(t, err, ErrBalanceNegative)
	require.Equal(t, "GIFT_QUOTA_INSUFFICIENT", infraerrors.Reason(err))
	require.Empty(t, adjuster.deductCall)
	require.Equal(t, 20.0, adjuster.paid)
	require.Equal(t, 5.0, adjuster.gift)
}

func (s *adminRechargeAffiliateAccruerStub) AccrueInviteRebate(_ context.Context, userID int64, amount float64) (float64, error) {
	s.calls = append(s.calls, adminRechargeAffiliateAccrual{userID: userID, amount: amount})
	return s.rebate, s.err
}

func adminRechargeSettingService(enabled bool) *SettingService {
	values := map[string]string{}
	if enabled {
		values[SettingKeyAffiliateAdminRechargeEnabled] = "true"
	}
	return NewSettingService(&settingRepoStub{values: values}, nil)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}
