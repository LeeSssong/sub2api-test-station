package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/domain"
	"github.com/shopspring/decimal"
)

func TestCompareCashEventTreatsDatabasePrecisionAsEquivalent(t *testing.T) {
	accountID := int64(42)
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeAccountPurchase,
		PaidAt:     time.Date(2026, 7, 31, 2, 0, 0, 123456789, time.UTC),
		AmountCNY:  decimal.RequireFromString("68.123456789"),
		SourceKind: accounting.SourceKindOwnedOAuth,
		AccountID:  &accountID,
		Notes:      "plus account purchase",
	}
	stored := accounting.CashEvent{
		EventType:       input.EventType,
		PaidAt:          time.Date(2026, 7, 31, 2, 0, 0, 123456000, time.UTC),
		AmountCNY:       decimal.RequireFromString("68.12345679"),
		SourceKind:      input.SourceKind,
		AccountID:       &accountID,
		Notes:           input.Notes,
		CreatedByUserID: 7,
	}

	if err := compareCashEvent(stored, input); err != nil {
		t.Fatalf("compareCashEvent database-equivalent replay: %v", err)
	}
}

func TestCreateCashEventRejectsInvalidActorAndIdempotencyKey(t *testing.T) {
	st := &Store{}
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeFee,
		PaidAt:     time.Now(),
		AmountCNY:  decimal.NewFromInt(1),
		SourceKind: accounting.SourceKindUpstreamAPIKey,
	}
	if _, _, err := st.CreateCashEvent(nil, domain.AdminActor{}, input, "key"); err == nil {
		t.Fatal("CreateCashEvent accepted invalid actor")
	}
	if _, _, err := st.CreateCashEvent(nil, domain.AdminActor{UserID: 1}, input, " "); err == nil {
		t.Fatal("CreateCashEvent accepted blank idempotency key")
	}
}

func TestAccountingMigrationIsIdempotent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	var cashEventsTable, snapshotsTable string
	if err := st.pool.QueryRow(ctx, `
		SELECT to_regclass('relay_ops.accounting_cash_events')::text,
			to_regclass('relay_ops.accounting_daily_snapshots')::text`,
	).Scan(&cashEventsTable, &snapshotsTable); err != nil {
		t.Fatalf("read accounting migration tables: %v", err)
	}
	if cashEventsTable != "relay_ops.accounting_cash_events" ||
		snapshotsTable != "relay_ops.accounting_daily_snapshots" {
		t.Fatalf("accounting migration tables = %q, %q", cashEventsTable, snapshotsTable)
	}
}

