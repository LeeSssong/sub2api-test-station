package service

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

func TestRecordOpenAIAccountModelFailure_502ImmediatelyStartsShortCooldown(t *testing.T) {
	for _, status := range []int{502, 503} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
			now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)

			decision := svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{
				AccountID: 35, CanonicalModel: "gpt-5.5", StatusCode: status, ErrorType: "transient_upstream", ImmediateCooldown: true, Now: now,
			})

			require.Equal(t, openAIModelTransientShortCooldown, decision.Cooldown)
			require.True(t, decision.BlockUntil.After(now))
			require.True(t, decision.ExcludeFromRequest)
		})
	}
}

func TestOpenAIModelTransient_SecondFailureCreatesShortModelBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)

	decision := state.recordFailure(35, "gpt-5.5", now.Add(time.Second))

	assert.Equal(t, 2, decision.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, decision.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(openAIModelTransientShortCooldown+2*time.Second)), "expired cooldown remains blocked until a half-open lease is acquired")
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
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(48*time.Second)), "expired cooldown remains blocked until a half-open lease is acquired")
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

func TestRecordOpenAIIncompleteStreamFailureUsesExistingTransientAndReplayBoundary(t *testing.T) {
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}

	preOutput := svc.recordOpenAIIncompleteStreamFailure(nil, 41, "gpt-5.6-sol", false, true, false, false)
	require.True(t, preOutput.ExcludeFromRequest)
	require.True(t, preOutput.CurrentRequestRetry)

	postOutput := svc.recordOpenAIIncompleteStreamFailure(nil, 41, "gpt-5.6-sol", true, true, false, false)
	require.True(t, postOutput.ExcludeFromRequest)
	require.False(t, postOutput.CurrentRequestRetry)
	require.Equal(t, 2, postOutput.FailureStreak)
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

	decision := state.recordFailure(35, "gpt-5.5", now.Add(openAIModelTransientStreakTTL+time.Second))

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
}

// A streak must not depend on how often the gateway is called. Sparse traffic
// used to reset the streak between every request, so a broken account+model was
// never cooled down and each request paid a failed attempt plus a failover.
func TestOpenAIModelTransient_StreakSurvivesSparseTraffic(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	gap := 5 * time.Minute
	require.Greater(t, gap, openAIModelTransientLongCooldown,
		"the gap must exceed every cooldown, otherwise this passes for the wrong reason")

	first := state.recordFailure(35, "gpt-5.5", now)
	second := state.recordFailure(35, "gpt-5.5", now.Add(gap))
	third := state.recordFailure(35, "gpt-5.5", now.Add(2*gap))

	assert.Equal(t, 1, first.FailureStreak)
	assert.Zero(t, first.Cooldown)
	assert.Equal(t, 2, second.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, second.Cooldown)
	assert.Equal(t, 3, third.FailureStreak)
	assert.Equal(t, openAIModelTransientLongCooldown, third.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*gap+time.Second)))
}

// A success between two sparse failures still clears the streak, so an account
// that intermittently works is not pushed into the long cooldown.
func TestOpenAIModelTransient_SuccessResetsStreakAcrossSparseTraffic(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	gap := 5 * time.Minute

	state.recordFailure(35, "gpt-5.5", now)
	state.recordSuccess(35, "gpt-5.5")

	decision := state.recordFailure(35, "gpt-5.5", now.Add(gap))

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(gap+time.Second)))
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
	assert.Equal(t, expired, snap[0].LastFailureAt)
}

func TestOpenAIModelTransient_HalfOpenLeaseIsSingleAfterCooldownExpiry(t *testing.T) {
	svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
	now := time.Date(2026, 8, 8, 6, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{now, now.Add(time.Second)} {
		svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{AccountID: 88, CanonicalModel: "gpt-5.5", StatusCode: 502, Now: at})
	}
	expired := now.Add(11 * time.Second)
	require.True(t, svc.openaiModelTransient.isBlocked(88, "gpt-5.5", expired), "expired cooldown must stay gated until a half-open lease")

	var granted atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.AcquireOpenAIAccountModelHalfOpenProbe(88, "gpt-5.5", expired) {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), granted.Load())

	svc.ReleaseOpenAIAccountModelHalfOpenProbe(88, "gpt-5.5", false, expired)
	require.False(t, svc.AcquireOpenAIAccountModelHalfOpenProbe(88, "gpt-5.5", expired), "failed probe renews cooldown")
}
