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

func SliceByDay(entries []HistoryEntry, loc *time.Location, now time.Time) (DaySlice, DaySlice) {
	if loc == nil {
		loc = time.UTC
	}
	todayDate := now.In(loc).Format(dateLayout)
	yesterdayDate := now.In(loc).AddDate(0, 0, -1).Format(dateLayout)

	buckets := map[string][]HistoryEntry{}
	for _, entry := range entries {
		date := entry.CheckedAt.In(loc).Format(dateLayout)
		if date == todayDate || date == yesterdayDate {
			buckets[date] = append(buckets[date], entry)
		}
	}
	return summarize(todayDate, buckets[todayDate]), summarize(yesterdayDate, buckets[yesterdayDate])
}

func summarize(date string, entries []HistoryEntry) DaySlice {
	slice := DaySlice{Date: date, SampleCount: len(entries)}
	if len(entries) == 0 {
		return slice
	}
	ordered := append([]HistoryEntry(nil), entries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CheckedAt.Before(ordered[j].CheckedAt)
	})

	ttfts := make([]float64, 0, len(ordered))
	for _, entry := range ordered {
		if entry.Status == statusSuccess {
			slice.SuccessCount++
			if entry.TTFTMS != nil {
				ttfts = append(ttfts, *entry.TTFTMS)
			}
			continue
		}
		if entry.ErrorCode != "" {
			slice.LastErrorCode = entry.ErrorCode
		}
	}
	slice.SuccessRate = float64(slice.SuccessCount) / float64(slice.SampleCount)
	slice.TTFTP95MS = percentile(ttfts, 0.95)
	return slice
}

func percentile(values []float64, q float64) *float64 {
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
