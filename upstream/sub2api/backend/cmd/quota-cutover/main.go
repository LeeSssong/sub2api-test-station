// Command quota-cutover performs a read-only quota migration dry-run. It never
// inserts opening grants or flips runtime flags; the reconciliation gate must
// be passed by a separately authorized release operation.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/quota/cutover"
	"github.com/Wei-Shaw/sub2api/internal/quota/reconciliation"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type dryRunOutput struct {
	BatchID    string                `json:"batch_id,omitempty"`
	QueriedAt  time.Time             `json:"queried_at"`
	Report     reconciliation.Report `json:"reconciliation"`
	GatePassed bool                  `json:"gate_passed"`
}

func runDryRun(ctx context.Context, db reconciliation.Queryer, batch string) (dryRunOutput, error) {
	snapshot, err := reconciliation.LoadSnapshot(ctx, db)
	if err != nil {
		return dryRunOutput{}, err
	}
	report := reconciliation.Evaluate(snapshot)
	gate := cutover.DryRunReport{Users: report.Global.Users, ReconciliationPassed: !report.HasDifferences(), UnattributedDelta: snapshot.LegacyUnknownResidual}
	return dryRunOutput{BatchID: batch, QueriedAt: time.Now().UTC(), Report: report, GatePassed: gate.ValidateCutoverGate() == nil}, nil
}

func main() {
	dsn := flag.String("dsn", "", "PostgreSQL DSN; queried using a READ ONLY transaction")
	batch := flag.String("batch", "", "dry-run batch identifier")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: quota-cutover -dsn postgres-dsn [-batch id]")
		os.Exit(2)
	}
	db, err := sql.Open("postgres", *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := runDryRun(context.Background(), tx, *batch)
	commitErr := tx.Commit()
	if err == nil {
		err = commitErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = decimal.Zero // retain decimal as an explicit boundary dependency for future opening output.
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if !out.GatePassed {
		os.Exit(1)
	}
}
