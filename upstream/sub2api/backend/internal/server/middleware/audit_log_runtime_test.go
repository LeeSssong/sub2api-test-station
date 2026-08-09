package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditSensitiveReadsIncludesAccountModelRuntime(t *testing.T) {
	require.Equal(t, "admin.account_model_runtime.read", auditSensitiveReads["GET /api/v1/admin/account-monitors/runtime"])
}
