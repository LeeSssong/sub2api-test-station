package controlplane

import (
	"context"
	"encoding/json"
	"time"
)

type Freshness struct {
	GeneratedAt        time.Time `json:"generated_at"`
	SourceWatermark    string    `json:"source_watermark"`
	FreshnessSeconds   int64     `json:"freshness_seconds"`
	Completeness       string    `json:"completeness"`
	CalculationVersion string    `json:"calculation_version"`
}
type ReadModel struct {
	Items          any       `json:"items"`
	Total          int       `json:"total"`
	Freshness      Freshness `json:"freshness"`
	Degraded       bool      `json:"degraded"`
	DegradedReason string    `json:"degraded_reason,omitempty"`
}

func (m ReadModel) MarshalJSON() ([]byte, error) {
	type raw ReadModel
	return json.Marshal(struct {
		raw
		GeneratedAt        time.Time `json:"generated_at"`
		SourceWatermark    string    `json:"source_watermark"`
		FreshnessSeconds   int64     `json:"freshness_seconds"`
		Completeness       string    `json:"completeness"`
		CalculationVersion string    `json:"calculation_version"`
	}{raw: raw(m), GeneratedAt: m.Freshness.GeneratedAt, SourceWatermark: m.Freshness.SourceWatermark, FreshnessSeconds: m.Freshness.FreshnessSeconds, Completeness: m.Freshness.Completeness, CalculationVersion: m.Freshness.CalculationVersion})
}

type Reader interface {
	Read(context.Context, string, map[string]string) (ReadModel, error)
}
type Refresher interface {
	RefreshAccount(context.Context, int64, string) error
}
