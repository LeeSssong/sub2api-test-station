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
