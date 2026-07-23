package ops

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
	"example.invalid/internal-test-service/internal/testsupport"
)

func TestTickSyncsAndReportsOncePerShanghaiHour(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := testsupport.NewFake()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cs := &credits.Service{Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000, CostMultiplierBPS: 700, CostPolicyID: "test-policy", CostPolicyQualified: true, Mode: "write"}
	reporter := &Reporter{Store: st, Credits: cs, Timezone: loc}
	scheduler := &Scheduler{Store: st, Credits: cs, Reporter: reporter, Timezone: loc}
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, loc)
	if err := scheduler.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Tick(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if scheduler.lastDaily != "2026-07-19" {
		t.Fatalf("last daily %s", scheduler.lastDaily)
	}
}

func TestReadOnlyTickObservesWithoutMutatingState(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RegisterUser(ctx, store.InternalUser{UserID: 7, InviterUserID: sql.NullInt64{}, JoinedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	fake := testsupport.NewFake()
	fake.AddUser(sub2api.User{ID: 7})
	fake.AddUsage(7, 100_000, true)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cs := &credits.Service{Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000, CostMultiplierBPS: 700, Mode: "read_only"}
	reporter := &Reporter{Store: st, Credits: cs, Timezone: loc}
	scheduler := &Scheduler{Store: st, Credits: cs, Reporter: reporter, Timezone: loc}

	if err := scheduler.Tick(ctx, time.Date(2026, 7, 21, 10, 1, 0, 0, loc)); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetUsageCursor(ctx, 7)
	if err != nil || after != 0 {
		t.Fatalf("usage cursor mutated: after=%d err=%v", after, err)
	}
	user, err := st.GetInternalUser(ctx, 7)
	if err != nil || user.FirstUsageAt.Valid {
		t.Fatalf("first usage mutated: %+v err=%v", user, err)
	}
	grants, err := st.ListAllGrants(ctx)
	if err != nil || len(grants) != 0 {
		t.Fatalf("grants mutated: %+v err=%v", grants, err)
	}
	if len(fake.Writes) != 0 {
		t.Fatalf("provider writes = %d", len(fake.Writes))
	}
	usage, err := st.SumSuccessfulUsage(ctx)
	if err != nil || usage != 0 {
		t.Fatalf("usage records mutated: usage=%s err=%v", usage, err)
	}
}

func TestRunRecordsCompletedTickStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	fake := testsupport.NewFake()
	cs := &credits.Service{Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000, Mode: "read_only"}
	ticks := make(chan time.Time, 1)
	scheduler := &Scheduler{Store: st, Credits: cs, Timezone: loc, TickC: ticks}
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	want := time.Date(2026, 7, 21, 10, 2, 0, 0, loc)
	ticks <- want
	deadline := time.Now().Add(time.Second)
	for {
		status := scheduler.Status()
		if status.LastTick.Equal(want) {
			if status.LastTickOK == false {
				t.Fatalf("status = %+v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tick was not recorded: %+v", status)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
}
