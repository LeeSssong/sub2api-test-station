package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestInitEntStartupRoutesRoleSpecificWork(t *testing.T) {
	tests := []struct {
		name string
		role config.ProcessRole
		want []string
	}{
		{name: "api verifies and validates only", role: config.ProcessRoleAPI, want: []string{"verify", "validate"}},
		{name: "worker runs mutating initialization", role: config.ProcessRoleWorker, want: []string{"apply", "bootstrap", "validate", "seed-default-groups", "seed-admin-concurrency"}},
		{name: "all preserves mutating initialization", role: config.ProcessRoleAll, want: []string{"apply", "bootstrap", "validate", "seed-default-groups", "seed-admin-concurrency"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			cfg := &config.Config{
				Server:  config.ServerConfig{ProcessRole: tt.role},
				RunMode: config.RunModeSimple,
			}
			err := initEntStartup(context.Background(), cfg, nil, nil, initEntStartupHooks{
				applyMigrations: func(context.Context, *sql.DB) error {
					calls = append(calls, "apply")
					return nil
				},
				verifyMigrations: func(context.Context, *sql.DB) error {
					calls = append(calls, "verify")
					return nil
				},
				bootstrapSecrets: func(context.Context, *ent.Client, *config.Config) error {
					calls = append(calls, "bootstrap")
					return nil
				},
				validateConfig: func(*config.Config) error {
					calls = append(calls, "validate")
					return nil
				},
				seedDefaultGroups: func(context.Context, *ent.Client) error {
					calls = append(calls, "seed-default-groups")
					return nil
				},
				seedAdminConcurrency: func(context.Context, *ent.Client) error {
					calls = append(calls, "seed-admin-concurrency")
					return nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, calls)
		})
	}
}

func TestNormalizeInitEntProcessRole(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{ProcessRole: " WORKER "}}

	err := normalizeInitEntProcessRole(cfg)

	require.NoError(t, err)
	require.Equal(t, config.ProcessRoleWorker, cfg.Server.ProcessRole)
}
