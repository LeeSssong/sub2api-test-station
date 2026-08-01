package main

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRunStartup_APIFailsClosedBeforeSetupWhenSetupIsNeeded(t *testing.T) {
	setupCalls := 0
	actions := startupActions{
		needsSetup: func() bool {
			return true
		},
		autoSetupEnabled: func() bool {
			setupCalls++
			return true
		},
		autoSetup: func() error {
			setupCalls++
			return nil
		},
		runSetupServer: func() {
			setupCalls++
		},
		runMainServer: func() {},
	}

	err := runStartup(config.ProcessRoleAPI, false, actions)

	require.ErrorContains(t, err, "api process role cannot run setup")
	require.Zero(t, setupCalls, "API startup must reject setup before invoking any setup callback")
}

func TestRunStartup_APIFailsClosedBeforeCLISetup(t *testing.T) {
	setupCalls := 0
	actions := startupActions{
		runCLI: func() error {
			setupCalls++
			return nil
		},
	}

	err := runStartup(config.ProcessRoleAPI, true, actions)

	require.ErrorContains(t, err, "api process role cannot run setup")
	require.Zero(t, setupCalls, "API startup must reject CLI setup before invoking it")
}
