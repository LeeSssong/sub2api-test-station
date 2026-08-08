package billing

import (
	"context"
	"testing"
	"time"
)

type fakeWriter struct {
	id     int64
	fields map[string]any
}

func (f *fakeWriter) UpdateAccount(_ context.Context, id int64, fields map[string]any) error {
	f.id = id
	f.fields = fields
	return nil
}
func TestBalanceSnapshotAndOfficialWriteContract(t *testing.T) {
	now := time.Now().UTC()
	if err := (BalanceSnapshot{AccountID: 1, Amount: "1.20", Currency: "USD", ObservedAt: now, FreshUntil: now.Add(time.Minute), Source: "provider"}).Validate(); err != nil {
		t.Fatal(err)
	}
	w := &fakeWriter{}
	if err := ApplyAccountUpdate(context.Background(), w, AccountUpdateCommand{CommandID: "cmd-1", ActorID: 9, AccountID: 1, IdempotencyKey: "idem-1", Fields: map[string]any{"rate_multiplier": "1.1"}}); err != nil {
		t.Fatal(err)
	}
	if w.id != 1 {
		t.Fatal("official writer not called")
	}
}
