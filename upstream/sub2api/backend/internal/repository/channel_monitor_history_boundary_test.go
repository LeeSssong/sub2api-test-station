package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorCurrentStateQueriesHonorHistoryBoundary(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *channelMonitorRepository) error
	}{
		{
			name: "latest per model",
			call: func(ctx context.Context, repo *channelMonitorRepository) error {
				_, err := repo.ListLatestPerModel(ctx, 7)
				return err
			},
		},
		{
			name: "availability",
			call: func(ctx context.Context, repo *channelMonitorRepository) error {
				_, err := repo.ComputeAvailability(ctx, 7, 7)
				return err
			},
		},
		{
			name: "latest batch",
			call: func(ctx context.Context, repo *channelMonitorRepository) error {
				_, err := repo.ListLatestForMonitorIDs(ctx, []int64{7})
				return err
			},
		},
		{
			name: "recent history batch",
			call: func(ctx context.Context, repo *channelMonitorRepository) error {
				_, err := repo.ListRecentHistoryForMonitors(ctx, []int64{7}, map[int64]string{7: "gpt-5.4"}, 5)
				return err
			},
		},
		{
			name: "availability batch",
			call: func(ctx context.Context, repo *channelMonitorRepository) error {
				_, err := repo.ComputeAvailabilityForMonitors(ctx, []int64{7}, 7)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured string
			matcher := sqlmock.QueryMatcherFunc(func(_, actual string) error {
				captured = actual
				return nil
			})
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			repo := &channelMonitorRepository{db: db}

			mock.ExpectQuery("ignored").WillReturnRows(sqlmock.NewRows([]string{
				"monitor_id", "model", "status", "latency_ms", "ping_latency_ms", "checked_at",
			}))
			err = tt.call(context.Background(), repo)
			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
			require.Contains(t, captured, "JOIN channel_monitors cm ON cm.id = h.monitor_id")
			require.Contains(t, captured, "h.checked_at >= cm.history_started_at")
		})
	}
}

func TestChannelMonitorListHistoryPreservesRawAdminHistory(t *testing.T) {
	content, err := os.ReadFile("channel_monitor_repo.go")
	require.NoError(t, err)
	source := string(content)
	start := strings.Index(source, "func (r *channelMonitorRepository) ListHistory(")
	require.NotEqual(t, -1, start)
	end := strings.Index(source[start:], "func (r *channelMonitorRepository) ListLatestPerModel(")
	require.NotEqual(t, -1, end)
	listHistory := source[start : start+end]
	require.NotContains(t, listHistory, "history_started_at")
	require.NotContains(t, listHistory, "JOIN channel_monitors")
}
