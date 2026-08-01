package accounthealth

import (
	"math"
	"sort"
	"time"
)

const (
	historyWindowSeconds  = 48 * 60 * 60
	historyLimitSlack     = 1.2
	historyLimitMin       = 100
	historyLimitMax       = 2000
	rollingWindowSeconds  = 60 * 60
	rollingLimitSlack     = 1.5
	rollingLimitMin       = 12
	rollingLimitMax       = 200
	defaultIntervalSecond = 300
	statusSuccess         = "success"
	dateLayout            = "2006-01-02"
)

type HistoryEntry struct {
	CheckedAt time.Time
	Status    string
	ErrorCode string
	TTFTMS    *float64
}

type DaySlice struct {
	Date          string
	SampleCount   int
	SuccessCount  int
	SuccessRate   float64
	TTFTP95MS     *float64
	LatestStatus  string
	LastErrorCode string
}

// HistoryLimitFor sizes the history page so that a full 48-hour window is
// covered at the current probe interval. Hard-coding the count would silently
// drop yesterday's samples once an administrator shortens the interval.
func HistoryLimitFor(intervalSeconds int) int {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultIntervalSecond
	}
	needed := int(math.Ceil(float64(historyWindowSeconds) / float64(intervalSeconds)))
	limit := int(float64(needed) * historyLimitSlack)
	if limit < historyLimitMin {
		return historyLimitMin
	}
	if limit > historyLimitMax {
		return historyLimitMax
	}
	return limit
}

// RollingWindowLimitFor sizes the history page for the one-hour alert window
// (ceil(3600/interval) with 1.5x slack; 300-second interval -> 18 entries).
// HistoryLimitFor must not be reused here: it covers the 48-hour daily-report
// window (692 entries at a 300-second interval), roughly 40x the data the
// rolling window needs on every 5-minute job round.
// The clamps matter: without an upper bound a one-second probe interval asks
// for 5400 entries, blows past the client's response ceiling, and every
// account silently falls back to the cumulative window — reviving the very
// staleness this window was added to remove.
func RollingWindowLimitFor(intervalSeconds int) int {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultIntervalSecond
	}
	needed := int(math.Ceil(float64(rollingWindowSeconds) / float64(intervalSeconds)))
	limit := int(math.Ceil(float64(needed) * rollingLimitSlack))
	if limit < rollingLimitMin {
		return rollingLimitMin
	}
	if limit > rollingLimitMax {
		return rollingLimitMax
	}
	return limit
}

// Aggregate summarizes the entries whose CheckedAt falls in the half-open
// window [from, to). It is the shared primitive behind SliceByDay's calendar
// days and the alert job's rolling hour. The returned Date carries the date of
// `from` in its own location; callers slicing arbitrary windows may ignore it.
// When multiple entries share CheckedAt, callers must provide the newest entry
// first. This matches Sub2API's checked_at DESC, id DESC history contract.
func Aggregate(entries []HistoryEntry, from, to time.Time) DaySlice {
	window := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.CheckedAt.Before(from) && entry.CheckedAt.Before(to) {
			window = append(window, entry)
		}
	}
	return summarize(from.Format(dateLayout), window)
}

// AccountSampleFrom turns an aggregated window into the classifier's input so
// the daily report and the availability job judge accounts by the same rules.
func AccountSampleFrom(slice DaySlice, id int64, name string, groups []string) AccountSample {
	return AccountSample{
		AccountID:   id,
		Name:        name,
		GroupNames:  groups,
		SuccessRate: slice.SuccessRate,
		SampleCount: slice.SampleCount,
		TTFTP95MS:   slice.TTFTP95MS,
		ErrorCode:   slice.LastErrorCode,
	}
}

func SliceByDay(entries []HistoryEntry, loc *time.Location, now time.Time) (DaySlice, DaySlice) {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	today := Aggregate(entries, todayStart, todayStart.AddDate(0, 0, 1))
	yesterday := Aggregate(entries, todayStart.AddDate(0, 0, -1), todayStart)
	return today, yesterday
}

func summarize(date string, entries []HistoryEntry) DaySlice {
	slice := DaySlice{Date: date, SampleCount: len(entries)}
	if len(entries) == 0 {
		return slice
	}
	ttfts := make([]float64, 0, len(entries))
	latestIndex := -1
	for index, entry := range entries {
		if entry.Status == statusSuccess {
			slice.SuccessCount++
			if entry.TTFTMS != nil {
				ttfts = append(ttfts, *entry.TTFTMS)
			}
		}
		if latestIndex == -1 || entry.CheckedAt.After(entries[latestIndex].CheckedAt) {
			latestIndex = index
		}
	}
	if latestIndex >= 0 {
		latest := entries[latestIndex]
		slice.LatestStatus = latest.Status
		if latest.Status != statusSuccess {
			slice.LastErrorCode = latest.ErrorCode
		}
	}
	slice.SuccessRate = float64(slice.SuccessCount) / float64(slice.SampleCount)
	slice.TTFTP95MS = Percentile(ttfts, 0.95)
	return slice
}

// Percentile returns the q-quantile (nearest-rank) of values, or nil when
// values is empty. Exported so report code reuses one implementation instead
// of growing ad-hoc sorts.
func Percentile(values []float64, q float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	value := sorted[index]
	return &value
}
