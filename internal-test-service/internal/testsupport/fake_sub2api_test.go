package testsupport

import (
	"context"
	"testing"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/sub2api"
)

func TestTimeoutAfterCommitIsIdempotent(t *testing.T) {
	f := NewFake()
	f.TimeoutAfterCommit = true
	err := f.AddBalance(context.Background(), 1, domain.CheckinGrant, "same-key", "checkin")
	if err == nil {
		t.Fatal("expected timeout")
	}
	f.TimeoutAfterCommit = false
	if err := f.AddBalance(context.Background(), 1, domain.CheckinGrant, "same-key", "checkin"); err != nil {
		t.Fatal(err)
	}
	b, _ := f.GetBalance(context.Background(), 1)
	if b.Balance != domain.CheckinGrant {
		t.Fatalf("balance %s", b.Balance)
	}
}

var _ sub2api.Client = (*Fake)(nil)
