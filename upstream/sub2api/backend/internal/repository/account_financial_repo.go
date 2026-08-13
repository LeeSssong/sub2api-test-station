package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/accountdailyfinancialvalue"
	"github.com/Wei-Shaw/sub2api/ent/accountfinancialsetting"
	"github.com/Wei-Shaw/sub2api/ent/usagecostreview"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/ent/usageupstreamcostevidence"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountFinancialRepository struct {
	client             *ent.Client
	now                func() time.Time
	reviewBeforeCreate func(int64) error
}

const accountFinancialSettingKey = "t03_r1_account_financial"

func NewAccountFinancialRepository(client *ent.Client) service.AccountFinancialRepository {
	return &accountFinancialRepository{client: client, now: time.Now}
}
func NewAccountFinancialRepositoryWithClock(client *ent.Client, now func() time.Time) service.AccountFinancialRepository {
	if now == nil {
		now = time.Now
	}
	return &accountFinancialRepository{client: client, now: now}
}

func (r *accountFinancialRepository) ReadSnapshot(ctx context.Context, q service.AccountFinancialSnapshotQuery) (*service.AccountFinancialSnapshot, error) {
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()
	setting, err := client.AccountFinancialSetting.Query().Where(accountfinancialsetting.KeyEQ(accountFinancialSettingKey)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	s := &service.AccountFinancialSnapshot{GeneratedAt: q.GeneratedAt}
	if setting != nil && setting.EnabledAt != nil {
		s.EnabledAt = *setting.EnabledAt
	} else {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s, nil
	}
	users, err := client.User.Query().Where(user.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		s.UserBalanceCNY += u.Balance
	}
	accounts, err := client.Account.Query().Where(account.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		s.Accounts = append(s.Accounts, service.AccountFinancialSnapshotAccount{ID: a.ID, Name: a.Name, Type: a.Type, Platform: a.Platform})
	}
	usageQ := client.UsageLog.Query()
	if !s.EnabledAt.IsZero() {
		usageQ = usageQ.Where(usagelog.CreatedAtGTE(s.EnabledAt))
	}
	if !q.From.IsZero() {
		usageQ = usageQ.Where(usagelog.CreatedAtGTE(q.From))
	}
	if !q.To.IsZero() {
		usageQ = usageQ.Where(usagelog.CreatedAtLT(q.To))
	}
	logs, err := usageQ.Order(ent.Asc(usagelog.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	evidence, err := client.UsageUpstreamCostEvidence.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	em := map[int64]*ent.UsageUpstreamCostEvidence{}
	for _, e := range evidence {
		em[e.UsageLogID] = e
	}
	reviews, err := client.UsageCostReview.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	rm := map[int64]*ent.UsageCostReview{}
	for _, v := range reviews {
		rm[v.UsageLogID] = v
	}
	for _, u := range logs {
		e := service.AccountFinancialSnapshotEntry{UsageLogID: u.ID, AccountID: u.AccountID, RequestID: u.RequestID, Model: u.Model, CreatedAt: u.CreatedAt, BusinessDate: u.CreatedAt.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02"), RevenueCNY: u.ActualCost, EvidenceStatus: "unavailable", ReasonCode: "evidence_not_registered"}
		if x := em[u.ID]; x != nil {
			e.EvidenceID = &x.ID
			e.EvidenceStatus = string(x.EvidenceStatus)
			e.EvidenceCostCNY = x.NormalizedCostCny
			e.SubActualCost = x.SubActualCost
			e.NewAPIQuota = x.NewapiQuota
			e.NewAPIQuotaPerUnit = x.NewapiQuotaPerUnit
			if x.ReasonCode != nil {
				e.ReasonCode = *x.ReasonCode
			}
		}
		if x := rm[u.ID]; x != nil {
			e.ReviewID = &x.ID
			e.ReviewCostCNY = &x.ManualCostCny
		}
		s.Entries = append(s.Entries, e)
	}
	days, err := client.AccountDailyFinancialValue.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		s.DailyValues = append(s.DailyValues, service.AccountFinancialDailyValue{AccountID: d.AccountID, BusinessDate: d.BusinessDate.Format("2006-01-02"), OAuthCostCNY: d.OauthCostCny, RevenueOverrideCNY: d.RevenueOverrideCny, RevenueOverrideAt: d.RevenueOverrideAt, RevenueEvidenceCutoffID: d.RevenueEvidenceCutoffID, RevenueReviewCutoffID: d.RevenueReviewCutoffID, CostOverrideCNY: d.CostOverrideCny, CostOverrideAt: d.CostOverrideAt, CostEvidenceCutoffID: d.CostEvidenceCutoffID, CostReviewCutoffID: d.CostReviewCutoffID})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *accountFinancialRepository) CreateReview(ctx context.Context, in service.UsageCostReviewInput) (*service.UsageCostReviewResult, error) {
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := r.createReview(ctx, tx.Client(), in)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
func (r *accountFinancialRepository) createReview(ctx context.Context, c *ent.Client, in service.UsageCostReviewInput) (*service.UsageCostReviewResult, error) {
	cost := 0.0
	if in.ManualCostCNY != nil {
		cost = *in.ManualCostCNY
	}
	u, err := c.UsageLog.Get(ctx, in.UsageLogID)
	if err != nil {
		return nil, err
	}
	a, err := c.Account.Get(ctx, u.AccountID)
	if err != nil {
		return nil, err
	}
	setting, err := c.AccountFinancialSetting.Query().Where(accountfinancialsetting.KeyEQ(accountFinancialSettingKey)).Only(ctx)
	if err != nil || setting.EnabledAt == nil || u.CreatedAt.Before(*setting.EnabledAt) || a.Type == "oauth" {
		return nil, service.ErrFinancialReviewNotEligible
	}
	if x, err := c.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(in.UsageLogID)).Only(ctx); err == nil {
		old := x.ManualCostCny
		return &service.UsageCostReviewResult{UsageLogID: x.UsageLogID, AccountID: u.AccountID, BusinessDate: financialBusinessDate(u.CreatedAt), OldManualCostCNY: &old, ManualCostCNY: x.ManualCostCny, ManualProfitCNY: x.ManualProfitCny}, nil
	}
	ev, err := c.UsageUpstreamCostEvidence.Query().Where(usageupstreamcostevidence.UsageLogIDEQ(u.ID)).Only(ctx)
	if err == nil && ev.EvidenceStatus == usageupstreamcostevidence.EvidenceStatusConfirmed {
		return nil, service.ErrFinancialReviewNotEligible
	}
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	when := in.ReviewedAt
	if when.IsZero() {
		when = r.now()
	}
	x, err := c.UsageCostReview.Create().SetUsageLogID(in.UsageLogID).SetReviewStatus(usagecostreview.ReviewStatusReviewed).SetManualCostCny(cost).SetManualProfitCny(u.ActualCost - cost).SetReviewedBy(in.ReviewedBy).SetReviewedAt(when).SetUpdatedAt(when).Save(ctx)
	if err != nil {
		if y, e := c.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(in.UsageLogID)).Only(ctx); e == nil {
			old := y.ManualCostCny
			return &service.UsageCostReviewResult{UsageLogID: y.UsageLogID, AccountID: u.AccountID, BusinessDate: financialBusinessDate(u.CreatedAt), OldManualCostCNY: &old, ManualCostCNY: y.ManualCostCny, ManualProfitCNY: y.ManualProfitCny}, nil
		}
		return nil, err
	}
	return &service.UsageCostReviewResult{Created: true, UsageLogID: x.UsageLogID, AccountID: u.AccountID, BusinessDate: financialBusinessDate(u.CreatedAt), ManualCostCNY: cost, ManualProfitCNY: u.ActualCost - cost}, nil
}

func (r *accountFinancialRepository) FreezeReviewFilter(ctx context.Context, f service.ReviewFilter) (int64, error) {
	q := r.client.UsageLog.Query().Order(ent.Desc(usagelog.FieldID))
	if f.AccountID != nil {
		q = q.Where(usagelog.AccountIDEQ(*f.AccountID))
	}
	if f.From != nil {
		q = q.Where(usagelog.CreatedAtGTE(*f.From))
	}
	if f.To != nil {
		q = q.Where(usagelog.CreatedAtLT(*f.To))
	}
	logs, err := q.All(ctx)
	if err != nil {
		return 0, err
	}
	max := int64(0)
	for _, u := range logs {
		if u.ID <= max {
			continue
		}
		if _, e := r.client.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(u.ID)).Only(ctx); e == nil {
			continue
		}
		if ev, e := r.client.UsageUpstreamCostEvidence.Query().Where(usageupstreamcostevidence.UsageLogIDEQ(u.ID)).Only(ctx); e == nil && ev.EvidenceStatus == usageupstreamcostevidence.EvidenceStatusConfirmed {
			continue
		}
		max = u.ID
	}
	return max, nil
}
func (r *accountFinancialRepository) ReviewFiltered(ctx context.Context, in service.ReviewFilteredInput) (*service.ReviewFilteredResult, error) {
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	c := tx.Client()
	setting, err := c.AccountFinancialSetting.Query().Where(accountfinancialsetting.KeyEQ(accountFinancialSettingKey)).Only(ctx)
	if err != nil || setting.EnabledAt == nil {
		return nil, service.ErrFinancialReviewNotEligible
	}
	base := c.UsageLog.Query().Where(usagelog.CreatedAtGTE(*setting.EnabledAt)).Order(ent.Asc(usagelog.FieldID))
	if in.Filter.AccountID != nil {
		base = base.Where(usagelog.AccountIDEQ(*in.Filter.AccountID))
	}
	if in.Filter.From != nil {
		base = base.Where(usagelog.CreatedAtGTE(*in.Filter.From))
	}
	if in.Filter.To != nil {
		base = base.Where(usagelog.CreatedAtLT(*in.Filter.To))
	}
	if in.Filter.Search != "" {
		base = base.Where(usagelog.Or(usagelog.RequestIDContainsFold(in.Filter.Search), usagelog.ModelContainsFold(in.Filter.Search)))
	}
	logs, err := base.All(ctx)
	if err != nil {
		return nil, err
	}
	if in.MaxUsageLogID == 0 {
		for _, u := range logs {
			if u.ID > in.MaxUsageLogID {
				in.MaxUsageLogID = u.ID
			}
		}
	}
	res := &service.ReviewFilteredResult{Cutoff: in.MaxUsageLogID, MaxUsageLogID: in.MaxUsageLogID}
	for _, u := range logs {
		if in.Filter.ReviewStatus != "" && in.Filter.ReviewStatus != "pending" {
			continue
		}
		if u.ID > in.MaxUsageLogID {
			continue
		}
		a, e := c.Account.Get(ctx, u.AccountID)
		if e != nil {
			return nil, e
		}
		if a.Type == "oauth" {
			continue
		}
		ev, e := c.UsageUpstreamCostEvidence.Query().Where(usageupstreamcostevidence.UsageLogIDEQ(u.ID)).Only(ctx)
		if e == nil && ev.EvidenceStatus == usageupstreamcostevidence.EvidenceStatusConfirmed {
			continue
		}
		if e != nil && !ent.IsNotFound(e) {
			return nil, e
		}
		projectedStatus := "unavailable"
		if ev != nil {
			projectedStatus = string(ev.EvidenceStatus)
		}
		if in.Filter.EvidenceStatus != "" && in.Filter.EvidenceStatus != projectedStatus {
			continue
		}
		res.Matched++
		if _, e := c.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(u.ID)).Only(ctx); e == nil {
			res.Skipped++
			continue
		}
		if r.reviewBeforeCreate != nil {
			if err := r.reviewBeforeCreate(u.ID); err != nil {
				return nil, err
			}
		}
		review, e := r.createReview(ctx, c, service.UsageCostReviewInput{UsageLogID: u.ID, ManualCostCNY: in.ManualCostCNY, ReviewedBy: in.ReviewedBy, ReviewedAt: in.ReviewedAt, RequestID: in.RequestID})
		if e != nil {
			return nil, e
		}
		if review.Created {
			res.Updated++
		} else {
			res.Skipped++
		}
		res.Reviews = append(res.Reviews, *review)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func financialBusinessDate(t time.Time) string {
	return t.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}

func (r *accountFinancialRepository) SetOAuthDailyCost(ctx context.Context, in service.OAuthDailyCostInput) (*service.FinancialMutationResult, error) {
	if err := service.ValidateFinancialToday(in.BusinessDate, r.now()); err != nil {
		return nil, err
	}
	a, err := r.client.Account.Get(ctx, in.AccountID)
	if err != nil {
		return nil, err
	}
	if err := service.ValidateFinancialOAuthType(a.Type); err != nil {
		return nil, err
	}
	if err := service.ValidateFinancialAmount(in.CostCNY); err != nil {
		return nil, err
	}
	day, _ := time.ParseInLocation("2006-01-02", in.BusinessDate, time.FixedZone("Asia/Shanghai", 8*3600))
	q := r.client.AccountDailyFinancialValue.Query().Where(accountdailyfinancialvalue.AccountIDEQ(in.AccountID), accountdailyfinancialvalue.BusinessDateEQ(day))
	x, err := q.Only(ctx)
	old := (*float64)(nil)
	if err == nil {
		old = x.OauthCostCny
		x, err = x.Update().SetNillableOauthCostCny(in.CostCNY).SetUpdatedBy(in.ActorUserID).Save(ctx)
	} else if ent.IsNotFound(err) {
		b := r.client.AccountDailyFinancialValue.Create().SetAccountID(in.AccountID).SetBusinessDate(day).SetUpdatedBy(in.ActorUserID).SetNillableOauthCostCny(in.CostCNY)
		x, err = b.Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &service.FinancialMutationResult{AccountID: x.AccountID, BusinessDate: in.BusinessDate, OldValue: old, NewValue: in.CostCNY}, nil
}
func (r *accountFinancialRepository) SetTodayOverride(ctx context.Context, in service.TodayOverrideInput) (*service.FinancialMutationResult, error) {
	if err := service.ValidateFinancialToday(in.BusinessDate, r.now()); err != nil {
		return nil, err
	}
	if err := service.ValidateFinancialAmount(in.RevenueCNY); err != nil {
		return nil, err
	}
	if err := service.ValidateFinancialAmount(in.CostCNY); err != nil {
		return nil, err
	}
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	day, _ := time.ParseInLocation("2006-01-02", in.BusinessDate, loc)
	end := day.AddDate(0, 0, 1)
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	c := tx.Client()
	maxEvidence, maxReview := int64(0), int64(0)
	evs, err := c.UsageUpstreamCostEvidence.Query().Where(usageupstreamcostevidence.HasUsageLogWith(usagelog.AccountIDEQ(in.AccountID), usagelog.CreatedAtGTE(day), usagelog.CreatedAtLT(end))).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if e.ID > maxEvidence {
			maxEvidence = e.ID
		}
	}
	revs, err := c.UsageCostReview.Query().Where(usagecostreview.HasUsageLogWith(usagelog.AccountIDEQ(in.AccountID), usagelog.CreatedAtGTE(day), usagelog.CreatedAtLT(end))).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range revs {
		if v.ID > maxReview {
			maxReview = v.ID
		}
	}
	q := c.AccountDailyFinancialValue.Query().Where(accountdailyfinancialvalue.AccountIDEQ(in.AccountID), accountdailyfinancialvalue.BusinessDateEQ(day))
	x, err := q.Only(ctx)
	if ent.IsNotFound(err) {
		x, err = c.AccountDailyFinancialValue.Create().SetAccountID(in.AccountID).SetBusinessDate(day).SetUpdatedBy(in.ActorUserID).Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	var old, newValue *float64
	kind := ""
	if in.RevenueCNY != nil {
		old = x.RevenueOverrideCny
		newValue = in.RevenueCNY
		kind = "revenue"
	}
	if in.CostCNY != nil {
		old = x.CostOverrideCny
		newValue = in.CostCNY
		kind = "cost"
	}
	up := x.Update().SetUpdatedBy(in.ActorUserID)
	if in.RevenueCNY != nil {
		up = up.SetRevenueOverrideCny(*in.RevenueCNY).SetRevenueOverrideAt(r.now()).SetRevenueEvidenceCutoffID(maxEvidence).SetRevenueReviewCutoffID(maxReview)
	}
	if in.CostCNY != nil {
		up = up.SetCostOverrideCny(*in.CostCNY).SetCostOverrideAt(r.now()).SetCostEvidenceCutoffID(maxEvidence).SetCostReviewCutoffID(maxReview)
	}
	x, err = up.Save(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, OldValue: old, NewValue: newValue, MutationKind: kind, CutoffEvidenceID: maxEvidence, CutoffReviewID: maxReview}, nil
}
func (r *accountFinancialRepository) GetUsageEvidence(ctx context.Context, id int64) (*service.UsageFinancialEvidence, error) {
	e, err := r.client.UsageUpstreamCostEvidence.Query().Where(usageupstreamcostevidence.UsageLogIDEQ(id)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return nil, err
		}
		u, x := r.client.UsageLog.Get(ctx, id)
		if x != nil {
			return nil, x
		}
		a, x := r.client.Account.Get(ctx, u.AccountID)
		if x != nil {
			return nil, x
		}
		if a.Type == "oauth" {
			return &service.UsageFinancialEvidence{UsageLogID: id}, nil
		}
		return &service.UsageFinancialEvidence{UsageLogID: id, EvidenceStatus: "unavailable", ReasonCode: "evidence_not_registered"}, nil
	}
	reason := ""
	if e.ReasonCode != nil {
		reason = *e.ReasonCode
	}
	x := &service.UsageFinancialEvidence{UsageLogID: id, EvidenceStatus: string(e.EvidenceStatus), ReasonCode: reason, NormalizedCostCNY: e.NormalizedCostCny}
	if v, err := r.client.UsageCostReview.Query().Where(usagecostreview.UsageLogIDEQ(id)).Only(ctx); err == nil {
		x.ReviewID = &v.ID
		x.ReviewCostCNY = &v.ManualCostCny
	}
	return x, nil
}
