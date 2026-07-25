package service

import (
	"math"
	"time"
)

const (
	AccountMonitorMultiplierSourceDeclared = "declared"
	AccountMonitorMultiplierSourceMeasured = "measured"

	AccountMonitorMultiplierStatusOK          = "ok"
	AccountMonitorMultiplierStatusStale       = "stale"
	AccountMonitorMultiplierStatusUnsupported = "unsupported"
	AccountMonitorMultiplierStatusFailed      = "failed"
	AccountMonitorMultiplierStatusUnavailable = "unavailable"
)

type AccountMultiplierService struct {
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	billingService     *BillingService
}

func NewAccountMultiplierService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	billingService *BillingService,
) *AccountMultiplierService {
	return &AccountMultiplierService{
		accountRepo:        accountRepo,
		accountTestService: accountTestService,
		billingService:     billingService,
	}
}

func (s *AccountMultiplierService) Resolve(account *Account, now time.Time) AccountMonitorMultiplier {
	if account == nil {
		return unavailableAccountMultiplier()
	}
	snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if snapshot == nil {
		if account.Extra != nil {
			if _, exists := account.Extra[UpstreamBillingProbeExtraKey]; exists {
				return AccountMonitorMultiplier{
					Source: AccountMonitorMultiplierSourceDeclared,
					Status: AccountMonitorMultiplierStatusFailed,
				}
			}
		}
		return unavailableAccountMultiplier()
	}
	switch snapshot.Status {
	case UpstreamBillingProbeStatusUnsupported:
		return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusUnsupported}
	case UpstreamBillingProbeStatusFailed:
		return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusFailed}
	case UpstreamBillingProbeStatusOK:
	default:
		return unavailableAccountMultiplier()
	}
	if snapshot.FreshUntil == nil || !now.Before(*snapshot.FreshUntil) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceDeclared,
			Status:     AccountMonitorMultiplierStatusStale,
			ObservedAt: accountMultiplierObservedAt(snapshot),
		}
	}
	value, ok := upstreamBillingRateAt(snapshot.Data, now)
	if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return AccountMonitorMultiplier{
			Source:     AccountMonitorMultiplierSourceDeclared,
			Status:     AccountMonitorMultiplierStatusFailed,
			ObservedAt: accountMultiplierObservedAt(snapshot),
		}
	}
	return AccountMonitorMultiplier{
		Value:      float64PointerCopy(value),
		Source:     AccountMonitorMultiplierSourceDeclared,
		Status:     AccountMonitorMultiplierStatusOK,
		ObservedAt: accountMultiplierObservedAt(snapshot),
	}
}

func unavailableAccountMultiplier() AccountMonitorMultiplier {
	return AccountMonitorMultiplier{Status: AccountMonitorMultiplierStatusUnavailable}
}

func accountMultiplierObservedAt(snapshot *UpstreamBillingProbeSnapshot) *time.Time {
	if snapshot == nil || snapshot.ReceivedAt == nil {
		return nil
	}
	observedAt := snapshot.ReceivedAt.UTC()
	return &observedAt
}

func float64PointerCopy(value float64) *float64 {
	return &value
}
