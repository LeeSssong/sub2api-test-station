package projection

import (
	"context"
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
