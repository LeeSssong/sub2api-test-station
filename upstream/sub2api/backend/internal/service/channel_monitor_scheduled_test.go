package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type scheduledChannelMonitorRepoStub struct {
	ChannelMonitorRepository
	monitor      *ChannelMonitor
	historyCalls int
	marked       int
}

func (s *scheduledChannelMonitorRepoStub) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	copy := *s.monitor
	copy.GroupID = cloneInt64Pointer(s.monitor.GroupID)
	copy.ExtraModels = append([]string(nil), s.monitor.ExtraModels...)
	return &copy, nil
}

func (s *scheduledChannelMonitorRepoStub) InsertHistoryBatch(context.Context, []*ChannelMonitorHistoryRow) error {
	s.historyCalls++
	return nil
}

func (s *scheduledChannelMonitorRepoStub) MarkChecked(context.Context, int64, time.Time) error {
	s.marked++
	return nil
}

type scheduledChannelUsageStub struct {
	used bool
	err  error
}

type scheduledChannelRuntimeStub struct{ runtime ChannelMonitorRuntime }

func (s scheduledChannelRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return s.runtime
}

func (s scheduledChannelUsageStub) HasAccountUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	return false, nil
}

func (s scheduledChannelUsageStub) HasGroupUsageInWindow(context.Context, int64, time.Time, time.Time) (bool, error) {
	return s.used, s.err
}

func TestGenerateChallengeIncludesUniqueNonce(t *testing.T) {
	first := generateChallenge()
	second := generateChallenge()
	require.NotEqual(t, first.Prompt, second.Prompt)
	require.True(t, validateChallenge(first.Expected, first.Expected))
	require.True(t, validateChallenge(second.Expected, second.Expected))
}

func TestRunScheduledCheckSkipsGroupWithCurrentBucketUsageWithoutHistory(t *testing.T) {
	groupID := int64(9)
	repo := &scheduledChannelMonitorRepoStub{monitor: &ChannelMonitor{
		ID: 7, Provider: MonitorProviderOpenAI, APIMode: MonitorAPIModeChatCompletions,
		Endpoint: "https://api.example.com", APIKey: "encrypted:key", PrimaryModel: "gpt-5.6-sol", GroupID: &groupID,
	}}
	svc := NewChannelMonitorService(repo, historyBoundaryEncryptor{})
	svc.SetRuntimeReader(scheduledChannelRuntimeStub{runtime: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	svc.SetActiveProbeUsageReader(scheduledChannelUsageStub{used: true})
	svc.now = func() time.Time { return time.Date(2026, 8, 27, 6, 3, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }

	results, err := svc.RunScheduledCheck(context.Background(), 7)
	require.ErrorIs(t, err, ErrChannelMonitorProbeSkipped)
	require.Nil(t, results)
	require.Zero(t, repo.historyCalls)
	require.Zero(t, repo.marked)
}

func TestRunScheduledCheckSkipsWithoutGroupAssociation(t *testing.T) {
	repo := &scheduledChannelMonitorRepoStub{monitor: &ChannelMonitor{ID: 7, Provider: MonitorProviderOpenAI, APIKey: "encrypted:key", PrimaryModel: "gpt-5.6-sol"}}
	svc := NewChannelMonitorService(repo, historyBoundaryEncryptor{})
	svc.SetRuntimeReader(scheduledChannelRuntimeStub{runtime: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	svc.SetActiveProbeUsageReader(scheduledChannelUsageStub{})

	results, err := svc.RunScheduledCheck(context.Background(), 7)
	require.ErrorIs(t, err, ErrChannelMonitorProbeSkipped)
	require.Nil(t, results)
	require.Zero(t, repo.historyCalls)
}
