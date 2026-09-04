package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type manualSlowTimer struct {
	fn      func()
	stopped bool
}

func TestFinishOpenAIFirstOutputSlowObservationRecordsClientAbandonmentOnlyAfterThreshold(t *testing.T) {
	now := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	var timer *manualSlowTimer
	tracker := newOpenAIFirstOutputSlowTracker(func() time.Time { return now }, func(_ time.Duration, fn func()) openAISlowTimer {
		timer = &manualSlowTimer{fn: fn}
		return timer
	})
	tracker.onSlow = func(key openAIFirstOutputSlowKey, ttftMS float64) {
		RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{Name: OpenAIEventFirstOutputSlow, AccountID: key.accountID, AttemptID: key.attemptID, UpstreamTTFTMs: int64(ttftMS)})
	}
	svc := &OpenAIGatewayService{openaiFirstOutputSlow: tracker}
	ctx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{AttemptID: "attempt-abandoned", AttemptNumber: 1, AccountID: 42, CanonicalModel: "gpt-5.5"})
	ctx = svc.BeginOpenAIFirstOutputSlowObservation(ctx, 6, 42, "attempt-abandoned", now)
	openAIResilienceEventLedger.Lock()
	openAIResilienceEventLedger.events = nil
	openAIResilienceEventLedger.Unlock()
	svc.FinishOpenAIFirstOutputSlowObservation(ctx, false, true, false)
	require.Empty(t, openAIResilienceEventsForWindow(time.Time{}, time.Time{}, "", nil))

	ctx = svc.BeginOpenAIFirstOutputSlowObservation(ctx, 6, 42, "attempt-abandoned", now)
	timer.Fire()
	svc.FinishOpenAIFirstOutputSlowObservation(ctx, false, true, true)
	events := openAIResilienceEventsForWindow(time.Time{}, time.Time{}, "", nil)
	require.Len(t, events, 2)
	require.Equal(t, OpenAIEventFirstOutputSlow, events[0].Name)
	require.Equal(t, OpenAIEventClientAbandonedAfterUpstreamWait, events[1].Name)
	require.Equal(t, "right_censored", events[1].Outcome)
}

func (t *manualSlowTimer) Stop() bool { t.stopped = true; return true }
func (t *manualSlowTimer) Fire() {
	if !t.stopped {
		t.fn()
	}
}

func TestOpenAIFirstOutputSlowTrackerReplacesAndRemovesEvidence(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	var timers []*manualSlowTimer
	tracker := newOpenAIFirstOutputSlowTracker(func() time.Time { return now }, func(_ time.Duration, fn func()) openAISlowTimer {
		timer := &manualSlowTimer{fn: fn}
		timers = append(timers, timer)
		return timer
	})

	first := tracker.Start(6, 42, "attempt-1", now)
	require.Len(t, timers, 1)
	timers[0].Fire()
	view := tracker.View(6, 42)
	require.Equal(t, 1, view.SlowCount)
	require.Equal(t, float64(60000), view.TTFTLowerBoundsMS[0])

	first.ObserveSemanticOutput(61000)
	view = tracker.View(6, 42)
	require.Equal(t, 1, view.SlowCount)
	require.Equal(t, float64(61000), view.TTFTLowerBoundsMS[0])
	require.True(t, view.Replaced)

	second := tracker.Start(6, 42, "attempt-2", now)
	timers[1].Fire()
	second.ObserveFailure(false)
	view = tracker.View(6, 42)
	require.Equal(t, 1, view.SlowCount)
}

func TestOpenAIFirstOutputSlowTrackerEarlyOutputAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	var timer *manualSlowTimer
	tracker := newOpenAIFirstOutputSlowTracker(func() time.Time { return now }, func(_ time.Duration, fn func()) openAISlowTimer {
		timer = &manualSlowTimer{fn: fn}
		return timer
	})
	observation := tracker.Start(6, 42, "attempt-1", now)
	observation.ObserveSemanticOutput(1200)
	timer.Fire()
	require.Zero(t, tracker.View(6, 42).SlowCount)

	observation = tracker.Start(6, 42, "attempt-2", now)
	timer.Fire()
	observation.ObserveFailure(true)
	require.Equal(t, 1, tracker.View(6, 42).SlowCount)
	now = now.Add(time.Hour + time.Second)
	require.Zero(t, tracker.View(6, 42).SlowCount)
}
