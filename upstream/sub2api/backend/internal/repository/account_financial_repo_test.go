package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	dbent "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "modernc.org/sqlite"
)

func newAccountFinancialRepositoryTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:account_financial_%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(dbent.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAccountFinancialRepositoryReviewIsIdempotentAndNilCostIsZero(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	account := client.Account.Create().SetName("sub").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	user := client.User.Create().SetEmail("review@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-review").SetName("review").SaveX(ctx)
	usage := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID("r1").SetModel("m").SetActualCost(10).SaveX(ctx)
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(usage.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
	repo := NewAccountFinancialRepository(client)
	input := service.UsageCostReviewInput{UsageLogID: usage.ID, ReviewedBy: 7, ReviewedAt: time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)}
	first, err := repo.CreateReview(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.CreateReview(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || second.Created || first.ManualCostCNY != 0 || first.ManualProfitCNY != 10 {
		t.Fatalf("idempotence/nil-cost failed: first=%#v second=%#v", first, second)
	}
}

func TestAccountFinancialRepositoryFilteredReviewFreezesMaxUsageLogID(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	account := client.Account.Create().SetName("sub").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	user := client.User.Create().SetEmail("filtered@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-filtered").SetName("filtered").SaveX(ctx)
	createPending := func(request string, cost float64) *ent.UsageLog {
		u := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID(request).SetModel("m").SetActualCost(cost).SaveX(ctx)
		client.UsageUpstreamCostEvidence.Create().SetUsageLogID(u.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
		return u
	}
	first := createPending("r1", 10)
	second := createPending("r2", 20)
	repo := NewAccountFinancialRepository(client)
	cutoff, err := repo.FreezeReviewFilter(ctx, service.ReviewFilter{AccountID: &account.ID})
	if err != nil {
		t.Fatal(err)
	}
	newer := createPending("r3", 30)
	result, err := repo.ReviewFiltered(ctx, service.ReviewFilteredInput{Filter: service.ReviewFilter{AccountID: &account.ID}, MaxUsageLogID: cutoff, ReviewedBy: 7, ReviewedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if cutoff != second.ID || result.MaxUsageLogID != second.ID || result.Matched != 2 || result.Updated != 2 || result.Skipped != 0 {
		t.Fatalf("frozen result=%#v cutoff=%d first=%d second=%d", result, cutoff, first.ID, second.ID)
	}
	count, err := client.UsageCostReview.Query().Where().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("reviews=%d; concurrent newer usage %d must remain pending", count, newer.ID)
	}
}

func TestAccountFinancialRepositorySnapshotBalanceIncludesDisabledExcludesDeletedAndFrozen(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	client.AccountFinancialSetting.Create().SetKey("account_financial").SetEnabledAt(time.Now().Add(-time.Hour)).SaveX(ctx)
	client.User.Create().SetEmail("a@example.com").SetPasswordHash("x").SetStatus("disabled").SetBalance(10).SetFrozenBalance(90).SaveX(ctx)
	deleted := client.User.Create().SetEmail("b@example.com").SetPasswordHash("x").SetBalance(20).SaveX(ctx)
	client.User.UpdateOne(deleted).SetDeletedAt(time.Now()).ExecX(ctx)
	snapshot, err := NewAccountFinancialRepository(client).ReadSnapshot(ctx, service.AccountFinancialSnapshotQuery{GeneratedAt: time.Now(), From: time.Now().Add(-24 * time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.UserBalanceCNY != 10 {
		t.Fatalf("balance=%v", snapshot.UserBalanceCNY)
	}
}
