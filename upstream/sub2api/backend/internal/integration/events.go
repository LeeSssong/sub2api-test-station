// Package integration contains the versioned, credential-free boundary between
// the official Sub2API core and the external relay-ops control plane.
package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const ContractVersion = 1

const (
	EventTypeRequestCompleted       = "request.completed"
	EventTypeAccountHealthChanged   = "account.health_changed"
	EventTypeAccountBalanceSnapshot = "account.balance_snapshot"
)

var supportedEventTypes = map[string]struct{}{
	EventTypeRequestCompleted:       {},
	EventTypeAccountHealthChanged:   {},
	EventTypeAccountBalanceSnapshot: {},
}

// Event is the stable envelope sent from core to relay-ops. Payload is JSON so
// adding fields remains backward-compatible without coupling the two binaries.
type Event struct {
	EventID         string          `json:"event_id"`
	Type            string          `json:"type"`
	OccurredAt      time.Time       `json:"occurred_at"`
	SourceVersion   string          `json:"source_version"`
	ContractVersion int             `json:"contract_version"`
	Payload         json.RawMessage `json:"payload"`
}

func NewEvent(eventType, sourceVersion string, occurredAt time.Time, payload any) (Event, error) {
	if validator, ok := payload.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return Event{}, fmt.Errorf("validate event payload: %w", err)
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal event payload: %w", err)
	}
	e := Event{
		EventID:         uuid.NewString(),
		Type:            eventType,
		OccurredAt:      occurredAt.UTC().Truncate(time.Microsecond),
		SourceVersion:   strings.TrimSpace(sourceVersion),
		ContractVersion: ContractVersion,
		Payload:         raw,
	}
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

