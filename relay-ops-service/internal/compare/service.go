package compare

import (
	"fmt"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

const MetricSchemaV1 = "relay-ops-comparison-v1"

type Metrics struct {
	SourceKind        string
	SourceRef         string
	ModelID           string
	WindowStart       time.Time
	WindowEnd         time.Time
	SampleCount       int64
	SuccessRate       float64
	SSECompletionRate float64
	TTFTP95MS         float64
	LatencyP95MS      float64
	Rate429           float64
	Rate5XX           float64
	TimeoutRate       float64
	CostMultiplier    domain.MultiplierBPS
}

type NativeMetrics Metrics
type ProbeMetrics Metrics

type ComparisonWindow struct {
	SchemaVersion string
	ModelID       string
	WindowStart   time.Time
	WindowEnd     time.Time
	Production    Metrics
	Candidate     Metrics
}

func Materialize(production NativeMetrics, candidate ProbeMetrics) (ComparisonWindow, error) {
	left := Metrics(production)
	right := Metrics(candidate)
	if left.ModelID == "" || left.ModelID != right.ModelID {
		return ComparisonWindow{}, fmt.Errorf("comparison models are incompatible")
	}
	if left.SourceKind == "" || right.SourceKind == "" || left.SourceKind == right.SourceKind {
		return ComparisonWindow{}, fmt.Errorf("comparison sources must remain distinct")
	}
	start := latest(left.WindowStart, right.WindowStart)
	end := earliest(left.WindowEnd, right.WindowEnd)
	if !end.After(start) {
		return ComparisonWindow{}, fmt.Errorf("comparison windows do not overlap")
	}
	if left.SampleCount <= 0 || right.SampleCount <= 0 {
		return ComparisonWindow{}, fmt.Errorf("comparison requires samples")
	}
	return ComparisonWindow{SchemaVersion: MetricSchemaV1, ModelID: left.ModelID, WindowStart: start.UTC(), WindowEnd: end.UTC(), Production: left, Candidate: right}, nil
}

type Policy struct {
	TTFTImprovementRate float64
	CostImprovementRate float64
	QualityTolerance    float64
}

func DefaultPolicy() Policy {
	return Policy{TTFTImprovementRate: 0.20, CostImprovementRate: 0.10, QualityTolerance: 0.005}
}

type CandidateComparison struct {
	MoreStable    bool
	FasterTTFT    bool
	Cheaper       bool
	OverallBetter bool
	Status        string
}

func Classify(window ComparisonWindow, policy Policy) CandidateComparison {
	production := window.Production
	candidate := window.Candidate
	qualityOK := candidate.SuccessRate+policy.QualityTolerance >= production.SuccessRate &&
		candidate.SSECompletionRate+policy.QualityTolerance >= production.SSECompletionRate &&
		candidate.Rate429 <= production.Rate429+policy.QualityTolerance &&
		candidate.Rate5XX <= production.Rate5XX+policy.QualityTolerance &&
		candidate.TimeoutRate <= production.TimeoutRate+policy.QualityTolerance
	moreStable := qualityOK && (candidate.SuccessRate > production.SuccessRate || candidate.SSECompletionRate > production.SSECompletionRate || candidate.Rate429 < production.Rate429 || candidate.Rate5XX < production.Rate5XX || candidate.TimeoutRate < production.TimeoutRate)
	faster := production.TTFTP95MS > 0 && candidate.TTFTP95MS <= production.TTFTP95MS*(1-policy.TTFTImprovementRate)
	cheaper := production.CostMultiplier > 0 && float64(candidate.CostMultiplier) <= float64(production.CostMultiplier)*(1-policy.CostImprovementRate)
	overall := qualityOK && (faster || cheaper)
	status := "not_better"
	if overall {
		status = "consider_recheck"
	} else if !qualityOK {
		status = "quality_regression"
	}
	return CandidateComparison{MoreStable: moreStable, FasterTTFT: faster, Cheaper: cheaper, OverallBetter: overall, Status: status}
}

func latest(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
func earliest(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}
