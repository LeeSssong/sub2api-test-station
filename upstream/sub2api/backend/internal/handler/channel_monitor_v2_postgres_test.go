package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// These SQL fixture adapters exercise the production APIKeyService permission
// rules. Authentication is injected; no external account or credential is used.
type monitorPGUsers struct {
	service.UserRepository
	db *sql.DB
}

func (r monitorPGUsers) GetByID(ctx context.Context, id int64) (*service.User, error) {
	u := &service.User{ID: id}
	err := r.db.QueryRowContext(ctx, `SELECT allowed_groups,restrict_public FROM users WHERE id=$1`, id).Scan(pq.Array(&u.AllowedGroups), &u.RestrictPublicGroups)
	return u, err
}

type monitorPGGroups struct {
	service.GroupRepository
	db *sql.DB
}

func (r monitorPGGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,platform,is_exclusive FROM groups WHERE status='active' AND deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.Group{}
	for rows.Next() {
		var g service.Group
		if err = rows.Scan(&g.ID, &g.Name, &g.Platform, &g.IsExclusive); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type monitorPGSubscriptions struct {
	service.UserSubscriptionRepository
	db *sql.DB
}

func (r monitorPGSubscriptions) ListActiveByUserID(ctx context.Context, id int64) ([]service.UserSubscription, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT group_id FROM fixture_subscriptions WHERE user_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.UserSubscription{}
	for rows.Next() {
		var s service.UserSubscription
		if err = rows.Scan(&s.GroupID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func TestChannelMonitorV2HandlerPostgres(t *testing.T) {
	dsn := os.Getenv("ISSUE004_DISPOSABLE_PG_DSN")
	if dsn == "" {
		t.Skip("set ISSUE004_DISPOSABLE_PG_DSN to a newly created disposable local database")
	}
	// Refuse accidental use of a business database even when this test is selected.
	require.Contains(t, dsn, "host=127.0.0.1")
	require.Contains(t, dsn, "dbname=issue004_disposable")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	exec := func(q string, args ...any) { _, err := db.ExecContext(ctx, q, args...); require.NoError(t, err) }
	exec(`CREATE SCHEMA issue004_handler_test;SET search_path TO issue004_handler_test`)
	defer db.ExecContext(ctx, `DROP SCHEMA issue004_handler_test CASCADE`)
	exec(`CREATE TABLE groups(id bigint PRIMARY KEY,name text,platform text,deleted_at timestamptz,rate_multiplier numeric DEFAULT 1,status text DEFAULT 'active',is_exclusive bool DEFAULT false);
 INSERT INTO groups(id,name,platform,is_exclusive) VALUES(7,'public','openai',false),(8,'private','openai',true),(9,'outside-config','openai',false);
 CREATE TABLE users(id bigint PRIMARY KEY,email text,username text,allowed_groups bigint[] DEFAULT '{}',restrict_public bool DEFAULT false);
 INSERT INTO users(id,email,username,restrict_public) VALUES(42,'fixture@example.invalid','fixture',false),(43,'empty@example.invalid','empty',true);
 CREATE TABLE fixture_subscriptions(user_id bigint,group_id bigint);
 CREATE TABLE accounts(id bigint,platform text);
 CREATE TABLE usage_logs(id bigserial,created_at timestamptz,request_id text,user_id bigint,group_id bigint,account_id bigint,model text,requested_model text,request_type smallint,actual_cost numeric,input_tokens bigint,output_tokens bigint,cache_creation_tokens bigint,cache_read_tokens bigint,first_token_ms bigint,duration_ms bigint);
 CREATE TABLE ops_error_logs(id bigserial,created_at timestamptz,request_id text,user_id bigint,group_id bigint,account_id bigint,platform text,model text,requested_model text,request_type smallint,status_code int,upstream_status_code int,error_type text,error_owner text,error_phase text,error_source text,error_message text,upstream_error_message text,upstream_error_detail text,error_body text,upstream_errors jsonb,is_count_tokens bool DEFAULT false,is_business_limited bool DEFAULT false);`)
	migrate := func(name string) {
		raw, e := os.ReadFile(filepath.Join("../..", "migrations", name))
		require.NoError(t, e)
		exec(string(raw))
	}
	for _, name := range []string{"194_channel_monitor_v2.sql", "196_channel_monitor_v2_ignored_error_categories.sql", "198_channel_monitor_v2_health_thresholds.sql", "199_channel_monitor_v2_fixed_rollups.sql", "254_channel_monitor_v2_candy.sql", "275_channel_monitor_v2_candy_multi_model.sql", "276_channel_monitor_v2_candy_history_min.sql", "287_channel_monitor_v2_request_scope.sql"} {
		migrate(name)
	}
	repo := repository.NewChannelMonitorV2Repository(db)
	rowJSON := func() string {
		var s string
		require.NoError(t, db.QueryRow(`SELECT (to_jsonb(c)-'card_timeline_mode')::text FROM channel_monitor_v2_config c`).Scan(&s))
		return s
	}
	exec(`UPDATE channel_monitor_v2_config SET version=17,group_ids='{7,8}',platforms='[{"platform":"openai","enabled":true,"models":["configured-but-idle"]}]',candy_history_limit=40,ignored_error_categories='{client_cancelled}',health_thresholds='{"error_rate_warning":0.05}'`)
	before := rowJSON()
	t.Run("migration-required-and-preserves-old-config", func(t *testing.T) {
		_, e := repo.GetConfig(ctx)
		var pg *pq.Error
		require.ErrorAs(t, e, &pg)
		require.Equal(t, "42703", string(pg.Code))
		migrate("288_channel_monitor_v2_card_timeline_mode.sql")
		require.JSONEq(t, before, rowJSON())
		cfg, e := repo.GetConfig(ctx)
		require.NoError(t, e)
		require.Equal(t, "samples", cfg.CardTimelineMode)
		require.Equal(t, 17, cfg.Version)
		migrate("288_channel_monitor_v2_card_timeline_mode.sql")
		require.JSONEq(t, before, rowJSON())
	})
	svc := service.NewChannelMonitorV2Service(repo)
	auth := service.NewAPIKeyService(nil, monitorPGUsers{db: db}, monitorPGGroups{db: db}, monitorPGSubscriptions{db: db}, nil, nil, nil)
	h := NewChannelMonitorV2Handler(svc, auth)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if id := c.GetHeader("X-Fixture-Subject"); id != "" {
			var uid int64
			_, _ = fmt.Sscan(id, &uid)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: uid})
		}
		c.Next()
	})
	router.GET("/config", h.GetConfig)
	router.PUT("/config", h.UpdateConfig)
	router.GET("/models", h.Models)
	router.GET("/matrix", h.Matrix)
	router.GET("/snapshot", h.Snapshot)
	call := func(method, url string, body []byte, uid string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if uid != "" {
			req.Header.Set("X-Fixture-Subject", uid)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	config := func() service.ChannelMonitorV2Config {
		w := call("GET", "/config", nil, "42")
		require.Equal(t, 200, w.Code, w.Body.String())
		var env struct {
			Data service.ChannelMonitorV2Config `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		return env.Data
	}
	t.Run("save-read-modes-legacy-omission-and-409", func(t *testing.T) {
		for _, mode := range []string{"windows", "samples", "windows"} {
			cfg := config()
			cfg.CardTimelineMode = mode
			raw, e := json.Marshal(cfg)
			require.NoError(t, e)
			w := call("PUT", "/config", raw, "42")
			require.Equal(t, 200, w.Code, w.Body.String())
			loaded := config()
			require.Equal(t, mode, loaded.CardTimelineMode)
			require.Equal(t, cfg.Version+1, loaded.Version)
			require.Equal(t, cfg.GroupIDs, loaded.GroupIDs)
			require.Equal(t, cfg.CandyHistoryLimit, loaded.CandyHistoryLimit)
			require.Equal(t, cfg.CandyProbes, loaded.CandyProbes)
			require.Equal(t, cfg.HealthThresholds, loaded.HealthThresholds)
			require.Equal(t, cfg.IgnoredErrorCategories, loaded.IgnoredErrorCategories)
			// Replaying migration cannot reset an already saved choice.
			migrate("288_channel_monitor_v2_card_timeline_mode.sql")
			require.Equal(t, mode, config().CardTimelineMode)
		}
		cfg := config()
		raw, e := json.Marshal(cfg)
		require.NoError(t, e)
		var legacy map[string]any
		require.NoError(t, json.Unmarshal(raw, &legacy))
		delete(legacy, "card_timeline_mode")
		delete(legacy, "platforms")
		delete(legacy, "candy_history_limit")
		legacy["refresh_interval_seconds"] = 60
		raw, e = json.Marshal(legacy)
		require.NoError(t, e)
		w := call("PUT", "/config", raw, "42")
		require.Equal(t, 200, w.Code, w.Body.String())
		loaded := config()
		require.Equal(t, "windows", loaded.CardTimelineMode)
		require.Equal(t, cfg.Platforms, loaded.Platforms)
		require.Equal(t, 40, loaded.CandyHistoryLimit)
		stale := cfg
		stale.CardTimelineMode = "samples"
		draft, e := json.Marshal(stale)
		require.NoError(t, e)
		retained := append([]byte(nil), draft...)
		w = call("PUT", "/config", draft, "42")
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Equal(t, retained, draft)
		require.Equal(t, loaded, config())
		invalid := loaded
		invalid.CardTimelineMode = "invalid"
		raw, e = json.Marshal(invalid)
		require.NoError(t, e)
		w = call("PUT", "/config", raw, "42")
		require.Equal(t, 400, w.Code)
		require.Equal(t, loaded, config())
	})
	// Real SQL aggregation, not inserted precomputed facts. Paid fixture traffic
	// intentionally does not pretend to fix the unresolved zero-cost outcome gap.
	start := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	end := start.Add(time.Hour)
	exec(`UPDATE channel_monitor_v2_watermarks SET request_type_rebuild_from=$1,backfill_cursor=$1,usage_coverage_start=$1,error_coverage_start=$1,data_through=$2,last_successful_at=NOW()`, start.Add(-time.Minute), end)
	for i := 0; i < 125; i++ {
		name := fmt.Sprintf("unlisted-%03d", i)
		exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,requested_model,request_type,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms) SELECT $1,$2||'-'||n,42,7,$2,$2,2,1,100,20,0,80,100,500 FROM generate_series(1,$3::int) n`, start, name, 1+i%3)
	}
	exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,request_type,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms) VALUES($1,'private-req',42,8,'private-model',2,1,100,20,0,80,100,500),($1,'outside-req',42,9,'outside-model',2,1,100,20,0,80,100,500)`, start)
	require.NoError(t, repo.RecomputeRange(ctx, start, end))
	t.Run("handler-permission-repository-and-complete-models", func(t *testing.T) {
		w := call("GET", "/models?range=24h&group_id=7,8,9", nil, "42")
		require.Equal(t, 200, w.Code, w.Body.String())
		var env struct {
			Data service.ChannelMonitorV2List[service.ChannelMonitorV2ModelRow] `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		require.Len(t, env.Data.Items, 125)
		names := map[string]bool{}
		for _, row := range env.Data.Items {
			require.True(t, strings.HasPrefix(row.Model, "unlisted-"))
			require.True(t, row.Metrics.HasSamples)
			require.Zero(t, row.Metrics.RequestCount)
			names[row.Model] = true
		}
		require.Len(t, names, 125)
		// Descending request volume is decided before redaction; stable ties by name.
		require.Equal(t, "unlisted-002", env.Data.Items[0].Model)
		require.Equal(t, "unlisted-005", env.Data.Items[1].Model)
		require.False(t, names["configured-but-idle"])
		require.False(t, names["__other__"])
		for _, url := range []string{"/models?range=24h&group_id=8", "/models?range=24h&group_id=9"} {
			denied := call("GET", url, nil, "42")
			require.Equal(t, 200, denied.Code, denied.Body.String())
			require.NoError(t, json.Unmarshal(denied.Body.Bytes(), &env))
			require.Empty(t, env.Data.Items)
		}
		empty := call("GET", "/models?range=24h&group_id=7", nil, "43")
		require.Equal(t, 200, empty.Code, empty.Body.String())
		require.NoError(t, json.Unmarshal(empty.Body.Bytes(), &env))
		require.Empty(t, env.Data.Items)
		unauth := call("GET", "/models?range=24h&group_id=7", nil, "")
		require.Equal(t, 401, unauth.Code)
		for _, url := range []string{"/snapshot?range=24h&group_id=7,8,9", "/matrix?range=24h&group_id=7,8,9&recent_samples=true"} {
			res := call("GET", url, nil, "42")
			require.Equal(t, 200, res.Code, res.Body.String())
			require.NotContains(t, res.Body.String(), "private-model")
			require.NotContains(t, res.Body.String(), "outside-model")
		}
	})
	t.Run("diagnoses-outcome-gap-with-real-aggregation", func(t *testing.T) {
		exec(`INSERT INTO groups(id,name,platform) VALUES(10,'outcome-fixtures','openai')`)
		// Truth labels are known from this synthetic scenario, not inferred by SQL.
		// Retry attempts have distinct physical request IDs, as persisted audit rows do.
		exec(`INSERT INTO usage_logs(created_at,request_id,user_id,group_id,model,request_type,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms) VALUES
   ($1,'paid',42,10,'paid-success',2,1,100,20,0,80,100,500),
   ($1,'free',42,10,'zero-cost-success',2,0,100,20,0,80,100,500),
   ($1,'failed',42,10,'failed-partial-paid',2,1,100,20,0,80,100,500),
   ($1,'cancel',42,10,'cancel-partial-paid',2,1,100,20,0,80,100,500),
   ($1,'retry-attempt-1',42,10,'retry-then-success',4,1,100,20,0,80,100,500),
   ($1,'retry-attempt-2',42,10,'retry-then-success',2,1,100,20,0,80,100,500)`, start)
		exec(`INSERT INTO ops_error_logs(created_at,request_id,user_id,group_id,platform,model,request_type,status_code,error_type,error_owner,error_phase,error_message) VALUES
   ($1,'failed',42,10,'openai','failed-partial-paid',2,502,'upstream','provider','forward','upstream failed'),
   ($1,'cancel',42,10,'openai','cancel-partial-paid',2,499,'client_cancelled','client','forward','client cancelled'),
   ($1,'ignored',42,10,'openai','ignored-cancel',2,499,'client_cancelled','client','forward','client cancelled')`, start)
		require.NoError(t, repo.RecomputeRange(ctx, start, end))
		rows, e := db.Query(`SELECT model,SUM(success_requests),SUM(error_requests),SUM(input_tokens),SUM(ttft_count) FROM channel_monitor_v2_metrics_1m WHERE group_id=10 GROUP BY model ORDER BY model`)
		require.NoError(t, e)
		defer rows.Close()
		observed := map[string][]int64{}
		for rows.Next() {
			var model string
			var successes, failures, tokens, ttft int64
			require.NoError(t, rows.Scan(&model, &successes, &failures, &tokens, &ttft))
			observed[model] = []int64{successes, failures, tokens, ttft}
		}
		require.NoError(t, rows.Err())
		require.Equal(t, []int64{1, 0, 100, 1}, observed["paid-success"])
		require.Equal(t, []int64{0, 0, 0, 0}, observed["zero-cost-success"])
		require.Equal(t, []int64{1, 1, 100, 1}, observed["failed-partial-paid"])
		require.Equal(t, []int64{1, 1, 100, 1}, observed["cancel-partial-paid"])
		require.Equal(t, []int64{1, 0, 200, 2}, observed["retry-then-success"])
		require.Equal(t, []int64{0, 1, 0, 0}, observed["ignored-cancel"])
		t.Logf("current unresolved outcome aggregation [success,error,input,ttft_count]: %#v", observed)
	})

}
