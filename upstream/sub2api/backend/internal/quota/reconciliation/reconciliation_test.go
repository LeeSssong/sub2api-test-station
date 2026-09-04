package reconciliation

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
)

func TestLoadSnapshotReadsWalletsAndGrantsWithoutWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT user_id, paid_quota_balance_usd, gift_quota_balance_usd FROM user_wallets`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "paid", "gift"}).AddRow(7, "3.25", "1.00"))
	mock.ExpectQuery(`SELECT id, user_id, paid_quota_usd, gift_quota_usd, consumed_paid_quota_usd, consumed_gift_quota_usd, refunded_paid_quota_usd, deducted_gift_quota_usd, reserved_paid_quota_usd, legacy_debt_offset_paid_quota_usd FROM user_quota_grants`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "paid", "gift", "pc", "gc", "pr", "gd", "rr", "off"}).AddRow(1, 7, "4", "1", "0", "0", "0", "0", "0", "0"))
	mock.ExpectQuery(`SELECT id::text, user_id, delta_usd, paid_quota_delta_usd, gift_quota_delta_usd, attribution_status, paid_grant_allocations, gift_grant_allocations FROM billing_usage_entries`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "delta", "paid", "gift", "status", "paid_alloc", "gift_alloc"}).AddRow("u1", 7, "0", "0", "0", "exact", "[]", "[]"))
	mock.ExpectQuery(`SELECT po.id::text, po.refunded_paid_quota_usd`).WillReturnRows(sqlmock.NewRows([]string{"id", "refunded", "adjusted"}))
	mock.ExpectQuery(`SELECT user_id::text \|\| ':' \|\| idempotency_key`).WillReturnRows(sqlmock.NewRows([]string{"key"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM billing_usage_entries WHERE attribution_status = 'exact'`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_quota_adjustments WHERE status IN`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_quota_adjustments a CROSS JOIN LATERAL`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(CASE WHEN attribution_status = 'legacy_unknown'`).WillReturnRows(sqlmock.NewRows([]string{"residual"}).AddRow("1.25000000"))
	snapshot, err := LoadSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Wallets) != 1 || snapshot.Wallets[0].PaidBalance.String() != "3.25" {
		t.Fatalf("unexpected wallets: %#v", snapshot.Wallets)
	}
	if len(snapshot.Grants) != 1 || snapshot.Grants[0].PaidGranted.String() != "4" {
		t.Fatalf("unexpected grants: %#v", snapshot.Grants)
	}
	if len(snapshot.Usage) != 1 || snapshot.Usage[0].Delta.String() != "0" || snapshot.Usage[0].AttributionStatus != "exact" {
		t.Fatalf("unexpected usage: %#v", snapshot.Usage)
	}
	if snapshot.LegacyUnknownResidual.String() != "1.25" {
		t.Fatalf("unexpected legacy residual: %s", snapshot.LegacyUnknownResidual)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateReportsWalletFormulaDifference(t *testing.T) {
	report := Evaluate(Snapshot{
		Wallets: []WalletSnapshot{{UserID: 7, PaidBalance: decimal.NewFromInt(3), GiftBalance: decimal.NewFromInt(1)}},
		Grants:  []GrantSnapshot{{ID: 1, UserID: 7, PaidGranted: decimal.NewFromInt(5), GiftGranted: decimal.NewFromInt(1), PaidConsumed: decimal.NewFromInt(1), GiftConsumed: decimal.Zero}},
		Usage:   []UsageSnapshot{{UserID: 7, GrantID: 1, PaidDelta: decimal.NewFromInt(1), AllocationValid: true}},
	})
	if !report.HasDifferences() {
		t.Fatal("expected wallet formula difference")
	}
	if report.Issues[0].Kind != IssueWalletFormula {
		t.Fatalf("unexpected issue kind: %s", report.Issues[0].Kind)
	}
}

func TestEvaluateReportsAllocationAndCrossUserGrant(t *testing.T) {
	report := Evaluate(Snapshot{
		Grants: []GrantSnapshot{{ID: 11, UserID: 2, PaidGranted: decimal.NewFromInt(4), PaidConsumed: decimal.NewFromInt(1)}},
		Usage:  []UsageSnapshot{{UserID: 3, GrantID: 11, PaidDelta: decimal.NewFromInt(2), AllocationValid: true}},
	})
	if !report.HasKind(IssueCrossUserGrant) || !report.HasKind(IssueAllocationMismatch) {
		t.Fatalf("expected cross-user and allocation issues, got %#v", report.Issues)
	}
}

func TestEvaluateReportsInvalidJSONDuplicateKeysUnknownReservationsAndRefundMismatch(t *testing.T) {
	report := Evaluate(Snapshot{
		DuplicateIdempotencyKeys: []string{"usage:k1"},
		InvalidAllocationRows:    2,
		UnknownReservations:      1,
		CrossOrderGrantRows:      1,
		LegacyUnknownResidual:    decimal.NewFromFloat(0.5),
		Refunds:                  []RefundSnapshot{{OrderID: "o1", Refunded: decimal.NewFromInt(2), Adjusted: decimal.NewFromInt(1)}},
	})
	for _, kind := range []IssueKind{IssueDuplicateIdempotency, IssueInvalidAllocation, IssueUnknownReservation, IssueCrossOrderGrant, IssueLegacyUnknown, IssueRefundMismatch} {
		if !report.HasKind(kind) {
			t.Fatalf("missing issue kind %s in %#v", kind, report.Issues)
		}
	}
}

func TestEvaluateCleanSnapshot(t *testing.T) {
	report := Evaluate(Snapshot{Wallets: []WalletSnapshot{{UserID: 1}}, Grants: []GrantSnapshot{{ID: 1, UserID: 1}}, Usage: []UsageSnapshot{{UserID: 1, AllocationValid: true}}})
	if report.HasDifferences() {
		t.Fatalf("clean snapshot reported issues: %#v", report.Issues)
	}
}

func TestEvaluateIncludesUserAndGlobalSummaries(t *testing.T) {
	report := Evaluate(Snapshot{
		Wallets: []WalletSnapshot{{UserID: 9}},
		Grants:  []GrantSnapshot{{ID: 1, UserID: 9}},
	})
	if _, ok := report.Users[9]; !ok {
		t.Fatalf("expected per-user summary")
	}
	if report.Global.Users != 1 || report.Global.Grants != 1 {
		t.Fatalf("unexpected global summary: %#v", report.Global)
	}
}

func TestValidAllocationJSONAcceptsCanonicalThreeFieldShape(t *testing.T) {
	if !validAllocationJSON([]byte(`[{"grant_id":1,"bucket":"paid","quota_usd":"2.00000000"}]`)) {
		t.Fatal("canonical allocation object must be accepted")
	}
	if validAllocationJSON([]byte(`[{"grant_id":1,"quota_usd":"2.00000000"}]`)) {
		t.Fatal("allocation without bucket must be rejected")
	}
}

func TestEvaluateReportsExactUsageDeltaMismatch(t *testing.T) {
	report := Evaluate(Snapshot{
		Grants: []GrantSnapshot{{ID: 1, UserID: 1, PaidConsumed: decimal.NewFromInt(2)}},
		Usage:  []UsageSnapshot{{ID: "u1", UserID: 1, PaidDelta: decimal.NewFromInt(-3), AttributionStatus: "exact", AllocationValid: true, Allocations: []Allocation{{GrantID: 1, Bucket: "paid", Quota: decimal.NewFromInt(2)}}}},
	})
	if !report.HasKind(IssueUsageDeltaMismatch) {
		t.Fatalf("expected signed delta mismatch: %#v", report.Issues)
	}
}

func TestEvaluateReportsExactUsageDeltaMismatchForEmptyAllocations(t *testing.T) {
	report := Evaluate(Snapshot{
		Usage: []UsageSnapshot{{ID: "u1", UserID: 1, PaidDelta: decimal.NewFromInt(-1), AttributionStatus: "exact", AllocationValid: true}},
	})
	if !report.HasKind(IssueUsageDeltaMismatch) {
		t.Fatalf("expected empty exact allocation mismatch: %#v", report.Issues)
	}
}
