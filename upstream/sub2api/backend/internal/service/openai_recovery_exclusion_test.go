package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIRecoveryExclusionIsTenantGroupScopedAndConsumedAfterContinuation(t *testing.T) {
	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	groupID := int64(12)
	otherGroupID := int64(13)
	scope := OpenAIRecoveryScope{TenantID: "api-key:71", GroupID: &groupID, SessionKey: "session-continue-1"}
	otherTenantScope := OpenAIRecoveryScope{TenantID: "api-key:72", GroupID: &groupID, SessionKey: "session-continue-1"}
	otherGroupScope := OpenAIRecoveryScope{TenantID: "api-key:71", GroupID: &otherGroupID, SessionKey: "session-continue-1"}

	svc.RecordOpenAIRecoveryFailedAccount(scope, 91, "gpt-5.5", now)

	excluded := svc.OpenAIRecoveryExcludedAccountIDs(scope, now.Add(time.Second))
	require.Contains(t, excluded, int64(91))
	require.Empty(t, svc.OpenAIRecoveryExcludedAccountIDs(otherTenantScope, now.Add(time.Second)))
	require.Empty(t, svc.OpenAIRecoveryExcludedAccountIDs(otherGroupScope, now.Add(time.Second)))

	svc.ConsumeOpenAIRecoveryExcludedAccounts(scope)
	require.Empty(t, svc.OpenAIRecoveryExcludedAccountIDs(scope, now.Add(2*time.Second)))
}
