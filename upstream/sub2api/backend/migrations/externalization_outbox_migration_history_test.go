package migrations

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalizationOutboxMigration200MatchesProductionChecksum(t *testing.T) {
	content, err := FS.ReadFile("200_externalization_outbox.sql")
	require.NoError(t, err)

	checksum := sha256.Sum256(content)
	require.Equal(t,
		"e4e2b329f9c0a1cedfd1a87fb1d945082da9cc2248afbcb4ebe4872ff03cd9d2",
		fmt.Sprintf("%x", checksum),
		"migration 200 was applied to production by commit 3fb79f5291961a99a50d13b3306937a8db156b04 and must remain byte-for-byte stable",
	)
}

func TestExternalizationOutboxClaimTokenMigrationIsSingleIdempotentExpand(t *testing.T) {
	content, err := FS.ReadFile("202_externalization_outbox_claim_token.sql")
	require.NoError(t, err)

	sql := strings.TrimSpace(string(content))
	require.Equal(t,
		"ALTER TABLE externalization_outbox ADD COLUMN IF NOT EXISTS claim_token TEXT;",
		sql,
	)
}
