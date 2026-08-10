package compare

import "testing"

func TestCompareReportRequiresAllGates(t *testing.T) {
	r := CompareReport{
		Passed:           true,
		Permission:       CheckEvidence{Passed: true, EvidenceRef: "permission:1"},
		Export:           CheckEvidence{Passed: true, EvidenceRef: "export:1"},
		Rollback:         CheckEvidence{Passed: true, EvidenceRef: "rollback:1"},
		Freshness:        FreshnessComparison{Passed: true},
		ContractComplete: true,
	}
	if !r.Eligible() {
		t.Fatal("complete report should pass")
	}
	r.Degraded.Degraded = true
	if r.Eligible() {
		t.Fatal("degraded report should fail")
	}
}
