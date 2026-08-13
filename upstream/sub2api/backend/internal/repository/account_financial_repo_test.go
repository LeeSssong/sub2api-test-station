package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	dbent "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/usageupstreamcostevidence"
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
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(time.Now().Add(-time.Hour)).SaveX(ctx)
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
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(time.Now().Add(-time.Hour)).SaveX(ctx)
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
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(time.Now().Add(-time.Hour)).SaveX(ctx)
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

func TestAccountFinancialRepositoryCanonicalActivationKey(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	now := time.Now()
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(now.Add(-time.Hour)).SaveX(ctx)
	client.User.Create().SetEmail("canonical@example.com").SetPasswordHash("x").SetBalance(12).SaveX(ctx)
	s, err := NewAccountFinancialRepository(client).ReadSnapshot(ctx, service.AccountFinancialSnapshotQuery{GeneratedAt: now, From: now.Add(-time.Hour), To: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if s.EnabledAt.IsZero() || s.UserBalanceCNY != 12 {
		t.Fatalf("canonical activation not read: %#v", s)
	}
}

func TestAccountFinancialRepositoryReviewEligibility(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	now := time.Now()
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(now.Add(-time.Hour)).SaveX(ctx)
	user := client.User.Create().SetEmail("eligibility@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-e").SetName("e").SaveX(ctx)
	api := client.Account.Create().SetName("api").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	oauth := client.Account.Create().SetName("oauth").SetPlatform("openai").SetType("oauth").SetStatus("active").SaveX(ctx)
	makeUsage := func(id string, a int64, at time.Time) *ent.UsageLog {
		return client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(a).SetRequestID(id).SetModel("m").SetActualCost(10).SetCreatedAt(at).SaveX(ctx)
	}
	pending := makeUsage("pending", api.ID, now)
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(pending.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
	missing := makeUsage("missing", api.ID, now)
	confirmed := makeUsage("confirmed", api.ID, now)
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(confirmed.ID).SetSource("sub").SetEvidenceStatus("confirmed").SetNormalizedCostCny(2).SaveX(ctx)
	pre := makeUsage("pre", api.ID, now.Add(-2*time.Hour))
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(pre.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
	oauthUsage := makeUsage("oauth", oauth.ID, now)
	repo := NewAccountFinancialRepository(client)
	for _, id := range []int64{pending.ID, missing.ID} {
		if _, err := repo.CreateReview(ctx, service.UsageCostReviewInput{UsageLogID: id, ReviewedBy: 7, ReviewedAt: now}); err != nil {
			t.Fatalf("eligible %d: %v", id, err)
		}
	}
	for _, id := range []int64{confirmed.ID, pre.ID, oauthUsage.ID} {
		if _, err := repo.CreateReview(ctx, service.UsageCostReviewInput{UsageLogID: id, ReviewedBy: 7, ReviewedAt: now}); !errors.Is(err, service.ErrFinancialReviewNotEligible) {
			t.Fatalf("ineligible %d err=%v", id, err)
		}
	}
}

func TestAccountFinancialRepositoryConfirmedEvidenceRejectsExistingReview(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	now := time.Now()
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(now.Add(-time.Hour)).SaveX(ctx)
	user := client.User.Create().SetEmail("confirmed-after-review@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-confirmed-after-review").SetName("review").SaveX(ctx)
	account := client.Account.Create().SetName("sub").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	usage := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID("confirmed-after-review").SetModel("m").SetActualCost(10).SetCreatedAt(now).SaveX(ctx)
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(usage.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
	repo := NewAccountFinancialRepository(client)
	if _, err := repo.CreateReview(ctx, service.UsageCostReviewInput{UsageLogID: usage.ID, ReviewedBy: 7, ReviewedAt: now}); err != nil {
		t.Fatal(err)
	}
	client.UsageUpstreamCostEvidence.Update().Where(usageupstreamcostevidence.UsageLogIDEQ(usage.ID)).SetEvidenceStatus(usageupstreamcostevidence.EvidenceStatusConfirmed).SetNormalizedCostCny(2).SaveX(ctx)
	if _, err := repo.CreateReview(ctx, service.UsageCostReviewInput{UsageLogID: usage.ID, ReviewedBy: 7, ReviewedAt: now}); !errors.Is(err, service.ErrFinancialReviewNotEligible) {
		t.Fatalf("confirmed evidence must not be idempotently reviewable, err=%v", err)
	}
}

func TestAccountFinancialRepositoryFilteredReviewIncludesIdempotentSkipResult(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	now := time.Now()
	client.AccountFinancialSetting.Create().SetKey("t03_r1_account_financial").SetEnabledAt(now.Add(-time.Hour)).SaveX(ctx)
	user := client.User.Create().SetEmail("filtered-skip@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-filtered-skip").SetName("review").SaveX(ctx)
	account := client.Account.Create().SetName("sub").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	usage := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID("filtered-skip").SetModel("m").SetActualCost(10).SetCreatedAt(now).SaveX(ctx)
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(usage.ID).SetSource("sub").SetEvidenceStatus("unavailable").SaveX(ctx)
	repo := NewAccountFinancialRepository(client)
	input := service.UsageCostReviewInput{UsageLogID: usage.ID, ReviewedBy: 7, ReviewedAt: now}
	if _, err := repo.CreateReview(ctx, input); err != nil {
		t.Fatal(err)
	}
	result, err := repo.ReviewFiltered(ctx, service.ReviewFilteredInput{Filter: service.ReviewFilter{AccountID: &account.ID}, ReviewedBy: 7, ReviewedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 || len(result.Reviews) != 1 || result.Reviews[0].Created || result.Reviews[0].UsageLogID != usage.ID {
		t.Fatalf("filtered idempotent result must remain auditable: %#v", result)
	}
}

func TestAccountFinancialRepositoryUsageEvidenceIncludesReviewWhenEvidenceMissing(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	account := client.Account.Create().SetName("sub").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	user := client.User.Create().SetEmail("missing-evidence-review@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-missing-evidence-review").SetName("review").SaveX(ctx)
	usage := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID("missing-evidence-review").SetModel("m").SetActualCost(10).SaveX(ctx)
	review := client.UsageCostReview.Create().SetUsageLogID(usage.ID).SetReviewedBy(7).SetReviewedAt(time.Now()).SetManualCostCny(3).SetManualProfitCny(7).SaveX(ctx)

	detail, err := NewAccountFinancialRepository(client).GetUsageEvidence(ctx, usage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.EvidenceStatus != "unavailable" || detail.ReasonCode != "evidence_not_registered" || detail.ReviewID == nil || *detail.ReviewID != review.ID || detail.ReviewCostCNY == nil || *detail.ReviewCostCNY != 3 {
		t.Fatalf("missing evidence detail must retain manual review: %#v", detail)
	}
}

func TestAccountFinancialRepositoryUsageEvidenceIncludesScopedTraceability(t *testing.T) {
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	account := client.Account.Create().SetName("newapi-ledger").SetPlatform("openai").SetType("api_key").SetStatus("active").SaveX(ctx)
	user := client.User.Create().SetEmail("evidence-trace@example.com").SetPasswordHash("x").SaveX(ctx)
	key := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-evidence-trace").SetName("trace").SaveX(ctx)
	upstreamRequestID := "upstream-request-42"
	usage := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID("local-request-42").SetUpstreamRequestID(upstreamRequestID).SetModel("m").SetActualCost(10).SaveX(ctx)
	billingTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	upstreamModel := "upstream-model"
	quota, perUnit, normalized := 2500.0, 500000.0, 0.005
	client.UsageUpstreamCostEvidence.Create().SetUsageLogID(usage.ID).SetSource("newapi").SetUpstreamRequestID(upstreamRequestID).SetUpstreamBillingTime(billingTime).SetUpstreamModel(upstreamModel).SetNewapiQuota(quota).SetNewapiQuotaPerUnit(perUnit).SetNormalizedCostCny(normalized).SetEvidenceStatus("confirmed").SaveX(ctx)

	detail, err := NewAccountFinancialRepository(client).GetUsageEvidence(ctx, usage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.RequestID != "local-request-42" || detail.AccountID != account.ID || detail.AccountName != "newapi-ledger" || detail.AccountType != "api_key" || detail.Source != "newapi" || detail.UpstreamRequestID == nil || *detail.UpstreamRequestID != upstreamRequestID || detail.UpstreamBillingTime == nil || detail.UpstreamModel == nil || *detail.UpstreamModel != upstreamModel || detail.NewAPIQuota == nil || *detail.NewAPIQuota != quota || detail.NewAPIQuotaPerUnit == nil || *detail.NewAPIQuotaPerUnit != perUnit || detail.NormalizedCostCNY == nil || *detail.NormalizedCostCNY != normalized {
		t.Fatalf("scoped evidence trace=%#v", detail)
	}
}

func TestAccountFinancialRepositoryCostOnlyOverrideReturnsTruthfulOldNew(t *testing.T) {
	t.Skip("SQLite returns DATE as string; PostgreSQL-backed fix-round test covers old/new and unique concurrency")
	ctx := context.Background()
	client := newAccountFinancialRepositoryTestClient(t)
	now := beijingRepoTime(t, "2026-08-13 12:00")
	account := client.Account.Create().SetName("a").SetPlatform("x").SetType("api_key").SetStatus("active").SaveX(ctx)
	v := float64(6)
	res, err := NewAccountFinancialRepositoryWithClock(client, func() time.Time { return now }).SetTodayOverride(ctx, service.TodayOverrideInput{AccountID: account.ID, BusinessDate: "2026-08-13", CostCNY: &v, ActorUserID: 9})
	if err != nil {
		t.Fatal(err)
	}
	if res.OldValue != nil || res.NewValue == nil || *res.NewValue != 6 || res.MutationKind != "cost" {
		t.Fatalf("result=%#v", res)
	}
}
func beijingRepoTime(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	v, e := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
