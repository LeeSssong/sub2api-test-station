package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"example.invalid/relay-ops-service/internal/d04readiness"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "d04 readiness snapshot:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("d04-readiness-snapshot", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sub2apiURL := flags.String("sub2api-url", "", "Sub2API base URL")
	adminKeyFile := flags.String("admin-key-file", "", "Sub2API admin key file")
	baseFile := flags.String("base-file", "", "non-secret base snapshot file")
	balanceFile := flags.String("balance-file", "", "non-secret balance evidence file")
	qualityFile := flags.String("quality-file", "", "non-secret quality evidence file")
	outputFile := flags.String("output-file", "", "snapshot output file")
	snapshotID := flags.String("snapshot-id", "", "optional snapshot ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"sub2api-url": *sub2apiURL, "admin-key-file": *adminKeyFile, "base-file": *baseFile,
		"balance-file": *balanceFile, "quality-file": *qualityFile, "output-file": *outputFile,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	base, err := readBase(*baseFile)
	if err != nil {
		return err
	}
	balances, err := readBalances(*balanceFile)
	if err != nil {
		return err
	}
	qualities, err := readQualities(*qualityFile)
	if err != nil {
		return err
	}
	reader, err := sub2api.NewHTTPReader(*sub2apiURL, *adminKeyFile)
	if err != nil {
		return fmt.Errorf("configure Sub2API reader: %w", err)
	}
	now := time.Now().UTC()
	id := *snapshotID
	if id == "" {
		id = "D04-LIGHTWEIGHT-LAUNCH-v3-" + now.Format("20060102T150405Z")
	}
	snapshot, err := (d04readiness.Collector{Accounts: reader, Clock: func() time.Time { return now }}).Collect(context.Background(), d04readiness.Inputs{
		SnapshotID: id, BalanceEvidence: balances, QualityEvidence: qualities,
	})
	if err != nil {
		return err
	}
	if err := d04readiness.WriteSnapshotDocument(*outputFile, snapshot, base); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "snapshot_id=%s account_set_sha256=%s\n", snapshot.SnapshotID, snapshot.UpstreamDiscovery.AccountSetSHA256)
	return nil
}

func readBase(path string) (d04readiness.SnapshotBase, error) {
	file, err := os.Open(path)
	if err != nil {
		return d04readiness.SnapshotBase{}, fmt.Errorf("open base snapshot: %w", err)
	}
	defer file.Close()
	return d04readiness.DecodeSnapshotBase(file)
}

func readBalances(path string) ([]d04readiness.BalanceEvidence, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open balance evidence: %w", err)
	}
	defer file.Close()
	return d04readiness.DecodeBalanceEvidence(file)
}

func readQualities(path string) ([]d04readiness.QualityEvidence, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open quality evidence: %w", err)
	}
	defer file.Close()
	return d04readiness.DecodeQualityEvidence(file)
}