func TestCreateCashEventNormalizesPrecisionAndDetectsChangedReplay(t *testing.T) {
	st, ctx := openMigratedAccountingStore(t)
	accountID := int64(42)
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeAccountPurchase,
		PaidAt:     time.Date(2026, 7, 31, 2, 0, 0, 123456789, time.UTC),
		AmountCNY:  decimal.RequireFromString("68.123456789"),
		SourceKind: accounting.SourceKindOwnedOAuth,
		AccountID:  &accountID,
		Notes:      "plus account purchase",
	}
	key := "accounting:precision-replay"

	first, created, err := st.CreateCashEvent(ctx, domain.AdminActor{UserID: 7}, input, key)
	if err != nil {
		t.Fatalf("first CreateCashEvent: %v", err)
	}
	if !created {
		t.Fatal("first CreateCashEvent reported replay")
	}
	if got := first.AmountCNY.StringFixed(8); got != "68.12345679" {
		t.Fatalf("first amount = %s, want 68.12345679", got)
	}
	wantPaidAt := time.Date(2026, 7, 31, 2, 0, 0, 123456000, time.UTC)
	if !first.PaidAt.Equal(wantPaidAt) {
		t.Fatalf("first paid_at = %s, want %s", first.PaidAt, wantPaidAt)
	}

	second, created, err := st.CreateCashEvent(ctx, domain.AdminActor{UserID: 8}, input, key)
	if err != nil {
		t.Fatalf("same replay CreateCashEvent: %v", err)
	}
	if created {
		t.Fatal("same replay CreateCashEvent reported insert")
	}
	assertSameCashEvent(t, second, first)
	if second.CreatedByUserID != 7 {
		t.Fatalf("replay actor = %d, want original actor 7", second.CreatedByUserID)
	}

	differentAccountID := int64(43)
	changedInputs := map[string]accounting.CashEventInput{
		"event type": {
			EventType: accounting.EventTypeFee, PaidAt: input.PaidAt, AmountCNY: input.AmountCNY,
			SourceKind: input.SourceKind, AccountID: input.AccountID, Notes: input.Notes,
		},
		"paid at": {
			EventType: input.EventType, PaidAt: input.PaidAt.Add(time.Microsecond), AmountCNY: input.AmountCNY,
			SourceKind: input.SourceKind, AccountID: input.AccountID, Notes: input.Notes,
		},
		"amount": {
			EventType: input.EventType, PaidAt: input.PaidAt, AmountCNY: input.AmountCNY.Add(decimal.NewFromFloat(0.01)),
			SourceKind: input.SourceKind, AccountID: input.AccountID, Notes: input.Notes,
		},
		"source kind": {
			EventType: input.EventType, PaidAt: input.PaidAt, AmountCNY: input.AmountCNY,
			SourceKind: accounting.SourceKindUpstreamAPIKey, AccountID: input.AccountID, Notes: input.Notes,
		},
		"account id": {
			EventType: input.EventType, PaidAt: input.PaidAt, AmountCNY: input.AmountCNY,
			SourceKind: input.SourceKind, AccountID: &differentAccountID, Notes: input.Notes,
		},
		"notes": {
			EventType: input.EventType, PaidAt: input.PaidAt, AmountCNY: input.AmountCNY,
			SourceKind: input.SourceKind, AccountID: input.AccountID, Notes: "changed notes",
		},
	}
	for name, changed := range changedInputs {
		t.Run(name, func(t *testing.T) {
			if _, _, err := st.CreateCashEvent(ctx, domain.AdminActor{UserID: 7}, changed, key); !errors.Is(err, ErrConflict) {
				t.Fatalf("changed replay error = %v, want ErrConflict", err)
			}
		})
	}
}

