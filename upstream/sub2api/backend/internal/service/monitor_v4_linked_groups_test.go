package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type linkedAvailableStub struct{ active, linked []Group }

func (s linkedAvailableStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return s.active, nil
}
func (s linkedAvailableStub) LinkedMonitorGroups(context.Context, int64) ([]Group, error) {
	return s.linked, nil
}

func TestMonitorV4VisibleGroupsIncludeDisabledLinkedIdentity(t *testing.T) {
	s := &MonitorV4Service{available: linkedAvailableStub{linked: []Group{{ID: 7, Name: "Disabled line", Status: "disabled"}}}}
	groups, err := s.withLinkedMonitorGroups(context.Background(), 42, []Group{{ID: 1, Status: StatusActive}})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "disabled", groups[1].Status)
}

func TestMonitorV4LinkedGroupsDeduplicatesExistingIdentity(t *testing.T) {
	s := &MonitorV4Service{available: linkedAvailableStub{linked: []Group{{ID: 1, Status: StatusActive}, {ID: 7, Status: "disabled"}}}}
	groups, err := s.withLinkedMonitorGroups(context.Background(), 42, []Group{{ID: 1, Status: StatusActive}})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, int64(1), groups[0].ID)
	require.Equal(t, int64(7), groups[1].ID)
}
