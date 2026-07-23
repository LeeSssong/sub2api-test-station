package credits

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/testsupport"
)

func newCreditService(t *testing.T) (*Service, *testsupport.Fake, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	fake := testsupport.NewFake()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	svc := &Service{Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000, DailyLoginCredit: domain.DailyLoginCredit, CostMultiplierBPS: 700, CostPolicyID: "test-policy", CostPolicyQualified: true, Mode: "write"}
	t.Cleanup(func() { _ = st.Close() })
	return svc, fake, st
}

func TestGrantDailyLoginAccumulatesAcrossShanghaiMidnight(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	first := time.Date(2026, 7, 19, 15, 59, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)
	result, err := svc.GrantDailyLogin(ctx, 1, first)
	if err != nil || result.Kind != domain.GrantDailyLogin || result.IdempotencyKey != "d04-login-1-2026-07-19" {
		t.Fatalf("first=%+v err=%v", result, err)
	}
	replay, err := svc.GrantDailyLogin(ctx, 1, first)
	if err != nil || !replay.AlreadyApplied {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err := svc.GrantDailyLogin(ctx, 1, second); err != nil {
		t.Fatal(err)
	}
	balance, _ := fake.GetBalance(ctx, 1)
	if balance.Balance != 40_000_000 {
		t.Fatalf("balance=%s", balance.Balance)
	}
}

func TestConcurrentDailyLoginHasOneProviderEffect(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.GrantDailyLogin(ctx, 1, now); err != nil {
				t.Errorf("grant: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	balance, _ := fake.GetBalance(ctx, 1)
	if balance.Balance != domain.DailyLoginCredit || len(balance.History) != 1 {
		t.Fatalf("balance=%s history=%d", balance.Balance, len(balance.History))
	}
}

func TestUncertainDailyLoginLocksWritesUntilReconciliation(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	fake.FailBalanceBeforeCommit = true
	if _, err := svc.GrantDailyLogin(ctx, 1, time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected uncertain provider write")
	}
	reason, _ := st.GetReadOnlyReason(ctx)
	if reason != "uncertain daily login write" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestUncertainDailyLoginReconcilesAfterReadOnlyLockWhenHistoryAppears(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	key := "d04-login-1-2026-07-19"
	if _, err := st.CreateGrant(ctx, store.Grant{
		UserID: 1, Kind: domain.GrantDailyLogin, Amount: domain.DailyLoginCredit,
		GrantDate:      sql.NullString{String: "2026-07-19", Valid: true},
		IdempotencyKey: key, Status: domain.TaskUncertain, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetReadOnlyReason(ctx, "uncertain daily login write"); err != nil {
		t.Fatal(err)
	}
	if err := fake.AddBalance(ctx, 1, domain.DailyLoginCredit, key, "D04 daily login credit"); err != nil {
		t.Fatal(err)
	}

	result, err := svc.GrantDailyLogin(ctx, 1, now)
	if err != nil || !result.AlreadyApplied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	grant, err := st.FindGrantByIdempotencyKey(ctx, key)
	if err != nil || grant.Status != domain.TaskSucceeded {
		t.Fatalf("grant=%+v err=%v", grant, err)
	}
}

func addInternalUser(t *testing.T, st *store.Store, id int64, inviter sql.NullInt64) {
	t.Helper()
	if err := st.RegisterUser(context.Background(), store.InternalUser{UserID: id, InviterUserID: inviter, JoinedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckInAccumulatesAcrossShanghaiMidnight(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	first := time.Date(2026, 7, 19, 15, 59, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)
	if _, err := svc.CheckIn(ctx, 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckIn(ctx, 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckIn(ctx, 1, second); err != nil {
		t.Fatal(err)
	}
	b, _ := fake.GetBalance(ctx, 1)
	if b.Balance != 40_000_000 {
		t.Fatalf("balance %s", b.Balance)
	}
}

func TestProcessUsageDoesNotIssueLaunchReferralRewards(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	addInternalUser(t, st, 2, sql.NullInt64{Int64: 1, Valid: true})
	_, err := st.CreateGrant(ctx, store.Grant{UserID: 1, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: 2, Valid: true}, IdempotencyKey: "d04-referral-2", Status: "reserved", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	fake.AddUsage(2, 100_000, false)
	if got, err := svc.ProcessUsage(ctx, 2); err != nil || got.ReferralRewards != 0 {
		t.Fatalf("failed usage %+v %v", got, err)
	}
	fake.AddUsage(2, 100_000, true)
	if got, err := svc.ProcessUsage(ctx, 2); err != nil || got.ReferralRewards != 0 {
		t.Fatalf("successful usage %+v %v", got, err)
	}
	if got, err := svc.ProcessUsage(ctx, 2); err != nil || got.Records != 0 {
		t.Fatalf("duplicate usage %+v %v", got, err)
	}
	b, _ := fake.GetBalance(ctx, 1)
	if b.Balance != 0 || len(b.History) != 0 {
		t.Fatalf("launch referral reward was issued: %+v", b)
	}
}

func TestProcessUsageDoesNotMutateHistoricalReferralReservation(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	addInternalUser(t, st, 2, sql.NullInt64{Int64: 1, Valid: true})
	key := "d04-referral-2"
	if _, err := st.CreateGrant(ctx, store.Grant{UserID: 1, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: 2, Valid: true}, IdempotencyKey: key, Status: "reserved", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	fake.AddUsage(2, 100_000, true)
	result, err := svc.ProcessUsage(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReferralRewards != 0 {
		t.Fatalf("provider history was reported as a new reward: %+v", result)
	}
	grant, err := st.FindGrantByIdempotencyKey(ctx, key)
	if err != nil || grant.Status != "reserved" {
		t.Fatalf("grant=%+v err=%v", grant, err)
	}
}

func TestTimeoutAfterCommitRecoversByHistory(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	fake.TimeoutAfterCommit = true
	result, err := svc.CheckIn(ctx, 1, time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Amount != domain.CheckinGrant {
		t.Fatalf("result %+v", result)
	}
	if reason, _ := st.GetReadOnlyReason(ctx); reason != "" {
		t.Fatalf("unexpected read only %s", reason)
	}
}

func TestPendingCheckInReconcilesProviderHistoryBeforeReportingSuccess(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	key := "d04-checkin-1-2026-07-19"
	if _, err := st.CreateGrant(ctx, store.Grant{UserID: 1, Kind: domain.GrantCheckin, Amount: domain.CheckinGrant, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: key, Status: domain.TaskPending, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := fake.AddBalance(ctx, 1, domain.CheckinGrant, key, "D04 daily check-in"); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CheckIn(ctx, 1, now)
	if err != nil || !result.AlreadyApplied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	grant, err := st.FindGrantByIdempotencyKey(ctx, key)
	if err != nil || grant.Status != domain.TaskSucceeded {
		t.Fatalf("grant=%+v err=%v", grant, err)
	}
}

func TestPendingCheckInIsNotReportedAsAppliedWithoutProviderEvidence(t *testing.T) {
	svc, _, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	key := "d04-checkin-1-2026-07-19"
	if _, err := st.CreateGrant(ctx, store.Grant{UserID: 1, Kind: domain.GrantCheckin, Amount: domain.CheckinGrant, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: key, Status: domain.TaskPending, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckIn(ctx, 1, now); err == nil {
		t.Fatal("pending grant without provider evidence was reported as applied")
	}
}

func TestReconciliationDriftEntersReadOnly(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	if err := fake.AddBalance(ctx, 1, 1000, "outside-write", "external"); err != nil {
		t.Fatal(err)
	}
	r, err := svc.ReconcileUser(ctx, 1)
	if err != nil || r.Match {
		t.Fatalf("reconciliation %+v %v", r, err)
	}
	if reason, _ := st.GetReadOnlyReason(ctx); reason == "" {
		t.Fatal("missing read-only reason")
	}
}
