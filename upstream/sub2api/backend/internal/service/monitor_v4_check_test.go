package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorV4CheckStatusNeverInventsTimeout(t *testing.T) {
	require.Equal(t, "failed", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "failed", ErrorCode: "account_test_error"}))
	require.Equal(t, "timeout", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "failed", ErrorCode: "timeout"}))
	slow := 15001.0
	require.Equal(t, "timeout", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "success", TTFTMS: &slow}))
	require.Equal(t, "failed", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "success"}))
	fast := 80.0
	require.Equal(t, "success", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "success", TTFTMS: &fast}))
}
func TestMonitorV4CheckAdmissionBoundsConcurrencyAndCooldown(t *testing.T) {
	s := &MonitorV4Service{}
	now := time.Now()
	require.NoError(t, s.beginCheck(1, now))
	require.Error(t, s.beginCheck(1, now))
	s.finishCheck(1)
	require.Error(t, s.beginCheck(1, now.Add(time.Second)))
	require.NoError(t, s.beginCheck(1, now.Add(time.Minute)))
}

type checkAvailableStub struct{ active, linked []Group }

func (s checkAvailableStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return s.active, nil
}
func (s checkAvailableStub) LinkedMonitorGroups(context.Context, int64) ([]Group, error) {
	return s.linked, nil
}

type checkNativeStub struct {
	monitorV4NativeReaderStub
	calls []int64
}

func (s *checkNativeStub) CheckMonitorGroup(_ context.Context, id int64) (AccountMonitorProbeResult, error) {
	s.calls = append(s.calls, id)
	v := 80.
	return AccountMonitorProbeResult{Status: "success", TTFTMS: &v, CheckedAt: time.Now()}, nil
}
func TestMonitorV4CheckRejectsForeignGroupsBeforeProbing(t *testing.T) {
	native := &checkNativeStub{}
	s := NewMonitorV4Service(nil, checkAvailableStub{active: []Group{{ID: 7, Status: StatusActive}}}, native, nil, nil)
	_, err := s.Check(context.Background(), 42, []int64{7, 99})
	require.Error(t, err)
	require.Empty(t, native.calls)
}
func TestMonitorV4CheckSkipsDisabledLinkedGroup(t *testing.T) {
	native := &checkNativeStub{}
	s := NewMonitorV4Service(nil, checkAvailableStub{linked: []Group{{ID: 7, Status: "disabled"}}}, native, nil, nil)
	results, err := s.Check(context.Background(), 42, []int64{7})
	require.NoError(t, err)
	require.Equal(t, "disabled", results[0].Status)
	require.Empty(t, native.calls)
}
func TestMonitorV4CheckDoesNotCallTotalTimeoutFirstTokenTimeout(t *testing.T) {
	fast := 80.
	require.Equal(t, "failed", monitorV4CheckStatus(AccountMonitorProbeResult{Status: "failed", ErrorCode: "timeout", TTFTMS: &fast}))
}
func TestManualCheckOnlyProbesRequestedGroup(t *testing.T) {
	repo := &manualGroupRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, GroupIDs: []int64{9}},
		{ID: 2, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}},
	}}
	s := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
	var ids []int64
	s.probeConnection = func(ctx context.Context, id int64, model, prompt, mode string) (AccountMonitorProbeResult, error) {
		ids = append(ids, id)
		require.Equal(t, 15*time.Second, ctx.Value(accountMonitorFirstTokenTimeoutKey{}))
		fast := 80.
		return AccountMonitorProbeResult{Status: "success", TTFTMS: &fast}, nil
	}
	_, err := s.CheckMonitorGroup(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, ids)
	require.Len(t, repo.results, 1)
	require.Equal(t, []int64{7}, repo.groupIDs)
}

func TestMonitorV4VisibleGroupsIncludeDisabledLinkedIdentity(t *testing.T) {
	s := &MonitorV4Service{available: checkAvailableStub{linked: []Group{{ID: 7, Name: "Disabled line", Status: "disabled"}}}}
	groups, err := s.withLinkedMonitorGroups(context.Background(), 42, []Group{{ID: 1, Status: StatusActive}})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "disabled", groups[1].Status)
}

