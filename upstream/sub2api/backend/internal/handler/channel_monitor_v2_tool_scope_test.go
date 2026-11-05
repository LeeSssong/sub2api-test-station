package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type v2ToolAccessStub struct {
	groups     []service.Group
	mappings   map[int64]service.GroupToolMapping
	mappingErr error
}

func (a *v2ToolAccessStub) GetAvailableGroups(context.Context, int64) ([]service.Group, error) {
	return a.groups, nil
}
func (a *v2ToolAccessStub) GetGroupToolMappings(context.Context, []int64) (map[int64]service.GroupToolMapping, error) {
	return a.mappings, a.mappingErr
}

func TestChannelMonitorV2ToolScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprint("admin=", admin), func(t *testing.T) {
			a := &v2ToolAccessStub{groups: []service.Group{{ID: 1, Platform: "openai", Status: "active"}, {ID: 2, Platform: "anthropic", Status: "active"}, {ID: 3, Platform: "composite", Status: "active"}, {ID: 4, Platform: "anthropic", Status: "active"}, {ID: 5, Platform: "openai", Status: "inactive"}, {ID: 6, Platform: "openai", Status: "active"}}, mappings: map[int64]service.GroupToolMapping{3: {ToolIDs: []string{"codex", "claude"}}, 4: {ToolIDs: []string{"codex"}}, 6: {ToolIDs: []string{"claude"}}, 9: {ToolIDs: []string{"codex"}}}}
			h := &ChannelMonitorV2Handler{apiKeyService: a}
			role := service.RoleUser
			if admin {
				role = service.RoleAdmin
			}
			c, _ := monitorV3Request("/snapshot?tool_id=codex", role, "")
			filter := service.ChannelMonitorV2Filter{GroupIDs: []int64{1, 2, 3, 4, 5, 6, 9}}
			require.True(t, h.scopeFilter(c, &filter, admin))
			require.True(t, filter.RestrictGroups)
			require.ElementsMatch(t, []int64{1, 3, 4}, filter.AllowedGroupIDs)
			// Even an account-visible group must be excluded after its mapping stops matching the tool.
			a.groups = []service.Group{{ID: 1, Platform: "openai", Status: "active"}}
			a.mappings[1] = service.GroupToolMapping{ToolIDs: []string{"claude"}}
			filter = service.ChannelMonitorV2Filter{GroupIDs: []int64{1}}
			require.True(t, h.scopeFilter(c, &filter, admin))
			require.True(t, filter.RestrictGroups)
			require.Empty(t, filter.AllowedGroupIDs)
			a.groups = nil
			filter = service.ChannelMonitorV2Filter{}
			require.True(t, h.scopeFilter(c, &filter, admin))
			require.True(t, filter.RestrictGroups)
			require.Empty(t, filter.AllowedGroupIDs)
		})
	}
}

func TestChannelMonitorV2ToolScopeRejectsInvalidOrUnavailableMapping(t *testing.T) {
	a := &v2ToolAccessStub{groups: []service.Group{{ID: 1, Platform: "openai"}}, mappingErr: fmt.Errorf("mapping unavailable")}
	h := &ChannelMonitorV2Handler{apiKeyService: a}
	for _, query := range []string{"tool_id=invalid", "tool_id=", "tool_id=codex"} {
		c, w := monitorV3Request("/models?"+query, service.RoleUser, "")
		filter := service.ChannelMonitorV2Filter{}
		require.False(t, h.scopeFilter(c, &filter, false))
		require.GreaterOrEqual(t, w.Code, 400)
	}
}
