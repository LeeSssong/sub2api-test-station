package ops

import (
	"context"
	"fmt"
	"time"

	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/store"
)

type Report struct {
	Date               string
	RegisteredUsers    int
	DailyLoginCredits  int
	SuccessfulUsage    domain.MicroUSD
	ActualProviderCost domain.MicroUSD
	CurrentBalances    domain.MicroUSD
	BudgetOccupancy    domain.MicroUSD
	BudgetRemaining    domain.MicroUSD
	ReadOnlyReason     string
	PendingJobs        int
	P95LatencyMS       int64
	ErrorCount         int
}
type Reporter struct {
	Store    *store.Store
	Credits  *credits.Service
	Timezone *time.Location
}

func (r *Reporter) Daily(ctx context.Context, date time.Time) (Report, error) {
	users, err := r.Store.ListInternalUsers(ctx)
	if err != nil {
		return Report{}, err
	}
	grants, err := r.Store.ListAllGrants(ctx)
	if err != nil {
		return Report{}, err
	}
	usage, err := r.Store.SumSuccessfulUsage(ctx)
	if err != nil {
		return Report{}, err
	}
	budget, err := r.Credits.Budget(ctx)
	if err != nil {
		return Report{}, err
	}
	reason, _ := r.Store.GetReadOnlyReason(ctx)
	dailyLoginCredits := 0
	for _, g := range grants {
		if g.Status != domain.TaskSucceeded {
			continue
		}
		if g.Kind == domain.GrantDailyLogin {
			dailyLoginCredits++
		}
	}
	return Report{Date: domain.ShanghaiDate(date, r.Timezone), RegisteredUsers: len(users), DailyLoginCredits: dailyLoginCredits, SuccessfulUsage: usage, ActualProviderCost: budget.ActualProviderCost, CurrentBalances: budget.CurrentBalances, BudgetOccupancy: budget.Occupancy, BudgetRemaining: budget.Remaining, ReadOnlyReason: reason}, nil
}

func (r Report) Markdown() string {
	return fmt.Sprintf("D04 首发计划日报 (%s)\n- 注册用户：%d\n- 每日登录发放：%d 笔\n- 成功消费：$%s\n- 预算成本：$%s\n- 当前余额：$%s\n- 预算占用：$%s\n- 预算剩余：$%s\n- 只读原因：%s", r.Date, r.RegisteredUsers, r.DailyLoginCredits, r.SuccessfulUsage, r.ActualProviderCost, r.CurrentBalances, r.BudgetOccupancy, r.BudgetRemaining, emptyAsOK(r.ReadOnlyReason))
}
func emptyAsOK(v string) string {
	if v == "" {
		return "无"
	}
	return v
}
