package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorV1WindowSamplePresence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		latest []*ChannelMonitorLatest
		checks map[int]int
		rates  map[int]float64
		want   []bool
	}{
		{name: "no history", want: []bool{false, false, false}},
		{name: "latest outside 7d inside 15d and 30d", latest: []*ChannelMonitorLatest{{Model: "model", Status: "operational", CheckedAt: time.Now().Add(-10 * 24 * time.Hour)}}, checks: map[int]int{15: 1, 30: 1}, rates: map[int]float64{15: 100, 30: 100}, want: []bool{false, true, true}},
		{name: "real all-failed zero", latest: []*ChannelMonitorLatest{{Model: "model", Status: "failed"}}, checks: map[int]int{7: 3, 15: 3, 30: 3}, want: []bool{true, true, true}},
		{name: "zero-check aggregate rows", checks: map[int]int{7: 0, 15: 0, 30: 0}, want: []bool{false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			av := map[int]map[string]*ChannelMonitorAvailability{}
			for _, days := range []int{7, 15, 30} {
				av[days] = map[string]*ChannelMonitorAvailability{}
				if n, ok := tc.checks[days]; ok {
					av[days]["model"] = &ChannelMonitorAvailability{Model: "model", WindowDays: days, TotalChecks: n, AvailabilityPct: tc.rates[days]}
				}
			}
			detail := mergeModelDetails(&ChannelMonitor{PrimaryModel: "model"}, tc.latest, av)
			raw, err := json.Marshal(detail[0])
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(raw, &fields))
			for i, key := range []string{"HasSamples7d", "HasSamples15d", "HasSamples30d"} {
				require.Equal(t, tc.want[i], fields[key], key)
			}
			require.Equal(t, tc.rates[7], detail[0].Availability7d)
			latest := indexLatestByModel(tc.latest)
			summary := buildStatusSummary(latest, av[7], "model", nil)
			view := buildUserViewFromSummary(&ChannelMonitor{PrimaryModel: "model"}, summary, pickLatest(tc.latest, "model"), nil)
			raw, err = json.Marshal(view)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &fields))
			require.Equal(t, tc.want[0], fields["HasSamples7d"])
		})
	}
}
