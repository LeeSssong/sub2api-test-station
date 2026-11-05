package handler

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitorV1SamplePresenceDTO(t *testing.T) {
	for _, value := range []bool{false, true} {
		// Fill via JSON so this test can fail on the pre-field implementation without a compilation error.
		var detail service.UserMonitorDetail
		raw, err := json.Marshal(map[string]any{"Models": []any{map[string]any{"Model": "model", "HasSamples7d": false, "HasSamples15d": value, "HasSamples30d": value}}})
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &detail))
		raw, err = json.Marshal(userMonitorDetailToResponse(&detail))
		require.NoError(t, err)
		var dto struct {
			Models []map[string]any `json:"models"`
		}
		require.NoError(t, json.Unmarshal(raw, &dto))
		require.Equal(t, false, dto.Models[0]["has_samples_7d"])
		require.Equal(t, value, dto.Models[0]["has_samples_15d"])
		require.Equal(t, value, dto.Models[0]["has_samples_30d"])
		var view service.UserMonitorView
		require.NoError(t, json.Unmarshal([]byte(`{"HasSamples7d":true}`), &view))
		raw, err = json.Marshal(userMonitorViewToItem(&view, false))
		require.NoError(t, err)
		require.Contains(t, string(raw), `"has_samples_7d":true`)
	}
}
