package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type quotaWalletRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewQuotaWalletRepository(client *dbent.Client, sqlDB *sql.DB) service.QuotaWalletRepository {
	return &quotaWalletRepository{client: client, sql: sqlDB}
}

func (r *quotaWalletRepository) GetSummary(ctx context.Context, userID int64) (service.QuotaSummary, error) {
	var w service.QuotaWallet
	err := scanOne(ctx, r.clientFrom(ctx), `SELECT user_id, cash_balance_cny, paid_quota_balance_usd, gift_quota_balance_usd, version, updated_at FROM user_wallets WHERE user_id=$1`, []any{userID}, &w.UserID, &w.CashBalanceCNY, &w.PaidQuotaBalanceUSD, &w.GiftQuotaBalanceUSD, &w.Version, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return service.QuotaSummary{}, service.ErrQuotaWalletNotFound
	}
	if err != nil {
		return service.QuotaSummary{}, err
	}
	return summary(w), nil
}

func (r *quotaWalletRepository) WithLockedWallet(ctx context.Context, userID int64, fn func(context.Context, *service.QuotaWallet) error) error {
	if userID <= 0 {
		return service.ErrQuotaWalletNotFound
	}
	tx, err := r.client.Tx(ctx)
	owned := err == nil
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		ctx = dbent.NewTxContext(ctx, tx)
	} else if existing := dbent.TxFromContext(ctx); existing != nil {
		tx = existing
	}
	c := tx.Client()
	// Lock the user first, so two initializers cannot copy different legacy balances.
	var legacy float64
	if err := scanOne(ctx, c, `SELECT balance FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, []any{userID}, &legacy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrQuotaWalletNotFound
		}
		return err
	}
	// Initialize once from the compatibility projection. The unique user_id constraint
	// makes this safe even if another transaction initialized the row earlier.
	_, err = c.ExecContext(ctx, `INSERT INTO user_wallets (user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,created_at,updated_at) VALUES ($1,0,$2,0,1,NOW(),NOW()) ON CONFLICT (user_id) DO NOTHING`, userID, legacy)
	if err != nil {
		return err
	}
	var w service.QuotaWallet
	err = scanOne(ctx, c, `SELECT id,user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,updated_at FROM user_wallets WHERE user_id=$1 FOR UPDATE`, []any{userID}, &w.ID, &w.UserID, &w.CashBalanceCNY, &w.PaidQuotaBalanceUSD, &w.GiftQuotaBalanceUSD, &w.Version, &w.UpdatedAt)
	if err != nil {
		return err
	}
	if err := fn(ctx, &w); err != nil {
		return err
	}
	if owned {
		return tx.Commit()
	}
	return nil
}

func (r *quotaWalletRepository) ApplyMutation(ctx context.Context, wallet *service.QuotaWallet, result service.QuotaMutationResult, recordType, idemKey, refType, refID, note string, operatorID *int64) (service.QuotaMutationResult, error) {
	if wallet == nil {
		return service.QuotaMutationResult{}, service.ErrQuotaWalletNotFound
	}
	client := r.clientFrom(ctx)
	if idemKey != "" {
		// Fingerprints deliberately exclude the calculated snapshot/version/time;
		// retrying the same request after the first commit must hash identically.
		fingerprint := requestFingerprint(recordType, refType, refID, note, result.CashDeltaCNY, result.PaidDeltaUSD, result.GiftDeltaUSD, result.PaidConsumedUSD, result.GiftConsumedUSD)
		var status string
		var ledgerID sql.NullInt64
		var oldFP string
		err := scanOne(ctx, client, `SELECT request_fingerprint,status,ledger_entry_id FROM quota_idempotency_records WHERE user_id=$1 AND idempotency_key=$2 FOR UPDATE`, []any{wallet.UserID, idemKey}, &oldFP, &status, &ledgerID)
		if err == nil {
			if oldFP != fingerprint {
				return service.QuotaMutationResult{}, service.ErrQuotaIdempotencyConflict
			}
			if ledgerID.Valid {
				prior, e := r.loadMutation(ctx, client, ledgerID.Int64)
				if e != nil {
					return service.QuotaMutationResult{}, e
				}
				prior.Idempotent = true
				return prior, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return service.QuotaMutationResult{}, err
		} else {
			if _, err := client.ExecContext(ctx, `INSERT INTO quota_idempotency_records (user_id,idempotency_key,request_fingerprint,status,created_at,updated_at) VALUES ($1,$2,$3,'processing',NOW(),NOW())`, wallet.UserID, idemKey, fingerprint); err != nil {
				return service.QuotaMutationResult{}, err
			}
		}
	}
	newCash := result.Summary.CashBalanceCNY
	newPaid := result.Summary.PaidQuotaBalanceUSD
	newGift := result.Summary.GiftQuotaBalanceUSD
	if _, err := client.ExecContext(ctx, `UPDATE user_wallets SET cash_balance_cny=$1,paid_quota_balance_usd=$2,gift_quota_balance_usd=$3,version=version+1,updated_at=NOW() WHERE user_id=$4`, newCash.InexactFloat64(), newPaid.InexactFloat64(), newGift.InexactFloat64(), wallet.UserID); err != nil {
		return service.QuotaMutationResult{}, err
	}
	if _, err := client.ExecContext(ctx, `UPDATE users SET balance=$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, newPaid.Add(newGift).InexactFloat64(), wallet.UserID); err != nil {
		return service.QuotaMutationResult{}, err
	}
	b := client.UserQuotaLedgerEntry.Create().SetUserID(wallet.UserID).SetRecordType(recordType).SetCashDeltaCny(result.CashDeltaCNY.InexactFloat64()).SetPaidQuotaDeltaUsd(result.PaidDeltaUSD.InexactFloat64()).SetGiftQuotaDeltaUsd(result.GiftDeltaUSD.InexactFloat64()).SetCashBeforeCny(wallet.CashBalanceCNY.InexactFloat64()).SetCashAfterCny(newCash.InexactFloat64()).SetPaidBeforeUsd(wallet.PaidQuotaBalanceUSD.InexactFloat64()).SetPaidAfterUsd(newPaid.InexactFloat64()).SetGiftBeforeUsd(wallet.GiftQuotaBalanceUSD.InexactFloat64()).SetGiftAfterUsd(newGift.InexactFloat64()).SetNote(note)
	if refType != "" {
		b.SetReferenceType(refType)
	}
	if refID != "" {
		b.SetReferenceID(refID)
	}
	if idemKey != "" {
		b.SetIdempotencyKey(idemKey)
	}
	if operatorID != nil {
		b.SetOperatorID(*operatorID)
	}
	entry, err := b.Save(ctx)
	if err != nil {
		return service.QuotaMutationResult{}, err
	}
	if idemKey != "" {
		_, err = client.ExecContext(ctx, `UPDATE quota_idempotency_records SET status='confirmed',ledger_entry_id=$1,updated_at=NOW() WHERE user_id=$2 AND idempotency_key=$3`, entry.ID, wallet.UserID, idemKey)
		if err != nil {
			return service.QuotaMutationResult{}, err
		}
	}
	result.LedgerEntryID, result.Summary.WalletVersion = entry.ID, wallet.Version+1
	result.Summary.UpdatedAt = time.Now().UTC()
	return result, nil
}

