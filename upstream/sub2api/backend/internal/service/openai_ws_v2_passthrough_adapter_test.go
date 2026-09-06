package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeOpenAIWSPassthroughFailoverUsageMarksAttemptUnsafeToReplay(t *testing.T) {
	t.Parallel()

	failoverErr := &UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	mergeOpenAIWSPassthroughFailoverUsage(failoverErr, true)

	require.True(t, failoverErr.UsageKnown)
	require.True(t, failoverErr.UnsafeToReplay)
}

func TestMergeOpenAIWSPassthroughFailoverUsagePreservesSafeAttempt(t *testing.T) {
	t.Parallel()

	failoverErr := &UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	mergeOpenAIWSPassthroughFailoverUsage(failoverErr, false)

	require.False(t, failoverErr.UsageKnown)
	require.False(t, failoverErr.UnsafeToReplay)
}
