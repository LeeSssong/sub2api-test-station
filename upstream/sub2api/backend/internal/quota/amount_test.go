package quota

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestAmountAdapterNormalizesToEightPlaces(t *testing.T) {
	a := AmountAdapter{}
	got, err := a.Parse("1.234567895")
	require.NoError(t, err)
	require.Equal(t, "1.23456790", a.Format(got))
}

func TestAmountAdapterRejectsNonFiniteLegacyFloat(t *testing.T) {
	a := AmountAdapter{}
	_, err := a.FromLegacyFloat(math.Inf(1))
	require.Error(t, err)
}

func TestAmountAdapterFormatsCanonicalDecimal(t *testing.T) {
	a := AmountAdapter{}
	require.Equal(t, "2.00000000", a.Format(decimal.RequireFromString("2")))
}
