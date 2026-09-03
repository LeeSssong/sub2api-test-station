package quota

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

const amountScale int32 = 8

// AmountAdapter is the single conversion boundary for new quota accounting.
type AmountAdapter struct{}

func (AmountAdapter) Parse(value string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid decimal amount: %w", err)
	}
	return normalize(d)
}

func (AmountAdapter) FromLegacyFloat(value float64) (decimal.Decimal, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero, fmt.Errorf("legacy amount is not finite")
	}
	return normalize(decimal.NewFromFloat(value))
}

func (AmountAdapter) Normalize(value decimal.Decimal) (decimal.Decimal, error) {
	return normalize(value)
}

func (AmountAdapter) Format(value decimal.Decimal) string {
	return value.Round(amountScale).StringFixed(amountScale)
}

func normalize(value decimal.Decimal) (decimal.Decimal, error) {
	return value.Round(amountScale), nil
}
