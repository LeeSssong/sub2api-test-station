package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

type ProcurementStatus string

const (
	ProcurementStatusActive      ProcurementStatus = "active"
	ProcurementStatusCostPending ProcurementStatus = "cost_pending"
	ProcurementStatusSettled     ProcurementStatus = "settled"
	ProcurementStatusPaused      ProcurementStatus = "paused"
	ProcurementStatusExpired     ProcurementStatus = "expired"
)

type SelfPurchasedProfitabilityRow struct {
	AccountID           int64             `json:"account_id"`
	Name                string            `json:"name"`
	Platform            string            `json:"platform"`
	AccountType         string            `json:"account_type"`
	Status              string            `json:"status"`
	ProcurementCostCNY  *float64          `json:"procurement_cost_cny"`
	EstimatedQuotaUSD   *float64          `json:"estimated_quota_usd"`
	StandardConsumedUSD float64           `json:"standard_consumed_usd"`
	Utilization         *float64          `json:"utilization"`
	ConfirmedCostCNY    float64           `json:"confirmed_cost_cny"`
	PendingCostCNY      float64           `json:"pending_cost_cny"`
	LossCNY             float64           `json:"procurement_loss_cny"`
	RevenueCNY          float64           `json:"revenue_cny"`
	NetProfitCNY        *float64          `json:"net_profit_cny"`
	Margin              *float64          `json:"margin"`
	CostStatus          ProcurementStatus `json:"cost_status"`
}
type SelfPurchasedProfitabilitySummary struct {
	ProcurementCostCNY  float64  `json:"procurement_cost_cny"`
	StandardConsumedUSD float64  `json:"standard_consumed_usd"`
	ConfirmedCostCNY    float64  `json:"confirmed_cost_cny"`
	PendingCostCNY      float64  `json:"pending_cost_cny"`
	LossCNY             float64  `json:"procurement_loss_cny"`
	RevenueCNY          float64  `json:"revenue_cny"`
	NetProfitCNY        *float64 `json:"net_profit_cny"`
	Margin              *float64 `json:"margin"`
	AccountCount        int      `json:"account_count"`
}
type SelfPurchasedProfitabilityReport struct {
	StartDate   string                            `json:"start_date"`
	EndDate     string                            `json:"end_date"`
	GeneratedAt time.Time                         `json:"generated_at"`
	Currency    string                            `json:"currency"`
	Summary     SelfPurchasedProfitabilitySummary `json:"summary"`
	Rows        []SelfPurchasedProfitabilityRow   `json:"rows"`
}

type procurementAggregate struct {
	row SelfPurchasedProfitabilityRow
	cap float64
}

func calculateProcurementMetrics(cost, quota, standard, revenue float64, settled bool) (confirmed, pending, loss float64, utilization, profit, margin *float64) {
	if quota <= 0 || cost < 0 || math.IsNaN(quota) || math.IsNaN(cost) {
		return 0, 0, 0, nil, nil, nil
	}
	if standard < 0 {
		standard = 0
	}
	confirmed = math.Min(cost, standard*(cost/quota))
	pending = math.Max(cost-confirmed, 0)
	u := standard / quota
	if u > 1 {
		u = 1
	}
	utilization = &u
	if settled {
		loss = pending
		pending = 0
	}
	p := revenue - confirmed - loss
	profit = &p
	if revenue != 0 {
		m := p / revenue
		margin = &m
	}
	return
}

