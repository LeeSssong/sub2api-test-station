package lab

import "testing"

func TestValidateEgressTargetAllowsOnlyLabMocks(t *testing.T) {
	if err := ValidateEgressTarget("http://admin-lab-mock-upstream:8091/v1", "admin-lab-mock-upstream", "admin-lab-mock-payment"); err != nil {
		t.Fatalf("lab mock rejected: %v", err)
	}
	for _, target := range []string{"https://api.openai.com/v1", "http://127.0.0.1:8080", "http://admin-lab-unknown:8099"} {
		if err := ValidateEgressTarget(target, "admin-lab-mock-upstream", "admin-lab-mock-payment"); err == nil {
			t.Fatalf("non-lab target accepted: %s", target)
		}
	}
}
