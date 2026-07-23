package qualityreports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.invalid/relay-ops-service/internal/compare"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/probes"
)

var ErrStale = errors.New("quality report is stale or mismatched")

type Report struct {
	ReportID     string
	ReportHash   string
	UpstreamID   domain.UpstreamID
	UpstreamName string
	JobKind      string
	Status       string
	QualityScore int
	TotalScore   int
	Direct       string
	Gateway      string
	Models       string
	Pricing      string
	Capacity     string
	Record       json.RawMessage
	RecordedAt   time.Time
	ExpiresAt    time.Time
}

type Repository interface {
	Get(context.Context, string) (Report, bool, error)
	Put(context.Context, Report) error
}

type Preview struct {
	ReportID   string
	ReportHash string
	Status     string
	Writes     int
	Summary    string
}

type Service struct {
	Repository Repository
	Clock      func() time.Time
}

func Build(upstreamID domain.UpstreamID, upstreamName string, result probes.FastResult) (Report, error) {
	if upstreamID <= 0 || result.RunID == "" || result.RecordedAt.IsZero() || !json.Valid(result.Record) {
		return Report{}, fmt.Errorf("fast result is incomplete")
	}
	var record struct {
		Metrics struct {
			SelectedModels []string `json:"selected_models"`
			Direct         struct {
				RequestCount int     `json:"request_count"`
				SuccessCount int     `json:"success_count"`
				SuccessRate  float64 `json:"success_rate"`
				Latency      struct {
					P95MS float64 `json:"p95_ms"`
				} `json:"latency"`
				TTFT struct {
					P95MS float64 `json:"p95_ms"`
				} `json:"ttft"`
			} `json:"direct"`
			Gateway struct {
				Status string `json:"status"`
			} `json:"gateway"`
			Capacity json.RawMessage `json:"capacity"`
		} `json:"metrics"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(result.Record, &record); err != nil {
		return Report{}, fmt.Errorf("decode fast result: %w", err)
	}
	technicalPassed := result.Status == "passed" && len(record.Errors) == 0 &&
		record.Metrics.Direct.RequestCount > 0 && record.Metrics.Direct.SuccessCount == record.Metrics.Direct.RequestCount
	reliabilityScore := 0
	generationScore := 0
	capacityScore := 0
	if technicalPassed {
		reliabilityScore = 40
		generationScore = 10
		if result.JobKind == "capacity_check" && len(record.Metrics.Capacity) > 0 && string(record.Metrics.Capacity) != "null" {
			capacityScore = 15
		}
	}
	decision := compare.EvaluateQualityFirst(compare.QualityEvidence{
		TechnicalPassed: technicalPassed, EvidenceFresh: true,
		BaselineKnown: false, GatewayKnown: record.Metrics.Gateway.Status == "passed", PricingKnown: false,
		ReliabilityScore: reliabilityScore, GenerationScore: generationScore, CapacityScore: capacityScore,
	})
	digest := sha256.Sum256(result.Record)
	ttl := map[string]time.Duration{
		"health_pulse": 30 * time.Minute, "catalog_quick": 12 * time.Hour, "capacity_check": 36 * time.Hour,
	}[result.JobKind]
	if ttl == 0 {
		return Report{}, fmt.Errorf("fast job kind is invalid")
	}
	gateway := record.Metrics.Gateway.Status
	if gateway == "" {
		gateway = "unknown"
	}
	return Report{
		ReportID: result.RunID, ReportHash: hex.EncodeToString(digest[:]), UpstreamID: upstreamID, UpstreamName: upstreamName,
		JobKind: result.JobKind, Status: decision.Status, QualityScore: decision.QualityScore, TotalScore: decision.TotalScore,
		Direct:  fmt.Sprintf("%d/%d, TTFT P95 %.0fms, total P95 %.0fms", record.Metrics.Direct.SuccessCount, record.Metrics.Direct.RequestCount, record.Metrics.Direct.TTFT.P95MS, record.Metrics.Direct.Latency.P95MS),
		Gateway: gateway, Models: fmt.Sprintf("%d selected", len(record.Metrics.SelectedModels)), Pricing: "unknown",
		Capacity: capacityLabel(result.JobKind, record.Metrics.Capacity), Record: append(json.RawMessage(nil), result.Record...),
		RecordedAt: result.RecordedAt.UTC(), ExpiresAt: result.RecordedAt.UTC().Add(ttl),
	}, nil
}

func (s Service) Preview(ctx context.Context, actor domain.AdminActor, reportID, reportHash string) (Preview, error) {
	if s.Repository == nil || actor.UserID <= 0 || reportID == "" || reportHash == "" {
		return Preview{}, ErrStale
	}
	report, found, err := s.Repository.Get(ctx, reportID)
	if err != nil {
		return Preview{}, err
	}
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock().UTC()
	}
	if !found || report.ReportHash != reportHash || !now.Before(report.ExpiresAt) {
		return Preview{}, ErrStale
	}
	return Preview{
		ReportID: report.ReportID, ReportHash: report.ReportHash, Status: "dry_run", Writes: 0,
		Summary: fmt.Sprintf("%s; no production writes", report.Status),
	}, nil
}

func capacityLabel(jobKind string, raw json.RawMessage) string {
	if jobKind != "capacity_check" || len(raw) == 0 || string(raw) == "null" {
		return "unknown"
	}
	return "bounded lower limit recorded"
}
