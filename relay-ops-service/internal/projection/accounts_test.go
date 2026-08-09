package projection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

func TestProjectionAccountsRejectsStaleOutOfOrderFields(t *testing.T) {
	p := NewAccounts()
	newer := time.Date(2026, 8, 9, 0, 0, 2, 0, time.UTC)
	older := newer.Add(-time.Second)
	input := []events.Event{
		accountHealthEvent("550e8400-e29b-41d4-a716-446655440002", newer, "healthy"),
		accountHealthEvent("550e8400-e29b-41d4-a716-446655440001", older, "unhealthy"),
		accountBalanceEvent("550e8400-e29b-41d4-a716-446655440004", newer, "12.50"),
		accountBalanceEvent("550e8400-e29b-41d4-a716-446655440003", older, "1.25"),
	}
	for _, event := range input {
		if err := p.Handle(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	row := p.Rows[7]
	if row.Status != "healthy" || row.Balance != "12.5" || !row.ObservedAt.Equal(newer) {
		t.Fatalf("stale event overwrote current row: %+v", row)
	}
	assertMetadata(t, row.Metadata, "550e8400-e29b-41d4-a716-446655440004", AccountsCalculationVersion)
}

func TestRebuildAccountsFromSnapshotAndEventsIsDeterministic(t *testing.T) {
	snapshotAt := time.Date(2026, 8, 8, 23, 59, 0, 0, time.UTC)
	snapshot := Snapshot{Accounts: []AccountRow{{
		AccountID: 7, Status: "healthy", Balance: "10.00", Currency: "USD", ObservedAt: snapshotAt,
		Metadata: Metadata{GeneratedAt: snapshotAt, SourceWatermark: "snapshot-1", FreshnessSeconds: 60, Completeness: CompletenessComplete, CalculationVersion: AccountsCalculationVersion},
	}}}
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	stream := []events.Event{
		accountBalanceEvent("550e8400-e29b-41d4-a716-446655440003", at.Add(2*time.Second), "12.50"),
		accountHealthEvent("550e8400-e29b-41d4-a716-446655440001", at, "unhealthy"),
		accountHealthEvent("550e8400-e29b-41d4-a716-446655440002", at.Add(time.Second), "healthy"),
		accountHealthEvent("550e8400-e29b-41d4-a716-446655440001", at, "unhealthy"),
	}
	if err := pRebuildAccounts(NewAccounts(), snapshot, stream); err != nil {
		t.Fatal(err)
	}
	first := NewAccounts()
	if err := first.Rebuild(context.Background(), snapshot, stream); err != nil {
		t.Fatal(err)
	}
	second := NewAccounts()
	if err := second.Rebuild(context.Background(), snapshot, []events.Event{stream[1], stream[3], stream[0], stream[2]}); err != nil {
		t.Fatal(err)
	}
	if first.Rows[7].Status != second.Rows[7].Status || first.Rows[7].Balance != second.Rows[7].Balance || first.Metadata.SourceWatermark != second.Metadata.SourceWatermark {
		t.Fatalf("rebuild differs: first=%+v second=%+v", first, second)
	}
	if first.Rows[7].Balance != "12.5" || first.Rows[7].Status != "healthy" {
		t.Fatalf("rebuilt account = %+v", first.Rows[7])
	}
}

func TestSnapshotJSONRoundTripPreservesPositionsForOutOfOrderRebuild(t *testing.T) {
	healthAt := time.Date(2026, 8, 10, 5, 0, 3, 0, time.UTC)
	balanceAt := healthAt.Add(time.Second)
	profitabilityAt := balanceAt.Add(time.Second)
	snapshot := Snapshot{
		Accounts: []AccountRow{{
			AccountID: 7, Status: "healthy", Balance: "12.50", Currency: "USD", ObservedAt: balanceAt,
			HealthOccurredAt: healthAt, HealthEventID: "health-current",
			BalanceOccurredAt: balanceAt, BalanceEventID: "balance-current",
			Metadata: Metadata{GeneratedAt: balanceAt, SourceWatermark: "balance-current", FreshnessSeconds: 0, Completeness: CompletenessComplete, CalculationVersion: AccountsCalculationVersion},
		}},
		Profitability: []ProfitabilityRow{{
			AccountID: 7, Requests: 5, Revenue: "10", Cost: "4", Profit: "6", Margin: "0.6", Rank: 1,
			SourceOccurredAt: profitabilityAt,
			Metadata:         Metadata{GeneratedAt: profitabilityAt, SourceWatermark: "profitability-current", FreshnessSeconds: 0, Completeness: CompletenessComplete, CalculationVersion: ProfitabilityCalculationVersion},
		}},
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped Snapshot
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatal(err)
	}
	account := roundTripped.Accounts[0]
	if !account.HealthOccurredAt.Equal(healthAt) || account.HealthEventID != "health-current" ||
		!account.BalanceOccurredAt.Equal(balanceAt) || account.BalanceEventID != "balance-current" {
		t.Fatalf("account positions lost after JSON round trip: %+v", account)
	}
	if got := roundTripped.Profitability[0].SourceOccurredAt; !got.Equal(profitabilityAt) {
		t.Fatalf("profitability position lost after JSON round trip: %s", got)
	}

	accounts := NewAccounts()
	accountEvents := []events.Event{
		accountHealthEvent("health-stale", healthAt.Add(-time.Second), "unhealthy"),
		accountBalanceEvent("balance-stale", balanceAt.Add(-time.Second), "1.25"),
	}
	if err := accounts.Rebuild(context.Background(), roundTripped, accountEvents); err != nil {
		t.Fatal(err)
	}
	if row := accounts.Rows[7]; row.Status != "healthy" || row.Balance != "12.5" {
		t.Fatalf("stale account events overwrote round-tripped snapshot: %+v", row)
	}

	profitability := NewProfitability()
	profitabilityEvents := []events.Event{
		requestEvent("profitability-stale", profitabilityAt.Add(-time.Second), 7, "100", "1", "1"),
		requestEvent("profitability-next", profitabilityAt.Add(time.Second), 7, "2", "1", "1"),
	}
	if err := profitability.Rebuild(context.Background(), roundTripped, profitabilityEvents); err != nil {
		t.Fatal(err)
	}
	if row := profitability.Rows[7]; row.Requests != 6 || row.Revenue != "12" || row.Cost != "5" {
		t.Fatalf("profitability rebuild re-applied pre-snapshot event: %+v", row)
	}
}

func pRebuildAccounts(p *Accounts, snapshot Snapshot, stream []events.Event) error {
	return p.Rebuild(context.Background(), snapshot, stream)
}

func assertMetadata(t *testing.T, metadata Metadata, watermark, version string) {
	t.Helper()
	if metadata.GeneratedAt.IsZero() || metadata.SourceWatermark != watermark || metadata.FreshnessSeconds < 0 || metadata.Completeness != CompletenessComplete || metadata.CalculationVersion != version {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func accountHealthEvent(id string, at time.Time, status string) events.Event {
	return events.Event{EventID: id, Type: events.AccountHealthChanged, OccurredAt: at, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"account_id":7,"status":"` + status + `","checked_at":"` + at.Format(time.RFC3339Nano) + `"}`)}
}

func accountBalanceEvent(id string, at time.Time, balance string) events.Event {
	return events.Event{EventID: id, Type: events.AccountBalanceSnapshot, OccurredAt: at, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"account_id":7,"balance":"` + balance + `","currency":"USD","captured_at":"` + at.Format(time.RFC3339Nano) + `"}`)}
}
