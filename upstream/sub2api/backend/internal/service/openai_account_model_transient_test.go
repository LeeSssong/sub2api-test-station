package service

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelTransient_FirstFailureDoesNotCreateLongBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)

	decision := state.recordFailure(35, "gpt-5.5", now)

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
	assert.False(t, state.isBlocked(35, "gpt-5.5", now))
}

func TestOpenAIModelTransient_SecondFailureCreatesShortModelBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)

	decision := state.recordFailure(35, "gpt-5.5", now.Add(time.Second))

	assert.Equal(t, 2, decision.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, decision.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(openAIModelTransientShortCooldown+2*time.Second)))
}

func TestOpenAIModelTransient_ThirdFailureCreatesFortyFiveSecondModelBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)
	state.recordFailure(35, "gpt-5.5", now.Add(time.Second))

	decision := state.recordFailure(35, "gpt-5.5", now.Add(2*time.Second))

	assert.Equal(t, 3, decision.FailureStreak)
	assert.Equal(t, 45*time.Second, decision.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(40*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(48*time.Second)))
}

func TestOpenAIModelTransient_BlockIsIsolatedByModel(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.6-terra", now)
	state.recordFailure(35, "GPT-5.6-TERRA", now.Add(time.Second))

	assert.True(t, state.isBlocked(35, "gpt-5.6-terra", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(47, "gpt-5.6-terra", now.Add(2*time.Second)))
}

func TestOpenAIModelTransient_SuccessClearsStreakAndBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)
	state.recordFailure(35, "gpt-5.5", now.Add(time.Second))
	require.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))

	state.recordSuccess(35, "gpt-5.5")

	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	decision := state.recordFailure(35, "gpt-5.5", now.Add(3*time.Second))
	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
}

func TestOpenAIModelTransient_StaleStreakExpires(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)

	decision := state.recordFailure(35, "gpt-5.5", now.Add(openAIModelTransientFailureWindow+time.Second))

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
}

func TestOpenAIModelTransient_IgnoresInvalidKeys(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)

	assert.Zero(t, state.recordFailure(0, "gpt-5.5", now).FailureStreak)
	assert.Zero(t, state.recordFailure(35, " ", now).FailureStreak)
	assert.False(t, state.isBlocked(0, "gpt-5.5", now))
	assert.False(t, state.isBlocked(35, "", now))
	assert.Equal(t, 0, state.size())
}

func TestOpenAIModelTransient_IgnoresOversizedModelKey(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	model := strings.Repeat("m", openAIModelTransientMaxModelBytes+1)

	decision := state.recordFailure(35, model, now)

	assert.Zero(t, decision.FailureStreak)
	assert.False(t, state.isBlocked(35, model, now))
	assert.Equal(t, 0, state.size())
}

func TestOpenAIModelTransient_StateIsBoundedAndConcurrencySafe(t *testing.T) {
	const maxEntries = 16
	state := newOpenAIAccountModelTransientState(maxEntries)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			model := fmt.Sprintf("gpt-test-%d", i)
			state.recordFailure(int64(i+1), model, now.Add(time.Duration(i)*time.Millisecond))
			_ = state.isBlocked(int64(i+1), model, now.Add(time.Second))
		}(i)
	}
	wg.Wait()

	assert.LessOrEqual(t, state.size(), maxEntries)
}

func TestOpenAIModelTransient_RuntimeDecisionAndHalfOpen(t *testing.T) {
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(128)}
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	base := OpenAIAccountModelFailureEvent{AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: 502, Now: now}
	d := svc.RecordOpenAIAccountModelFailure(nil, base)
	assert.Equal(t, 1, d.FailureStreak)
	assert.True(t, d.ExcludeFromRequest)
	assert.False(t, d.CurrentRequestRetry)
	d = svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: 502, SafeToReplay: true, Now: now.Add(time.Second)})
	assert.Equal(t, 2, d.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, d.Cooldown)
	assert.True(t, d.ExcludeFromRequest)
	assert.False(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", now.Add(2*time.Second)))
	expired := now.Add(openAIModelTransientShortCooldown + time.Second)
	assert.True(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", expired))
	assert.False(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", expired))
	svc.ReleaseOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", true, expired)
	assert.False(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", expired))
}

func TestOpenAIModelTransient_HardFailureDoesNotMutate(t *testing.T) {
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(128)}
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	for _, status := range []int{401, 402, 403, 404} {
		d := svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: status, ErrorType: "transient", Now: now})
		assert.True(t, d.ExcludeFromRequest)
		assert.Zero(t, d.FailureStreak)
	}
	assert.False(t, svc.isOpenAIAccountModelRuntimeBlocked(&Account{ID: 35}, "gpt-5.5"))
}

func TestOpenAIModelTransient_HalfOpenFailureExtendsCooldown(t *testing.T) {
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(128)}
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: 502, Now: now})
	svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: 502, Now: now.Add(time.Second)})
	expired := now.Add(11 * time.Second)
	require.True(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", expired))
	svc.ReleaseOpenAIAccountModelHalfOpenProbe(35, "gpt-5.5", false, expired)
	snap := svc.SnapshotOpenAIAccountModelRuntime(expired)
	require.Len(t, snap, 1)
	assert.Equal(t, expired.Add(openAIModelTransientShortCooldown), snap[0].BlockUntil)
}
