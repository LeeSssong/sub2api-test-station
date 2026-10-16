package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAstraProbeNeverGatesOrEvictsAccount(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetAstraRoutingLoader(func(context.Context) config.AstraRoutingSettings {
		return config.AstraRoutingSettings{AccountScheduling: true, CookiePool: config.CodexGatewayPinConfig{Enabled: true, TargetAccountIDs: []int64{300}}}
	})
	account := &Account{ID: 300, Schedulable: true}
	repo := &astraModeRepo{astraSchedulingRepo: astraSchedulingRepo{accounts: map[int64]*Account{300: account}}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: repo}
	require.NoError(t, svc.checkAstraSchedulingRoute(t.Context(), account))
	for _, code := range []string{"source_probe_cooldown", "target_not_verified", "target_probe_failed", "astra_rotation_exhausted", "configuration_changed"} {
		original := errors.New(code)
		require.Same(t, original, svc.astraRouteFailover(t.Context(), account, original))
		require.True(t, account.Schedulable)
	}
	require.Empty(t, repo.writes)
	require.Zero(t, repo.calls)
}

func TestAstraAPILocalFailureDoesNotOverrideSingletonScheduling(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI}}
	settings := config.AstraRoutingSettings{AccountScheduling: true, Revision: "current", CookiePool: config.CodexGatewayPinConfig{Enabled: true, TargetAccountIDs: []int64{300}}}
	cfg.SetAstraRoutingLoader(func(context.Context) config.AstraRoutingSettings { return settings })
	account := &Account{ID: 300, Schedulable: true}
	repo := &astraSchedulingRepo{accounts: map[int64]*Account{300: account}}
	service := &OpenAIGatewayService{cfg: cfg, accountRepo: repo, httpUpstream: &astraSetupUpstream{snapshot: AstraGatewayRuntime{Revision: "current"}}}
	require.NoError(t, service.checkAstraSchedulingRoute(t.Context(), account), "cold API proceeds to request-local upstream preparation")
	var failover *UpstreamFailoverError
	require.ErrorAs(t, service.astraRouteFailover(t.Context(), account, errors.New("astra_route_not_ready")), &failover)
	require.True(t, account.Schedulable)
	require.Empty(t, repo.writes, "API readiness must never overwrite worker scheduling")
}
