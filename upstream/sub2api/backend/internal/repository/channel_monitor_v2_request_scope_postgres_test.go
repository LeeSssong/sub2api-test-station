package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2RequestScopePostgres(t *testing.T) {
	dsn := os.Getenv("MONITOR_V2_TEST_DSN")
	if dsn == "" {
		t.Skip("set MONITOR_V2_TEST_DSN for isolated PostgreSQL verification")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	exec := func(q string, args ...any) { _, e := db.ExecContext(ctx, q, args...); require.NoError(t, e) }
	exec(`CREATE SCHEMA monitor_v2_scope_test; SET search_path TO monitor_v2_scope_test`)
	defer db.ExecContext(ctx, `DROP SCHEMA monitor_v2_scope_test CASCADE`)
	exec(`CREATE TABLE groups(id bigint PRIMARY KEY,name text,platform text,deleted_at timestamptz,rate_multiplier numeric DEFAULT 1,status text DEFAULT 'active');INSERT INTO groups VALUES(7,'mixed','openai',NULL,1,'active'),(8,'sync-only','openai',NULL,1,'active')`)
	migration := func(name string) {
		b, e := os.ReadFile(filepath.Join("../..", "migrations", name))
		require.NoError(t, e)
		exec(string(b))
	}
	for _, name := range []string{"194_channel_monitor_v2.sql", "196_channel_monitor_v2_ignored_error_categories.sql", "198_channel_monitor_v2_health_thresholds.sql", "199_channel_monitor_v2_fixed_rollups.sql", "254_channel_monitor_v2_candy.sql"} {
		migration(name)
	}
	start := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	end := start.Add(time.Hour)
	exec(`INSERT INTO channel_monitor_v2_metrics_1m(bucket_start,platform,group_id,model,success_requests,error_requests) VALUES($1,'openai',7,'legacy',5,95)`, start.Add(-3*time.Hour))
	exec(`UPDATE channel_monitor_v2_watermarks SET backfill_cursor=$1,data_through=$2,last_successful_at=NOW(),usage_coverage_start=$1,error_coverage_start=$1`, start.Add(-24*time.Hour), end)
	exec(`ALTER TABLE channel_monitor_v2_config ADD COLUMN candy_history_limit integer DEFAULT 100`)
	exec(`
 CREATE TABLE accounts(id bigint,platform text);CREATE TABLE users(id bigint,email text,username text);INSERT INTO users VALUES(1,'fixture@example.invalid','fixture');
 CREATE TABLE usage_logs(id bigserial,created_at timestamptz,request_id text,user_id bigint,group_id bigint,account_id bigint,model text,requested_model text,request_type smallint,actual_cost numeric,input_tokens bigint,output_tokens bigint,cache_creation_tokens bigint,cache_read_tokens bigint,first_token_ms bigint,duration_ms bigint);
 CREATE TABLE ops_error_logs(id bigserial,created_at timestamptz,request_id text,user_id bigint,group_id bigint,account_id bigint,platform text,model text,requested_model text,request_type smallint,status_code int,upstream_status_code int,error_type text,error_owner text,error_phase text,error_source text,error_message text,upstream_error_message text,upstream_error_detail text,error_body text,upstream_errors jsonb,is_count_tokens bool DEFAULT false,is_business_limited bool DEFAULT false);`)
	exec(`INSERT INTO usage_logs(created_at,request_id) VALUES($1,'source-floor')`, start.Add(-2*time.Hour))
	exec(`INSERT INTO ops_error_logs(created_at,request_id,status_code) VALUES($1,'source-floor',200)`, start.Add(-2*time.Hour))
	migration("287_channel_monitor_v2_request_scope.sql")
	migration("287_channel_monitor_v2_request_scope.sql") // Idempotent replay must preserve rows and progress.
	exec(`INSERT INTO channel_monitor_v2_metrics_rollup(bucket_seconds,bucket_start,platform,group_id,model,success_requests,error_requests,request_type) VALUES(3600,$1,'openai',7,'legacy-boundary',5,95,-1)`, start.Add(-2*time.Hour))
	exec(`INSERT INTO channel_monitor_v2_metrics_1m(bucket_start,platform,group_id,model,success_requests,error_requests,request_type) VALUES($1,'openai',7,'legacy-boundary',5,95,-1)`, start.Add(-2*time.Hour))
	repo := &channelMonitorV2Repository{db: db}
	cfg, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	b, err := json.Marshal(cfg)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, false, doc["exclude_sync_requests"])
	setScope := func(value bool) {
		doc["exclude_sync_requests"] = value
		b, e := json.Marshal(doc)
		require.NoError(t, e)
		require.NoError(t, json.Unmarshal(b, cfg))
	}
	setScope(true)
	saved, err := repo.UpdateConfig(ctx, *cfg, cfg.Version)
	require.NoError(t, err)
	loaded, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, saved, loaded)
	_, err = repo.UpdateConfig(ctx, *cfg, cfg.Version)
	require.ErrorIs(t, err, service.ErrChannelMonitorV2ConfigConflict)

	for _, typ := range []any{1, 2, 3, 0, nil} {
		exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,request_type,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms) VALUES($1,'usage-'||COALESCE($2::text,'null'),1,7,'model',$2::smallint,1,100,20,0,80,100,500)`, start, typ)
		exec(`INSERT INTO ops_error_logs(created_at,request_id,user_id,group_id,platform,model,request_type,status_code,error_type,error_owner,error_phase,error_message) VALUES($1,'error-'||COALESCE($2::text,'null'),1,7,'openai','model',$2::smallint,503,'local_capacity_exhausted','platform','routing','capacity')`, start, typ)
	}
	exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,request_type,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms) VALUES($1,'only-sync',1,8,'model',1,1,100,20,0,80,100,500)`, start)
	require.NoError(t, repo.RecomputeRange(ctx, start, end))
	require.NoError(t, repo.RecomputeRange(ctx, start, end))
	filter := service.ChannelMonitorV2Filter{Start: start, End: end, Bucket: 5 * time.Minute, GroupIDs: []int64{7}, IncludeRecentSamples: true}
	for _, exclude := range []bool{false, true} {
		setScope(exclude)
		where, args, _ := channelMonitorV2WhereWithRollup(filter, *cfg, "m")
		var success, errors, tokens int64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COALESCE(SUM(success_requests),0),COALESCE(SUM(error_requests),0),COALESCE(SUM(input_tokens+output_tokens),0) FROM `+channelMonitorV2MetricsTable(filter)+` m `+where, args...).Scan(&success, &errors, &tokens))
		want := int64(5)
		if exclude {
			want = 4
		}
		require.Equal(t, want, success)
		require.Equal(t, want, errors)
		require.Equal(t, want*120, tokens)
		snapshot, e := repo.GetSnapshot(ctx, filter, *cfg, true)
		require.NoError(t, e)
		require.Equal(t, want, snapshot.Metrics.SuccessRequests)
		require.Equal(t, want, snapshot.Metrics.ErrorRequests)
		require.Equal(t, want, snapshot.Metrics.TTFT.SampleCount)
		models, e := repo.GetModels(ctx, filter, *cfg, true)
		require.NoError(t, e)
		require.Len(t, models.Items, 1)
		require.Equal(t, want, models.Items[0].Metrics.SuccessRequests)
		errorsList, e := repo.GetErrors(ctx, filter, *cfg, true)
		require.NoError(t, e)
		require.Len(t, errorsList.Items, 1)
		require.Equal(t, want, errorsList.Items[0].Count)
		var detailTotal int64
		for _, d := range errorsList.Items[0].Details {
			detailTotal += d.Count
		}
		require.Equal(t, want, detailTotal)
		users, e := repo.GetUsers(ctx, filter, *cfg, true)
		require.NoError(t, e)
		require.Len(t, users.Items, 1)
		require.Equal(t, want, users.Items[0].Metrics.SuccessRequests)
		matrix, e := repo.GetMatrix(ctx, filter, *cfg, service.ChannelMonitorV2GroupByPlatformGroup, true)
		require.NoError(t, e)
		require.NotEmpty(t, matrix.Items)
		require.Equal(t, want, matrix.Items[0].Metrics.SuccessRequests)
		require.NotNil(t, matrix.Items[0].SampleBuckets)
		require.NotEmpty(t, *matrix.Items[0].SampleBuckets)
		require.Equal(t, want, (*matrix.Items[0].SampleBuckets)[0].Metrics.TTFT.SampleCount)
		query, args := channelMonitorV2RecentSampleQuery([]string{"openai"}, []int64{7}, start, end, filter, *cfg)
		rows, e := db.QueryContext(ctx, query, args...)
		require.NoError(t, e)
		require.True(t, rows.Next())
		var at time.Time
		var platform string
		var gid, s, fail, ua, attempts, in, out, cc, cr, tt, tc, ds, dc int64
		require.NoError(t, rows.Scan(&at, &platform, &gid, &s, &fail, &ua, &attempts, &in, &out, &cc, &cr, &tt, &tc, &ds, &dc))
		require.Equal(t, want, s)
		require.Equal(t, want, fail)
		rows.Close()
	}
	setScope(true)
	filter.GroupIDs = []int64{8}
	where, args, _ := channelMonitorV2WhereWithRollup(filter, *cfg, "m")
	var total int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+channelMonitorV2MetricsTable(filter)+` m `+where, args...).Scan(&total))
	require.Zero(t, total)
	snapshot, err := repo.GetSnapshot(ctx, filter, *cfg, true)
	require.NoError(t, err)
	require.Zero(t, snapshot.Metrics.RequestCount)
	require.Equal(t, "unknown", string(snapshot.Health.Overall))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&total))
	require.Equal(t, 7, total)
	require.NoError(t, db.QueryRow(`SELECT SUM(success_requests+error_requests) FROM channel_monitor_v2_metrics_1m WHERE group_id=7 AND model='model'`).Scan(&total))
	require.Equal(t, 10, total)
	// A type-only migration must retain the old unfiltered aggregate while
	// advertising incomplete coverage for the new filtered history.
	var legacyType int
	require.NoError(t, db.QueryRow(`SELECT request_type FROM channel_monitor_v2_metrics_1m WHERE model='legacy'`).Scan(&legacyType))
	require.Equal(t, -1, legacyType)
	// Legacy mixed history is never presented as filtered data before backfill.
	filter.GroupIDs = []int64{7}
	filter.Start = start.Add(-time.Hour)
	where, args, _ = channelMonitorV2WhereWithRollup(filter, *cfg, "m")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+channelMonitorV2MetricsTable(filter)+` m `+where+` AND model='legacy'`, args...).Scan(&total))
	require.Zero(t, total)
	coverage, err := repo.loadCoverage(ctx, filter, *cfg)
	require.NoError(t, err)
	require.False(t, coverage.CoverageComplete)
	// A cleaned source interval must retain legacy facts and V3 totals.
	require.NoError(t, repo.RecomputeRange(ctx, start.Add(-4*time.Hour), start.Add(-2*time.Hour)))
	require.NoError(t, db.QueryRow(`SELECT SUM(success_requests+error_requests) FROM channel_monitor_v2_metrics_1m WHERE model='legacy'`).Scan(&total))
	require.Equal(t, 100, total)
	// Reclassifying the available interval advances coverage.
	require.NoError(t, repo.RecomputeRange(ctx, filter.Start, start))
	coverage, err = repo.loadCoverage(ctx, filter, *cfg)
	require.NoError(t, err)
	require.True(t, coverage.CoverageComplete)
	require.NoError(t, repo.RecomputeRange(ctx, start.Add(-3*time.Hour), end))
	require.NoError(t, db.QueryRow(`SELECT SUM(success_requests+error_requests) FROM channel_monitor_v2_metrics_rollup WHERE model='legacy-boundary' AND bucket_seconds=3600`).Scan(&total))
	require.Equal(t, 100, total)
	// A deployment within an active hour must keep old facts and add new traffic,
	// including when a later refresh crosses the next hour boundary.
	floor := start.Add(33 * time.Minute)
	exec(`UPDATE channel_monitor_v2_watermarks SET request_type_rebuild_from=$1, backfill_cursor=$1`, floor)
	exec(`INSERT INTO channel_monitor_v2_metrics_1m(bucket_start,platform,group_id,model,success_requests,error_requests,request_type) VALUES($1,'openai',7,'active-boundary',100,0,-1)`, start.Add(10*time.Minute))
	exec(`INSERT INTO channel_monitor_v2_metrics_rollup(bucket_seconds,bucket_start,platform,group_id,model,success_requests,error_requests,request_type) VALUES(3600,$1,'openai',7,'active-boundary',100,0,-1)`, start)
	exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,request_type,actual_cost) VALUES($1,'after-migration',1,7,'active-boundary',2,1)`, start.Add(40*time.Minute))
	require.NoError(t, repo.RecomputeRange(ctx, floor, end))
	require.NoError(t, repo.RecomputeRange(ctx, end.Add(-10*time.Minute), end.Add(time.Minute)))
	require.NoError(t, db.QueryRow(`SELECT SUM(success_requests+error_requests) FROM channel_monitor_v2_metrics_rollup WHERE model='active-boundary' AND bucket_seconds=3600`).Scan(&total))
	require.Equal(t, 101, total)
}
