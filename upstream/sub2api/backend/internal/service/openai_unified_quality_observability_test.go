package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordOpenAISchedulerSelectionProjectsUnifiedQualityDecision(t *testing.T) {
	openAIResilienceEventLedger.Lock()
	previous := append([]OpenAIResilienceEvent(nil), openAIResilienceEventLedger.events...)
	openAIResilienceEventLedger.events = nil
	openAIResilienceEventLedger.Unlock()
	t.Cleanup(func() {
		openAIResilienceEventLedger.Lock()
		openAIResilienceEventLedger.events = previous
		openAIResilienceEventLedger.Unlock()
	})

	groupID := int64(11)
	windowEnd := time.Now().UTC()
	ctx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{
		LogicalRequestID: "logical-1",
		AttemptID:        "logical-1:1",
		AttemptNumber:    1,
		AccountID:        42,
		CanonicalModel:   "gpt-5.4",
	})

	RecordOpenAISchedulerSelection(ctx, PlatformOpenAI, &groupID, OpenAIAccountScheduleDecision{
		SelectionLayer:       openAIAccountScheduleLayerUnifiedQuality,
		SelectedAccountID:    42,
		SelectedRank:         2,
		UnifiedQuality:       true,
		ImageIntent:          false,
		CandidateAccountIDs:  []int64{7, 42, 99},
		ExcludedAccountIDs:   []int64{3},
		QualityWindowEnd:     windowEnd,
		QualitySnapshotStale: true,
		ProfitMode:           "native",
		ProfitBypass:         true,
		ProfitBypassReason:   "margin_below",
		ExtraRetryCount:      2,
		ExtraUsed:            1,
		SwitchCount:          1,
		SafeToReplay:         true,
		SwitchAllowed:        true,
		SwitchBlockReason:    "",
		StopReason:           "",
		NativeSlotWaitMs:     12,
		RoutingMs:            34,
		UpstreamTTFTMs:       56,
		TotalMs:              78,
	})

	events := openAIResilienceEventsForWindow(windowEnd.Add(-time.Minute), windowEnd.Add(time.Minute), PlatformOpenAI, &groupID)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "logical-1", event.CorrelationID)
	require.Equal(t, "logical-1:1", event.AttemptID)
	require.Equal(t, int64(42), event.SelectedAccountID)
	require.Equal(t, 2, event.SelectedRank)
	require.True(t, event.UnifiedQuality)
	require.Equal(t, windowEnd, event.QualityWindowEnd)
	require.True(t, event.QualitySnapshotStale)
	require.Equal(t, "margin_below", event.ProfitBypassReason)
	require.Equal(t, 2, event.ExtraRetryCount)
	require.Equal(t, 1, event.ExtraUsed)
	require.Equal(t, int64(12), event.NativeSlotWaitMs)
	require.Equal(t, int64(78), event.TotalMs)
}
