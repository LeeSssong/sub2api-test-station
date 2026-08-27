package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountMonitorProbePromptIsUnique(t *testing.T) {
	first := accountMonitorProbePrompt()
	second := accountMonitorProbePrompt()
	require.NotEqual(t, first, second)
	require.True(t, strings.HasPrefix(first, "ping "))
	require.Len(t, strings.TrimPrefix(first, "ping "), 36)
}
