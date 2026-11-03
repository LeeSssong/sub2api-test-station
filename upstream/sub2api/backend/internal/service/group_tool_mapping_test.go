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

func TestResolveGroupToolsDefaultsAndExplicitLegacyExceptions(t *testing.T) {
	for platform, tool := range map[string]string{"openai": "codex", "anthropic": "claude", "gemini": "gemini", "deepseek": "deepseek", "grok": "grok"} {
		ids, source := ResolveGroupTools(platform, nil)
		require.Equal(t, []string{tool}, ids)
		require.Equal(t, "platform", source)
		ids, source = ResolveGroupTools(platform, []string{tool})
		require.Equal(t, []string{tool}, ids)
		require.Equal(t, "platform", source)
	}
	ids, source := ResolveGroupTools("openai", []string{"claude", "gemini"})
	require.Equal(t, []string{"claude", "gemini"}, ids)
	require.Equal(t, "legacy_exception", source)
	ids, source = ResolveGroupTools("composite", []string{"codex"})
	require.Equal(t, []string{"codex"}, ids)
	require.Equal(t, "legacy_exception", source)
	ids, source = ResolveGroupTools("composite", nil)
	require.Empty(t, ids)
	require.Equal(t, "unclassified", source)
}
