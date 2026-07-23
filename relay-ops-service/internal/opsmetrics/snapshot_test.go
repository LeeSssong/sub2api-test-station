package opsmetrics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestCollectProjectsPublicGroupsAndActiveSchedulableAccounts(t *testing.T) {
	reader := fakeReader{
		groups: []sub2api.Group{
			{ID: 2, Name: "Public Pro", Status: "active"},
			{ID: 3, Name: "Private Internal", Status: "active", IsExclusive: true},
			{ID: 4, Name: "Public Plus", Status: "active"},
		},
		accountRows: []sub2api.Account{
			{ID: 11, Name: "Disabled", Status: "disabled", Schedulable: true},
			{ID: 12, Name: "Paused", Status: "active", Schedulable: false},
			{ID: 30, Name: "Current", Status: "active", Schedulable: true, GroupIDs: []int64{4, 3, 2, 4}},
		},
		ops: map[object]sub2api.OpsSnapshot{
			{groupID: 2}: opsSnapshot(20),
			{groupID: 4}: opsSnapshot(10),
		},
		errs: map[object]error{
			{accountID: 30}: errors.New("upstream body containing a secret must not escape"),
		},
	}

	snapshot, err := Collect(context.Background(), &reader, time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Groups) != 2 || snapshot.Groups[0].ID != 2 || snapshot.Groups[1].ID != 4 {
		t.Fatalf("groups = %#v", snapshot.Groups)
	}
	if snapshot.Groups[0].Status != "ok" || snapshot.Groups[0].RequestCount != 20 {
		t.Fatalf("first group = %#v", snapshot.Groups[0])
	}
	if snapshot.Groups[1].Status != "sample_insufficient" || snapshot.Groups[1].RequestCount != 10 {
		t.Fatalf("second group = %#v", snapshot.Groups[1])
	}
	if len(snapshot.Accounts) != 1 || snapshot.Accounts[0].ID != 30 || snapshot.Accounts[0].Name != "Current" {
		t.Fatalf("accounts = %#v", snapshot.Accounts)
	}
	if account := snapshot.Accounts[0]; account.Status != "read_failed" || account.ErrorCode != "ops_snapshot_unavailable" {
		t.Fatalf("account = %#v", account)
	}
	if got := snapshot.Accounts[0].PublicGroupNames; len(got) != 2 || got[0] != "Public Pro" || got[1] != "Public Plus" {
		t.Fatalf("public group names = %#v", got)
	}
	for _, row := range snapshot.Groups {
		assertEvidenceHash(t, row.EvidenceHash)
	}
	assertEvidenceHash(t, snapshot.Accounts[0].EvidenceHash)
	if strings.Contains(snapshot.Accounts[0].ErrorCode, "secret") {
		t.Fatalf("error code leaks native error: %q", snapshot.Accounts[0].ErrorCode)
	}
	for _, query := range reader.queries {
		if query.TimeRange != "15m" {
			t.Fatalf("query = %#v, want 15m window", query)
		}
	}
}

func TestRuntimePercentageDisplaysUseTheirSourceScales(t *testing.T) {
	t.Parallel()

	group := GroupRuntime{ErrorRate: 0.075, SLA: 97.5}
	account := AccountRuntime{ErrorRate: 0.2, SLA: 99.5}
	if group.ErrorRateDisplay() != "7.50%" || group.SLADisplay() != "97.50%" {
		t.Fatalf("group displays = %q, %q", group.ErrorRateDisplay(), group.SLADisplay())
	}
	if account.ErrorRateDisplay() != "20.00%" || account.SLADisplay() != "99.50%" {
		t.Fatalf("account displays = %q, %q", account.ErrorRateDisplay(), account.SLADisplay())
	}
}

func TestCollectReturnsSourceErrorWhenGroupOrAccountListFails(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader fakeReader
		want   error
	}{
		{
			name:   "groups",
			reader: fakeReader{groupErr: errors.New("reader response contains secret-group-token")},
			want:   ErrGroupsSourceUnavailable,
		},
		{
			name:   "accounts",
			reader: fakeReader{accountErr: errors.New("reader response contains secret-account-token")},
			want:   ErrAccountsSourceUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Collect(context.Background(), &test.reader, time.Now())
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "secret-") {
				t.Fatalf("source error leaks native reader message: %q", err)
			}
		})
	}
}

func TestCollectCanonicalizesAccountGroupIDsForOrderingAndEvidence(t *testing.T) {
	first := fakeReader{accountRows: []sub2api.Account{
		{ID: 30, Name: "Later", Status: "active", Schedulable: true, GroupIDs: []int64{9, 2, 4}},
		{ID: 10, Name: "First", Status: "active", Schedulable: true, GroupIDs: []int64{4, 2, 9}},
	}}
	second := fakeReader{accountRows: []sub2api.Account{
		{ID: 30, Name: "Later", Status: "active", Schedulable: true, GroupIDs: []int64{2, 9, 4}},
		{ID: 10, Name: "First", Status: "active", Schedulable: true, GroupIDs: []int64{9, 4, 2}},
	}}

	firstSnapshot, err := Collect(context.Background(), &first, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := Collect(context.Background(), &second, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(firstSnapshot.Accounts) != 2 || firstSnapshot.Accounts[0].ID != 10 || firstSnapshot.Accounts[1].ID != 30 {
		t.Fatalf("accounts = %#v", firstSnapshot.Accounts)
	}
	if got := firstSnapshot.Accounts[0].GroupIDs; len(got) != 3 || got[0] != 2 || got[1] != 4 || got[2] != 9 {
		t.Fatalf("group IDs = %#v", got)
	}
	if firstSnapshot.Accounts[0].EvidenceHash != secondSnapshot.Accounts[0].EvidenceHash || firstSnapshot.Accounts[1].EvidenceHash != secondSnapshot.Accounts[1].EvidenceHash {
		t.Fatalf("evidence hashes vary with source group order: %#v != %#v", firstSnapshot.Accounts, secondSnapshot.Accounts)
	}
}

func assertEvidenceHash(t *testing.T, value string) {
	t.Helper()
	if len(value) != 64 {
		t.Fatalf("evidence hash = %q", value)
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			t.Fatalf("evidence hash = %q", value)
		}
	}
}

type object struct {
	groupID   int64
	accountID int64
}

type fakeReader struct {
	groups      []sub2api.Group
	accountRows []sub2api.Account
	groupErr    error
	accountErr  error
	ops         map[object]sub2api.OpsSnapshot
	errs        map[object]error
	queries     []sub2api.OpsQuery
}

func (r *fakeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return r.groups, r.groupErr
}

func (r *fakeReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return r.accountRows, r.accountErr
}

func (r *fakeReader) GetOpsSnapshot(_ context.Context, query sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	r.queries = append(r.queries, query)
	key := object{groupID: query.GroupID, accountID: query.AccountID}
	if err := r.errs[key]; err != nil {
		return sub2api.OpsSnapshot{}, err
	}
	return r.ops[key], nil
}

func opsSnapshot(requests int64) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{
		RequestCountTotal: requests,
		ErrorRate:         0.025,
		SLA:               97.5,
		TTFT:              sub2api.Percentiles{P95MS: 1250},
		Duration:          sub2api.Percentiles{P95MS: 2500},
	}}
}