type manualGroupRepoStub struct {
	accountMonitorRepoStub
	groupIDs []int64
}

func (s *manualGroupRepoStub) InsertGroupProbeResult(ctx context.Context, id int64, result AccountMonitorProbeResult, run string) error {
	s.groupIDs = append(s.groupIDs, id)
	return s.InsertResult(ctx, result, run)
}

func TestManualCheckRejectsModelOutsideGroupAllowlist(t *testing.T) {
	repo := &manualGroupRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{ID: 2, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, Platform: PlatformOpenAI}}}
	s := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
	called := false
	s.probeConnection = func(context.Context, int64, string, string, string) (AccountMonitorProbeResult, error) {
		called = true
		return AccountMonitorProbeResult{}, nil
	}
	group := Group{ID: 7, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"not-a-real-model"}}}
	_, err := s.CheckMonitorGroup(context.WithValue(context.Background(), monitorV4ProbeGroupKey{}, group), 7)
	require.Error(t, err)
	require.False(t, called)
}

func TestCancelledCheckNeverLabelsAnActiveLineDisabled(t *testing.T) {
	for i := 0; i < 20; i++ {
		native := &checkNativeStub{}
		s := NewMonitorV4Service(nil, checkAvailableStub{active: []Group{{ID: 7, Status: StatusActive}}}, native, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		results, err := s.Check(ctx, 42, []int64{7})
		require.NoError(t, err)
		require.Equal(t, "failed", results[0].Status)
		require.Empty(t, native.calls)
	}
}

func TestCurrentOperationalExpiresAndHonorsManagementState(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Minute)
	withinJitterBudget := now.Add(-6 * time.Minute)
	stale := now.Add(-8 * time.Minute)
	require.True(t, monitorV4CurrentOperational(MonitorV4Group{Status: StatusActive, CurrentOperational: true, SourceUpdatedAt: &fresh}, now))
	require.True(t, monitorV4CurrentOperational(MonitorV4Group{Status: StatusActive, CurrentOperational: true, SourceUpdatedAt: &withinJitterBudget}, now))
	require.False(t, monitorV4CurrentOperational(MonitorV4Group{Status: StatusActive, CurrentOperational: true, SourceUpdatedAt: &stale}, now))
	require.False(t, monitorV4CurrentOperational(MonitorV4Group{Status: "inactive", CurrentOperational: true, SourceUpdatedAt: &fresh}, now))
	require.False(t, monitorV4CurrentOperational(MonitorV4Group{Status: StatusActive, CurrentOperational: true}, now))
}

type mappedMonitorGroupRepo struct{ monitorV4GroupRepoStub }

func (s *mappedMonitorGroupRepo) ReadGroupToolMappings(_ context.Context, ids []int64) (map[int64]GroupToolMapping, error) {
	return map[int64]GroupToolMapping{9: {GroupID: 9, ToolIDs: []string{"codex"}}}, nil
}
func (s *mappedMonitorGroupRepo) ReplaceGroupToolMapping(context.Context, int64, []string, int64) (GroupToolMapping, error) {
	return GroupToolMapping{}, nil
}
func TestRefreshIncludesExplicitToolMappedGroups(t *testing.T) {
	repo := &mappedMonitorGroupRepo{monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}, {ID: 9, Status: StatusActive}}}}
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{}}
	store := &monitorV4RefreshStoreStub{}
	s := NewMonitorV4Service(repo, checkAvailableStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	s.SetSnapshotStore(store)
	require.NoError(t, s.RefreshMonitorV4Snapshots(context.Background(), time.Now()))
	require.Equal(t, []int64{7, 9}, native.groupIDs)
}
func TestMappingReturnedEvenWhenGroupHasNoObservations(t *testing.T) {
	repo := &mappedMonitorGroupRepo{}
	s := &MonitorV4Service{groupRepo: repo}
	snapshot, err := s.snapshotWithGroups(context.Background(), MonitorV4Window1H, time.Now(), time.Now().Add(-time.Hour), []Group{{ID: 9, Status: StatusActive}}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"codex"}, snapshot.Groups[0].ToolIDs)
	require.Nil(t, snapshot.Groups[0].TTFTP50MS)
	require.Nil(t, snapshot.Groups[0].SuccessRate)
}
