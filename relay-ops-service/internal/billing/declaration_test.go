package billing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBillingProvisionDeclarationRequiresCanonical0600NonSecretJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "billing-source.json")
	contents := `{
  "version": 1,
  "actor_user_id": 42,
  "production": {
    "name": "billing-primary",
    "base_url": "https://billing.example/v1",
    "adapter_type": "sub2api",
    "pricing_url": "https://billing.example/pricing",
    "usage_url": "https://billing.example/usage",
    "group_ids": [7]
  },
  "billing": {
    "login_url": "https://billing.example/login",
    "billing_account_id": 8123,
    "bearer_secret_file": "/run/secrets/upstream-sessions/billing-primary"
  }
}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	declaration, err := LoadBillingProvisionDeclaration(path)
	if err != nil {
		t.Fatal(err)
	}
	if declaration.ActorUserID != 42 || declaration.Production.Name != "billing-primary" || declaration.Billing.BillingAccountID != 8123 {
		t.Fatalf("declaration=%#v", declaration)
	}
	if strings.Contains(declaration.Billing.BearerSecretFile, "token-value") {
		t.Fatal("declaration retained a bearer credential")
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBillingProvisionDeclaration(path); err == nil {
		t.Fatal("0640 declaration accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"unexpected":"value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBillingProvisionDeclaration(path); err == nil {
		t.Fatal("unknown declaration field accepted")
	}
}
