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
	row        SelfPurchasedProfitabilityRow
	hasPending bool
	quotaBasis float64
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
	rows, err := s.db.QueryContext(ctx, `
WITH versions AS (
    SELECT v.account_id, v.version_no, v.cost_cny, v.estimated_usable_quota_usd,
           v.effective_at, v.ended_at, v.status, v.settled_at, v.loss_cny
      FROM account_procurement_cost_versions v
    UNION ALL
    SELECT a.id, 0, a.procurement_cost_cny, a.estimated_usable_quota_usd,
           COALESCE(a.procurement_cost_effective_at, a.created_at), NULL, 'active', NULL, 0
      FROM accounts a
     WHERE a.procurement_cost_cny IS NOT NULL
       AND a.estimated_usable_quota_usd IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM account_procurement_cost_versions v WHERE v.account_id = a.id)
), scoped AS (
    SELECT a.id, a.name, a.platform, a.type,
           CASE WHEN a.expires_at IS NOT NULL AND a.expires_at <= NOW() THEN 'expired' ELSE a.status END AS status,
           v.version_no, v.cost_cny, v.estimated_usable_quota_usd,
           v.effective_at, v.ended_at, v.status AS version_status,
           v.settled_at, v.loss_cny,
           COALESCE(SUM(CASE
             WHEN COALESCE(ul.billing_mode, 'token') = 'token'
              AND COALESCE(ul.image_count, 0) = 0
              AND COALESCE(ul.video_count, 0) = 0
              AND COALESCE(ul.usage_completeness, 'complete') = 'complete'
              AND COALESCE(ul.request_type, 0) <> 4
              AND (COALESCE(ul.input_tokens,0) + COALESCE(ul.output_tokens,0) + COALESCE(ul.cache_creation_tokens,0) + COALESCE(ul.cache_read_tokens,0)) > 0
             THEN ul.total_cost ELSE 0 END), 0)::double precision AS standard_consumed,
           COALESCE(SUM(ul.actual_cost), 0)::double precision AS revenue
      FROM accounts a
      JOIN versions v ON v.account_id = a.id
 LEFT JOIN usage_logs ul ON ul.account_id = a.id
       AND ul.created_at >= GREATEST(v.effective_at, $1)
       AND ul.created_at < LEAST(COALESCE(v.ended_at, $2), $2)
     WHERE a.deleted_at IS NULL
       AND ((v.effective_at < $2 AND COALESCE(v.ended_at, $2) > $1)
         OR (v.settled_at >= $1 AND v.settled_at < $2))
  GROUP BY a.id, a.name, a.platform, a.type, a.status, a.expires_at,
           v.version_no, v.cost_cny, v.estimated_usable_quota_usd,
           v.effective_at, v.ended_at, v.status, v.settled_at, v.loss_cny
)
SELECT id,name,platform,type,status,version_no,cost_cny,estimated_usable_quota_usd,
       effective_at,ended_at,version_status,settled_at,loss_cny,standard_consumed,revenue
  FROM scoped ORDER BY id,version_no`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aggs := map[int64]*procurementAggregate{}
	for rows.Next() {
		var id int64
		var version int
		var name, platform, typ, status, versionStatus string
		var effective time.Time
		var ended, settled sql.NullTime
		var cost, quota sql.NullFloat64
		var loss, standard, revenue float64
		if err := rows.Scan(&id, &name, &platform, &typ, &status, &version, &cost, &quota,
			&effective, &ended, &versionStatus, &settled, &loss, &standard, &revenue); err != nil {
			return nil, err
		}
		a := aggs[id]
		if a == nil {
			a = &procurementAggregate{row: SelfPurchasedProfitabilityRow{AccountID: id, Name: name, Platform: platform, AccountType: typ, Status: status, CostStatus: ProcurementStatusCostPending}}
			aggs[id] = a
		}
		a.row.StandardConsumedUSD += standard
		a.row.RevenueCNY += revenue
		if cost.Valid && quota.Valid {
			confirmed, pending, _, _, _, _ := calculateProcurementMetrics(cost.Float64, quota.Float64, standard, revenue, false)
			a.row.ConfirmedCostCNY += confirmed
			if settled.Valid || versionStatus == "settled" {
				a.row.LossCNY += loss
				if cost.Float64 > 0 {
					a.quotaBasis += math.Min(math.Max(standard, 0), quota.Float64) + loss/(cost.Float64/quota.Float64)
				}
			} else if versionStatus == string(ProcurementStatusActive) {
				a.row.PendingCostCNY += pending
				a.quotaBasis += quota.Float64
			} else {
				a.quotaBasis += math.Min(math.Max(standard, 0), quota.Float64)
			}
		} else {
			a.hasPending = true
		}
		a.row.CostStatus = ProcurementStatus(versionStatus)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := &SelfPurchasedProfitabilityReport{StartDate: start.Format("2006-01-02"), EndDate: end.Add(-time.Nanosecond).Format("2006-01-02"), GeneratedAt: s.now().UTC(), Currency: "CNY", Rows: make([]SelfPurchasedProfitabilityRow, 0, len(aggs))}
	for _, a := range aggs {
		r := a.row
		if a.hasPending {
			r.ProcurementCostCNY = nil
			r.EstimatedQuotaUSD = nil
			r.CostStatus = ProcurementStatusCostPending
			out.Rows = append(out.Rows, r)
			continue
		}
		procurementCost := r.ConfirmedCostCNY + r.PendingCostCNY + r.LossCNY
		r.ProcurementCostCNY = procurementFloat64Ptr(procurementCost)
		r.EstimatedQuotaUSD = procurementFloat64Ptr(a.quotaBasis)
		if *r.EstimatedQuotaUSD > 0 {
			u := r.StandardConsumedUSD / *r.EstimatedQuotaUSD
			if u > 1 {
				u = 1
			}
			r.Utilization = &u
		}
		profit := r.RevenueCNY - r.ConfirmedCostCNY - r.LossCNY
		r.NetProfitCNY = &profit
		if r.RevenueCNY != 0 {
			margin := profit / r.RevenueCNY
			r.Margin = &margin
		}
		if r.CostStatus == "" || r.CostStatus == ProcurementStatusActive {
			r.CostStatus = ProcurementStatusActive
		}
		out.Summary.ProcurementCostCNY += *r.ProcurementCostCNY
		out.Summary.StandardConsumedUSD += r.StandardConsumedUSD
		out.Summary.ConfirmedCostCNY += r.ConfirmedCostCNY
		out.Summary.PendingCostCNY += r.PendingCostCNY
		out.Summary.LossCNY += r.LossCNY
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

func procurementFloat64Ptr(v float64) *float64 { return &v }

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
	var existing, existingAccountID int64
	err = tx.QueryRowContext(ctx, `SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id=$1`, in.RequestID).Scan(&existing, &existingAccountID)
	if err == nil {
		if existingAccountID != in.AccountID {
			return false, errors.New("settlement idempotency key conflict")
		}
		return true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var id int64
	var cost, quota float64
	var effective time.Time
	var versionStatus, accountStatus string
	var expiresAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT v.id,v.cost_cny,v.estimated_usable_quota_usd,v.effective_at,v.status,a.status,a.expires_at
        FROM account_procurement_cost_versions v
        JOIN accounts a ON a.id=v.account_id AND a.deleted_at IS NULL
       WHERE v.account_id=$1 ORDER BY v.version_no DESC LIMIT 1 FOR UPDATE`, in.AccountID).Scan(&id, &cost, &quota, &effective, &versionStatus, &accountStatus, &expiresAt)
	if err != nil {
		return false, err
	}
	if versionStatus == "settled" || versionStatus == "expired" {
		return true, tx.Commit()
	}
	if versionStatus != string(ProcurementStatusActive) {
		return false, errors.New("procurement version is not active")
	}
	permanentlyUnavailable := accountStatus == StatusDisabled || accountStatus == StatusError || (expiresAt.Valid && !expiresAt.Time.After(s.now()))
	if !permanentlyUnavailable {
		return false, errors.New("account is not permanently unavailable")
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_procurement_cost_versions v SET status='settled',settled_at=NOW(),ended_at=NOW(),settlement_request_id=$2,
        loss_cny=GREATEST(v.cost_cny-LEAST(v.cost_cny,COALESCE((SELECT SUM(CASE
          WHEN COALESCE(ul.billing_mode,'token')='token' AND COALESCE(ul.image_count,0)=0
           AND COALESCE(ul.video_count,0)=0 AND COALESCE(ul.usage_completeness,'complete')='complete'
           AND COALESCE(ul.request_type,0)<>4
           AND (COALESCE(ul.input_tokens,0)+COALESCE(ul.output_tokens,0)+COALESCE(ul.cache_creation_tokens,0)+COALESCE(ul.cache_read_tokens,0))>0
          THEN COALESCE(ul.total_cost,0) ELSE 0 END)
          FROM usage_logs ul WHERE ul.account_id=v.account_id AND ul.created_at>=v.effective_at)*v.cost_cny/v.estimated_usable_quota_usd,0)),0),
        updated_at=NOW() WHERE v.id=$1`, id, in.RequestID)
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
	var replayAccountID int64
	replayErr := tx.QueryRowContext(ctx, `SELECT account_id FROM account_procurement_cost_versions WHERE request_id=$1`, in.RequestID).Scan(&replayAccountID)
	if replayErr == nil {
		if replayAccountID != in.AccountID {
			return errors.New("procurement idempotency key conflict")
		}
		return tx.Commit()
	}
	if !errors.Is(replayErr, sql.ErrNoRows) {
		return replayErr
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
	now := s.now().UTC()
	nextCost, nextQuota := 0.0, 0.0
	if in.CostCNY != nil {
		nextCost, nextQuota = *in.CostCNY, *in.QuotaUSD
	}
	if oldID > 0 {
		if in.CostCNY != nil {
			var consumed float64
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE
            WHEN COALESCE(billing_mode,'token')='token' AND COALESCE(image_count,0)=0
             AND COALESCE(video_count,0)=0 AND COALESCE(usage_completeness,'complete')='complete'
             AND COALESCE(request_type,0)<>4
             AND (COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(cache_creation_tokens,0)+COALESCE(cache_read_tokens,0))>0
            THEN total_cost ELSE 0 END),0)::double precision
	          FROM usage_logs WHERE account_id=$1 AND created_at >= $2 AND created_at < $3`, in.AccountID, oldEffective, now).Scan(&consumed); err != nil {
				return err
			}
			confirmed := math.Min(oldCost, consumed*(oldCost/oldQuota))
			remainingCost := math.Max(*in.CostCNY-confirmed, 0)
			remainingQuota := math.Max(*in.QuotaUSD-consumed, 0)
			nextCost = remainingCost
			nextQuota = remainingQuota
			if nextQuota <= 0 {
				return errors.New("remaining estimated quota must be > 0")
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE account_procurement_cost_versions SET ended_at=$2,status='ended',updated_at=$2 WHERE id=$1`, oldID, now); err != nil {
			return err
		}
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_no),0)+1 FROM account_procurement_cost_versions WHERE account_id=$1`, in.AccountID).Scan(&next); err != nil {
		return err
	}
	if in.CostCNY == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET procurement_cost_cny=NULL,estimated_usable_quota_usd=NULL,procurement_cost_effective_at=NULL,updated_at=$2 WHERE id=$1`, in.AccountID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_procurement_cost_versions(account_id,version_no,effective_at,status,actor_user_id,request_id,created_at,updated_at) VALUES($1,$2,$3,'cost_pending',$4,$5,$6,$6)`, in.AccountID, next, now, in.ActorUserID, in.RequestID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,method,path,request_id,status_code,extra) VALUES(NULLIF($1,0),'account.procurement.update','PUT',$2,$3,200,jsonb_build_object('account_id',$4,'cleared',true))`, in.ActorUserID, "/admin/accounts/"+fmt.Sprint(in.AccountID)+"/procurement", in.RequestID, in.AccountID); err != nil {
			return err
		}
		return tx.Commit()
	}
	effective := now
	if oldID == 0 {
		effective = created
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO account_procurement_cost_versions(account_id,version_no,cost_cny,estimated_usable_quota_usd,effective_at,status,actor_user_id,request_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'active',$6,$7,$8,$8)`, in.AccountID, next, nextCost, nextQuota, effective, in.ActorUserID, in.RequestID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET procurement_cost_cny=$2,estimated_usable_quota_usd=$3,procurement_cost_effective_at=$4,updated_at=$5 WHERE id=$1`, in.AccountID, nextCost, nextQuota, effective, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,method,path,request_id,status_code,extra) VALUES(NULLIF($1,0),'account.procurement.update','PUT',$2,$3,200,jsonb_build_object('account_id',$4,'cost_cny',$5,'quota_usd',$6))`, in.ActorUserID, "/admin/accounts/"+fmt.Sprint(in.AccountID)+"/procurement", in.RequestID, in.AccountID, nextCost, nextQuota); err != nil {
		return err
	}
	return tx.Commit()
}
