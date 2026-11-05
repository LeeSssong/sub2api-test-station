package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitorV1StableGroupIdentity(t *testing.T) {
	id := int64(7)
	raw, err := json.Marshal(userMonitorViewToItem(&service.UserMonitorView{ID: 1, GroupID: &id, GroupName: "duplicate"}, false))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"group_id":7`)
	raw, err = json.Marshal(userMonitorDetailToResponse(&service.UserMonitorDetail{ID: 1, GroupID: &id}))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"group_id":7`)
}

// A tool view must intersect real mappings and account permissions, including public components.
func TestChannelMonitorV3ToolScope(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			h, access, repo := monitorV3HandlerFixtureWithRepo()
			access.groups = []service.Group{{ID: 4, Platform: "openai"}, {ID: 8, Platform: "composite"}, {ID: 10, Platform: "openai"}}
			access.mappings = map[int64]service.GroupToolMapping{8: {GroupID: 8, ToolIDs: []string{"codex", "claude"}}, 10: {GroupID: 10, ToolIDs: []string{"claude"}}}
			cat := int64(1)
			repo.components = append(repo.components,
				service.ChannelMonitorV3Component{ID: 3, CategoryID: &cat, Name: "Combined", GroupID: 8, Enabled: true},
				service.ChannelMonitorV3Component{ID: 4, CategoryID: &cat, Name: "Legacy exception", GroupID: 10, Enabled: true},
				service.ChannelMonitorV3Component{ID: 5, CategoryID: &cat, Name: "Public outside account", GroupID: 99, Enabled: true, Visibility: service.ChannelMonitorV3VisibilityPublic})
			c, recorder := monitorV3Request("/channel-monitor-v3/status?tool_id=codex", role, "")
			h.Status(c)
			require.Equal(t, 200, recorder.Code, recorder.Body.String())
			result := decodeMonitorV3Status(t, recorder)
			require.Len(t, result.Categories, 1)
			require.Len(t, result.Categories[0].Components, 2)
			require.Equal(t, []string{"Mine", "Combined"}, []string{result.Categories[0].Components[0].Name, result.Categories[0].Components[1].Name})
		})
	}
}

func TestChannelMonitorV3ToolScopeEmptyAndInvalid(t *testing.T) {
	h, access := monitorV3HandlerFixture()
	access.groups = nil
	c, r := monitorV3Request("/status?tool_id=codex", service.RoleUser, "")
	h.Status(c)
	require.Equal(t, 200, r.Code)
	require.Empty(t, decodeMonitorV3Status(t, r).Categories)
	c, r = monitorV3Request("/status?tool_id=all", service.RoleUser, "")
	h.Status(c)
	require.Equal(t, 400, r.Code)
	c, r = monitorV3Request("/status?tool_id=codex", service.RoleAdmin, "")
	c.Keys = nil
	h.Status(c)
	require.Equal(t, 401, r.Code)
	access.err = fmt.Errorf("authorization unavailable")
	c, r = monitorV3Request("/status?tool_id=codex", service.RoleAdmin, "")
	h.Status(c)
	require.NotEqual(t, 200, r.Code)
}

type toolScopeV1Repo struct {
	service.ChannelMonitorRepository
	monitors    []*service.ChannelMonitor
	aggregated  [][]int64
	detailReads []int64
}

