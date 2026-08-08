package projection

import "time"

type Profitability struct {
	Metadata Metadata `json:"metadata"`
	Revenue  string   `json:"revenue"`
	Cost     string   `json:"cost"`
	Profit   string   `json:"profit"`
	Margin   string   `json:"margin"`
}

func NewProfitability() Profitability {
	return Profitability{Metadata: Metadata{CalculationVersion: "profitability-v1", Completeness: "empty", GeneratedAt: time.Time{}}}
}
