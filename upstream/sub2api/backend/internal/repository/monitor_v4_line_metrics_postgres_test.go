package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorV4LineMetricsPostgres(t *testing.T) {
	dsn := os.Getenv("MONITOR_LINE_TEST_DSN")
	if dsn == "" {
		t.Skip("set MONITOR_LINE_TEST_DSN for isolated PostgreSQL fixture")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TEMP TABLE usage_logs(group_id bigint,created_at timestamptz,first_token_ms bigint);
CREATE TEMP TABLE ops_error_logs(group_id bigint,created_at timestamptz,status_code int,is_business_limited boolean,error_type text,is_count_tokens boolean);
CREATE TEMP TABLE account_monitor_results(group_id bigint,checked_at timestamptz,first_token_ms bigint);`)
	require.NoError(t, err)
	start := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	// One slow request in one hour and nine fast ones in another: the overall
	// median is 10, whereas averaging hourly medians would incorrectly give 505.
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs VALUES
 (7,$1,1000),(7,$1::timestamptz-interval '1 microsecond',99999),
 (7,$2,99999),(99,$1,99999),(8,$1,0),(9,$1,NULL),(9,$1,-1)`, start, end)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs SELECT 7,$1::timestamptz+interval '2 hours',10 FROM generate_series(1,9)`, start)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO account_monitor_results VALUES(7,$1,99999),(10,$1,99999)`, start)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO ops_error_logs VALUES
 (7,$1,500,FALSE,'upstream',FALSE),(7,$1,500,FALSE,'count_tokens',TRUE),
 (7,$1,500,TRUE,'limited',FALSE),(7,$1,499,FALSE,'client_canceled',FALSE),
 (7,$1,200,FALSE,'none',FALSE),(7,$2,500,FALSE,'upstream',FALSE),
 (99,$1,500,FALSE,'upstream',FALSE)`, start, end)
	require.NoError(t, err)
	repo := &accountMonitorRepository{db: db}
	rows, err := repo.ReadMonitorV4LineMetrics(ctx, []int64{7, 8, 9, 10}, start, end)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	require.Equal(t, 7, int(rows[0].ID))
	require.Equal(t, 11, rows[0].SLARequestCount)
	require.Equal(t, 10, rows[0].SLASuccessCount)
	require.Equal(t, 10, rows[0].TTFTSampleCount)
	require.Equal(t, 10.0, *rows[0].TTFTP50MS)
	require.Equal(t, 0.0, *rows[1].TTFTP50MS)
	require.Equal(t, 1, rows[1].TTFTSampleCount)
	require.Equal(t, 2, rows[2].SLASuccessCount)
	require.Nil(t, rows[2].TTFTP50MS)
	require.Zero(t, rows[2].TTFTSampleCount)
	require.Zero(t, rows[3].SLARequestCount)
	require.Nil(t, rows[3].TTFTP50MS)
	require.Zero(t, rows[3].TTFTSampleCount)
}