func (r *quotaWalletRepository) loadMutation(ctx context.Context, c *dbent.Client, id int64) (service.QuotaMutationResult, error) {
	var e service.QuotaLedgerEntry
	err := scanOne(ctx, c, `SELECT id,user_id,cash_delta_cny,paid_quota_delta_usd,gift_quota_delta_usd,cash_after_cny,paid_after_usd,gift_after_usd FROM user_quota_ledger_entries WHERE id=$1`, []any{id}, &e.ID, &e.UserID, &e.CashDeltaCNY, &e.PaidQuotaDeltaUSD, &e.GiftQuotaDeltaUSD, &e.CashAfterCNY, &e.PaidAfterUSD, &e.GiftAfterUSD)
	if err != nil {
		return service.QuotaMutationResult{}, err
	}
	s := service.QuotaSummary{UserID: e.UserID, CashBalanceCNY: e.CashAfterCNY, PaidQuotaBalanceUSD: e.PaidAfterUSD, GiftQuotaBalanceUSD: e.GiftAfterUSD, TotalQuotaBalanceUSD: e.PaidAfterUSD.Add(e.GiftAfterUSD)}
	return service.QuotaMutationResult{Summary: s, CashDeltaCNY: e.CashDeltaCNY, PaidDeltaUSD: e.PaidQuotaDeltaUSD, GiftDeltaUSD: e.GiftQuotaDeltaUSD, LedgerEntryID: e.ID}, nil
}

func (r *quotaWalletRepository) ListLedger(ctx context.Context, userID int64, page, pageSize int, recordType string) ([]service.QuotaLedgerEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	args := []any{userID}
	where := "WHERE user_id=$1"
	if recordType != "" {
		args = append(args, recordType)
		where += fmt.Sprintf(" AND record_type=$%d", len(args))
	}
	var total int
	if err := scanOne(ctx, r.clientFrom(ctx), "SELECT COUNT(*) FROM user_quota_ledger_entries "+where, args, &total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, offset)
	q := fmt.Sprintf(`SELECT id,user_id,record_type,cash_delta_cny,paid_quota_delta_usd,gift_quota_delta_usd,cash_before_cny,cash_after_cny,paid_before_usd,paid_after_usd,gift_before_usd,gift_after_usd,COALESCE(reference_type,''),COALESCE(reference_id,''),COALESCE(idempotency_key,''),note,operator_id,status,created_at FROM user_quota_ledger_entries %s ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.clientFrom(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []service.QuotaLedgerEntry{}
	for rows.Next() {
		var e service.QuotaLedgerEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.RecordType, &e.CashDeltaCNY, &e.PaidQuotaDeltaUSD, &e.GiftQuotaDeltaUSD, &e.CashBeforeCNY, &e.CashAfterCNY, &e.PaidBeforeUSD, &e.PaidAfterUSD, &e.GiftBeforeUSD, &e.GiftAfterUSD, &e.ReferenceType, &e.ReferenceID, &e.IdempotencyKey, &e.Note, &e.OperatorID, &e.Status, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *quotaWalletRepository) clientFrom(ctx context.Context) *dbent.Client {
	return clientFromContext(ctx, r.client)
}
func summary(w service.QuotaWallet) service.QuotaSummary {
	return service.QuotaSummary{UserID: w.UserID, CashBalanceCNY: w.CashBalanceCNY, PaidQuotaBalanceUSD: w.PaidQuotaBalanceUSD, GiftQuotaBalanceUSD: w.GiftQuotaBalanceUSD, TotalQuotaBalanceUSD: w.PaidQuotaBalanceUSD.Add(w.GiftQuotaBalanceUSD), WalletVersion: w.Version, UpdatedAt: w.UpdatedAt}
}
func requestFingerprint(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%v\x00", p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func scanOne(ctx context.Context, c *dbent.Client, query string, args []any, dest ...any) error {
	rows, err := c.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if e := rows.Err(); e != nil {
			return e
		}
		return sql.ErrNoRows
	}
	return rows.Scan(dest...)
}
