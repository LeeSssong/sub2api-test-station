// Package accounting contains the pure domain model for whole-site accounting.
//
// Values in this package are deliberately represented with decimal.Decimal.
// Site balance is a fixed one-to-one CNY/USD ledger, so no foreign-exchange
// conversion belongs in the accounting domain.
package accounting

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

type SourceKind string

const (
	SourceKindOwnedOAuth     SourceKind = "owned_oauth"
	SourceKindUpstreamAPIKey SourceKind = "upstream_apikey"
)

const (
	EventTypeAccountPurchase = "account_purchase"
	EventTypeUpstreamTopup   = "upstream_topup"
	EventTypeRefund          = "refund"
	EventTypeFee             = "fee"
)

type CashEventInput struct {
	EventType  string
	PaidAt     time.Time
	AmountCNY  decimal.Decimal
	SourceKind SourceKind
	AccountID  *int64
	Notes      string
}

type CashEvent struct {
	ID              int64
	EventType       string
	PaidAt          time.Time
	AmountCNY       decimal.Decimal
	SourceKind      SourceKind
	AccountID       *int64
	Notes           string
	CreatedByUserID int64
	CreatedAt       time.Time
}

type CashEventTotals struct {
	OutflowCNY         decimal.Decimal
	UnlinkedOutflowCNY decimal.Decimal
	EventCount         int64
}

type DayWindow struct {
	Start time.Time
	End   time.Time
}

type ExclusionPolicy struct {
	InternalUserIDs   []int64
	InternalAPIKeyIDs []int64
}

type UsageTotals struct {
	ExternalRevenueCNY    decimal.Decimal
	ExternalRequests      int64
	InternalRequests      int64
	CustomerCostCNY       decimal.Decimal
	InternalCostCNY       decimal.Decimal
	OwnedOAuthCostCNY     decimal.Decimal
	UpstreamAPIKeyCostCNY decimal.Decimal
}

type DailySnapshot struct {
	ReportDate              time.Time
	ExternalRevenueCNY      decimal.Decimal
	ExternalRequests        int64
	InternalRequests        int64
	CustomerResourceCostCNY decimal.Decimal
	InternalResourceCostCNY decimal.Decimal
	ResourceCostCNY         decimal.Decimal
	OperatingGrossProfitCNY decimal.Decimal
	CashOutflowCNY          decimal.Decimal
	CashNetResultCNY        decimal.Decimal
	UnlinkedCashOutflowCNY  decimal.Decimal
	CashEventCount          int64
	OwnedOAuthCostCNY       decimal.Decimal
	UpstreamAPIKeyCostCNY   decimal.Decimal
}

var (
	credentialLikeNotePattern = regexp.MustCompile(`(?i)(?:(?:api[_-]?key|access[_-]?token|refresh[_-]?token|oauth[_-]?token|password|passwd|secret|cookie|private[_-]?key)\s*(?:=|:|\s)\s*\S+|token\s*(?:=|:)\s*\S+|(?:authorization|bearer)\s+(?:bearer\s+)?\S+)`)
	openAIKeyPattern          = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])sk-(?:proj-)?[a-z0-9_-]{16,}(?:$|[^a-z0-9_-])`)
	jwtPattern                = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}(?:$|[^A-Za-z0-9_-])`)
	opaqueTokenPattern        = regexp.MustCompile(`[A-Za-z0-9_+/=-]{40,}`)
)

var (
	accountingLocation = func() *time.Location {
		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			return time.FixedZone("Asia/Shanghai", 8*60*60)
		}
		return location
	}()
)

// LocalDay returns midnight at the beginning of the Asia/Shanghai calendar day
// containing t. The returned value always carries the accounting location.
func LocalDay(t time.Time) time.Time {
	local := t.In(accountingLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, accountingLocation)
}

// NewDayWindow returns the half-open [start,end) window for t's local day.
func NewDayWindow(t time.Time) DayWindow {
	start := LocalDay(t)
	return DayWindow{Start: start, End: start.AddDate(0, 0, 1)}
}

