package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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
	if _, err := decimal.NewFromString(s.Amount); err != nil {
		return errors.New("invalid balance amount")
	}
	return nil
}

// IsFreshAt reports whether a balance fact may still be used at now. A fact
// expires exactly at FreshUntil so callers never treat the boundary as fresh.
func (s BalanceSnapshot) IsFreshAt(now time.Time) bool {
	return s.Validate() == nil && now.Before(s.FreshUntil)
}

type BalanceValue struct {
	Amount   string
	Currency string
}

type BalanceReader interface {
	ReadBalance(context.Context) (BalanceValue, error)
}

type BalanceSnapshotWriter interface {
	AppendBalanceSnapshot(context.Context, BalanceSnapshot) (bool, error)
}

type balanceReaderFunc func(context.Context) (BalanceValue, error)

func (f balanceReaderFunc) ReadBalance(ctx context.Context) (BalanceValue, error) { return f(ctx) }

// BalanceCollector writes only append-only control-plane facts. It has no
// dependency on the core account tables or the request-routing path.
type BalanceCollector struct {
	Reader   BalanceReader
	Writer   BalanceSnapshotWriter
	Now      func() time.Time
	FreshFor time.Duration
	Source   string
}

func (c BalanceCollector) Collect(ctx context.Context, accountID int64) (BalanceSnapshot, error) {
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	return c.CollectAt(ctx, accountID, now)
}

// CollectAt gives one scheduled observation a stable identity across retries.
func (c BalanceCollector) CollectAt(ctx context.Context, accountID int64, observedAt time.Time) (BalanceSnapshot, error) {
	if accountID <= 0 || c.Reader == nil || c.Writer == nil || c.FreshFor <= 0 || strings.TrimSpace(c.Source) == "" {
		return BalanceSnapshot{}, errors.New("balance collector is not configured")
	}
	value, err := c.Reader.ReadBalance(ctx)
	if err != nil {
		return BalanceSnapshot{}, fmt.Errorf("read provider balance: %w", err)
	}
	observedAt = observedAt.UTC()
	snapshot := BalanceSnapshot{
		AccountID: accountID, Amount: strings.TrimSpace(value.Amount), Currency: strings.ToUpper(strings.TrimSpace(value.Currency)),
		ObservedAt: observedAt, FreshUntil: observedAt.Add(c.FreshFor), Source: strings.TrimSpace(c.Source),
	}
	if err := snapshot.Validate(); err != nil {
		return BalanceSnapshot{}, err
	}
	if _, err := c.Writer.AppendBalanceSnapshot(ctx, snapshot); err != nil {
		return BalanceSnapshot{}, fmt.Errorf("append balance snapshot: %w", err)
	}
	return snapshot, nil
}

type AccountUpdateCommand struct {
	CommandID      string         `json:"command_id"`
	ActorID        int64          `json:"actor_id"`
	AccountID      int64          `json:"account_id"`
	Fields         map[string]any `json:"fields"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type OfficialAccountWriter interface {
	SendAccountUpdateCommand(context.Context, AccountUpdateCommand) error
}

func ApplyAccountUpdate(ctx context.Context, writer OfficialAccountWriter, command AccountUpdateCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if writer == nil {
		return errors.New("official account writer is unavailable")
	}
	return writer.SendAccountUpdateCommand(ctx, command)
}

var ErrAccountUpdatePending = errors.New("account update command remains pending")
var ErrAccountUpdateFailed = errors.New("account update command previously failed")

type AccountUpdateCommandAudit interface {
	ClaimAccountUpdateCommand(context.Context, AccountUpdateCommand, string, int) (bool, string, error)
	CompleteAccountUpdateCommand(context.Context, AccountUpdateCommand, string, int) error
}

func (c AccountUpdateCommand) CanonicalPayloadHash() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(c.Fields)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (c AccountUpdateCommand) Validate() error {
	if c.ActorID <= 0 || c.AccountID <= 0 || strings.TrimSpace(c.CommandID) == "" || strings.TrimSpace(c.IdempotencyKey) == "" || len(c.Fields) == 0 {
		return errors.New("invalid account update command")
	}
	for field := range c.Fields {
		switch field {
		case "rate_multiplier", "priority", "status":
		default:
			return fmt.Errorf("account update field %q is not allowed", field)
		}
	}
	return nil
}

// ExecuteAccountUpdate reuses the control-plane's durable command audit for
// a narrowly allowed official account update. A replay never reaches the
// official writer, and a completion failure is returned to the caller.
func ExecuteAccountUpdate(ctx context.Context, writer OfficialAccountWriter, audit AccountUpdateCommandAudit, command AccountUpdateCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if writer == nil || audit == nil {
		return errors.New("account update command dependencies are unavailable")
	}
	payloadHash, err := command.CanonicalPayloadHash()
	if err != nil {
		return err
	}
	dispatch, storedResult, err := audit.ClaimAccountUpdateCommand(ctx, command, payloadHash, 1)
	if err != nil {
		return err
	}
	if !dispatch {
		if storedResult == "pending" || storedResult == "processing" || storedResult == "" {
			return ErrAccountUpdatePending
		}
		if storedResult == "failed" {
			return ErrAccountUpdateFailed
		}
		return nil
	}
	err = writer.SendAccountUpdateCommand(ctx, command)
	result := "accepted"
	if err != nil {
		result = "failed"
	}
	if completeErr := audit.CompleteAccountUpdateCommand(ctx, command, result, 1); completeErr != nil {
		return errors.Join(err, completeErr)
	}
	return err
}
