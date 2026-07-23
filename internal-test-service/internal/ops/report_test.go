package ops

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/testsupport"
)

func TestDailyReportContainsBudgetAndNoSecrets(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := testsupport.NewFake()
	if err := st.RegisterUser(ctx, store.InternalUser{UserID: 1, JoinedAt: time.Now(), InviterUserID: sql.NullInt64{}}); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cs := &credits.Service{Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000, DailyLoginCredit: domain.DailyLoginCredit, CostMultiplierBPS: 700, CostPolicyID: "test-policy", CostPolicyQualified: true, Mode: "write"}
	if _, err := cs.GrantDailyLogin(ctx, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	reporter := &Reporter{Store: st, Credits: cs, Timezone: loc}
	report, err := reporter.Daily(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := report.Markdown()
	if !contains(text, "预算占用") || !contains(text, "每日登录发放") || contains(text, "推荐") || contains(text, "签到") || contains(text, "Bearer") || contains(text, "x-api-key") {
		t.Fatalf("report %s", text)
	}
	if report.DailyLoginCredits != 1 || report.CurrentBalances != domain.DailyLoginCredit {
		t.Fatalf("report %+v", report)
	}
}
func contains(s, n string) bool {
	for i := 0; i+len(n) <= len(s); i++ {
		if s[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
