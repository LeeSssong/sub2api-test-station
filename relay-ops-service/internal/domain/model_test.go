package domain

import "testing"

func TestParseMicroUSDUsesExactDecimalArithmetic(t *testing.T) {
	t.Parallel()

	tests := map[string]MicroUSD{
		"0":        0,
		"0.000001": 1,
		"1.25":     1_250_000,
		"120":      120_000_000,
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMicroUSD(input)
			if err != nil {
				t.Fatalf("ParseMicroUSD(%q): %v", input, err)
			}
			if got != want {
				t.Fatalf("ParseMicroUSD(%q) = %d, want %d", input, got, want)
			}
		})
	}
}

func TestParseMicroUSDRejectsInvalidPrecisionAndSigns(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "-1", "+1", "1.0000001", "NaN", "1e3"} {
		if _, err := ParseMicroUSD(input); err == nil {
			t.Fatalf("ParseMicroUSD(%q) unexpectedly succeeded", input)
		}
	}
}

func TestParseMultiplierBPS(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]MultiplierBPS{"0.05": 500, "0.1": 1000, "1": 10_000} {
		got, err := ParseMultiplierBPS(input)
		if err != nil {
			t.Fatalf("ParseMultiplierBPS(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseMultiplierBPS(%q) = %d, want %d", input, got, want)
		}
	}
}
