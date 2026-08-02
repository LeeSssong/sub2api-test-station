package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"example.invalid/relay-ops-service/internal/upstreams"
)

const maxBillingProvisionDeclarationBytes = 64 << 10

// BillingProvisionDeclaration is a root-owned, non-secret operator input.
// The bearer itself is referenced only by a mounted filename.
type BillingProvisionDeclaration struct {
	Version     int64                        `json:"version"`
	ActorUserID int64                        `json:"actor_user_id"`
	Production  BillingProductionDeclaration `json:"production"`
	Billing     BillingSessionDeclaration    `json:"billing"`
}

type BillingProductionDeclaration struct {
	Name           string  `json:"name"`
	BaseURL        string  `json:"base_url"`
	AdapterType    string  `json:"adapter_type"`
	PricingURL     string  `json:"pricing_url"`
	UsageURL       string  `json:"usage_url"`
	PerformanceURL string  `json:"performance_url"`
	GroupIDs       []int64 `json:"group_ids"`
	MonitorID      int64   `json:"monitor_id"`
}

type BillingSessionDeclaration struct {
	LoginURL         string `json:"login_url"`
	BillingAccountID int64  `json:"billing_account_id"`
	BearerSecretFile string `json:"bearer_secret_file"`
}

// LoadBillingProvisionDeclaration accepts only a regular, non-symlink 0600
// JSON file. Errors intentionally omit its contents and filename.
func LoadBillingProvisionDeclaration(path string) (BillingProvisionDeclaration, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is unavailable")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > maxBillingProvisionDeclarationBytes {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBillingProvisionDeclarationBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxBillingProvisionDeclarationBytes {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is unavailable")
	}
	defer clearProvisionDeclaration(raw)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var declaration BillingProvisionDeclaration
	if err := decoder.Decode(&declaration); err != nil {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is invalid")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is invalid")
	}
	if declaration.Version != 1 || declaration.ActorUserID <= 0 || declaration.Billing.BillingAccountID <= 0 || strings.TrimSpace(declaration.Billing.BearerSecretFile) == "" {
		return BillingProvisionDeclaration{}, fmt.Errorf("billing declaration is invalid")
	}
	return declaration, nil
}

func (d BillingProvisionDeclaration) ProvisionInput() BillingProvisionInput {
	return BillingProvisionInput{
		Production: upstreams.ProductionInput{
			Name: d.Production.Name, BaseURL: d.Production.BaseURL, AdapterType: d.Production.AdapterType,
			PricingURL: d.Production.PricingURL, UsageURL: d.Production.UsageURL, PerformanceURL: d.Production.PerformanceURL,
			GroupIDs: append([]int64(nil), d.Production.GroupIDs...), MonitorID: d.Production.MonitorID,
		},
		BearerSecretFile: d.Billing.BearerSecretFile,
		LoginURL:         d.Billing.LoginURL,
		BillingAccountID: d.Billing.BillingAccountID,
	}
}

func clearProvisionDeclaration(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
