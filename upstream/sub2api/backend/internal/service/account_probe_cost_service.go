package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// AccountProbeCostRecorder is the narrow operational-ledger boundary used by
// explicitly classified account probes. It deliberately has no user or API-key
// identity and never writes usage_logs.
type AccountProbeCostRecorder interface {
	Record(context.Context, ProbeRecordInput) error
}

type ProbeRecordInput struct {
	AccountID    int64
	GroupID      *int64
	Group        *Group
	AccountRate  float64
	Kind         ProbeKind
	RunID        string
	Model        string
	Tokens       UsageTokens
	Completeness ProbeUsageCompleteness
	Outcome      ProbeOutcome
	ErrorCode    string
}

// ProbeObservation is kept request-local and is not added to the account-test
// SSE contract. It records only provider usage supplied by a completed probe.
type ProbeObservation struct {
	Model        string
	Tokens       UsageTokens
	Completeness ProbeUsageCompleteness
}

type accountProbeUsageObserver struct {
	model  string
	tokens UsageTokens
	seen   bool
}

func (o *accountProbeUsageObserver) observeModel(model string) {
	if o == nil || o.model != "" {
		return
	}
	o.model = strings.TrimSpace(model)
}

func (o *accountProbeUsageObserver) observeJSON(raw []byte) {
	if o == nil || len(raw) == 0 {
		return
	}
	type usagePayload struct {
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		CachedTokens     int `json:"cached_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	var payload struct {
		Usage    *usagePayload `json:"usage"`
		Response struct {
			Usage *usagePayload `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	usage := payload.Usage
	if usage == nil {
		usage = payload.Response.Usage
	}
	if usage == nil {
		return
	}
	input := usage.InputTokens
	if input == 0 {
		input = usage.PromptTokens
	}
	output := usage.OutputTokens
	if output == 0 {
		output = usage.CompletionTokens
	}
	cached := usage.CachedTokens
	if cached == 0 {
		cached = usage.PromptDetails.CachedTokens
	}
	if input < 0 || output < 0 || cached < 0 {
		return
	}
	o.tokens = UsageTokens{InputTokens: input, OutputTokens: output, CacheReadTokens: cached}
	o.seen = true
}

func (o *accountProbeUsageObserver) observation(_ string, _ ProbeOutcome, _ string) ProbeObservation {
	if o == nil || !o.seen {
		return ProbeObservation{Model: o.model, Completeness: ProbeUsageUnknown}
	}
	return ProbeObservation{Model: o.model, Tokens: o.tokens, Completeness: ProbeUsageComplete}
}

type accountProbeUsageObserverKey struct{}

// AccountProbeCostService converts a complete observation through the native
// BillingService and appends the resulting immutable operational row.
type AccountProbeCostService struct {
	billingService *BillingService
	repository     AccountProbeCostRepository
}

func NewAccountProbeCostService(billingService *BillingService, repository AccountProbeCostRepository) *AccountProbeCostService {
	return &AccountProbeCostService{billingService: billingService, repository: repository}
}

func (s *AccountProbeCostService) Record(ctx context.Context, input ProbeRecordInput) error {
	var cost *decimal.Decimal
	errorCode := strings.TrimSpace(input.ErrorCode)
	if input.Completeness == ProbeUsageComplete {
		if s == nil || s.billingService == nil {
			errorCode = "probe_pricing_unavailable"
		} else {
			breakdown, err := s.billingService.CalculateCostUnified(CostInput{
				Ctx:            ctx,
				Model:          input.Model,
				GroupID:        input.GroupID,
				Group:          input.Group,
				Tokens:         input.Tokens,
				RateMultiplier: input.AccountRate,
			})
			if err != nil || breakdown == nil {
				errorCode = "probe_pricing_unavailable"
			} else {
				value := decimal.NewFromFloat(breakdown.ActualCost)
				cost = &value
			}
		}
	}
	if s == nil || s.repository == nil {
		return context.Canceled
	}
	var errorCodePtr *string
	if errorCode != "" {
		errorCodePtr = &errorCode
	}
	return s.repository.Append(ctx, AccountProbeCostLog{
		ProbeRunID:          input.RunID,
		AccountID:           input.AccountID,
		GroupID:             input.GroupID,
		ProbeKind:           input.Kind,
		Model:               input.Model,
		InputTokens:         int64(input.Tokens.InputTokens),
		OutputTokens:        int64(input.Tokens.OutputTokens),
		CacheCreationTokens: int64(input.Tokens.CacheCreationTokens),
		CacheReadTokens:     int64(input.Tokens.CacheReadTokens),
		AccountCost:         cost,
		UsageCompleteness:   input.Completeness,
		ProbeOutcome:        input.Outcome,
		ErrorCode:           errorCodePtr,
		CreatedAt:           time.Now().UTC(),
	})
}
