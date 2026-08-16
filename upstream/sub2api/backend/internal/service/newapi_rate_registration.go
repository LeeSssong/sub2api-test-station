package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
)

const newAPIRateRefreshLease = 5 * time.Minute

// NewAPIRateMultiplierRegistrar performs automatic registration after a usage
// row is durably inserted. It deliberately sits behind the existing registrar
// hook so registration failures cannot affect billing or usage persistence.
type NewAPIRateMultiplierRegistrar struct {
	usageRepo   UsageLogRepository
	accountRepo NewAPIRateRefreshRepository
	lookup      *SubUpstreamCostService
	now         func() time.Time
}

func NewNewAPIRateMultiplierRegistrar(usageRepo UsageLogRepository, accountRepo NewAPIRateRefreshRepository) *NewAPIRateMultiplierRegistrar {
	return &NewAPIRateMultiplierRegistrar{
		usageRepo: usageRepo, accountRepo: accountRepo,
		lookup: NewSubUpstreamCostService(NewUsageService(usageRepo, nil, nil, nil)),
		now:    timezone.Now,
	}
}

func (r *NewAPIRateMultiplierRegistrar) RegisterOnce(ctx context.Context, usageLogID int64) error {
	if r == nil || r.usageRepo == nil || r.accountRepo == nil || r.lookup == nil {
		return nil
	}
	usage, err := r.usageRepo.GetByID(ctx, usageLogID)
	if err != nil || usage == nil || !newAPIRateRegistrationUsageCandidate(usage) {
		return err
	}
	_, _, _, err = r.RegisterUsage(ctx, usage)
	return err
}

func (r *NewAPIRateMultiplierRegistrar) RegisterUsage(ctx context.Context, usage *UsageLog) (*newAPIUpstreamUsageRecord, bool, string, error) {
	if r == nil || r.accountRepo == nil || r.lookup == nil || !newAPIRateRegistrationUsageCandidate(usage) {
		return nil, false, "", nil
	}
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	refreshDate, err := beijingRefreshDate(now)
	if err != nil {
		return nil, false, "", err
	}
	claimToken := uuid.NewString()
	claimed, err := r.accountRepo.ClaimNewAPIRateRefresh(ctx, usage.Account.ID, refreshDate, claimToken, now.Add(newAPIRateRefreshLease))
	if err != nil || !claimed {
		return nil, false, "", err
	}
	release := true
	defer func() {
		if release {
			_ = r.accountRepo.ReleaseNewAPIRateRefresh(context.WithoutCancel(ctx), usage.Account.ID, claimToken)
		}
	}()
	baseURL, apiKey, ok := subCredentials(usage.Account)
	if !ok {
		return nil, true, "credentials_unavailable", nil
	}
	endpoint, err := newAPIEndpointURL(baseURL, "/api/log/token")
	if err != nil {
		return nil, true, "endpoint_unavailable", nil
	}
	record, reason, _ := r.lookup.findNewAPIRecord(ctx, endpoint, apiKey, usage)
	if reason != "" || !newAPIRateMultiplierRegistrationEligible(usage, record) {
		return record, true, reason, nil
	}
	if err := r.accountRepo.CompleteNewAPIRateRefresh(ctx, NewAPIRateRefreshCompletion{
		AccountID: usage.Account.ID, ClaimToken: claimToken, RefreshDate: refreshDate,
		GroupRatio: *record.GroupRatio, ObservedAt: now, UsageLogID: usage.ID,
	}); err != nil {
		return record, true, "", err
	}
	release = false
	return record, true, "", nil
}

func beijingRefreshDate(now time.Time) (string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return "", err
	}
	return now.In(loc).Format("2006-01-02"), nil
}

func newAPIRateRegistrationUsageCandidate(usage *UsageLog) bool {
	return usage != nil && usage.ID > 0 && usage.Account != nil && strings.TrimSpace(usage.UpstreamRequestIDOrEmpty()) != "" && newAPIRateRegistrationIdentity(usage.Account)
}
