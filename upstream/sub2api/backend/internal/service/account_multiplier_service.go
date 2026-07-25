package service

import (
	"context"
	"fmt"
	"time"
)

type AccountMultiplierNativeProbe interface {
	ProbeAccount(context.Context, int64) (*UpstreamBillingProbeSnapshot, error)
}

type AccountMultiplierMeasurementRefresher interface {
	RefreshAccount(
		context.Context,
		int64,
		bool,
	) (*UpstreamMultiplierMeasurementSnapshot, error)
}

type AccountMultiplierService struct {
	accountRepo AccountRepository
	native      AccountMultiplierNativeProbe
	measurement AccountMultiplierMeasurementRefresher
	now         func() time.Time
}

func NewAccountMultiplierService(
	accountRepo AccountRepository,
	native AccountMultiplierNativeProbe,
	measurement AccountMultiplierMeasurementRefresher,
) *AccountMultiplierService {
	return &AccountMultiplierService{
		accountRepo: accountRepo,
		native:      native,
		measurement: measurement,
		now:         time.Now,
	}
}

func (s *AccountMultiplierService) RefreshAccount(
	ctx context.Context,
	accountID int64,
	force bool,
) error {
	if s == nil || s.accountRepo == nil {
		return fmt.Errorf("account multiplier service is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if !isUpstreamBillingProbeAccount(account) {
		return nil
	}
	now := s.currentTime().UTC()
	if !force && resolveAccountMonitorMultiplier(account, now).Status == AccountMonitorMultiplierStatusOK {
		return nil
	}

	declaration := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if !force && declaration != nil && !declaration.NextProbeAt.IsZero() &&
		now.Before(declaration.NextProbeAt) {
		if declaration.Status == UpstreamBillingProbeStatusUnsupported {
			return s.refreshMeasurement(ctx, accountID, false)
		}
		return nil
	}
	if s.native == nil {
		return fmt.Errorf("native account multiplier probe is unavailable")
	}
	declaration, err = s.native.ProbeAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if declaration != nil && declaration.Status == UpstreamBillingProbeStatusUnsupported {
		return s.refreshMeasurement(ctx, accountID, force)
	}
	return nil
}

func (s *AccountMultiplierService) refreshMeasurement(
	ctx context.Context,
	accountID int64,
	force bool,
) error {
	if s.measurement == nil {
		return fmt.Errorf("measured account multiplier probe is unavailable")
	}
	_, err := s.measurement.RefreshAccount(ctx, accountID, force)
	return err
}

func (s *AccountMultiplierService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}
