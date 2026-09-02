package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type manualSlowTimer struct {
	fn      func()
	stopped bool
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
