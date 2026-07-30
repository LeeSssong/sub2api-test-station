package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorToResponseIncludesStableGroupID(t *testing.T) {
	groupID := int64(16)

	response := channelMonitorToResponse(&service.ChannelMonitor{
		GroupID:   &groupID,
		GroupName: "GPT-PLUS-内测",
	})

	require.Equal(t, &groupID, response.GroupID)
	require.Equal(t, "GPT-PLUS-内测", response.GroupName)
}
