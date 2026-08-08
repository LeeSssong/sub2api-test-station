package projection

import (
	"context"
	"example.invalid/relay-ops-service/internal/events"
	"testing"
	"time"
)

func TestAccountsProjectionAppliesHealthAndBalance(t *testing.T) {
	p := NewAccounts()
	at := time.Now().UTC()
	for _, e := range []events.Event{{EventID: "1", Type: events.AccountHealthChanged, OccurredAt: at, SourceVersion: "core", ContractVersion: events.ContractVersion, Payload: []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:00Z"}`)}, {EventID: "2", Type: events.AccountBalanceSnapshot, OccurredAt: at, SourceVersion: "core", ContractVersion: events.ContractVersion, Payload: []byte(`{"account_id":7,"balance":"1.25","currency":"USD","captured_at":"2026-08-09T00:00:01Z"}`)}} {
		if err := p.Handle(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if p.Rows[7].Balance != "1.25" || p.Metadata.Completeness != "complete" {
		t.Fatalf("projection not complete: %+v", p)
	}
}
