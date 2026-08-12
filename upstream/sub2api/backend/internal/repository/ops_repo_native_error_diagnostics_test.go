package repository

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsErrorListSelectsCanonicalBusinessLimitFlag(t *testing.T) {
	src, err := os.ReadFile("ops_repo.go")
	require.NoError(t, err)
	text := string(src)
	listStart := strings.Index(text, "func (r *opsRepository) ListErrorLogs")
	detailStart := strings.Index(text, "func (r *opsRepository) GetErrorLogByID")
	require.Greater(t, listStart, -1)
	require.Greater(t, detailStart, listStart)
	listSource := text[listStart:detailStart]
	require.Contains(t, listSource, "e.is_business_limited")
	require.Contains(t, listSource, "&item.IsBusinessLimited")
}
