package controlplane

import (
	"context"
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
type Reader interface {
	Read(string, map[string]string) (ReadModel, error)
}
type Refresher interface {
	RefreshAccount(context.Context, int64, string) error
}
