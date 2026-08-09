package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var ErrUsageBillingRequestIDRequired = errors.New("usage billing request_id is required")
var ErrUsageBillingRequestConflict = errors.New("usage billing request fingerprint conflict")

// UsageCompleteness describes how much upstream usage was observed for an
// attempt. Unknown usage is retained as an audit row but must not create a
// customer charge; partial usage is chargeable only for the observed amount
// and requires later reconciliation.
type UsageCompleteness string

const (
	UsageCompletenessComplete UsageCompleteness = "complete"
	UsageCompletenessPartial  UsageCompleteness = "partial"
	UsageCompletenessUnknown  UsageCompleteness = "unknown"
)

func (c UsageCompleteness) Normalize() UsageCompleteness {
	switch c {
	case UsageCompletenessComplete, UsageCompletenessPartial, UsageCompletenessUnknown:
		return c
	default:
		return UsageCompletenessUnknown
	}
}

// normalizeUsageCompleteness infers a conservative completeness state when a
// caller did not provide one explicitly. Any observed usage is complete unless
// the attempt also reports output/usage production without a final snapshot.
func normalizeUsageCompleteness(c UsageCompleteness, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens, imageCount int, usageKnown, outputStarted, usageProduced bool) UsageCompleteness {
	if strings.TrimSpace(string(c)) != "" {
		return c.Normalize()
	}
	if outputStarted && usageProduced {
		return UsageCompletenessPartial
	}
	if usageKnown || inputTokens > 0 || outputTokens > 0 || cacheCreationTokens > 0 || cacheReadTokens > 0 || imageCount > 0 {
		return UsageCompletenessComplete
	}
	return UsageCompletenessUnknown
}

// UsageBillingCommand describes one billable request that must be applied at most once.
type UsageBillingCommand struct {
	RequestID string
	// LogicalRequestID remains stable across upstream retries and failover.
	LogicalRequestID string
	// AttemptID identifies the individual upstream call for audit purposes; it
	// is intentionally excluded from the dedup key.
	AttemptID              string
	APIKeyID               int64
	RequestFingerprint     string
	RequestPayloadHash     string
	UsageCompleteness      UsageCompleteness
	ReconciliationRequired bool
	UnsafeToReplay         bool

	UserID              int64
	AccountID           int64
	SubscriptionID      *int64
	AccountType         string
	Model               string
	ServiceTier         string
	ReasoningEffort     string
	BillingType         int8
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	ImageCount          int
	MediaType           string

	BalanceCost         float64
	SubscriptionCost    float64
	APIKeyQuotaCost     float64
	APIKeyRateLimitCost float64
	AccountQuotaCost    float64
}

func (c *UsageBillingCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.LogicalRequestID = strings.TrimSpace(c.LogicalRequestID)
	hasExplicitLogicalRequestID := c.LogicalRequestID != ""
	c.AttemptID = strings.TrimSpace(c.AttemptID)
	if c.LogicalRequestID == "" {
		c.LogicalRequestID = c.RequestID
	}
	if c.RequestID == "" {
		c.RequestID = c.LogicalRequestID
	}
	c.UsageCompleteness = c.UsageCompleteness.Normalize()
	// Unknown usage is an audit-only outcome. Preserve observed token fields for
	// reconciliation, but never let an incomplete upstream attempt charge or
	// claim the customer billing idempotency key.
	if c.UsageCompleteness == UsageCompletenessUnknown {
		c.BalanceCost = 0
		c.SubscriptionCost = 0
		c.APIKeyQuotaCost = 0
		c.APIKeyRateLimitCost = 0
		c.AccountQuotaCost = 0
	}
	if c.UsageCompleteness == UsageCompletenessPartial {
		c.ReconciliationRequired = true
	}
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildUsageBillingFingerprint(c)
	}
	if hasExplicitLogicalRequestID {
		c.RequestID = usageBillingDedupRequestID(c.LogicalRequestID, c.RequestFingerprint)
	}
}

// usageBillingDedupRequestID keeps the existing physical (request_id,
// api_key_id) uniqueness while making the logical request plus the immutable
// observed-usage fingerprint the customer-billing boundary.
func usageBillingDedupRequestID(logicalRequestID, requestFingerprint string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(logicalRequestID) + "|" + strings.TrimSpace(requestFingerprint)))
	return "billing:" + hex.EncodeToString(sum[:])
}

func buildUsageBillingFingerprint(c *UsageBillingCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%s|%d|%0.10f|%0.10f|%0.10f|%0.10f|%0.10f",
		c.UserID,
		c.AccountID,
		c.APIKeyID,
		strings.TrimSpace(c.AccountType),
		strings.TrimSpace(c.Model),
		strings.TrimSpace(c.ServiceTier),
		strings.TrimSpace(c.ReasoningEffort),
		c.BillingType,
		c.InputTokens,
		c.OutputTokens,
		c.CacheCreationTokens,
		c.CacheReadTokens,
		c.ImageCount,
		strings.TrimSpace(c.MediaType),
		valueOrZero(c.SubscriptionID),
		c.BalanceCost,
		c.SubscriptionCost,
		c.APIKeyQuotaCost,
		c.APIKeyRateLimitCost,
		c.AccountQuotaCost,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func HashUsageRequestPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// AccountQuotaState holds the post-increment quota state returned by the DB transaction.
// All values are post-update (i.e., already include the increment).
type AccountQuotaState struct {
	TotalUsed   float64
	TotalLimit  float64
	DailyUsed   float64
	DailyLimit  float64
	WeeklyUsed  float64
	WeeklyLimit float64
}

type UsageBillingApplyResult struct {
	Applied              bool
	APIKeyQuotaExhausted bool
	NewBalance           *float64           // post-deduction balance (nil = no balance deduction)
	BalanceOverdrafted   bool               // true when the sufficient-balance guard missed and debt was still recorded
	QuotaState           *AccountQuotaState // post-increment quota state (nil = no quota increment)
}

// BatchImageBalanceHoldCommand describes an idempotent balance hold operation.
type BatchImageBalanceHoldCommand struct {
	RequestID          string
	APIKeyID           int64
	RequestFingerprint string
	RequestPayloadHash string
	UserID             int64
	BatchID            string
	HoldAmount         float64
	ActualAmount       float64
}

func (c *BatchImageBalanceHoldCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.BatchID = strings.TrimSpace(c.BatchID)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildBatchImageBalanceHoldFingerprint(c)
	}
}

func buildBatchImageBalanceHoldFingerprint(c *BatchImageBalanceHoldCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%s|%0.10f|%0.10f",
		c.UserID,
		c.APIKeyID,
		strings.TrimSpace(c.BatchID),
		c.HoldAmount,
		c.ActualAmount,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type BatchImageBalanceHoldResult struct {
	Applied       bool
	NewBalance    *float64
	FrozenBalance *float64
}

type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error)
	ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
}
