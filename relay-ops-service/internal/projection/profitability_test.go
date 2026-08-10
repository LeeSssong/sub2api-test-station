package projection

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

func TestRebuildProfitabilityAccountingAndRankingMatchesGoldenSample(t *testing.T) {
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Profitability: []ProfitabilityRow{{
		AccountID: 7, Requests: 1, Revenue: "10.00", Cost: "3.00", Profit: "7.00", Rank: 1,
		Metadata: Metadata{GeneratedAt: at.Add(-time.Minute), SourceWatermark: "snapshot-1", FreshnessSeconds: 60, Completeness: CompletenessComplete, CalculationVersion: ProfitabilityCalculationVersion},
	}}}
	eventsAfterSnapshot := []events.Event{
		requestEvent("550e8400-e29b-41d4-a716-446655440003", at.Add(3*time.Second), 7, "2.00", "1.00", "1.00"),
		requestEvent("550e8400-e29b-41d4-a716-446655440002", at.Add(2*time.Second), 8, "5.00", "1.00", "1.00"),
		requestEvent("550e8400-e29b-41d4-a716-446655440001", at.Add(time.Second), 7, "1.00", "0.50", "0.75"),
		requestEvent("550e8400-e29b-41d4-a716-446655440002", at.Add(2*time.Second), 8, "5.00", "1.00", "1.00"),
	}

	profitability := NewProfitability()
	if err := profitability.Rebuild(context.Background(), snapshot, eventsAfterSnapshot); err != nil {
		t.Fatal(err)
	}
	if profitability.Requests != 4 || profitability.Revenue != "18" || profitability.Cost != "5.5" || profitability.Profit != "12.5" {
		t.Fatalf("profitability totals = %+v", profitability)
	}
	if profitability.Rows[7].Requests != 3 || profitability.Rows[7].Profit != "8.5" || profitability.Rows[7].Rank != 1 {
		t.Fatalf("account 7 = %+v", profitability.Rows[7])
	}
	if profitability.Rows[8].Requests != 1 || profitability.Rows[8].Profit != "4" || profitability.Rows[8].Rank != 2 {
		t.Fatalf("account 8 = %+v", profitability.Rows[8])
	}
	assertMetadata(t, profitability.Rows[7].Metadata, "550e8400-e29b-41d4-a716-446655440003", ProfitabilityCalculationVersion)

	accounting := NewAccounting()
	if err := accounting.Rebuild(context.Background(), snapshot, eventsAfterSnapshot); err != nil {
		t.Fatal(err)
	}
	if accounting.Requests != 3 || accounting.Revenue != "8" || accounting.Cost != "2.5" {
		t.Fatalf("incremental accounting = %+v", accounting)
	}
	assertMetadata(t, accounting.Metadata, "550e8400-e29b-41d4-a716-446655440003", AccountingCalculationVersion)

	reconciliation := NewReconciliation()
	if err := reconciliation.Rebuild(context.Background(), Snapshot{}, eventsAfterSnapshot); err != nil {
		t.Fatal(err)
	}
	if reconciliation.Matched != 2 || reconciliation.Exceptions != 1 {
		t.Fatalf("reconciliation = %+v", reconciliation)
	}
	assertMetadata(t, reconciliation.Metadata, "550e8400-e29b-41d4-a716-446655440003", ReconciliationCalculationVersion)
}

func TestProjectionProfitabilityUsesDecimalAggregationWithoutFloatDrift(t *testing.T) {
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	p := NewProfitability()
	for index, charge := range []string{"0.1", "0.2"} {
		event := requestEvent([]string{"550e8400-e29b-41d4-a716-446655440001", "550e8400-e29b-41d4-a716-446655440002"}[index], at.Add(time.Duration(index)*time.Second), 7, charge, "0.01", "0.01")
		if err := p.Handle(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if p.Revenue != "0.3" || p.Cost != "0.02" || p.Profit != "0.28" {
		t.Fatalf("decimal totals = revenue %q cost %q profit %q", p.Revenue, p.Cost, p.Profit)
	}
}

func requestEvent(id string, at time.Time, accountID int64, charge, actualCost, costUSD string) events.Event {
	return events.Event{EventID: id, Type: events.RequestCompleted, OccurredAt: at, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"request_id":"request-` + id + `","account_id":` + decimalAccountID(accountID) + `,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"` + charge + `","actual_cost":"` + actualCost + `","cost_usd":"` + costUSD + `","currency":"USD"}`)}
}

func decimalAccountID(value int64) string {
	if value == 7 {
		return "7"
	}
	return "8"
}
