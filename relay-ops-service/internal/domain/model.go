package domain

import (
	"fmt"
	"strconv"
	"strings"
)

type MicroUSD int64
type MultiplierBPS int64
type UpstreamID int64

type AdminActor struct {
	UserID int64
}

func ParseMicroUSD(value string) (MicroUSD, error) {
	parsed, err := parseFixed(value, 6)
	return MicroUSD(parsed), err
}

func ParseMultiplierBPS(value string) (MultiplierBPS, error) {
	parsed, err := parseFixed(value, 4)
	return MultiplierBPS(parsed), err
}

func parseFixed(value string, precision int) (int64, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "+-eE") {
		return 0, fmt.Errorf("invalid decimal")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
		return 0, fmt.Errorf("invalid decimal")
	}
	if len(parts) == 2 && len(parts[1]) > precision {
		return 0, fmt.Errorf("too many decimal places")
	}
	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return 0, fmt.Errorf("invalid decimal")
			}
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", precision-len(fraction))
	raw := parts[0] + fraction
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decimal out of range")
	}
	return parsed, nil
}
