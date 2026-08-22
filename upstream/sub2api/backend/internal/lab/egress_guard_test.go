package lab

import "testing"

func TestValidateEgressTargetAllowsOnlyLabMocks(t *testing.T) {
	if err := ValidateEgressTarget("http://admin-lab-mock-upstream:8091/v1", "admin-lab-mock-upstream", "admin-lab-mock-payment"); err != nil {
		t.Fatalf("lab mock rejected: %v", err)
	}
	for _, target := range []string{"https://api.openai.com/v1", "http://127.0.0.1:8080", "http://admin-lab-unknown:8099", "http://admin-lab-mock-upstream.evil.invalid/v1", "http://user:password@admin-lab-mock-upstream:8091/v1"} {
		if err := ValidateEgressTarget(target, "admin-lab-mock-upstream", "admin-lab-mock-payment"); err == nil {
			t.Fatalf("non-lab target accepted: %s", target)
		}
	}
}

func TestValidateEgressTargetRejectsAllowedPublicIPAndInvalidURL(t *testing.T) {
	for _, target := range []string{"http://8.8.8.8:8091/v1", "http://admin-lab-mock-upstream:%38%30%39%31/v1", "http:///missing-host"} {
		if err := ValidateEgressTarget(target, "admin-lab-mock-upstream", "8.8.8.8"); err == nil {
			t.Fatalf("unsafe target accepted: %s", target)
		}
	}
}
