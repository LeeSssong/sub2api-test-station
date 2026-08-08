package billing

import (
	"context"
	"errors"
	"strings"
	"time"
)

type BalanceSnapshot struct {
	AccountID  int64     `json:"account_id"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	ObservedAt time.Time `json:"observed_at"`
	FreshUntil time.Time `json:"fresh_until"`
	Source     string    `json:"source"`
}

func (s BalanceSnapshot) Validate() error {
	if s.AccountID <= 0 || strings.TrimSpace(s.Amount) == "" || strings.TrimSpace(s.Currency) == "" || s.ObservedAt.IsZero() || s.FreshUntil.Before(s.ObservedAt) || strings.TrimSpace(s.Source) == "" {
		return errors.New("invalid balance snapshot")
	}
	return nil
}

type AccountUpdateCommand struct {
	CommandID      string         `json:"command_id"`
	ActorID        int64          `json:"actor_id"`
	AccountID      int64          `json:"account_id"`
	Fields         map[string]any `json:"fields"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type OfficialAccountWriter interface {
	UpdateAccount(context.Context, int64, map[string]any) error
}

func ApplyAccountUpdate(ctx context.Context, writer OfficialAccountWriter, command AccountUpdateCommand) error {
	if command.ActorID <= 0 || command.AccountID <= 0 || strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" || len(command.Fields) == 0 {
		return errors.New("invalid account update command")
	}
	if writer == nil {
		return errors.New("official account writer is unavailable")
	}
	return writer.UpdateAccount(ctx, command.AccountID, command.Fields)
}