func TestCreateCashEventIsConcurrentIdempotent(t *testing.T) {
	st, ctx := openMigratedAccountingStore(t)
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeUpstreamTopup,
		PaidAt:     time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC),
		AmountCNY:  decimal.RequireFromString("200"),
		SourceKind: accounting.SourceKindUpstreamAPIKey,
		Notes:      "concurrent replay",
	}

	const workers = 16
	start := make(chan struct{})
	results := make(chan struct {
		event   accounting.CashEvent
		created bool
		err     error
	}, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			event, created, err := st.CreateCashEvent(
				ctx,
				domain.AdminActor{UserID: 11},
				input,
				"accounting:concurrent-replay",
			)
			results <- struct {
				event   accounting.CashEvent
				created bool
				err     error
			}{event: event, created: created, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var firstID int64
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent CreateCashEvent: %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if firstID == 0 {
			firstID = result.event.ID
		} else if result.event.ID != firstID {
			t.Fatalf("concurrent event ID = %d, want %d", result.event.ID, firstID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent created count = %d, want 1", createdCount)
	}
}

func TestReadUsageTotalsExcludesInternalRevenueButIncludesAllCostsAtShanghaiBoundary(t *testing.T) {
	st, ctx := openMigratedAccountingStore(t)
	window := accounting.NewDayWindow(time.Now())
	fixture := newAccountingUsageFixture(t, st)

	customerUser, customerKey := fixture.addUser(t, "user")
	adminUser, adminKey := fixture.addUser(t, "admin")
	internalUser, internalUserKey := fixture.addUser(t, "user")
	internalKeyUser, internalKey := fixture.addUser(t, "user")
	oauthAccount := fixture.addAccount(t, "oauth")
	apiKeyAccount := fixture.addAccount(t, "apikey")

	fixture.addUsage(t, customerUser, customerKey, oauthAccount, "8.00", "99.00", "4.00", "1.5", window.Start)
	fixture.addUsage(t, adminUser, adminKey, apiKeyAccount, "50.00", "2.00", "", "1", window.Start.Add(time.Hour))
	fixture.addUsage(t, internalUser, internalUserKey, oauthAccount, "50.00", "1.50", "", "1", window.Start.Add(2*time.Hour))
	fixture.addUsage(t, internalKeyUser, internalKey, apiKeyAccount, "50.00", "2.00", "", "1", window.Start.Add(3*time.Hour))
	fixture.addUsage(t, customerUser, customerKey, oauthAccount, "999.00", "999.00", "", "1", window.Start.Add(-time.Nanosecond))
	fixture.addUsage(t, customerUser, customerKey, oauthAccount, "999.00", "999.00", "", "1", window.End)

	got, err := st.ReadUsageTotals(ctx, window, accounting.ExclusionPolicy{
		InternalUserIDs:   []int64{internalUser},
		InternalAPIKeyIDs: []int64{internalKey},
	})
	if err != nil {
		t.Fatalf("ReadUsageTotals: %v", err)
	}
	if got.ExternalRevenueCNY.StringFixed(2) != "8.00" || got.ExternalRequests != 1 {
		t.Fatalf("external totals = %s / %d, want 8.00 / 1", got.ExternalRevenueCNY, got.ExternalRequests)
	}
	if got.InternalRequests != 3 {
		t.Fatalf("internal requests = %d, want 3", got.InternalRequests)
	}
	if got.CustomerCostCNY.StringFixed(2) != "6.00" {
		t.Fatalf("customer cost = %s, want 6.00", got.CustomerCostCNY)
	}
	if got.InternalCostCNY.StringFixed(2) != "5.50" {
		t.Fatalf("internal cost = %s, want 5.50", got.InternalCostCNY)
	}
	if got.CustomerCostCNY.Add(got.InternalCostCNY).StringFixed(2) != "11.50" {
		t.Fatalf("all cost = %s, want 11.50", got.CustomerCostCNY.Add(got.InternalCostCNY))
	}
	if got.OwnedOAuthCostCNY.StringFixed(2) != "7.50" {
		t.Fatalf("owned OAuth cost = %s, want 7.50", got.OwnedOAuthCostCNY)
	}
	if got.UpstreamAPIKeyCostCNY.StringFixed(2) != "4.00" {
		t.Fatalf("upstream API key cost = %s, want 4.00", got.UpstreamAPIKeyCostCNY)
	}
}

func TestCashEventTotalsAndListHandleRefundsAndShanghaiBoundary(t *testing.T) {
	st, ctx := openMigratedAccountingStore(t)
	window := accounting.NewDayWindow(time.Now())
	accountID := int64(501)
	actor := domain.AdminActor{UserID: 19}

	purchase, _, err := st.CreateCashEvent(ctx, actor, accounting.CashEventInput{
		EventType:  accounting.EventTypeAccountPurchase,
		PaidAt:     window.Start,
		AmountCNY:  decimal.RequireFromString("100.00"),
		SourceKind: accounting.SourceKindOwnedOAuth,
		AccountID:  &accountID,
		Notes:      "linked purchase",
	}, "accounting:list-purchase")
	if err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	refund, _, err := st.CreateCashEvent(ctx, actor, accounting.CashEventInput{
		EventType:  accounting.EventTypeRefund,
		PaidAt:     window.Start.Add(12 * time.Hour),
		AmountCNY:  decimal.RequireFromString("-20.00"),
		SourceKind: accounting.SourceKindOwnedOAuth,
		Notes:      "unlinked refund",
	}, "accounting:list-refund")
	if err != nil {
		t.Fatalf("create refund: %v", err)
	}
	if _, _, err := st.CreateCashEvent(ctx, actor, accounting.CashEventInput{
		EventType:  accounting.EventTypeFee,
		PaidAt:     window.End,
		AmountCNY:  decimal.RequireFromString("5.00"),
		SourceKind: accounting.SourceKindUpstreamAPIKey,
		Notes:      "next day fee",
	}, "accounting:list-next-day"); err != nil {
		t.Fatalf("create next-day fee: %v", err)
	}

	totals, err := st.ReadCashEventTotals(ctx, window)
	if err != nil {
		t.Fatalf("ReadCashEventTotals: %v", err)
	}
	if totals.OutflowCNY.StringFixed(2) != "80.00" {
		t.Fatalf("outflow = %s, want 80.00", totals.OutflowCNY)
	}
	if totals.UnlinkedOutflowCNY.StringFixed(2) != "-20.00" {
		t.Fatalf("unlinked outflow = %s, want -20.00", totals.UnlinkedOutflowCNY)
	}
	if totals.EventCount != 2 {
		t.Fatalf("event count = %d, want 2", totals.EventCount)
	}

	events, err := st.ListCashEvents(ctx, window.Start, window.End, 10)
	if err != nil {
		t.Fatalf("ListCashEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event list length = %d, want 2", len(events))
	}
	assertSameCashEvent(t, events[0], refund)
	assertSameCashEvent(t, events[1], purchase)
}

func TestDailySnapshotRoundTripAndUpdate(t *testing.T) {
	st, ctx := openMigratedAccountingStore(t)
	reportDate := accounting.LocalDay(time.Now())
	if _, found, err := st.ReadDailySnapshot(ctx, reportDate); err != nil || found {
		t.Fatalf("missing ReadDailySnapshot = found %v, err %v", found, err)
	}

	first := accounting.DailySnapshot{
		ReportDate:              reportDate,
		ExternalRevenueCNY:      decimal.RequireFromString("8.00"),
		ExternalRequests:        1,
		InternalRequests:        3,
		CustomerResourceCostCNY: decimal.RequireFromString("6.00"),
		InternalResourceCostCNY: decimal.RequireFromString("5.50"),
		ResourceCostCNY:         decimal.RequireFromString("11.50"),
		OperatingGrossProfitCNY: decimal.RequireFromString("-3.50"),
		CashOutflowCNY:          decimal.RequireFromString("80.00"),
		CashNetResultCNY:        decimal.RequireFromString("-72.00"),
		UnlinkedCashOutflowCNY:  decimal.RequireFromString("-20.00"),
		CashEventCount:          2,
		OwnedOAuthCostCNY:       decimal.RequireFromString("7.50"),
		UpstreamAPIKeyCostCNY:   decimal.RequireFromString("4.00"),
	}
	if err := st.UpsertDailySnapshot(ctx, first); err != nil {
		t.Fatalf("first UpsertDailySnapshot: %v", err)
	}
	got, found, err := st.ReadDailySnapshot(ctx, reportDate.Add(12*time.Hour))
	if err != nil || !found {
		t.Fatalf("first ReadDailySnapshot = found %v, err %v", found, err)
	}
	assertSameDailySnapshot(t, got, first)

	updated := first
	updated.ExternalRevenueCNY = decimal.RequireFromString("9.25")
	updated.ExternalRequests = 2
	updated.CashEventCount = 3
	if err := st.UpsertDailySnapshot(ctx, updated); err != nil {
		t.Fatalf("updated UpsertDailySnapshot: %v", err)
	}
	got, found, err = st.ReadDailySnapshot(ctx, reportDate)
	if err != nil || !found {
		t.Fatalf("updated ReadDailySnapshot = found %v, err %v", found, err)
	}
	assertSameDailySnapshot(t, got, updated)
}

func openMigratedAccountingStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st, ctx
}

func assertSameCashEvent(t *testing.T, got, want accounting.CashEvent) {
	t.Helper()
	if got.ID != want.ID ||
		got.EventType != want.EventType ||
		!got.PaidAt.Equal(want.PaidAt) ||
		!got.AmountCNY.Equal(want.AmountCNY) ||
		got.SourceKind != want.SourceKind ||
		!sameInt64Ptr(got.AccountID, want.AccountID) ||
		got.Notes != want.Notes ||
		got.CreatedByUserID != want.CreatedByUserID ||
		!got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("cash event = %#v, want %#v", got, want)
	}
}

