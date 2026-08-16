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

// AccountFinancialActivationReader is deliberately small: Task 3 only needs
// the persisted activation boundary, not the Task 4 financial read model.
type AccountFinancialActivationReader interface {
	EnabledAt(ctx context.Context) (*time.Time, error)
}

type UsageCostEvidenceRegistrar struct {
	usageRepo    UsageLogRepository
	evidenceRepo UsageCostEvidenceRepository
	activation   AccountFinancialActivationReader
	lookup       *SubUpstreamCostService
	newAPIRate   *NewAPIRateMultiplierRegistrar
}

func NewUsageCostEvidenceRegistrar(usageRepo UsageLogRepository, evidenceRepo UsageCostEvidenceRepository, activation AccountFinancialActivationReader) *UsageCostEvidenceRegistrar {
	return &UsageCostEvidenceRegistrar{
		usageRepo:    usageRepo,
		evidenceRepo: evidenceRepo,
		activation:   activation,
		lookup:       NewSubUpstreamCostService(NewUsageService(usageRepo, nil, nil, nil)),
	}
}

// SetNewAPIRateRefreshRepository wires the NewAPI registration flow into the
// same post-usage registrar used by both gateway implementations.
func (r *UsageCostEvidenceRegistrar) SetNewAPIRateRefreshRepository(repo NewAPIRateRefreshRepository) {
	if r != nil {
		r.newAPIRate = NewNewAPIRateMultiplierRegistrar(r.usageRepo, repo)
	}
}

func (r *UsageCostEvidenceRegistrar) RegisterOnce(ctx context.Context, usageLogID int64) error {
	if r == nil || r.usageRepo == nil {
		return errors.New("usage cost evidence registrar unavailable")
	}
	if r.evidenceRepo == nil || r.activation == nil || r.lookup == nil {
		return errors.New("usage cost evidence registrar unavailable")
	}
	usage, err := r.usageRepo.GetByID(ctx, usageLogID)
	if err != nil {
		return err
	}
	var matchedNewAPI *newAPIUpstreamUsageRecord
	newAPIRecordLoaded := false
	newAPIRecordReason := ""
	if r.newAPIRate != nil && newAPIRateRegistrationUsageCandidate(usage) {
		// Registration is best effort and independent from the historical
		// evidence activation boundary.
		matchedNewAPI, newAPIRecordLoaded, newAPIRecordReason, _ = r.newAPIRate.RegisterUsage(ctx, usage)
	}
	// Only native Sub/New API-key accounts have this upstream-ledger contract.
	// Batch-image Vertex and OpenAI Live use different settlement protocols.
	if !isUsageCostEvidenceEligibleAccount(usage.Account) {
		return nil
	}
	enabledAt, err := r.activation.EnabledAt(ctx)
	if err != nil {
		return err
	}
	if enabledAt == nil || usage.CreatedAt.Before(*enabledAt) {
		return nil
	}
	if strings.TrimSpace(usage.UpstreamRequestIDOrEmpty()) == "" {
		_, err = r.evidenceRepo.CreateOnce(ctx, &UsageCostEvidence{
			UsageLogID: usage.ID,
			Source:     evidenceSourceForAccount(usage.Account),
			Status:     UsageCostEvidenceStatusUnavailable,
			ReasonCode: "request_id_missing",
			RecordedAt: time.Now().UTC(),
		})
		return err
	}

	var result upstreamCostEvidenceLookup
	if usageCostLedgerForAccount(usage.Account) == usageCostLedgerNewAPI {
		baseURL, apiKey, ok := subCredentials(usage.Account)
		if ok {
			result, matchedNewAPI = r.lookup.lookupNewAPIEvidenceWithRecord(ctx, baseURL, apiKey, usage, matchedNewAPI, newAPIRecordLoaded, newAPIRecordReason)
		} else {
			result = upstreamCostEvidenceLookup{Source: UsageCostEvidenceSourceNewAPI, ReasonCode: "credentials_unavailable"}
		}
	} else {
		result = r.lookup.lookupEvidence(ctx, usage)
	}
	evidence := &UsageCostEvidence{
		UsageLogID:         usage.ID,
		Source:             result.Source,
		UpstreamRequestID:  cloneStringPtr(usage.UpstreamRequestID),
		SubActualCost:      result.SubActualCost,
		NewAPIQuota:        result.NewAPIQuota,
		NewAPIQuotaPerUnit: result.NewAPIQuotaPerUnit,
		Status:             UsageCostEvidenceStatusUnavailable,
		ReasonCode:         normalizeUsageCostEvidenceReason(result.ReasonCode),
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

func isUsageCostEvidenceEligibleAccount(account *Account) bool {
	return usageCostLedgerForAccount(account) != usageCostLedgerUnknown
}

func normalizeUsageCostEvidenceReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "credentials_unavailable", "authentication_rejected", "request_unavailable", "response_unavailable", "endpoint_unsupported", "record_not_found":
		return strings.TrimSpace(reason)
	case "request_id_unavailable", "request_id_missing":
		return "request_id_missing"
	case "endpoint_unavailable":
		return "endpoint_unsupported"
	case "pagination_unavailable":
		return "response_unavailable"
	default:
		return "response_unavailable"
	}
}

func evidenceSourceForAccount(account *Account) UsageCostEvidenceSource {
	if usageCostLedgerForAccount(account) == usageCostLedgerNewAPI {
		return UsageCostEvidenceSourceNewAPI
	}
	return UsageCostEvidenceSourceSub
}
