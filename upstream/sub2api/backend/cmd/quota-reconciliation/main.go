package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/quota/reconciliation"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type rawSnapshot struct {
	Wallets []struct {
		UserID int64  `json:"user_id"`
		Paid   string `json:"paid_balance_usd"`
		Gift   string `json:"gift_balance_usd"`
	} `json:"wallets"`
	Grants []struct {
		ID           int64  `json:"id"`
		UserID       int64  `json:"user_id"`
		Paid         string `json:"paid_granted_usd"`
		Gift         string `json:"gift_granted_usd"`
		PaidConsumed string `json:"paid_consumed_usd"`
		GiftConsumed string `json:"gift_consumed_usd"`
		PaidRefunded string `json:"paid_refunded_usd"`
		GiftDeducted string `json:"gift_deducted_usd"`
		PaidReserved string `json:"paid_reserved_usd"`
		LegacyOffset string `json:"legacy_debt_offset_usd"`
	} `json:"grants"`
	Usage []struct {
		ID                string `json:"id"`
		UserID            int64  `json:"user_id"`
		GrantID           int64  `json:"grant_id"`
		Delta             string `json:"delta_usd"`
		Paid              string `json:"paid_delta_usd"`
		Gift              string `json:"gift_delta_usd"`
		AllocationValid   bool   `json:"allocation_valid"`
		AttributionStatus string `json:"attribution_status"`
	} `json:"usage"`
	Refunds []struct {
		OrderID  string `json:"order_id"`
		Refunded string `json:"refunded_usd"`
		Adjusted string `json:"adjusted_usd"`
	} `json:"refunds"`
	DuplicateIdempotencyKeys []string `json:"duplicate_idempotency_keys"`
	InvalidAllocationRows    int      `json:"invalid_allocation_rows"`
	UnknownReservations      int      `json:"unknown_reservations"`
	LegacyUnknownResidual    string   `json:"legacy_unknown_residual_usd"`
}

func parseDecimal(value string) (decimal.Decimal, error) {
	if value == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(value)
}

func parseSnapshot(data []byte) (reconciliation.Snapshot, error) {
	var raw rawSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return reconciliation.Snapshot{}, err
	}
	s := reconciliation.Snapshot{DuplicateIdempotencyKeys: raw.DuplicateIdempotencyKeys, InvalidAllocationRows: raw.InvalidAllocationRows, UnknownReservations: raw.UnknownReservations}
	for _, w := range raw.Wallets {
		paid, err := parseDecimal(w.Paid)
		if err != nil {
			return s, err
		}
		gift, err := parseDecimal(w.Gift)
		if err != nil {
			return s, err
		}
		s.Wallets = append(s.Wallets, reconciliation.WalletSnapshot{UserID: w.UserID, PaidBalance: paid, GiftBalance: gift})
	}
	for _, g := range raw.Grants {
		vals := make([]decimal.Decimal, 8)
		texts := []string{g.Paid, g.Gift, g.PaidConsumed, g.GiftConsumed, g.PaidRefunded, g.GiftDeducted, g.PaidReserved, g.LegacyOffset}
		for i, text := range texts {
			var err error
			vals[i], err = parseDecimal(text)
			if err != nil {
				return s, err
			}
		}
		s.Grants = append(s.Grants, reconciliation.GrantSnapshot{ID: g.ID, UserID: g.UserID, PaidGranted: vals[0], GiftGranted: vals[1], PaidConsumed: vals[2], GiftConsumed: vals[3], PaidRefunded: vals[4], GiftDeducted: vals[5], PaidReserved: vals[6], LegacyDebtOffset: vals[7]})
	}
	for _, u := range raw.Usage {
		paid, err := parseDecimal(u.Paid)
		if err != nil {
			return s, err
		}
		gift, err := parseDecimal(u.Gift)
		if err != nil {
			return s, err
		}
		delta, err := parseDecimal(u.Delta)
		if err != nil {
			return s, err
		}
		s.Usage = append(s.Usage, reconciliation.UsageSnapshot{ID: u.ID, UserID: u.UserID, GrantID: u.GrantID, Delta: delta, PaidDelta: paid, GiftDelta: gift, AllocationValid: u.AllocationValid, AttributionStatus: u.AttributionStatus})
	}
	for _, f := range raw.Refunds {
		refunded, err := parseDecimal(f.Refunded)
		if err != nil {
			return s, err
		}
		adjusted, err := parseDecimal(f.Adjusted)
		if err != nil {
			return s, err
		}
		s.Refunds = append(s.Refunds, reconciliation.RefundSnapshot{OrderID: f.OrderID, Refunded: refunded, Adjusted: adjusted})
	}
	var err error
	s.LegacyUnknownResidual, err = parseDecimal(raw.LegacyUnknownResidual)
	return s, err
}

func main() {
	input := flag.String("input", "", "read-only JSON snapshot")
	dsn := flag.String("dsn", "", "PostgreSQL DSN; reads through a READ ONLY transaction")
	batch := flag.String("batch", "", "reconciliation batch identifier")
	database := flag.String("database", "", "database version label")
	flag.Parse()
	if (*input == "") == (*dsn == "") {
		fmt.Fprintln(os.Stderr, "usage: quota-reconciliation exactly one of -input snapshot.json or -dsn postgres-dsn [-batch id] [-database version]")
		os.Exit(2)
	}
	var snapshot reconciliation.Snapshot
	var err error
	if *input != "" {
		data, readErr := os.ReadFile(*input)
		if readErr != nil {
			fmt.Fprintln(os.Stderr, readErr)
			os.Exit(1)
		}
		snapshot, err = parseSnapshot(data)
	} else {
		var db *sql.DB
		db, err = sql.Open("postgres", *dsn)
		if err == nil {
			defer db.Close()
			var tx *sql.Tx
			tx, err = db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
			if err == nil {
				snapshot, err = reconciliation.LoadSnapshot(context.Background(), tx)
				if commitErr := tx.Commit(); err == nil {
					err = commitErr
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report := reconciliation.Evaluate(snapshot)
	report.BatchID = *batch
	report.Database = *database
	report.QueriedAt = time.Now().UTC()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
	if report.HasDifferences() {
		os.Exit(1)
	}
}
