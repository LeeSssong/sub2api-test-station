package compare

import "testing"

func TestCompareReportRequiresAllGates(t *testing.T) {
	r := CompareReport{Passed: true, PermissionPassed: true, ExportPassed: true, RollbackPassed: true}
	if !r.Eligible() {
		t.Fatal("complete report should pass")
	}
	r.Degraded = true
	if r.Eligible() {
		t.Fatal("degraded report should fail")
	}
}
