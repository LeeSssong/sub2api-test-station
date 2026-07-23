package compare

import (
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestMaterializeKeepsSourcesSeparateAndRejectsIncompatibleWindows(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	production := NativeMetrics{SourceKind: "sub2api_native_monitor", SourceRef: "monitor:9", ModelID: "gpt-a", WindowStart: start, WindowEnd: start.Add(time.Hour), SampleCount: 60, SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 2000, CostMultiplier: 1_000}
	candidate := ProbeMetrics{SourceKind: "relay_ops_candidate_probe", SourceRef: "probe:17", ModelID: "gpt-a", WindowStart: start, WindowEnd: start.Add(time.Hour), SampleCount: 4, SuccessRate: 1, SSECompletionRate: 1, TTFTP95MS: 1500, CostMultiplier: 800}
	window, err := Materialize(production, candidate)
	if err != nil || window.Production.SourceKind == window.Candidate.SourceKind || window.SchemaVersion != MetricSchemaV1 {
		t.Fatalf("window=%#v err=%v", window, err)
	}
	candidate.ModelID = "gpt-b"
	if _, err := Materialize(production, candidate); err == nil {
		t.Fatal("incompatible models accepted")
	}
}

func TestClassifyRequiresMeaningfulImprovementWithoutQualityRegression(t *testing.T) {
	t.Parallel()
	window := ComparisonWindow{
		Production: Metrics{SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 2000, CostMultiplier: domain.MultiplierBPS(1_000)},
		Candidate:  Metrics{SuccessRate: 0.99, SSECompletionRate: 0.99, TTFTP95MS: 1500, CostMultiplier: domain.MultiplierBPS(850)},
	}
	result := Classify(window, DefaultPolicy())
	if !result.FasterTTFT || !result.Cheaper || !result.OverallBetter {
		t.Fatalf("comparison=%#v", result)
	}
	window.Candidate.SuccessRate = 0.90
	result = Classify(window, DefaultPolicy())
	if result.OverallBetter || result.MoreStable {
		t.Fatalf("regressed comparison=%#v", result)
	}
}

func TestEvaluateQualityFirstAppliesHardGatesBeforeScore(t *testing.T) {
	t.Parallel()
	evidence := QualityEvidence{
		TechnicalPassed: false, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		MaterialImprovement: true,
	}

	decision := EvaluateQualityFirst(evidence)

	if decision.Status != StatusBlocked || decision.Eligible || decision.TotalScore != 100 || !contains(decision.HardGateReasons, "technical_failure") {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestEvaluateQualityFirstRequiresQualityAndMandatoryEvidence(t *testing.T) {
	t.Parallel()
	unknown := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true,
		ReliabilityScore: 40, LatencyScore: 15, GenerationScore: 10, CapacityScore: 15,
	})
	if unknown.Status != StatusNeedsEvidence || unknown.QualityScore != 80 || unknown.Eligible ||
		!contains(unknown.MissingEvidence, "production_baseline") || !contains(unknown.MissingEvidence, "gateway_measurement") || !contains(unknown.MissingEvidence, "verified_pricing") {
		t.Fatalf("unknown=%#v", unknown)
	}

	low := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 0, PriceScore: 10,
		MaterialImprovement: true,
	})
	if low.Status != StatusNotBetter || low.QualityScore != 75 || low.Eligible {
		t.Fatalf("low=%#v", low)
	}
}

func TestEvaluateQualityFirstMakesOnlyCompleteBetterEvidenceEligible(t *testing.T) {
	t.Parallel()
	decision := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 25, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		MaterialImprovement: true,
	})
	if decision.Status != StatusEligibleForManualSwitch || decision.QualityScore != 90 || decision.TotalScore != 100 || !decision.Eligible {
		t.Fatalf("decision=%#v", decision)
	}

	regressed := EvaluateQualityFirst(QualityEvidence{
		TechnicalPassed: true, EvidenceFresh: true, BaselineKnown: true, GatewayKnown: true, PricingKnown: true,
		ReliabilityScore: 40, LatencyScore: 20, GenerationScore: 10, CapacityScore: 15, PriceScore: 10,
		LatencyRegression: true, MaterialImprovement: true,
	})
	if regressed.Status != StatusBlocked || !contains(regressed.HardGateReasons, "latency_regression") {
		t.Fatalf("regressed=%#v", regressed)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
