package service

import (
	"sort"
	"sync"
	"time"
)

const openAIFirstOutputSlowThreshold = 60 * time.Second

type openAISlowTimer interface{ Stop() bool }

type openAIFirstOutputSlowKey struct {
	groupID, accountID int64
	attemptID          string
}

type openAIFirstOutputSlowEntry struct {
	observedAt time.Time
	ttftMS     float64
	replaced   bool
}

type OpenAIFirstOutputSlowView struct {
	SlowCount         int
	TTFTLowerBoundsMS []float64
	Replaced          bool
}

type OpenAIFirstOutputSlowTracker struct {
	mu      sync.Mutex
	now     func() time.Time
	after   func(time.Duration, func()) openAISlowTimer
	onSlow  func(openAIFirstOutputSlowKey, float64)
	entries map[openAIFirstOutputSlowKey]openAIFirstOutputSlowEntry
}

type OpenAIFirstOutputObservation struct {
	tracker *OpenAIFirstOutputSlowTracker
	key     openAIFirstOutputSlowKey
	timer   openAISlowTimer
	once    sync.Once
}

type stdOpenAISlowTimer struct{ timer *time.Timer }

func (t stdOpenAISlowTimer) Stop() bool { return t.timer.Stop() }

func newOpenAIFirstOutputSlowTracker(now func() time.Time, after func(time.Duration, func()) openAISlowTimer) *OpenAIFirstOutputSlowTracker {
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = func(d time.Duration, fn func()) openAISlowTimer { return stdOpenAISlowTimer{time.AfterFunc(d, fn)} }
	}
	return &OpenAIFirstOutputSlowTracker{now: now, after: after, entries: make(map[openAIFirstOutputSlowKey]openAIFirstOutputSlowEntry)}
}

func (t *OpenAIFirstOutputSlowTracker) Start(groupID, accountID int64, attemptID string, startedAt time.Time) *OpenAIFirstOutputObservation {
	if t == nil || accountID <= 0 || attemptID == "" {
		return &OpenAIFirstOutputObservation{}
	}
	key := openAIFirstOutputSlowKey{groupID: groupID, accountID: accountID, attemptID: attemptID}
	observation := &OpenAIFirstOutputObservation{tracker: t, key: key}
	delay := openAIFirstOutputSlowThreshold - t.now().Sub(startedAt)
	if delay < 0 {
		delay = 0
	}
	observation.timer = t.after(delay, func() { t.markSlow(key) })
	return observation
}

func (t *OpenAIFirstOutputSlowTracker) markSlow(key openAIFirstOutputSlowKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupLocked(t.now())
	if _, exists := t.entries[key]; !exists {
		t.entries[key] = openAIFirstOutputSlowEntry{observedAt: t.now(), ttftMS: 60000}
		if t.onSlow != nil {
			t.onSlow(key, 60000)
		}
	}
}

func (o *OpenAIFirstOutputObservation) ObserveSemanticOutput(ttftMS int) {
	if o == nil || o.tracker == nil {
		return
	}
	if o.timer != nil {
		o.timer.Stop()
	}
	o.tracker.mu.Lock()
	defer o.tracker.mu.Unlock()
	if entry, ok := o.tracker.entries[o.key]; ok {
		if ttftMS > 0 {
			entry.ttftMS = float64(ttftMS)
		}
		entry.replaced = true
		o.tracker.entries[o.key] = entry
	}
}

func (o *OpenAIFirstOutputObservation) ObserveFailure(keepUnknown bool) {
	if o == nil || o.tracker == nil {
		return
	}
	if o.timer != nil {
		o.timer.Stop()
	}
	if keepUnknown {
		return
	}
	o.tracker.mu.Lock()
	delete(o.tracker.entries, o.key)
	o.tracker.mu.Unlock()
}

func (t *OpenAIFirstOutputSlowTracker) View(groupID, accountID int64) OpenAIFirstOutputSlowView {
	if t == nil {
		return OpenAIFirstOutputSlowView{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupLocked(t.now())
	view := OpenAIFirstOutputSlowView{}
	for key, entry := range t.entries {
		if key.groupID != groupID || key.accountID != accountID {
			continue
		}
		view.SlowCount++
		view.TTFTLowerBoundsMS = append(view.TTFTLowerBoundsMS, entry.ttftMS)
		view.Replaced = view.Replaced || entry.replaced
	}
	sort.Float64s(view.TTFTLowerBoundsMS)
	return view
}

func (t *OpenAIFirstOutputSlowTracker) viewAccount(accountID int64) OpenAIFirstOutputSlowView {
	if t == nil {
		return OpenAIFirstOutputSlowView{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupLocked(t.now())
	view := OpenAIFirstOutputSlowView{}
	for key, entry := range t.entries {
		if key.accountID != accountID {
			continue
		}
		view.SlowCount++
		view.TTFTLowerBoundsMS = append(view.TTFTLowerBoundsMS, entry.ttftMS)
		view.Replaced = view.Replaced || entry.replaced
	}
	sort.Float64s(view.TTFTLowerBoundsMS)
	return view
}

func (t *OpenAIFirstOutputSlowTracker) cleanupLocked(now time.Time) {
	for key, entry := range t.entries {
		if now.Sub(entry.observedAt) >= time.Hour {
			delete(t.entries, key)
		}
	}
}