func (r *toolScopeV1Repo) ListEnabled(context.Context) ([]*service.ChannelMonitor, error) {
	return r.monitors, nil
}
func (r *toolScopeV1Repo) GetByID(_ context.Context, id int64) (*service.ChannelMonitor, error) {
	for _, m := range r.monitors {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, service.ErrChannelMonitorNotFound
}
func (r *toolScopeV1Repo) ListLatestForMonitorIDs(_ context.Context, ids []int64) (map[int64][]*service.ChannelMonitorLatest, error) {
	r.aggregated = append(r.aggregated, append([]int64(nil), ids...))
	return nil, nil
}
func (r *toolScopeV1Repo) ComputeAvailabilityForMonitors(_ context.Context, ids []int64, _ int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	r.aggregated = append(r.aggregated, append([]int64(nil), ids...))
	return nil, nil
}
func (r *toolScopeV1Repo) ListRecentHistoryForMonitors(_ context.Context, ids []int64, _ map[int64]string, _ int) (map[int64][]*service.ChannelMonitorHistoryEntry, error) {
	r.aggregated = append(r.aggregated, append([]int64(nil), ids...))
	return nil, nil
}
func (r *toolScopeV1Repo) ListLatestPerModel(_ context.Context, id int64) ([]*service.ChannelMonitorLatest, error) {
	r.detailReads = append(r.detailReads, id)
	return nil, nil
}
func (r *toolScopeV1Repo) ComputeAvailability(context.Context, int64, int) ([]*service.ChannelMonitorAvailability, error) {
	return nil, nil
}

func TestChannelMonitorV1ToolListAndDetailScope(t *testing.T) {
	a, b := int64(4), int64(9)
	repo := &toolScopeV1Repo{monitors: []*service.ChannelMonitor{
		{ID: 1, GroupID: &a, GroupName: "same name", Name: "allowed", Enabled: true},
		{ID: 2, GroupID: &b, GroupName: "same name", Name: "forbidden", Enabled: true},
		{ID: 3, GroupName: "same name", Name: "unbound legacy", Enabled: true},
	}}
	access := &monitorV3AccessStub{groups: []service.Group{{ID: 4, Platform: "openai"}}}
	h := &ChannelMonitorUserHandler{monitorService: service.NewChannelMonitorService(repo, nil), access: access}
	c, r := monitorV3Request("/channel-monitors?tool_id=codex", service.RoleUser, "")
	h.List(c)
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), `"group_id":4`)
	require.NotContains(t, r.Body.String(), "forbidden")
	require.NotContains(t, r.Body.String(), "unbound legacy")
	for _, ids := range repo.aggregated {
		require.Equal(t, []int64{1}, ids, "filter before reading histories")
	}
	c, r = monitorV3Request("/channel-monitors/2/status?tool_id=codex", service.RoleUser, "")
	c.Params = gin.Params{{Key: "id", Value: "2"}}
	h.GetStatus(c)
	require.Equal(t, 404, r.Code)
	require.Empty(t, repo.detailReads)
	c, r = monitorV3Request("/channel-monitors/1/status?tool_id=codex", service.RoleUser, "")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.GetStatus(c)
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), `"group_id":4`)
	access.groups = nil
	repo.aggregated = nil
	c, r = monitorV3Request("/channel-monitors?tool_id=codex", service.RoleUser, "")
	h.List(c)
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), `"items":[]`)
	require.Empty(t, repo.aggregated)
	c, r = monitorV3Request("/channel-monitors", service.RoleUser, "")
	h.List(c)
	require.Contains(t, r.Body.String(), "unbound legacy", "unscoped native page keeps its prior behavior")
}

func TestChannelMonitorV3ScopedIncidentsAndUnclassified(t *testing.T) {
	h, access, repo := monitorV3HandlerFixtureWithRepo()
	access.groups = []service.Group{{ID: 4, Platform: "openai"}}
	repo.components[1].Visibility = service.ChannelMonitorV3VisibilityPublic
	c, r := monitorV3Request("/incidents?tool_id=codex", service.RoleUser, "")
	h.Incidents(c)
	require.Equal(t, 200, r.Code)
	require.NotContains(t, r.Body.String(), "Exclusive")
	// Native rule: unclassified components stay out of categories; a configured featured component is handled by native service.
	repo.components[0].CategoryID = nil
	c, r = monitorV3Request("/status?tool_id=codex", service.RoleUser, "")
	h.Status(c)
	result := decodeMonitorV3Status(t, r)
	require.Empty(t, result.Categories)
	require.Nil(t, result.Featured)
	repo.components[0].GroupDeleted = true
	c, r = monitorV3Request("/status?tool_id=codex", service.RoleUser, "")
	h.Status(c)
	require.Empty(t, decodeMonitorV3Status(t, r).Categories)
	access.err = fmt.Errorf("mapping unavailable")
	c, r = monitorV3Request("/incidents?tool_id=codex", service.RoleUser, "")
	h.Incidents(c)
	require.NotEqual(t, 200, r.Code)
}
