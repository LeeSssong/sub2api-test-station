package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeGroupToolIDs(t *testing.T) {
	ids, err := NormalizeGroupToolIDs([]string{"codex", "claude", "codex"})
	require.NoError(t, err)
	require.Equal(t, []string{"claude", "codex"}, ids)
	_, err = NormalizeGroupToolIDs([]string{"OpenAI"})
	require.Error(t, err)
	ids, err = NormalizeGroupToolIDs(nil)
	require.NoError(t, err)
	require.NotNil(t, ids)
}

func TestNormalizeGroupToolIDsGemini(t *testing.T) {
	ids, err := NormalizeGroupToolIDs([]string{"gemini", "codex", "gemini", "deepseek", "grok", "claude"})
	require.NoError(t, err)
	require.Equal(t, []string{"claude", "codex", "deepseek", "gemini", "grok"}, ids)
	_, err = NormalizeGroupToolIDs([]string{"gemini", "unknown"})
	require.Error(t, err)
}
