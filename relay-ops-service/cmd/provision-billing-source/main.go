package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/upstreams"
)

const defaultDeclarationPath = "/run/secrets/billing-source-declaration.json"

type billingProvisioner interface {
	Provision(context.Context, domain.AdminActor, billing.BillingProvisionInput) (billing.BillingProvisionResult, error)
}

type commandDependencies struct {
	EUID        func() int
	Load        func(string) (billing.BillingProvisionDeclaration, error)
	Provisioner billingProvisioner
}

type commandOutput struct {
	Status           string `json:"status"`
	UpstreamID       int64  `json:"upstream_id"`
	BillingAccountID int64  `json:"billing_account_id"`
}

func main() {
	declarationPath := flag.String("declaration", defaultDeclarationPath, "root-owned 0600 billing source declaration")
	flag.Parse()
	if os.Geteuid() != 0 {
		log.Fatal("billing provision must run as root")
	}
	ctx := context.Background()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatal("billing provision configuration unavailable")
	}
	database, err := store.Open(ctx, cfg.DatabaseURLFile)
	if err != nil {
		log.Fatal("billing provision database unavailable")
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		log.Fatal("billing provision migration unavailable")
	}
	service := billing.ProvisioningService{
		Repository: database,
		Upstreams:  upstreams.Service{Resolver: net.DefaultResolver},
		Sessions:   billing.SessionRegistrationService{Resolver: net.DefaultResolver},
	}
	if err := execute(ctx, *declarationPath, os.Stdout, commandDependencies{
		EUID: os.Geteuid, Load: billing.LoadBillingProvisionDeclaration, Provisioner: service,
	}); err != nil {
		// Deliberately avoid returning error text: validation errors must never
		// turn an operator-provided secret reference into a log line.
		log.Fatal("billing provision rejected")
	}
}

func execute(ctx context.Context, declarationPath string, output io.Writer, dependencies commandDependencies) error {
	if dependencies.EUID == nil || dependencies.EUID() != 0 {
		return errors.New("billing provision must run as root")
	}
	if dependencies.Load == nil || dependencies.Provisioner == nil {
		return errors.New("billing provision dependencies are unavailable")
	}
	declaration, err := dependencies.Load(declarationPath)
	if err != nil {
		return fmt.Errorf("load billing declaration: %w", err)
	}
	result, err := dependencies.Provisioner.Provision(ctx, domain.AdminActor{UserID: declaration.ActorUserID}, declaration.ProvisionInput())
	if err != nil {
		return fmt.Errorf("persist billing provision: %w", err)
	}
	status := "configured"
	if result.AlreadyConfigured {
		status = "already_configured"
	}
	return json.NewEncoder(output).Encode(commandOutput{Status: status, UpstreamID: int64(result.UpstreamID), BillingAccountID: result.BillingAccountID})
}
