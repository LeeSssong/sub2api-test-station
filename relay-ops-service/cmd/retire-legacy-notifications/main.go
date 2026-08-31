package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"

	"example.invalid/relay-ops-service/internal/legacyretirement"
)

const (
	confirmationEnvironment = "RELAY_OPS_RETIRE_LEGACY_NOTIFICATIONS_CONFIRM"
	defaultDatabaseURLFile  = "/run/secrets/relay-ops-database-url"
)

type retirementDatabase interface {
	legacyretirement.Database
	Close()
}

type databaseOpener func(context.Context, string) (retirementDatabase, error)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, func(ctx context.Context, path string) (retirementDatabase, error) {
		return legacyretirement.Open(ctx, path)
	}); err != nil {
		log.Fatal("legacy notification retirement command failed")
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, output io.Writer, open databaseOpener) error {
	flags := flag.NewFlagSet("retire-legacy-notifications", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	execute := flags.Bool("execute", false, "permanently delete authorized legacy notification tables")
	databaseURLFile := flags.String("database-url-file", defaultDatabaseURLFile, "protected relay-ops database URL file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	database, err := open(ctx, *databaseURLFile)
	if err != nil {
		return err
	}
	defer database.Close()
	return legacyretirement.Run(ctx, database, legacyretirement.Options{
		Execute:      *execute,
		Confirmation: getenv(confirmationEnvironment),
	}, output)
}
