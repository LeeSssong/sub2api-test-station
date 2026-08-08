package reconciliation

import (
	"encoding/json"
	"testing"

	"example.invalid/relay-ops-service/internal/pricing"
	"github.com/shopspring/decimal"
)

func TestEstimateUpstreamStandardCostUsesStoredUpstreamPriceTable(t *testing.T) {
	payload, err := json.Marshal(pricing.Evidence{
		SchemaVersion: pricing.EvidenceSchemaVersion,
		Confidence:    "structured_json",
		SourceURL:     "https://provider.example/pricing",
		Models: []pricing.ModelPrice{{
			ModelID: "gpt-test", Input: "1.25", Output: "10",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cost, ok := EstimateUpstreamStandardCost(payload, "gpt-test", 1_000_000, 500_000)
	if !ok || !cost.Equal(decimal.RequireFromString("6.25")) {
		t.Fatalf("cost=%s ok=%v", cost, ok)
	}
}

func TestEstimateUpstreamStandardCostRejectsAbsentModelPrice(t *testing.T) {
	payload := []byte(`{"schema_version":"pricing-evidence-v2","confidence":"structured_json","source_url":"https://provider.example/pricing","models":[{"model_id":"other-model","input":"1","output":"2"}]}`)
	if _, ok := EstimateUpstreamStandardCost(payload, "gpt-test", 10, 5); ok {
		t.Fatal("unmatched model price accepted as upstream evidence")
	}
}
