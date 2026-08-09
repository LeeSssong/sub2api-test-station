package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIRecoveryExclusionCarriesFailedAccountModelAcrossLogicalRequest(t *testing.T) {
	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	svc.RecordOpenAIRecoveryFailedAccount("session-continue-1", 91, "gpt-5.5", now)

	excluded := svc.OpenAIRecoveryExcludedAccountIDs("session-continue-1", now.Add(time.Second))
	require.Contains(t, excluded, int64(91))
	require.Empty(t, svc.OpenAIRecoveryExcludedAccountIDs("another-session", now.Add(time.Second)))
}
