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
			{ID: 4, Name: "Public Plus", Status: "active"},
		},
		accountRows: []sub2api.Account{
			{ID: 11, Name: "Disabled", Status: "disabled", Schedulable: true},
			{ID: 12, Name: "Paused", Status: "active", Schedulable: false},
			{ID: 30, Name: "Current", Status: "active", Schedulable: true, GroupIDs: []int64{2, 4}},
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

func TestCollectReturnsSourceErrorWhenGroupOrAccountListFails(t *testing.T) {
	_, err := Collect(context.Background(), &fakeReader{groupErr: errors.New("groups unavailable")}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "list groups") {
		t.Fatalf("group list error = %v", err)
	}
	_, err = Collect(context.Background(), &fakeReader{accountErr: errors.New("accounts unavailable")}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "list accounts") {
		t.Fatalf("account list error = %v", err)
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
