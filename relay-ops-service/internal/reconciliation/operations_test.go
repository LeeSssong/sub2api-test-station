package reconciliation

import (
	"testing"
	"time"
)

func TestValidateOperationsScopeNormalizesAndRejectsInvalidInput(t *testing.T) {
	groupID := int64(3)
	scope, err := ValidateOperationsScope(OperationsScope{
		GroupID:  groupIDPointer(&groupID),
		Start:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		Currency: "usd",
		Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("ValidateOperationsScope: %v", err)
	}
	if scope.Currency != "USD" || scope.Timezone != "Asia/Shanghai" || scope.GroupID == nil || *scope.GroupID != 3 {
		t.Fatalf("normalized scope = %#v", scope)
	}

	invalidGroup := int64(0)
	if _, err := ValidateOperationsScope(OperationsScope{
		GroupID: &invalidGroup, Start: scope.Start, End: scope.End, Currency: "USD", Timezone: "UTC",
	}); err == nil {
		t.Fatal("zero group_id accepted")
	}
	if _, err := ValidateOperationsScope(OperationsScope{
		Start: scope.Start, End: scope.End, Currency: "USD", Timezone: "Mars/Olympus",
	}); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}

func groupIDPointer(value *int64) *int64 { return value }
