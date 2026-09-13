//go:build unit

package service

import "testing"

func TestNormalizeAdminGiftDeductionReason(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: defaultAdminGiftDeductionReason},
		{name: "whitespace", in: " \t\n ", want: defaultAdminGiftDeductionReason},
		{name: "trims explicit reason", in: "  manual cleanup  ", want: "manual cleanup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAdminGiftDeductionReason(tt.in); got != tt.want {
				t.Fatalf("normalizeAdminGiftDeductionReason(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
