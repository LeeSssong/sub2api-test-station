package cutover

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidOpening = errors.New("invalid migration opening")
	ErrCutoverBlocked = errors.New("quota cutover blocked by reconciliation gate")
)

type Opening struct {
	UserID int64
	Paid   decimal.Decimal
	Gift   decimal.Decimal
	Note   string
}

func BuildOpening(userID int64, paid decimal.Decimal, note string) (Opening, error) {
	if userID <= 0 || strings.TrimSpace(note) == "" {
		return Opening{}, ErrInvalidOpening
	}
	return Opening{UserID: userID, Paid: paid, Gift: decimal.Zero, Note: strings.TrimSpace(note)}, nil
}

type DryRunReport struct {
	Users                int
	UnattributedDelta    decimal.Decimal
	ReconciliationPassed bool
}

func (r DryRunReport) ValidateCutoverGate() error {
	if r.Users < 0 || !r.UnattributedDelta.IsZero() || !r.ReconciliationPassed {
		return ErrCutoverBlocked
	}
	return nil
}

type State string

const (
	StatePrepared   State = "prepared"
	StateReconciled State = "reconciled"
	StateCutover    State = "cutover"
)

type StateMachine struct{ state State }

func (s *StateMachine) Advance(next State) error {
	if s == nil {
		return ErrCutoverBlocked
	}
	if s.state == next && next == StateCutover {
		return nil
	}
	switch {
	case s.state == "" && next == StatePrepared:
	case s.state == StatePrepared && next == StateReconciled:
	case s.state == StateReconciled && next == StateCutover:
	default:
		return ErrCutoverBlocked
	}
	s.state = next
	return nil
}

func (s StateMachine) State() State { return s.state }
