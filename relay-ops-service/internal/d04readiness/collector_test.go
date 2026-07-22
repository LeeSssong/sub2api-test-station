package d04readiness

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestCollectorDiscoversOnlyActiveSchedulableAccounts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	balance := 20.0
	reader := &fakeAccountReader{accounts: []sub2api.Account{
		{ID: 42, Name: "Second", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{9}},
		{ID: 7, Name: "First", Platform: "openai", Status: "active", Schedulable: true},
		{ID: 99, Name: "Scheduling disabled", Status: "active", Schedulable: false},
		{ID: 100, Name: "Inactive", Status: "disabled", Schedulable: true},
	}}
	collector := Collector{Accounts: reader, Clock: func() time.Time { return now }}
	snapshot, err := collector.Collect(context.Background(), Inputs{
		SnapshotID: "snapshot-1",
		BalanceEvidence: []BalanceEvidence{
			{AccountID: 42, BalanceUSD: &balance, RecordedAt: now.Add(-time.Minute)},
			{AccountID: 99, BalanceUSD: &balance, RecordedAt: now.Add(-time.Minute)},
		},
		QualityEvidence: []QualityEvidence{{
			AccountID: 42, Source: AccountAttributedNaturalTraffic, RecordedAt: now.Add(-time.Minute),
			SampleCount: 30, SuccessRate: 0.99, ErrorRate: 0.01, TTFTP95MS: 2000, TotalLatencyP95MS: 8000,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 {
		t.Fatalf("ListAccounts calls = %d, want 1", reader.calls)
	}
	if len(snapshot.ActiveUpstreams) != 2 || snapshot.ActiveUpstreams[0].AccountID != 7 || snapshot.ActiveUpstreams[1].AccountID != 42 {
		t.Fatalf("active upstreams = %#v", snapshot.ActiveUpstreams)
	}
	if snapshot.ActiveUpstreams[0].BalanceUSD != nil || snapshot.ActiveUpstreams[0].QualityRecordedAt != nil {
		t.Fatalf("missing evidence must remain missing: %#v", snapshot.ActiveUpstreams[0])
	}
	if snapshot.ActiveUpstreams[1].BalanceUSD == nil || *snapshot.ActiveUpstreams[1].BalanceUSD != 20 {
		t.Fatalf("account 42 balance = %#v", snapshot.ActiveUpstreams[1].BalanceUSD)
	}
	if snapshot.UpstreamDiscovery.AccountSetSHA256 == "" {
		t.Fatal("account-set hash is empty")
	}
	for _, account := range snapshot.ActiveUpstreams {
		if account.AccountID == 99 || account.AccountID == 100 {
			t.Fatalf("sidecar or inactive account entered membership: %#v", account)
		}
	}
}

type fakeAccountReader struct {
	accounts []sub2api.Account
	err      error
	calls    int
}

func (f *fakeAccountReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	f.calls++
	return f.accounts, f.err
}
