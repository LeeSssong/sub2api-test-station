package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelProbesRejectNonV1RuntimeModes(t *testing.T) {
	for _, mode := range []string{ChannelMonitorModeV2, ChannelMonitorModeV3, ChannelMonitorModeNativeProbe, ChannelMonitorModeHybridPerformance} {
		t.Run(mode, func(t *testing.T) {
			svc := NewChannelMonitorService(nil, nil)
			svc.SetRuntimeReader(retiredProbeRuntime{mode: mode})
			results, err := svc.RunCheck(context.Background(), 1)
			require.ErrorIs(t, err, ErrChannelMonitorActiveProbesRetired)
			require.Nil(t, results)
		})
	}
}

type retiredProbeRuntime struct{ mode string }

func (r retiredProbeRuntime) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return ChannelMonitorRuntime{Enabled: true, Mode: r.mode}
}
