package reconciliation

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type AdapterType string

const (
	AdapterSub2API AdapterType = "sub2api"
	AdapterNewAPI  AdapterType = "newapi"
)

type ReconcileStatus string

const (
	StatusPending   ReconcileStatus = "pending"
	StatusMatched   ReconcileStatus = "matched"
	StatusException ReconcileStatus = "exception"
	StatusManual    ReconcileStatus = "manual"
	StatusConflict  ReconcileStatus = "conflict"
)

type AttemptInput struct {
	AttemptID         string
	LocalRequestID    string
	AccountID         int64
	AdapterType       AdapterType
	UpstreamRequestID string
	Model             string
	InputTokens       int64
	OutputTokens      int64
	UserCharge        decimal.Decimal
	Currency          string
	RequestStatus     string
	CompletedAt       time.Time
}

type Attempt struct {
	ID int64
	AttemptInput
	ReconcileStatus ReconcileStatus
	MatchedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ManualAdjustmentInput struct {
	AttemptID      int64
	Amount         decimal.Decimal
	Notes          string
	ActorUserID    int64
	IdempotencyKey string
}

type TransactionSource string

const (
	SourceAutomaticCharge  TransactionSource = "automatic_charge"
	SourceAutomaticRefund  TransactionSource = "automatic_refund"
	SourceManualAdjustment TransactionSource = "manual_adjustment"
	SourceManualReversal   TransactionSource = "manual_reversal"
)

type AutomaticTransactionInput struct {
	AttemptID      int64
	AccountID      int64
	SourceType     TransactionSource
	SourceRecordID string
	Amount         decimal.Decimal
	Currency       string
	OccurredAt     time.Time
	IdempotencyKey string
}

type Transaction struct {
	ID              int64
	AttemptID       *int64
	AccountID       int64
	SourceType      TransactionSource
	SourceRecordID  string
	Amount          decimal.Decimal
	Currency        string
	Effective       bool
	OccurredAt      time.Time
	IdempotencyKey  string
	Notes           string
	CreatedByUserID *int64
	CreatedAt       time.Time
}

type Exception struct {
	ID              int64
	Attempt         Attempt
	ReasonCode      string
	Details         string
	RetryCount      int
	FirstDetectedAt time.Time
	LastCheckedAt   time.Time
}

type Summary struct {
	TotalAttempts     int64           `json:"total_attempts"`
	MatchedAttempts   int64           `json:"matched_attempts"`
	PendingAttempts   int64           `json:"pending_attempts"`
	ConflictAttempts  int64           `json:"conflict_attempts"`
	CoverageKnown     bool            `json:"coverage_known"`
	CoverageRatio     decimal.Decimal `json:"coverage_ratio"`
	UpstreamCost      decimal.Decimal `json:"upstream_cost"`
	UserCharge        decimal.Decimal `json:"user_charge"`
	PaperProfit       decimal.Decimal `json:"paper_profit"`
	Currency          string          `json:"currency"`
	ObservedAt        time.Time       `json:"observed_at"`
	CollectionPartial bool            `json:"collection_partial"`
}

func ValidateAttempt(input AttemptInput) (AttemptInput, error) {
	input.AttemptID = strings.TrimSpace(input.AttemptID)
	input.LocalRequestID = strings.TrimSpace(input.LocalRequestID)
	input.UpstreamRequestID = strings.TrimSpace(input.UpstreamRequestID)
	input.Model = strings.TrimSpace(input.Model)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.AttemptID == "" || len(input.AttemptID) > 200 {
		return AttemptInput{}, fmt.Errorf("attempt_id is required and must be at most 200 bytes")
	}
	if input.LocalRequestID == "" || len(input.LocalRequestID) > 200 {
		return AttemptInput{}, fmt.Errorf("local_request_id is required and must be at most 200 bytes")
	}
	if input.AccountID <= 0 {
		return AttemptInput{}, fmt.Errorf("account_id must be positive")
	}
	if input.AdapterType != AdapterSub2API && input.AdapterType != AdapterNewAPI {
		return AttemptInput{}, fmt.Errorf("adapter_type is invalid")
	}
	if input.InputTokens < 0 || input.OutputTokens < 0 || input.UserCharge.IsNegative() {
		return AttemptInput{}, fmt.Errorf("tokens and user_charge must be non-negative")
	}
	if len(input.Currency) != 3 {
		return AttemptInput{}, fmt.Errorf("currency must be a three-letter code")
	}
	if input.RequestStatus != "success" && input.RequestStatus != "failed" {
		return AttemptInput{}, fmt.Errorf("request_status is invalid")
	}
	if input.CompletedAt.IsZero() {
		return AttemptInput{}, fmt.Errorf("completed_at is required")
	}
	input.CompletedAt = input.CompletedAt.UTC().Truncate(time.Microsecond)
	return input, nil
}

func ValidateManualAdjustment(input ManualAdjustmentInput) (ManualAdjustmentInput, error) {
	input.Notes = strings.TrimSpace(input.Notes)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.AttemptID <= 0 {
		return ManualAdjustmentInput{}, fmt.Errorf("attempt_id must be positive")
	}
	if !input.Amount.IsPositive() {
		return ManualAdjustmentInput{}, fmt.Errorf("amount must be positive")
	}
	if input.ActorUserID <= 0 {
		return ManualAdjustmentInput{}, fmt.Errorf("actor_user_id must be positive")
	}
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return ManualAdjustmentInput{}, fmt.Errorf("idempotency_key is required and must be at most 200 bytes")
	}
	if len(input.Notes) > 1000 {
		return ManualAdjustmentInput{}, fmt.Errorf("notes must be at most 1000 bytes")
	}
	return input, nil
}

func ValidateAutomaticTransaction(input AutomaticTransactionInput) (AutomaticTransactionInput, error) {
	input.SourceRecordID = strings.TrimSpace(input.SourceRecordID)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.AttemptID <= 0 || input.AccountID <= 0 {
		return AutomaticTransactionInput{}, fmt.Errorf("attempt_id and account_id must be positive")
	}
	if input.SourceType != SourceAutomaticCharge && input.SourceType != SourceAutomaticRefund {
		return AutomaticTransactionInput{}, fmt.Errorf("source_type is invalid")
	}
	if input.SourceType == SourceAutomaticCharge && !input.Amount.IsPositive() {
		return AutomaticTransactionInput{}, fmt.Errorf("automatic charge amount must be positive")
	}
	if input.SourceType == SourceAutomaticRefund && !input.Amount.IsNegative() {
		return AutomaticTransactionInput{}, fmt.Errorf("automatic refund amount must be negative")
	}
	if len(input.Currency) != 3 {
		return AutomaticTransactionInput{}, fmt.Errorf("currency must be a three-letter code")
	}
	if input.OccurredAt.IsZero() {
		return AutomaticTransactionInput{}, fmt.Errorf("occurred_at is required")
	}
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return AutomaticTransactionInput{}, fmt.Errorf("idempotency_key is required and must be at most 200 bytes")
	}
	input.OccurredAt = input.OccurredAt.UTC().Truncate(time.Microsecond)
	return input, nil
}

func CoverageRatio(matched, total int64) decimal.Decimal {
	if total <= 0 {
		return decimal.Zero
	}
	if matched < 0 {
		matched = 0
	}
	if matched > total {
		matched = total
	}
	return decimal.NewFromInt(matched).Div(decimal.NewFromInt(total))
}
