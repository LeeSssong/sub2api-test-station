package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ContractVersion = 1

const (
	RequestCompleted       = "request.completed"
	AccountHealthChanged   = "account.health_changed"
	AccountBalanceSnapshot = "account.balance_snapshot"
)

type Event struct {
	EventID         string          `json:"event_id"`
	Type            string          `json:"type"`
	OccurredAt      time.Time       `json:"occurred_at"`
	SourceVersion   string          `json:"source_version"`
	ContractVersion int             `json:"contract_version"`
	Payload         json.RawMessage `json:"payload"`
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.Type) == "" || e.OccurredAt.IsZero() || strings.TrimSpace(e.SourceVersion) == "" {
		return errors.New("incomplete event envelope")
	}
	if e.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported contract version %d", e.ContractVersion)
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("event payload must be valid JSON")
	}
	return nil
}
