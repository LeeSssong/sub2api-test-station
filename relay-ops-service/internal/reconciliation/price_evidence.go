package reconciliation

import (
	"encoding/json"
	"strings"

	"example.invalid/relay-ops-service/internal/pricing"
	"github.com/shopspring/decimal"
)

var oneMillionTokens = decimal.NewFromInt(1_000_000)

// EstimateUpstreamStandardCost derives a request estimate only from a stored,
// structured upstream pricing snapshot. Model names are exact and ambiguous
// price rows are rejected because request attempts do not retain a tier.
// Prices in pricing.Evidence are USD per one million tokens.
func EstimateUpstreamStandardCost(normalizedPricing []byte, model string, promptTokens, completionTokens int64) (decimal.Decimal, bool) {
	model = strings.TrimSpace(model)
	if len(normalizedPricing) == 0 || model == "" || promptTokens < 0 || completionTokens < 0 {
		return decimal.Zero, false
	}
	var evidence pricing.Evidence
	if err := json.Unmarshal(normalizedPricing, &evidence); err != nil ||
		evidence.SchemaVersion != pricing.EvidenceSchemaVersion ||
		evidence.Confidence != "structured_json" || strings.TrimSpace(evidence.SourceURL) == "" {
		return decimal.Zero, false
	}
	var selected *pricing.ModelPrice
	for index := range evidence.Models {
		candidate := &evidence.Models[index]
		if strings.TrimSpace(candidate.ModelID) != model {
			continue
		}
		if selected != nil {
			return decimal.Zero, false
		}
		selected = candidate
	}
	if selected == nil {
		return decimal.Zero, false
	}
	input, ok := upstreamPriceComponent(selected.Input, promptTokens)
	if !ok {
		return decimal.Zero, false
	}
	output, ok := upstreamPriceComponent(selected.Output, completionTokens)
	if !ok {
		return decimal.Zero, false
	}
	return input.Add(output), true
}

func upstreamPriceComponent(raw string, tokens int64) (decimal.Decimal, bool) {
	if tokens == 0 {
		return decimal.Zero, true
	}
	price, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || price.IsNegative() {
		return decimal.Zero, false
	}
	return price.Mul(decimal.NewFromInt(tokens)).Div(oneMillionTokens), true
}
