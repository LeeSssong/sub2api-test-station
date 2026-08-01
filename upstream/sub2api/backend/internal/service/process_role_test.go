package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProcessRoleLifecycle(t *testing.T) {
	tests := []struct {
		name string
		got  func() bool
		want bool
	}{
		{name: "all starts request-local work", got: func() bool { return ShouldStartRequestLocal(config.ProcessRoleAll) }, want: true},
		{name: "api keeps request-local flush work enabled", got: func() bool { return ShouldStartRequestLocal(config.ProcessRoleAPI) }, want: true},
		{name: "worker disables request-local work", got: func() bool { return ShouldStartRequestLocal(config.ProcessRoleWorker) }, want: false},
		{name: "all starts singleton work", got: func() bool { return ShouldStartSingleton(config.ProcessRoleAll) }, want: true},
		{name: "api does not enable singleton work", got: func() bool { return ShouldStartSingleton(config.ProcessRoleAPI) }, want: false},
		{name: "worker starts singleton work", got: func() bool { return ShouldStartSingleton(config.ProcessRoleWorker) }, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.got())
		})
	}
}
