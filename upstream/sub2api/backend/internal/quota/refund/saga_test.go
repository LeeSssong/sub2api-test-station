package refund

import "testing"

func TestTransitionAllowsRetryableProviderStates(t *testing.T) {
	cases := [][2]string{{"pending", "unknown"}, {"unknown", "reconciling"}, {"reconciling", "pending"}, {"pending", "completed"}}
	for _, c := range cases {
		if err := ValidateTransition(c[0], c[1]); err != nil {
			t.Fatalf("%s -> %s: %v", c[0], c[1], err)
		}
	}
}

func TestTransitionRejectsFailedForRetryableProviderError(t *testing.T) {
	if err := ValidateTransition("pending", "failed"); err == nil {
		t.Fatal("pending must not transition directly to failed")
	}
}

func TestTransitionAllowsFailedOnlyFromExplicitTerminalStates(t *testing.T) {
	if err := ValidateTransition("not_started", "failed"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition("completed", "pending"); err == nil {
		t.Fatal("completed must be terminal")
	}
}