// ValidateCashEvent verifies the operator-entered portion of a cash event and
// returns the unchanged input when it is valid.
func ValidateCashEvent(input CashEventInput) (CashEventInput, error) {
	switch input.EventType {
	case EventTypeAccountPurchase, EventTypeUpstreamTopup, EventTypeRefund, EventTypeFee:
	default:
		return CashEventInput{}, fmt.Errorf("invalid cash event type %q", input.EventType)
	}
	if input.PaidAt.IsZero() {
		return CashEventInput{}, fmt.Errorf("paid_at is required")
	}
	switch input.EventType {
	case EventTypeRefund:
		if !input.AmountCNY.IsNegative() {
			return CashEventInput{}, fmt.Errorf("refund amount_cny must be negative")
		}
	default:
		if !input.AmountCNY.IsPositive() {
			return CashEventInput{}, fmt.Errorf("amount_cny must be positive")
		}
	}
	switch input.SourceKind {
	case SourceKindOwnedOAuth, SourceKindUpstreamAPIKey:
	default:
		return CashEventInput{}, fmt.Errorf("invalid source_kind %q", input.SourceKind)
	}
	if input.AccountID != nil && *input.AccountID <= 0 {
		return CashEventInput{}, fmt.Errorf("account_id must be positive")
	}
	if !utf8.ValidString(input.Notes) {
		return CashEventInput{}, fmt.Errorf("notes must be valid UTF-8")
	}
	if len([]byte(input.Notes)) > 500 {
		return CashEventInput{}, fmt.Errorf("notes must be at most 500 UTF-8 bytes")
	}
	if containsCredentialLikeValue(input.Notes) {
		return CashEventInput{}, fmt.Errorf("notes must not contain credential-like values")
	}
	return input, nil
}

func containsCredentialLikeValue(notes string) bool {
	if credentialLikeNotePattern.MatchString(notes) ||
		openAIKeyPattern.MatchString(notes) ||
		jwtPattern.MatchString(notes) {
		return true
	}
	for _, candidate := range opaqueTokenPattern.FindAllString(notes, -1) {
		var hasLetter, hasDigit, hasBase64Punctuation bool
		for _, char := range candidate {
			switch {
			case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z':
				hasLetter = true
			case char >= '0' && char <= '9':
				hasDigit = true
			case char == '_' || char == '+' || char == '/' || char == '=' || char == '-':
				hasBase64Punctuation = true
			}
		}
		if hasLetter && (len(candidate) >= 48 || hasDigit || hasBase64Punctuation) {
			return true
		}
	}
	return false
}

// ClassifySourceKind maps the native account type names to accounting source
// kinds. It intentionally rejects unknown values rather than silently
// assigning an accounting category.
func ClassifySourceKind(accountType string) (SourceKind, error) {
	switch accountType {
	case "oauth":
		return SourceKindOwnedOAuth, nil
	case "apikey":
		return SourceKindUpstreamAPIKey, nil
	default:
		return "", fmt.Errorf("unsupported account type %q", accountType)
	}
}

// BuildSnapshot combines usage and cash totals into the idempotently persisted
// whole-site report for one Shanghai calendar day.
func BuildSnapshot(date time.Time, usage UsageTotals, cash CashEventTotals) DailySnapshot {
	resourceCost := usage.CustomerCostCNY.Add(usage.InternalCostCNY)
	return DailySnapshot{
		ReportDate:              LocalDay(date),
		ExternalRevenueCNY:      usage.ExternalRevenueCNY,
		ExternalRequests:        usage.ExternalRequests,
		InternalRequests:        usage.InternalRequests,
		CustomerResourceCostCNY: usage.CustomerCostCNY,
		InternalResourceCostCNY: usage.InternalCostCNY,
		ResourceCostCNY:         resourceCost,
		OperatingGrossProfitCNY: usage.ExternalRevenueCNY.Sub(resourceCost),
		CashOutflowCNY:          cash.OutflowCNY,
		CashNetResultCNY:        usage.ExternalRevenueCNY.Sub(cash.OutflowCNY),
		UnlinkedCashOutflowCNY:  cash.UnlinkedOutflowCNY,
		CashEventCount:          cash.EventCount,
		OwnedOAuthCostCNY:       usage.OwnedOAuthCostCNY,
		UpstreamAPIKeyCostCNY:   usage.UpstreamAPIKeyCostCNY,
	}
}
