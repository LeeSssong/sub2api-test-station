package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateChallengeIncludesUniqueNonce(t *testing.T) {
	first := generateChallenge()
	second := generateChallenge()
	require.NotEqual(t, first.Prompt, second.Prompt)
	require.True(t, validateChallenge(first.Expected, first.Expected))
	require.True(t, validateChallenge(second.Expected, second.Expected))
}