func assertSameDailySnapshot(t *testing.T, got, want accounting.DailySnapshot) {
	t.Helper()
	if !got.ReportDate.Equal(accounting.LocalDay(want.ReportDate)) ||
		!got.ExternalRevenueCNY.Equal(want.ExternalRevenueCNY) ||
		got.ExternalRequests != want.ExternalRequests ||
		got.InternalRequests != want.InternalRequests ||
		!got.CustomerResourceCostCNY.Equal(want.CustomerResourceCostCNY) ||
		!got.InternalResourceCostCNY.Equal(want.InternalResourceCostCNY) ||
		!got.ResourceCostCNY.Equal(want.ResourceCostCNY) ||
		!got.OperatingGrossProfitCNY.Equal(want.OperatingGrossProfitCNY) ||
		!got.CashOutflowCNY.Equal(want.CashOutflowCNY) ||
		!got.CashNetResultCNY.Equal(want.CashNetResultCNY) ||
		!got.UnlinkedCashOutflowCNY.Equal(want.UnlinkedCashOutflowCNY) ||
		got.CashEventCount != want.CashEventCount ||
		!got.OwnedOAuthCostCNY.Equal(want.OwnedOAuthCostCNY) ||
		!got.UpstreamAPIKeyCostCNY.Equal(want.UpstreamAPIKeyCostCNY) {
		t.Fatalf("daily snapshot = %#v, want %#v", got, want)
	}
}

