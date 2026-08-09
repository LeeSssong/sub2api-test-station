package sub2api

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCoreOutboxRequiresExplicitCoreDatabaseURL(t *testing.T) {
	for _, value := range []string{"", "   "} {
		if _, err := NewCoreOutbox(context.Background(), value); err == nil {
			t.Fatalf("NewCoreOutbox(%q) accepted missing URL", value)
		}
	}
}

func TestCoreOutboxReclaimsExpiredLeaseAndRejectsStaleOwner(t *testing.T) {
	url := os.Getenv("RELAY_OPS_TEST_CORE_DATABASE_URL")
	if url == "" {
		t.Skip("RELAY_OPS_TEST_CORE_DATABASE_URL is required")
	}
	ctx := context.Background()
	outbox, err := NewCoreOutbox(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	_, err = outbox.pool.Exec(ctx, `DROP TABLE IF EXISTS externalization_outbox; CREATE TABLE externalization_outbox (event_id TEXT PRIMARY KEY, event_type TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL, source_version TEXT NOT NULL, contract_version INTEGER NOT NULL, payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending', available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), claimed_by TEXT, claim_token TEXT, claimed_at TIMESTAMPTZ, attempts INTEGER NOT NULL DEFAULT 0, published_at TIMESTAMPTZ, last_error_class TEXT, last_error TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = outbox.pool.Exec(ctx, `INSERT INTO externalization_outbox (event_id,event_type,occurred_at,source_version,contract_version,payload) VALUES ('550e8400-e29b-41d4-a716-446655440111','account.health_changed',$1,'core',1,'{"account_id":7,"status":"healthy","checked_at":"2026-08-10T00:00:00Z"}')`, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	first, err := outbox.ClaimBatch(ctx, "relay-a", 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	if _, err := outbox.pool.Exec(ctx, `UPDATE externalization_outbox SET claimed_at=NOW()-INTERVAL '61 seconds' WHERE event_id=$1`, first[0].Event.EventID); err != nil {
		t.Fatal(err)
	}
	second, err := outbox.ClaimBatch(ctx, "relay-b", 1)
	if err != nil || len(second) != 1 || second[0].Token == first[0].Token {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	if err := outbox.MarkPublished(ctx, first[0], "relay-a"); err == nil {
		t.Fatal("stale publish succeeded")
	}
	if err := outbox.MarkFailed(ctx, first[0], "relay-a", context.Canceled); err == nil {
		t.Fatal("stale failure acknowledgement succeeded")
	}
	if err := outbox.MarkPublished(ctx, second[0], "relay-b"); err != nil {
		t.Fatal(err)
	}
}

func TestCoreOutboxPumpRequiresPersistentConsumer(t *testing.T) {
	if err := (&CoreOutbox{}).PumpOnce(context.Background(), "relay-ops", nil); err == nil {
		t.Fatal("PumpOnce accepted nil consumer")
	}
}
