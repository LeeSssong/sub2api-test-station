package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulerLogWriterStub struct {
	inputs []OpenAISchedulerLogInsert
}

func (s *schedulerLogWriterStub) BatchInsertOpenAISchedulerLogs(_ context.Context, inputs []OpenAISchedulerLogInsert) (int, error) {
	s.inputs = append(s.inputs, inputs...)
	return len(inputs), nil
}

func TestOpenAISchedulerLogSinkEnqueueCopiesEventWithoutBlocking(t *testing.T) {
	sink := NewOpenAISchedulerLogSink(1)
	event := OpenAIResilienceEvent{
		Name:                OpenAIEventSchedulerSelection,
		CandidateAccountIDs: []int64{1, 2},
		ExcludeReasons:      map[string]int{"cooldown": 1},
	}
	require.True(t, sink.Enqueue(event))
	event.CandidateAccountIDs[0] = 99
	event.ExcludeReasons["cooldown"] = 99
	require.False(t, sink.Enqueue(event))

	queued := <-sink.queue
	require.Equal(t, []int64{1, 2}, queued.CandidateAccountIDs)
	require.Equal(t, 1, queued.ExcludeReasons["cooldown"])
	require.EqualValues(t, 1, sink.Health().DroppedCount)
}

func TestOpenAISchedulerLogSinkRejectsNonSchedulerEvents(t *testing.T) {
	sink := NewOpenAISchedulerLogSink(1)
	require.False(t, sink.Enqueue(OpenAIResilienceEvent{Name: OpenAIEventFirstOutputSlow}))
	require.Equal(t, 0, len(sink.queue))
}

func TestOpenAISchedulerLogRetentionCutoffIsSevenDays(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	require.Equal(t, now.Add(-7*24*time.Hour), OpenAISchedulerLogRetentionCutoff(now))
}

func TestOpenAISchedulerLogSinkFlushesEventAsControlledDecisionSnapshot(t *testing.T) {
	writer := &schedulerLogWriterStub{}
	sink := NewOpenAISchedulerLogSinkWithWriter(writer, 2)
	sink.Enqueue(OpenAIResilienceEvent{
		At: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC), Name: OpenAIEventSchedulerSelection,
		CorrelationID: "logical-1", AttemptID: "attempt-1", AttemptNumber: 1, AccountID: 7,
		CandidateAccountIDs: []int64{7, 8}, ExcludeReasons: map[string]int{"cooldown": 2},
	})
	require.NoError(t, sink.Flush(context.Background()))
	require.Len(t, writer.inputs, 1)
	require.Equal(t, "logical-1", writer.inputs[0].LogicalRequestID)
	require.Equal(t, "openai-unified-quality-v1", writer.inputs[0].AlgorithmVersion)
	require.Contains(t, writer.inputs[0].DecisionJSON, "candidate_account_ids")
}