type accountingUsageFixture struct {
	st       *Store
	ctx      context.Context
	nonce    int64
	users    []int64
	accounts []int64
}

func newAccountingUsageFixture(t *testing.T, st *Store) *accountingUsageFixture {
	t.Helper()
	fixture := &accountingUsageFixture{
		st:    st,
		ctx:   context.Background(),
		nonce: time.Now().UnixNano(),
	}
	t.Cleanup(func() {
		if len(fixture.users) > 0 {
			if _, err := st.pool.Exec(context.Background(), `DELETE FROM public.users WHERE id = ANY($1::BIGINT[])`, fixture.users); err != nil {
				t.Errorf("cleanup accounting users: %v", err)
			}
		}
		if len(fixture.accounts) > 0 {
			if _, err := st.pool.Exec(context.Background(), `DELETE FROM public.accounts WHERE id = ANY($1::BIGINT[])`, fixture.accounts); err != nil {
				t.Errorf("cleanup accounting accounts: %v", err)
			}
		}
	})
	return fixture
}

func (f *accountingUsageFixture) addUser(t *testing.T, role string) (int64, int64) {
	t.Helper()
	index := len(f.users) + 1
	email := fmt.Sprintf("accounting-store-%d-%d@example.invalid", f.nonce, index)
	var userID int64
	if err := f.st.pool.QueryRow(f.ctx, `
		INSERT INTO public.users (email, password_hash, role)
		VALUES ($1, 'not-a-real-password-hash', $2)
		RETURNING id`, email, role).Scan(&userID); err != nil {
		t.Fatalf("insert accounting user fixture: %v", err)
	}
	f.users = append(f.users, userID)
	var apiKeyID int64
	if err := f.st.pool.QueryRow(f.ctx, `
		INSERT INTO public.api_keys (user_id, key, name)
		VALUES ($1, $2, 'accounting store test')
		RETURNING id`,
		userID, fmt.Sprintf("accounting-test-sentinel-%d-%d", f.nonce, index),
	).Scan(&apiKeyID); err != nil {
		t.Fatalf("insert accounting API key fixture: %v", err)
	}
	return userID, apiKeyID
}

func (f *accountingUsageFixture) addAccount(t *testing.T, accountType string) int64 {
	t.Helper()
	var accountID int64
	if err := f.st.pool.QueryRow(f.ctx, `
		INSERT INTO public.accounts (name, platform, type)
		VALUES ($1, 'accounting-test', $2)
		RETURNING id`,
		fmt.Sprintf("accounting-store-%d-%d", f.nonce, len(f.accounts)+1), accountType,
	).Scan(&accountID); err != nil {
		t.Fatalf("insert accounting account fixture: %v", err)
	}
	f.accounts = append(f.accounts, accountID)
	return accountID
}

func (f *accountingUsageFixture) addUsage(
	t *testing.T,
	userID, apiKeyID, accountID int64,
	actualCost, totalCost, accountStatsCost, multiplier string,
	createdAt time.Time,
) {
	t.Helper()
	var accountStats any
	if accountStatsCost != "" {
		accountStats = accountStatsCost
	}
	if _, err := f.st.pool.Exec(f.ctx, `
		INSERT INTO public.usage_logs (
			user_id, api_key_id, account_id, model, actual_cost, total_cost,
			account_stats_cost, account_rate_multiplier, created_at
		) VALUES ($1, $2, $3, 'accounting-test-model', $4, $5, $6, $7, $8)`,
		userID, apiKeyID, accountID, actualCost, totalCost, accountStats, multiplier, createdAt.UTC(),
	); err != nil {
		t.Fatalf("insert accounting usage fixture: %v", err)
	}
}
