package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/domain"
)

func TestExecuteRequiresRootAndPrintsOnlyRedactedResult(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := execute(context.Background(), "/run/secrets/billing-source-declaration.json", &output, commandDependencies{
		EUID: func() int { return 10002 },
	})
	if err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("non-root error=%v", err)
	}

	output.Reset()
	err = execute(context.Background(), "/run/secrets/billing-source-declaration.json", &output, commandDependencies{
		EUID: func() int { return 0 },
		Load: func(string) (billing.BillingProvisionDeclaration, error) {
			return billing.BillingProvisionDeclaration{ActorUserID: 42}, nil
		},
		Provisioner: fakeProvisioner{result: billing.BillingProvisionResult{UpstreamID: 17, BillingAccountID: 8123}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{`"status":"configured"`, `"upstream_id":17`, `"billing_account_id":8123`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output=%q missing %q", got, want)
		}
	}
	if strings.Contains(got, "bearer") || strings.Contains(got, "secret") || strings.Contains(got, "billing-source-declaration") {
		t.Fatalf("output leaked protected detail: %q", got)
	}
}

type fakeProvisioner struct {
	result billing.BillingProvisionResult
	err    error
}

func (p fakeProvisioner) Provision(context.Context, domain.AdminActor, billing.BillingProvisionInput) (billing.BillingProvisionResult, error) {
	if p.err != nil {
		return billing.BillingProvisionResult{}, p.err
	}
	return p.result, nil
}
