package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountModelAdminActionsExposeAndOnlyClearTransientState(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}

	_, err := svc.ImmediatelyCooldownAccountModel(context.Background(), 71, "gpt-5.5", 30*time.Second, now)
	require.NoError(t, err)
	runtime := svc.SnapshotOpenAIAccountModelRuntime(now)
	require.Len(t, runtime, 1)
	require.Equal(t, "cooldown", runtime[0].State)
	require.Equal(t, 71, int(runtime[0].AccountID))
	require.Equal(t, "gpt-5.5", runtime[0].CanonicalModel)
	require.False(t, runtime[0].LastFailureAt.IsZero())
	require.Equal(t, http.StatusServiceUnavailable, runtime[0].LastStatusCode)
	require.Equal(t, "admin_immediate_cooldown", runtime[0].LastErrorType)

	require.NoError(t, svc.RestoreAccountModelScheduling(context.Background(), 71, "gpt-5.5"))
	require.Empty(t, svc.SnapshotOpenAIAccountModelRuntime(now))
}

func TestOpenAIAccountModelAdminProbeAcquiresAndReleasesHalfOpenLease(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	_, err := svc.ImmediatelyCooldownAccountModel(context.Background(), 72, "gpt-5.5", time.Second, now)
	require.NoError(t, err)

	probed, err := svc.ProbeAccountModelOnce(context.Background(), 72, "gpt-5.5", now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, probed)
	runtime := svc.SnapshotOpenAIAccountModelRuntime(now.Add(2 * time.Second))
	require.Len(t, runtime, 1)
	require.True(t, runtime[0].HalfOpenInFlight)
	probed, err = svc.ProbeAccountModelOnce(context.Background(), 72, "gpt-5.5", now.Add(2*time.Second))
	require.NoError(t, err)
	require.False(t, probed)
	svc.ReleaseOpenAIAccountModelHalfOpenProbe(72, "gpt-5.5", true, now.Add(3*time.Second))
	// Recovery is hysteretic: one successful probe keeps the account in
	// half-open until a second independent success confirms stability.
	require.Len(t, svc.SnapshotOpenAIAccountModelRuntime(now.Add(3*time.Second)), 1)
	require.True(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(72, "gpt-5.5", now.Add(4*time.Second)))
	svc.ReleaseOpenAIAccountModelHalfOpenProbe(72, "gpt-5.5", true, now.Add(5*time.Second))
	require.Empty(t, svc.SnapshotOpenAIAccountModelRuntime(now.Add(5*time.Second)))
}

func TestOpenAIAccountModelAdminCooldownSurvivesFailureStreakWindow(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	_, err := svc.ImmediatelyCooldownAccountModel(context.Background(), 73, "gpt-5.5", 5*time.Minute, now)
	require.NoError(t, err)
	require.True(t, svc.openaiModelTransient.isBlocked(73, "gpt-5.5", now.Add(2*time.Minute)))
	runtime := svc.SnapshotOpenAIAccountModelRuntime(now.Add(2 * time.Minute))
	require.Len(t, runtime, 1)
	require.Equal(t, "cooldown", runtime[0].State)
}
