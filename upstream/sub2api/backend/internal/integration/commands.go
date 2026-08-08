package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	CommandAccountUpdate = "account.update"
)

var allowedCommands = map[string]struct{}{
	CommandAccountUpdate: {},
}

type Command struct {
	CommandID       string          `json:"command_id"`
	ActorID         string          `json:"actor_id"`
	Name            string          `json:"name"`
	Payload         json.RawMessage `json:"payload"`
	ContractVersion int             `json:"contract_version"`
}

func NewCommand(name, actorID string, payload any) (Command, error) {
	if validator, ok := payload.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return Command{}, fmt.Errorf("validate command payload: %w", err)
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Command{}, fmt.Errorf("marshal command payload: %w", err)
	}
	c := Command{
		CommandID:       uuid.NewString(),
		ActorID:         strings.TrimSpace(actorID),
		Name:            name,
		Payload:         raw,
		ContractVersion: ContractVersion,
	}
	if err := c.Validate(); err != nil {
		return Command{}, err
	}
	return c, nil
}

func (c Command) Validate() error {
	parsedID, err := uuid.Parse(c.CommandID)
	if err != nil {
		return fmt.Errorf("command_id must be a UUID: %w", err)
	}
	if parsedID.Version() != uuid.Version(4) {
		return fmt.Errorf("command_id must be a UUIDv4")
	}
	if strings.TrimSpace(c.ActorID) == "" {
		return errors.New("actor_id is required")
	}
	if _, ok := allowedCommands[c.Name]; !ok {
		return fmt.Errorf("command %q is not allowed by core", c.Name)
	}
	if c.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported contract_version %d", c.ContractVersion)
	}
	if len(bytes.TrimSpace(c.Payload)) == 0 || bytes.Equal(bytes.TrimSpace(c.Payload), []byte("null")) {
		return errors.New("payload is required")
	}
	var value any
	if err := json.Unmarshal(c.Payload, &value); err != nil {
		return fmt.Errorf("payload must be valid JSON: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("payload must be a JSON object")
	}
	if err := validateCredentialFree(value, "payload"); err != nil {
		return err
	}
	if c.Name == CommandAccountUpdate {
		var update AccountUpdate
		if err := json.Unmarshal(c.Payload, &update); err != nil {
			return fmt.Errorf("decode %s payload: %w", c.Name, err)
		}
		if err := update.Validate(); err != nil {
			return fmt.Errorf("validate %s payload: %w", c.Name, err)
		}
	}
	return nil
}

type AccountUpdate struct {
	AccountID int64          `json:"account_id"`
	Fields    map[string]any `json:"fields"`
}

func (p AccountUpdate) Validate() error {
	if p.AccountID <= 0 {
		return errors.New("account_id must be positive")
	}
	if len(p.Fields) == 0 {
		return errors.New("fields must not be empty")
	}
	return validateCredentialFree(p.Fields, "fields")
}
