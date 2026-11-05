package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func timelineConfig(t *testing.T, mode string) service.ChannelMonitorV2Config {
	t.Helper()
	var cfg service.ChannelMonitorV2Config
	require.NoError(t, json.Unmarshal([]byte(`{"version":3,"enabled":true,"refresh_interval_seconds":300,"platforms":[{"platform":"openai","enabled":true}],"candy_history_limit":100,"card_timeline_mode":"`+mode+`"}`), &cfg))
	return cfg
}

func timelineMode(t *testing.T, cfg *service.ChannelMonitorV2Config) any {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	return fields["card_timeline_mode"]
}

type timelineConfigRepo struct {
	service.ChannelMonitorV2Repository
	current service.ChannelMonitorV2Config
	saved   *service.ChannelMonitorV2Config
}

func (r *timelineConfigRepo) GetConfig(context.Context) (*service.ChannelMonitorV2Config, error) {
	return &r.current, nil
}
func (r *timelineConfigRepo) UpdateConfig(_ context.Context, cfg service.ChannelMonitorV2Config, _ int) (*service.ChannelMonitorV2Config, error) {
	r.saved = &cfg
	return &cfg, nil
}

func TestChannelMonitorV2TimelineValidationAndLegacySave(t *testing.T) {
	for _, mode := range []string{"windows", "samples", "invalid", ""} {
		t.Run(mode, func(t *testing.T) {
			repo := &timelineConfigRepo{current: timelineConfig(t, "windows")}
			updated, err := service.NewChannelMonitorV2Service(repo).UpdateConfig(context.Background(), timelineConfig(t, mode), 3, 7)
			if mode == "invalid" {
				require.ErrorIs(t, err, service.ErrChannelMonitorV2InvalidConfig)
				require.Nil(t, repo.saved)
				return
			}
			require.NoError(t, err)
			if mode == "" {
				mode = "windows"
			}
			require.Equal(t, mode, timelineMode(t, updated))
		})
	}
	t.Run("default for missing current mode", func(t *testing.T) {
		repo := &timelineConfigRepo{current: timelineConfig(t, "")}
		updated, err := service.NewChannelMonitorV2Service(repo).UpdateConfig(context.Background(), timelineConfig(t, ""), 3, 7)
		require.NoError(t, err)
		require.Equal(t, "samples", timelineMode(t, updated))
	})
}

func TestChannelMonitorV2TimelineConfigSQLRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	columns := []string{"version", "enabled", "refresh_interval_seconds", "platforms", "group_ids", "ignored_error_categories", "health_thresholds", "candy_probes", "candy_history_limit", "exclude_sync_requests", "card_timeline_mode", "updated_at", "updated_by"}
	rows := func(version int) *sqlmock.Rows {
		return sqlmock.NewRows(columns).AddRow(version, true, 300, `[{"platform":"openai","enabled":true}]`, "{}", "{}", "{}", "[]", 100, false, "windows", time.Now(), nil)
	}
	repo := NewChannelMonitorV2Repository(db)
	mock.ExpectQuery(`SELECT version, enabled, refresh_interval_seconds[\s\S]*card_timeline_mode[\s\S]*FROM channel_monitor_v2_config`).WillReturnRows(rows(3))
	cfg, err := repo.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "windows", timelineMode(t, cfg))
	mock.ExpectQuery(`UPDATE channel_monitor_v2_config[\s\S]*card_timeline_mode = \$12[\s\S]*WHERE id = 1 AND version = \$8`).WithArgs(true, 300, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), nil, 3, sqlmock.AnyArg(), 100, false, "windows").WillReturnRows(rows(4))
	updated, err := repo.UpdateConfig(context.Background(), *cfg, 3)
	require.NoError(t, err)
	require.Equal(t, 4, updated.Version)
	require.Equal(t, "windows", timelineMode(t, updated))
	mock.ExpectQuery(`UPDATE channel_monitor_v2_config`).WillReturnError(sql.ErrNoRows)
	_, err = repo.UpdateConfig(context.Background(), *cfg, 3)
	require.ErrorIs(t, err, service.ErrChannelMonitorV2ConfigConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
