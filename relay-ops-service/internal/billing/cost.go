package billing

import (
	"fmt"
	"math"

	"example.invalid/relay-ops-service/internal/domain"
)

func EstimateEffectiveMultiplier(standard, actual domain.MicroUSD) (domain.MultiplierBPS, error) {
	if standard <= 0 || actual < 0 {
		return 0, fmt.Errorf("cost values are invalid")
	}
	if int64(actual) > math.MaxInt64/10_000 {
		return 0, fmt.Errorf("cost values are out of range")
	}
	result := (int64(actual)*10_000 + int64(standard)/2) / int64(standard)
	return domain.MultiplierBPS(result), nil
}
