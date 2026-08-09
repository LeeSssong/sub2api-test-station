package repository

import "strings"

// effectiveAccountCostSQL returns the final account-cost snapshot expression,
// preserving the legacy formula for rows written before account_cost existed.
func effectiveAccountCostSQL(columnPrefix string) string {
	prefix := strings.TrimSpace(columnPrefix)
	if prefix != "" && !strings.HasSuffix(prefix, ".") {
		prefix += "."
	}
	return "COALESCE(" + prefix + "account_cost, COALESCE(" + prefix + "account_stats_cost, " + prefix + "total_cost) * COALESCE(" + prefix + "account_rate_multiplier, 1))"
}
