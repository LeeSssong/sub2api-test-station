package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorGroupReaderStub struct {
	groups map[int64]*Group
}

func (s *channelMonitorGroupReaderStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	group := s.groups[id]
	if group == nil {
		return nil, ErrGroupNotFound
	}
	cloned := *group
	return &cloned, nil
}

type channelMonitorGroupUpdateRepoStub struct {
	ChannelMonitorRepository
	monitor *ChannelMonitor
	updated *ChannelMonitor
}

func (s *channelMonitorGroupUpdateRepoStub) GetByID(_ context.Context, _ int64) (*ChannelMonitor, error) {
	cloned := *s.monitor
	cloned.GroupID = cloneInt64Pointer(s.monitor.GroupID)
	return &cloned, nil
}

func (s *channelMonitorGroupUpdateRepoStub) Update(_ context.Context, monitor *ChannelMonitor) error {
	cloned := *monitor
	cloned.GroupID = cloneInt64Pointer(monitor.GroupID)
	s.updated = &cloned
	return nil
}

type channelMonitorGroupEncryptorStub struct{}

func (channelMonitorGroupEncryptorStub) Encrypt(value string) (string, error) {
	return value, nil
}

func (channelMonitorGroupEncryptorStub) Decrypt(value string) (string, error) {
	return value, nil
}

func TestChannelMonitorUpdateRejectsProviderGroupMismatch(t *testing.T) {
	groupID := int64(16)
	repo := &channelMonitorGroupUpdateRepoStub{monitor: &ChannelMonitor{
		ID:              13,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeResponses,
		PrimaryModel:    "gpt-5.6-sol",
		GroupName:       "GPT-PLUS-内测",
		GroupID:         &groupID,
		IntervalSeconds: 60,
	}}
	groups := &channelMonitorGroupReaderStub{groups: map[int64]*Group{
		groupID: {
			ID:       groupID,
			Name:     "GPT-PLUS-内测",
			Platform: MonitorProviderOpenAI,
			Status:   StatusActive,
		},
	}}
	svc := NewChannelMonitorService(repo, channelMonitorGroupEncryptorStub{}, groups)
	anthropic := MonitorProviderAnthropic

	_, err := svc.Update(context.Background(), 13, ChannelMonitorUpdateParams{Provider: &anthropic})

	require.ErrorIs(t, err, ErrChannelMonitorInvalidGroup)
	require.Nil(t, repo.updated)
}

func TestChannelMonitorUpdateCanonicalizesStableGroupName(t *testing.T) {
	groupID := int64(16)
	wrongName := "legacy alias"
	repo := &channelMonitorGroupUpdateRepoStub{monitor: &ChannelMonitor{
		ID:              13,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeResponses,
		PrimaryModel:    "gpt-5.6-sol",
		IntervalSeconds: 60,
	}}
	groups := &channelMonitorGroupReaderStub{groups: map[int64]*Group{
		groupID: {
			ID:       groupID,
			Name:     "GPT-PLUS-内测",
			Platform: MonitorProviderOpenAI,
			Status:   StatusActive,
		},
	}}
	svc := NewChannelMonitorService(repo, channelMonitorGroupEncryptorStub{}, groups)

	_, err := svc.Update(context.Background(), 13, ChannelMonitorUpdateParams{
		GroupID:   &groupID,
		GroupName: &wrongName,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, &groupID, repo.updated.GroupID)
	require.Equal(t, "GPT-PLUS-内测", repo.updated.GroupName)
}

func TestApplyMonitorUpdateClearGroupClearsLegacyNameAtomically(t *testing.T) {
	groupID := int64(16)
	existing := &ChannelMonitor{
		GroupID:   &groupID,
		GroupName: "GPT-PLUS-内测",
	}

	err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{ClearGroup: true})

	require.NoError(t, err)
	require.Nil(t, existing.GroupID)
	require.Empty(t, existing.GroupName)
}