// stableEventID derives a UUIDv4-shaped identifier from an immutable business
// identity. This preserves the envelope's UUIDv4 contract while making retries
// idempotent at the outbox primary key.
func stableEventID(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	var id uuid.UUID
	copy(id[:], sum[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func newStableEvent(eventType, sourceVersion string, occurredAt time.Time, identity string, payload any) (Event, error) {
	e, err := NewEvent(eventType, sourceVersion, occurredAt, payload)
	if err != nil {
		return Event{}, err
	}
	e.EventID = stableEventID(eventType + "\x00" + identity)
	return e, nil
}

func (e Event) Validate() error {
	parsedID, err := uuid.Parse(e.EventID)
	if err != nil {
		return fmt.Errorf("event_id must be a UUID: %w", err)
	}
	if parsedID.Version() != uuid.Version(4) {
		return fmt.Errorf("event_id must be a UUIDv4")
	}
	if _, ok := supportedEventTypes[e.Type]; !ok {
		return fmt.Errorf("unsupported event type %q", e.Type)
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if e.SourceVersion == "" {
		return errors.New("source_version is required")
	}
	if e.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported contract_version %d", e.ContractVersion)
	}
	if len(bytes.TrimSpace(e.Payload)) == 0 || bytes.Equal(bytes.TrimSpace(e.Payload), []byte("null")) {
		return errors.New("payload is required")
	}
	var value any
	if err := json.Unmarshal(e.Payload, &value); err != nil {
		return fmt.Errorf("payload must be valid JSON: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("payload must be a JSON object")
	}
	if err := validateCredentialFree(value, "payload"); err != nil {
		return err
	}
	if err := validateTypedPayload(e.Type, e.Payload); err != nil {
		return err
	}
	return nil
}

func validateTypedPayload(eventType string, payload json.RawMessage) error {
	var validator interface{ Validate() error }
	switch eventType {
	case EventTypeRequestCompleted:
		var value RequestCompleted
		if err := json.Unmarshal(payload, &value); err != nil {
			return fmt.Errorf("decode %s payload: %w", eventType, err)
		}
		validator = value
	case EventTypeAccountHealthChanged:
		var value AccountHealthChanged
		if err := json.Unmarshal(payload, &value); err != nil {
			return fmt.Errorf("decode %s payload: %w", eventType, err)
		}
		validator = value
	case EventTypeAccountBalanceSnapshot:
		var value AccountBalanceSnapshot
		if err := json.Unmarshal(payload, &value); err != nil {
			return fmt.Errorf("decode %s payload: %w", eventType, err)
		}
		validator = value
	}
	if err := validator.Validate(); err != nil {
		return fmt.Errorf("validate %s payload: %w", eventType, err)
	}
	return nil
}

// ValidateDecimalString is shared by event payload constructors and consumers.
func ValidateDecimalString(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be a decimal string", field)
	}
	if _, err := decimal.NewFromString(value); err != nil {
		return fmt.Errorf("%s must be a decimal string: %w", field, err)
	}
	return nil
}

type RequestCompleted struct {
	RequestID           string    `json:"request_id"`
	AccountID           int64     `json:"account_id"`
	GroupID             *int64    `json:"group_id,omitempty"`
	Model               string    `json:"model"`
	RequestedModel      string    `json:"requested_model,omitempty"`
	UpstreamModel       string    `json:"upstream_model,omitempty"`
	ActualResponseModel string    `json:"actual_response_model,omitempty"`
	PromptTokens        int64     `json:"prompt_tokens"`
	CompletionTokens    int64     `json:"completion_tokens"`
	InputTokens         int64     `json:"input_tokens,omitempty"`
	OutputTokens        int64     `json:"output_tokens,omitempty"`
	UserCharge          string    `json:"user_charge"`
	ActualCost          string    `json:"actual_cost"`
	CostUSD             string    `json:"cost_usd,omitempty"`
	LatencyMS           int64     `json:"latency_ms,omitempty"`
	Currency            string    `json:"currency"`
	OccurredAt          time.Time `json:"occurred_at,omitempty"`
}

func (p RequestCompleted) Validate() error {
	if strings.TrimSpace(p.RequestID) == "" || p.AccountID <= 0 || strings.TrimSpace(p.Model) == "" {
		return errors.New("request.completed requires request_id, account_id and model")
	}
	if p.PromptTokens < 0 || p.CompletionTokens < 0 || p.InputTokens < 0 || p.OutputTokens < 0 || p.LatencyMS < 0 {
		return errors.New("token counts cannot be negative")
	}
	if err := ValidateDecimalString("user_charge", p.UserCharge); err != nil {
		return err
	}
	if err := ValidateDecimalString("actual_cost", p.ActualCost); err != nil {
		return err
	}
	if strings.TrimSpace(p.Currency) == "" {
		return errors.New("currency is required")
	}
	return nil
}

func NewRequestCompletedEvent(sourceVersion string, occurredAt time.Time, payload RequestCompleted) (Event, error) {
	if payload.RequestedModel == "" {
		payload.RequestedModel = payload.Model
	}
	if payload.InputTokens == 0 {
		payload.InputTokens = payload.PromptTokens
	}
	if payload.OutputTokens == 0 {
		payload.OutputTokens = payload.CompletionTokens
	}
	if payload.CostUSD == "" {
		payload.CostUSD = payload.ActualCost
	}
	payload.OccurredAt = occurredAt.UTC()
	if err := payload.Validate(); err != nil {
		return Event{}, err
	}
	return newStableEvent(EventTypeRequestCompleted, sourceVersion, occurredAt, payload.RequestID, payload)
}

type AccountHealthChanged struct {
	AccountID     int64     `json:"account_id"`
	Status        string    `json:"status"`
	CheckedAt     time.Time `json:"checked_at"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorCategory string    `json:"error_category,omitempty"`
	ObservedAt    time.Time `json:"observed_at,omitempty"`
	ProbeVersion  string    `json:"probe_version,omitempty"`
}

// HealthChanged is the concise name used by the externalization adapter.
type HealthChanged = AccountHealthChanged

func NewHealthChangedEvent(sourceVersion string, occurredAt time.Time, payload AccountHealthChanged) (Event, error) {
	if payload.CheckedAt.IsZero() {
		payload.CheckedAt = payload.ObservedAt
	}
	if payload.ObservedAt.IsZero() {
		payload.ObservedAt = payload.CheckedAt
	}
	if err := payload.Validate(); err != nil {
		return Event{}, err
	}
	identityAt := payload.ObservedAt
	if identityAt.IsZero() {
		identityAt = payload.CheckedAt
	}
	identity, _ := json.Marshal(struct {
		AccountID     int64     `json:"account_id"`
		Status        string    `json:"status"`
		ErrorCategory string    `json:"error_category"`
		ObservedAt    time.Time `json:"observed_at"`
	}{payload.AccountID, payload.Status, payload.ErrorCategory, identityAt.UTC()})
	return newStableEvent(EventTypeAccountHealthChanged, sourceVersion, occurredAt, string(identity), payload)
}

func (p AccountHealthChanged) Validate() error {
	if p.AccountID <= 0 || strings.TrimSpace(p.Status) == "" || (p.CheckedAt.IsZero() && p.ObservedAt.IsZero()) {
		return errors.New("account.health_changed requires account_id, status and checked_at")
	}
	if p.CheckedAt.IsZero() {
		p.CheckedAt = p.ObservedAt
	}
	if p.ObservedAt.IsZero() {
		p.ObservedAt = p.CheckedAt
	}
	return nil
}

type AccountBalanceSnapshot struct {
	AccountID  int64     `json:"account_id"`
	Balance    string    `json:"balance"`
	Currency   string    `json:"currency"`
	CapturedAt time.Time `json:"captured_at"`
}

func (p AccountBalanceSnapshot) Validate() error {
	if p.AccountID <= 0 || p.CapturedAt.IsZero() || strings.TrimSpace(p.Currency) == "" {
		return errors.New("account.balance_snapshot requires account_id, currency and captured_at")
	}
	return ValidateDecimalString("balance", p.Balance)
}

func validateCredentialFree(value any, path string) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if normalized == "token" || normalized == "access_token" || normalized == "refresh_token" ||
				normalized == "cookie" || normalized == "authorization" || normalized == "api_key" ||
				normalized == "key" || normalized == "password" || normalized == "secret" ||
				strings.Contains(normalized, "api_key") || strings.Contains(normalized, "authorization") {
				return fmt.Errorf("%s.%s is forbidden in integration payloads", path, key)
			}
			if err := validateCredentialFree(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if err := validateCredentialFree(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