func (s *AccountProfitabilityService) GetSelfPurchasedReport(ctx context.Context, start, end time.Time) (*SelfPurchasedProfitabilityReport, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("account profitability database is unavailable")
	}
	if !end.After(start) {
		return nil, ErrInvalidAccountProfitabilityRange
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,a.platform,a.type,a.status,a.created_at,a.procurement_cost_cny,a.estimated_usable_quota_usd,a.procurement_cost_effective_at, COALESCE(ul.total_cost,0),COALESCE(ul.actual_cost,0),COALESCE(v.cost_cny, a.procurement_cost_cny),COALESCE(v.estimated_usable_quota_usd,a.estimated_usable_quota_usd),COALESCE(v.status,'active'),COALESCE(v.settled_at IS NOT NULL,false) FROM accounts a LEFT JOIN usage_logs ul ON ul.account_id=a.id AND ul.created_at >= $1 AND ul.created_at < $2 LEFT JOIN account_procurement_cost_versions v ON v.account_id=a.id AND ul.created_at >= v.effective_at AND (v.ended_at IS NULL OR ul.created_at < v.ended_at) WHERE a.deleted_at IS NULL AND (a.type='oauth' OR a.procurement_cost_cny IS NOT NULL) ORDER BY a.id`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aggs := map[int64]*procurementAggregate{}
	for rows.Next() {
		var id int64
		var name, platform, typ, status string
		var created time.Time
		var cost, quota, effective sql.NullFloat64
		var standard, revenue, vc, vq float64
		var vstatus string
		var settled bool
		if err := rows.Scan(&id, &name, &platform, &typ, &status, &created, &cost, &quota, &effective, &standard, &revenue, &vc, &vq, &vstatus, &settled); err != nil {
			return nil, err
		}
		a := aggs[id]
		if a == nil {
			a = &procurementAggregate{row: SelfPurchasedProfitabilityRow{AccountID: id, Name: name, Platform: platform, AccountType: typ, Status: status, CostStatus: ProcurementStatusCostPending}}
			aggs[id] = a
		}
		if cost.Valid {
			x := cost.Float64
			a.row.ProcurementCostCNY = &x
		}
		if quota.Valid {
			x := quota.Float64
			a.row.EstimatedQuotaUSD = &x
		}
		a.row.StandardConsumedUSD += standard
		a.row.RevenueCNY += revenue
		if vc >= 0 && vq > 0 {
			a.cap = vc
			a.row.ProcurementCostCNY = &a.cap
			a.row.EstimatedQuotaUSD = &vq
			if vstatus != "" {
				a.row.CostStatus = ProcurementStatus(vstatus)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := &SelfPurchasedProfitabilityReport{StartDate: start.Format("2006-01-02"), EndDate: end.Add(-time.Nanosecond).Format("2006-01-02"), GeneratedAt: s.now().UTC(), Currency: "CNY", Rows: make([]SelfPurchasedProfitabilityRow, 0, len(aggs))}
	for _, a := range aggs {
		r := a.row
		if r.ProcurementCostCNY == nil || r.EstimatedQuotaUSD == nil {
			r.CostStatus = ProcurementStatusCostPending
			out.Rows = append(out.Rows, r)
			continue
		}
		settled := r.CostStatus == ProcurementStatusSettled || r.CostStatus == ProcurementStatusExpired
		c, p, l, u, profit, margin := calculateProcurementMetrics(*r.ProcurementCostCNY, *r.EstimatedQuotaUSD, r.StandardConsumedUSD, r.RevenueCNY, settled)
		r.ConfirmedCostCNY = c
		r.PendingCostCNY = p
		r.LossCNY = l
		r.Utilization = u
		r.NetProfitCNY = profit
		r.Margin = margin
		if r.CostStatus == "" || r.CostStatus == ProcurementStatusActive {
			r.CostStatus = ProcurementStatusActive
		}
		out.Summary.ProcurementCostCNY += *r.ProcurementCostCNY
		out.Summary.StandardConsumedUSD += r.StandardConsumedUSD
		out.Summary.ConfirmedCostCNY += c
		out.Summary.PendingCostCNY += p
		out.Summary.LossCNY += l
		out.Summary.RevenueCNY += r.RevenueCNY
		out.Rows = append(out.Rows, r)
	}
	out.Summary.AccountCount = len(out.Rows)
	if out.Summary.RevenueCNY != 0 {
		p := out.Summary.RevenueCNY - out.Summary.ConfirmedCostCNY - out.Summary.LossCNY
		out.Summary.NetProfitCNY = &p
		m := p / out.Summary.RevenueCNY
		out.Summary.Margin = &m
	}
	return out, nil
}

type ProcurementSettlementInput struct {
	AccountID         int64
	RequestID, Reason string
	ActorUserID       int64
}

func (s *AccountProfitabilityService) SettleProcurement(ctx context.Context, in ProcurementSettlementInput) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("account profitability database is unavailable")
	}
	if in.AccountID <= 0 || in.RequestID == "" {
		return false, errors.New("account_id and request_id are required")
	}
	if in.Reason != "expired" && in.Reason != "administrator_confirmed_expired" {
		return false, errors.New("invalid settlement reason")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM account_procurement_cost_versions WHERE request_id=$1`, in.RequestID).Scan(&existing)
	if err == nil {
		return true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var id int64
	var cost, quota float64
	var effective time.Time
	var status string
	err = tx.QueryRowContext(ctx, `SELECT id,cost_cny,estimated_usable_quota_usd,effective_at,status FROM account_procurement_cost_versions WHERE account_id=$1 AND ended_at IS NULL FOR UPDATE`, in.AccountID).Scan(&id, &cost, &quota, &effective, &status)
	if err != nil {
		return false, err
	}
	if status == "settled" || status == "expired" {
		return true, tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_procurement_cost_versions v SET status='settled',settled_at=NOW(),ended_at=NOW(),request_id=$2,loss_cny=GREATEST(v.cost_cny-LEAST(v.cost_cny,COALESCE((SELECT SUM(COALESCE(ul.total_cost,0)) FROM usage_logs ul WHERE ul.account_id=v.account_id AND ul.created_at>=v.effective_at)*v.cost_cny/v.estimated_usable_quota_usd,0)),0),updated_at=NOW() WHERE v.id=$1`, id, in.RequestID)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,method,path,request_id,status_code,extra) VALUES(NULLIF($1,0),'account.procurement.settle','POST',$2,$3,200,jsonb_build_object('account_id',$4,'reason',$5))`, in.ActorUserID, "/admin/accounts/"+fmt.Sprint(in.AccountID)+"/procurement/settle", in.RequestID, in.AccountID, in.Reason)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

type ProcurementConfigInput struct {
	AccountID         int64
	CostCNY, QuotaUSD *float64
	ActorUserID       int64
	RequestID         string
}

func (s *AccountProfitabilityService) UpdateProcurementConfig(ctx context.Context, in ProcurementConfigInput) error {
	if s == nil || s.db == nil {
		return errors.New("account profitability database is unavailable")
	}
	if in.AccountID <= 0 || in.RequestID == "" {
		return errors.New("account_id and request_id are required")
	}
	if (in.CostCNY == nil) != (in.QuotaUSD == nil) {
		return errors.New("cost and quota must be provided together")
	}
	if in.CostCNY != nil && (*in.CostCNY < 0 || *in.QuotaUSD <= 0 || math.IsNaN(*in.CostCNY) || math.IsNaN(*in.QuotaUSD) || math.IsInf(*in.CostCNY, 0) || math.IsInf(*in.QuotaUSD, 0)) {
		return errors.New("invalid procurement values")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var replay int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM account_procurement_cost_versions WHERE request_id=$1`, in.RequestID).Scan(&replay); err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var created time.Time
	if err := tx.QueryRowContext(ctx, `SELECT created_at FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.AccountID).Scan(&created); err != nil {
		return err
	}
	var oldID int64
	var oldCost, oldQuota float64
	var oldEffective time.Time
	if err := tx.QueryRowContext(ctx, `SELECT id,cost_cny,estimated_usable_quota_usd,effective_at FROM account_procurement_cost_versions WHERE account_id=$1 AND ended_at IS NULL FOR UPDATE`, in.AccountID).Scan(&oldID, &oldCost, &oldQuota, &oldEffective); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now().UTC()
	if oldID > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE account_procurement_cost_versions SET ended_at=$2,status='ended',updated_at=$2 WHERE id=$1`, oldID, now); err != nil {
			return err
		}
	}
	if in.CostCNY == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET procurement_cost_cny=NULL,estimated_usable_quota_usd=NULL,procurement_cost_effective_at=NULL,updated_at=$2 WHERE id=$1`, in.AccountID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,method,path,request_id,status_code,extra) VALUES(NULLIF($1,0),'account.procurement.update','PUT',$2,$3,200,jsonb_build_object('account_id',$4,'cleared',true))`, in.ActorUserID, "/admin/accounts/"+fmt.Sprint(in.AccountID)+"/procurement", in.RequestID, in.AccountID); err != nil {
			return err
		}
		return tx.Commit()
	}
	var next int
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_no),0)+1 FROM account_procurement_cost_versions WHERE account_id=$1`, in.AccountID).Scan(&next)
	effective := now
	if oldID == 0 {
		effective = created
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO account_procurement_cost_versions(account_id,version_no,cost_cny,estimated_usable_quota_usd,effective_at,status,actor_user_id,request_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'active',$6,$7,$8,$8)`, in.AccountID, next, *in.CostCNY, *in.QuotaUSD, effective, in.ActorUserID, in.RequestID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET procurement_cost_cny=$2,estimated_usable_quota_usd=$3,procurement_cost_effective_at=$4,updated_at=$5 WHERE id=$1`, in.AccountID, *in.CostCNY, *in.QuotaUSD, effective, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,method,path,request_id,status_code,extra) VALUES(NULLIF($1,0),'account.procurement.update','PUT',$2,$3,200,jsonb_build_object('account_id',$4,'cost_cny',$5,'quota_usd',$6))`, in.ActorUserID, "/admin/accounts/"+fmt.Sprint(in.AccountID)+"/procurement", in.RequestID, in.AccountID, *in.CostCNY, *in.QuotaUSD); err != nil {
		return err
	}
	return tx.Commit()
}
