package sub2api

import (
	"context"
	"testing"
)

func TestCoreOutboxRequiresExplicitCoreDatabaseURL(t *testing.T) {
	for _, value := range []string{"", "   "} {
		if _, err := NewCoreOutbox(context.Background(), value); err == nil {
			t.Fatalf("NewCoreOutbox(%q) accepted missing URL", value)
		}
	}
}

func TestCoreOutboxPumpRequiresPersistentConsumer(t *testing.T) {
	if err := (&CoreOutbox{}).PumpOnce(context.Background(), "relay-ops", nil); err == nil {
		t.Fatal("PumpOnce accepted nil consumer")
	}
}
