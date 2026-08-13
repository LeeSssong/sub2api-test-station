package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

type UsageCostEvidenceStatus string

const (
	UsageCostEvidenceStatusConfirmed     UsageCostEvidenceStatus = "confirmed"
	UsageCostEvidenceStatusConfirmedZero UsageCostEvidenceStatus = "confirmed_zero"
	UsageCostEvidenceStatusUnavailable   UsageCostEvidenceStatus = "unavailable"
)

type UsageCostEvidenceSource string

const (
	UsageCostEvidenceSourceSub    UsageCostEvidenceSource = "sub"
	UsageCostEvidenceSourceNewAPI UsageCostEvidenceSource = "newapi"
)

type UsageCostEvidence struct {
	UsageLogID         int64
	Source             UsageCostEvidenceSource
	UpstreamRequestID  *string
	SubActualCost      *float64
	NewAPIQuota        *float64
	NewAPIQuotaPerUnit *float64
	NormalizedCostCNY  *float64
	ProfitCNY          *float64
	Status             UsageCostEvidenceStatus
	ReasonCode         string
	RecordedAt         time.Time
}

type UsageCostEvidenceRepository interface {
	CreateOnce(ctx context.Context, evidence *UsageCostEvidence) (inserted bool, err error)
}

type UsageCostEvidenceRegistrar struct {
	usageRepo    UsageLogRepository
	evidenceRepo UsageCostEvidenceRepository
	lookup       *SubUpstreamCostService
}

func NewUsageCostEvidenceRegistrar(usageRepo UsageLogRepository, evidenceRepo UsageCostEvidenceRepository) *UsageCostEvidenceRegistrar {
	return &UsageCostEvidenceRegistrar{
		usageRepo:    usageRepo,
		evidenceRepo: evidenceRepo,
		lookup:       NewSubUpstreamCostService(NewUsageService(usageRepo, nil, nil, nil)),
	}
}

func (r *UsageCostEvidenceRegistrar) RegisterOnce(ctx context.Context, usageLogID int64) error {
	if r == nil || r.usageRepo == nil || r.evidenceRepo == nil || r.lookup == nil {
		return errors.New("usage cost evidence registrar unavailable")
	}
	usage, err := r.usageRepo.GetByID(ctx, usageLogID)
	if err != nil {
		return err
	}
	if usage.Account != nil && usage.Account.Type == AccountTypeOAuth {
		return nil
	}
	if strings.TrimSpace(usage.UpstreamRequestIDOrEmpty()) == "" {
		_, err = r.evidenceRepo.CreateOnce(ctx, &UsageCostEvidence{
			UsageLogID: usage.ID,
			Source:     evidenceSourceForAccount(usage.Account),
			Status:     UsageCostEvidenceStatusUnavailable,
			ReasonCode: "request_id_unavailable",
			RecordedAt: time.Now().UTC(),
		})
		return err
	}

	result := r.lookup.lookupEvidence(ctx, usage)
	evidence := &UsageCostEvidence{
		UsageLogID:         usage.ID,
		Source:             result.Source,
		UpstreamRequestID:  cloneStringPtr(usage.UpstreamRequestID),
		SubActualCost:      result.SubActualCost,
		NewAPIQuota:        result.NewAPIQuota,
		NewAPIQuotaPerUnit: result.NewAPIQuotaPerUnit,
		Status:             UsageCostEvidenceStatusUnavailable,
		ReasonCode:         strings.TrimSpace(result.ReasonCode),
		RecordedAt:         time.Now().UTC(),
	}
	if result.Found {
		cost := result.Cost
		profit := usage.ActualCost - cost
		evidence.NormalizedCostCNY = &cost
		evidence.ProfitCNY = &profit
		evidence.ReasonCode = ""
		evidence.Status = UsageCostEvidenceStatusConfirmed
		if cost == 0 {
			evidence.Status = UsageCostEvidenceStatusConfirmedZero
		}
	}
	_, err = r.evidenceRepo.CreateOnce(ctx, evidence)
	return err
}

func evidenceSourceForAccount(account *Account) UsageCostEvidenceSource {
	if isNewAPIUsageLedger(account) {
		return UsageCostEvidenceSourceNewAPI
	}
	return UsageCostEvidenceSourceSub
}
