package credits

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/store"
)

func TestBudgetReportsButDoesNotReserveHistoricalReferralRewards(t *testing.T) {
	svc, fake, st := newCreditService(t)
	ctx := context.Background()
	addInternalUser(t, st, 1, sql.NullInt64{})
	addInternalUser(t, st, 2, sql.NullInt64{Int64: 1, Valid: true})
	if _, err := svc.CheckIn(ctx, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateGrant(ctx, structGrant(1, 2))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Budget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.PendingReferralReservations != 1 {
		t.Fatalf("pending %d", snap.PendingReferralReservations)
	}
	if snap.CurrentBalances != domain.CheckinGrant {
		t.Fatalf("balances %s", snap.CurrentBalances)
	}
	if snap.Occupancy != 1_400_000 {
		t.Fatalf("occupancy=%s includes a retired referral reservation", snap.Occupancy)
	}
	_ = fake
}

func TestCanGrantDailyLoginUsesTwentyDollarGrantCost(t *testing.T) {
	svc, _, _ := newCreditService(t)
	svc.TotalBudget = 1_000_000
	ok, err := svc.CanGrantDailyLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("daily login gate used less than the configured USD 20 grant")
	}
}

func structGrant(referrer, invitee int64) store.Grant {
	return store.Grant{UserID: referrer, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: invitee, Valid: true}, IdempotencyKey: "d04-referral-budget", Status: "reserved", CreatedAt: time.Now()}
}
