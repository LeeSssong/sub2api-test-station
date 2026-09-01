package cutover

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestBuildOpeningPreservesSignedLegacyBalanceAndZeroGift(t *testing.T) {
	opening, err := BuildOpening(42, decimal.NewFromInt(-12), "legacy balance")
	if err != nil {
		t.Fatal(err)
	}
	if opening.UserID != 42 || !opening.Paid.Equal(decimal.NewFromInt(-12)) || !opening.Gift.IsZero() {
		t.Fatalf("unexpected opening: %+v", opening)
	}
}

func TestDryRunReportsResidualAndBlocksCutover(t *testing.T) {
	report := DryRunReport{Users: 1, UnattributedDelta: decimal.NewFromInt(3), ReconciliationPassed: false}
	if err := report.ValidateCutoverGate(); err == nil {
		t.Fatal("unreconciled residual must block cutover")
	}
}

func TestCutoverStateMachineRequiresReconciliationAndIsIdempotent(t *testing.T) {
	var sm StateMachine
	if err := sm.Advance(StatePrepared); err != nil {
		t.Fatal(err)
	}
	if err := sm.Advance(StateCutover); err == nil {
		t.Fatal("cutover before reconciliation must fail")
	}
	if err := sm.Advance(StateReconciled); err != nil {
		t.Fatal(err)
	}
	if err := sm.Advance(StateCutover); err != nil {
		t.Fatal(err)
	}
	if err := sm.Advance(StateCutover); err != nil {
		t.Fatal("repeating cutover should be idempotent")
	}
}
