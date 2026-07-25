package sub2api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/adminauth"
)

func TestReaderAccountRoutingContract(t *testing.T) {
	t.Parallel()

	requests := make([]string, 0, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "admin-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/admin/groups/3":
			fmt.Fprint(w, `{"data":{"id":3,"name":"GPT-Pro","platform":"openai","rate_multiplier":1,"is_exclusive":false,"status":"active"}}`)
		case "GET /api/v1/admin/accounts/11":
			fmt.Fprint(w, `{"data":{"id":11,"name":"GPT-Pro primary","platform":"openai","status":"active","schedulable":true,"group_ids":[3,8],"credentials_status":{"has_api_key":true}}}`)
		case "GET /api/v1/admin/accounts/11/models":
			fmt.Fprint(w, `{"data":[{"id":"gpt-5.6-sol"},{"id":"gpt-5.6-terra"}]}`)
		case "PUT /api/v1/admin/accounts/12":
			assertOnlyJSONField(t, r, "group_ids", []any{float64(3), float64(9)})
			fmt.Fprint(w, `{"data":{"id":12,"name":"GPT-Pro backup","platform":"openai","status":"active","schedulable":true,"group_ids":[3,9],"credentials_status":{"has_api_key":true}}}`)
		case "POST /api/v1/admin/accounts/12/schedulable":
			assertOnlyJSONField(t, r, "schedulable", true)
			fmt.Fprint(w, `{"data":{"id":12,"name":"GPT-Pro backup","platform":"openai","status":"active","schedulable":true,"group_ids":[9],"credentials_status":{"has_api_key":true}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	reader := newTestReader(t, server.URL)
	ctx := context.Background()
	group, err := reader.GetGroup(ctx, 3)
	if err != nil || group.Name != "GPT-Pro" {
		t.Fatalf("GetGroup = %#v, %v", group, err)
	}
	account, err := reader.GetAccount(ctx, 11)
	if err != nil || account.ID != 11 || len(account.GroupIDs) != 2 || !account.CredentialsStatus["has_api_key"] {
		t.Fatalf("GetAccount = %#v, %v", account, err)
	}
	models, err := reader.GetAccountModels(ctx, 11)
	if err != nil || len(models) != 2 || models[0].ID != "gpt-5.6-sol" {
		t.Fatalf("GetAccountModels = %#v, %v", models, err)
	}
	updated, err := reader.SetAccountGroups(ctx, 12, []int64{3, 9})
	if err != nil || len(updated.GroupIDs) != 2 {
		t.Fatalf("SetAccountGroups = %#v, %v", updated, err)
	}
	schedulable, err := reader.SetAccountSchedulable(ctx, 12, true)
	if err != nil || !schedulable.Schedulable {
		t.Fatalf("SetAccountSchedulable = %#v, %v", schedulable, err)
	}
	want := []string{
		"GET /api/v1/admin/groups/3",
		"GET /api/v1/admin/accounts/11",
		"GET /api/v1/admin/accounts/11/models",
		"PUT /api/v1/admin/accounts/12",
		"POST /api/v1/admin/accounts/12/schedulable",
	}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestHTTPReaderSyncUpstreamModelsUsesNativeAdminEndpoint(t *testing.T) {
	t.Parallel()

	requests := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/17/models/sync-upstream" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "admin-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if r.ContentLength > 0 {
			t.Errorf("content length = %d, want empty request", r.ContentLength)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"models":["gpt-5.7-sol","gpt-5.7"]}}`)
	}))
	defer server.Close()

	models, err := newTestReader(t, server.URL).SyncUpstreamModels(context.Background(), 17)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(models), fmt.Sprint([]Model{{ID: "gpt-5.7"}, {ID: "gpt-5.7-sol"}}); got != want {
		t.Fatalf("models = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(requests), "[POST /api/v1/admin/accounts/17/models/sync-upstream]"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestHTTPReaderSyncUpstreamModelsRejectsInvalidDirectory(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"data":{}}`,
		`{"data":{"models":[]}}`,
		`{"data":{"models":["gpt-5.7","gpt-5.7"]}}`,
		`{"data":{"models":[" gpt-5.7"]}}`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, body)
			}))
			defer server.Close()

			_, err := newTestReader(t, server.URL).SyncUpstreamModels(context.Background(), 17)
			if !IsSchemaMismatch(err) {
				t.Fatalf("error = %v, want schema mismatch", err)
			}
		})
	}
}

func TestReaderListsAccountsAcrossPages(t *testing.T) {
	t.Parallel()

	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "admin-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("page_size"); got != "100" {
			t.Errorf("page_size = %q", got)
		}
		requests = append(requests, r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, `{"data":{"items":[{"id":11,"status":"active","schedulable":true}],"total":2,"page":1,"page_size":100}}`)
		case "2":
			fmt.Fprint(w, `{"data":{"items":[{"id":12,"status":"disabled","schedulable":false}],"total":2,"page":2,"page_size":100}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	accounts, err := newTestReader(t, server.URL).ListAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 11 || accounts[1].ID != 12 {
		t.Fatalf("accounts = %#v, want IDs 11,12", accounts)
	}
	if got, want := strings.Join(requests, ","), "1,2"; got != want {
		t.Fatalf("pages = %q, want %q", got, want)
	}
}

func TestReaderRejectsInvalidAccountPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body func(page string) string
	}{
		{
			name: "missing total",
			body: func(string) string {
				return `{"data":{"items":[],"page":1,"page_size":100}}`
			},
		},
		{
			name: "duplicate account",
			body: func(page string) string {
				if page == "1" {
					return `{"data":{"items":[{"id":11,"status":"active","schedulable":true}],"total":2,"page":1,"page_size":100}}`
				}
				return `{"data":{"items":[{"id":11,"status":"active","schedulable":true}],"total":2,"page":2,"page_size":100}}`
			},
		},
		{
			name: "empty page before total",
			body: func(string) string {
				return `{"data":{"items":[],"total":1,"page":1,"page_size":100}}`
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, test.body(r.URL.Query().Get("page")))
			}))
			defer server.Close()
			_, err := newTestReader(t, server.URL).ListAccounts(context.Background())
			if !IsSchemaMismatch(err) {
				t.Fatalf("ListAccounts error = %v, want schema mismatch", err)
			}
		})
	}
}

func TestReaderAccountRoutingRejectsConflictWithoutLeakingBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"error":"mixed_channel_warning","secret":"must-not-leak"}`)
	}))
	defer server.Close()

	_, err := newTestReader(t, server.URL).SetAccountGroups(context.Background(), 12, []int64{3})
	if status, ok := HTTPStatus(err); !ok || status != http.StatusConflict {
		t.Fatalf("HTTPStatus = %d, %v, error %v", status, ok, err)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("error leaked body: %v", err)
	}
}

func assertOnlyJSONField(t *testing.T, r *http.Request, key string, want any) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("body = %#v, want exactly one field", body)
	}
	if fmt.Sprint(body[key]) != fmt.Sprint(want) {
		t.Fatalf("body[%q] = %#v, want %#v", key, body[key], want)
	}
}

func TestReaderUsesGETAdminKeyAndDecodesNativeResources(t *testing.T) {
	t.Parallel()

	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "admin-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		requests[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/channels":
			fmt.Fprint(w, `{"data":{"items":[{"id":7,"name":"GPT-Pro","status":"active","group_ids":[3],"model_pricing":[]}]}}`)
		case "/api/v1/admin/groups/all":
			fmt.Fprint(w, `{"data":[{"id":3,"name":"GPT-Pro","platform":"openai","rate_multiplier":1,"is_exclusive":false,"status":"active"}]}`)
		case "/api/v1/admin/channel-monitors":
			fmt.Fprint(w, `{"data":{"items":[{"id":9,"name":"GPT-Pro","group_name":"GPT-Pro","enabled":true,"primary_model":"gpt-5.6-sol","primary_status":"operational","primary_latency_ms":1493,"availability_7d":98.76}]}}`)
		case "/api/v1/admin/channel-monitors/9/history":
			fmt.Fprint(w, `{"data":{"items":[{"id":11,"model":"gpt-5.6-sol","status":"operational","latency_ms":1493,"ping_latency_ms":270,"checked_at":"2026-07-19T08:00:00Z"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	reader := newTestReader(t, server.URL)
	ctx := context.Background()
	channels, err := reader.ListChannels(ctx)
	if err != nil || len(channels) != 1 || channels[0].ID != 7 {
		t.Fatalf("ListChannels = %#v, %v", channels, err)
	}
	groups, err := reader.ListGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].CustomerVisible() != true {
		t.Fatalf("ListGroups = %#v, %v", groups, err)
	}
	monitors, err := reader.ListChannelMonitors(ctx)
	if err != nil || len(monitors) != 1 || monitors[0].PrimaryLatencyMS != 1493 {
		t.Fatalf("ListChannelMonitors = %#v, %v", monitors, err)
	}
	history, err := reader.GetChannelMonitorHistory(ctx, 9, "gpt-5.6-sol", 60)
	if err != nil || len(history) != 1 || history[0].PingLatencyMS != 270 {
		t.Fatalf("GetChannelMonitorHistory = %#v, %v", history, err)
	}
	for _, path := range []string{"/api/v1/admin/channels", "/api/v1/admin/groups/all", "/api/v1/admin/channel-monitors", "/api/v1/admin/channel-monitors/9/history"} {
		if requests[path] != 1 {
			t.Fatalf("requests[%q] = %d", path, requests[path])
		}
	}
}

func TestReaderDecodesOpsAndUsageWithoutUserDetails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/ops/dashboard/snapshot-v2":
			if r.URL.Query().Get("time_range") != "24h" || r.URL.Query().Get("group_id") != "3" {
				t.Errorf("ops query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":{"generated_at":"2026-07-19T08:00:00Z","overview":{"start_time":"2026-07-18T08:00:00Z","end_time":"2026-07-19T08:00:00Z","success_count":99,"error_count_total":1,"request_count_total":100,"request_count_sla":100,"sla":99,"error_rate":1,"upstream_error_rate":0,"duration":{"p50_ms":1200,"p95_ms":3200},"ttft":{"p50_ms":500,"p95_ms":1400}}}}`)
		case "/api/v1/admin/usage/stats":
			fmt.Fprint(w, `{"data":{"total_requests":100,"total_input_tokens":1000,"total_output_tokens":500,"total_cache_tokens":3500,"total_cache_creation_tokens":500,"total_cache_read_tokens":3000,"total_tokens":5000,"total_cost":1.5,"total_actual_cost":0.15,"total_account_cost":0.1,"average_duration_ms":1600}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	reader := newTestReader(t, server.URL)
	ops, err := reader.GetOpsSnapshot(context.Background(), OpsQuery{TimeRange: "24h", GroupID: 3})
	if err != nil || ops.Overview.TTFT.P95MS != 1400 || ops.Overview.SLA != 99 {
		t.Fatalf("GetOpsSnapshot = %#v, %v", ops, err)
	}
	usage, err := reader.GetUsageStats(context.Background(), UsageQuery{GroupID: 3, Period: "24h"})
	if err != nil || usage.TotalActualCost != 0.15 || usage.TotalAccountCost != 0.1 || !usage.CacheMetricsPresent ||
		usage.TotalCacheCreationTokens != 500 || usage.TotalCacheReadTokens != 3000 || usage.TotalTokens != 5000 {
		t.Fatalf("GetUsageStats = %#v, %v", usage, err)
	}
}

func TestReaderAddsAccountIDFilterToNativeAggregates(t *testing.T) {
	queries := make(map[string]string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/ops/dashboard/snapshot-v2":
			queries["ops"] = r.URL.Query().Get("account_id")
			fmt.Fprint(w, `{"data":{"generated_at":"2026-07-23T00:00:00Z","overview":{}}}`)
		case "/api/v1/admin/usage/stats":
			queries["usage"] = r.URL.Query().Get("account_id")
			fmt.Fprint(w, `{"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	reader := newTestReader(t, server.URL)
	if _, err := reader.GetOpsSnapshot(context.Background(), OpsQuery{TimeRange: "15m", AccountID: 42}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.GetUsageStats(context.Background(), UsageQuery{Period: "24h", AccountID: 42}); err != nil {
		t.Fatal(err)
	}
	if got, want := queries["ops"], "42"; got != want {
		t.Fatalf("ops account_id = %q, want %q", got, want)
	}
	if got, want := queries["usage"], "42"; got != want {
		t.Fatalf("usage account_id = %q, want %q", got, want)
	}
}

func TestReaderMarksMissingCacheUsageFieldsUnconfirmed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"total_requests":1,"total_input_tokens":10,"total_output_tokens":5,"total_cost":0.1,"total_actual_cost":0.1,"total_account_cost":0.01,"average_duration_ms":100}}`)
	}))
	defer server.Close()

	usage, err := newTestReader(t, server.URL).GetUsageStats(context.Background(), UsageQuery{Period: "24h"})
	if err != nil {
		t.Fatal(err)
	}
	if usage.CacheMetricsPresent {
		t.Fatalf("missing cache fields reported as confirmed: %#v", usage)
	}
}

func TestVerifyAdminSessionForwardsBearerAndRequiresAdmin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer browser-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if got := r.Header.Get("User-Agent"); got != "Mozilla/5.0 session-browser" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("X-Forwarded-For"); got != "203.0.113.9" {
			t.Errorf("X-Forwarded-For = %q", got)
		}
		if got := r.Header.Get("X-Real-IP"); got != "203.0.113.9" {
			t.Errorf("X-Real-IP = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("Cookie must not be forwarded: %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Errorf("admin key must not be forwarded: %q", got)
		}
		fmt.Fprint(w, `{"data":{"id":42,"role":"admin","status":"active"}}`)
	}))
	defer server.Close()
	identity, err := newTestReader(t, server.URL).VerifyAdminSession(context.Background(), adminauth.Session{
		Bearer:       "browser-token",
		UserAgent:    "Mozilla/5.0 session-browser",
		ForwardedFor: "203.0.113.9",
		RealIP:       "203.0.113.9",
	})
	if err != nil || identity.UserID != 42 {
		t.Fatalf("VerifyAdminSession = %#v, %v", identity, err)
	}
}

func TestReaderRedactsErrorBodiesAndCapsResponses(t *testing.T) {
	t.Parallel()

	t.Run("redacts body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"secret":"upstream-key-must-not-leak"}`)
		}))
		defer server.Close()
		_, err := newTestReader(t, server.URL).ListChannels(context.Background())
		if err == nil || strings.Contains(err.Error(), "upstream-key-must-not-leak") {
			t.Fatalf("error leaked body: %v", err)
		}
		if status, ok := HTTPStatus(err); !ok || status != http.StatusUnauthorized {
			t.Fatalf("HTTPStatus = %d, %v", status, ok)
		}
	})

	t.Run("caps body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"data":{"items":["`+strings.Repeat("x", maxResponseBytes)+`"]}}`)
		}))
		defer server.Close()
		_, err := newTestReader(t, server.URL).ListChannels(context.Background())
		if !IsResponseTooLarge(err) {
			t.Fatalf("error = %v, want response-too-large", err)
		}
	})

	t.Run("preserves context deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, err := newTestReader(t, server.URL).ListChannels(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want DeadlineExceeded", err)
		}
	})

	t.Run("rejects list schema drift", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"data":{"unexpected":[]}}`)
		}))
		defer server.Close()
		_, err := newTestReader(t, server.URL).ListChannels(context.Background())
		if !IsSchemaMismatch(err) {
			t.Fatalf("error = %v, want schema mismatch", err)
		}
	})
}

func TestHTTPReaderListAccountMonitorsDecodesNativeProjection(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/account-monitors" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "admin-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{
			"schema_version":2,
			"observed_at":"2026-07-25T07:00:00Z",
			"stale":false,
			"settings":{"interval_seconds":300,"updated_by":7,"updated_at":"2026-07-25T06:58:00Z"},
			"accounts":[{
				"account_id":11,
				"name":"账号 A",
				"platform":"openai",
				"account_type":"oauth",
				"status":"active",
				"schedulable":true,
				"group_ids":[3],
				"group_names":["GPT-Pro"],
				"model_id":"gpt-5.6-sol",
				"latest_status":"passed",
				"error_code":"",
				"sample_count":4,
				"success_rate":0.75,
				"ttft_p50_ms":150,
				"ttft_p95_ms":210,
				"latency_p95_ms":900,
				"multiplier":{"value":0.1,"source":"declared","status":"ok","observed_at":"2026-07-25T06:58:00Z"},
				"request_count":100,
				"error_count":2,
				"today_stats":{"requests":100,"tokens":3400,"cost":1.2,"standard_cost":2.4,"user_cost":1.8},
				"usage_windows":[
					{"name":"daily","utilization":0.42,"resets_at":"2026-07-26T00:00:00Z","requests":12,"tokens":340}
				],
				"latest":{"status":"success","ttft_ms":150,"latency_ms":740,"checked_at":"2026-07-25T06:59:00Z"},
				"checked_at":"2026-07-25T06:59:00Z",
				"stale":false
			},{
				"account_id":12,
				"name":"账号 B",
				"platform":"openai",
				"account_type":"oauth",
				"status":"active",
				"schedulable":true,
				"group_ids":[3],
				"group_names":["GPT-Pro"],
				"model_id":"gpt-5.6-sol",
				"latest_status":"passed",
				"error_code":"",
				"sample_count":4,
				"success_rate":0.95,
				"ttft_p50_ms":100,
				"ttft_p95_ms":120,
				"latency_p95_ms":500,
				"multiplier":{"value":0.08,"source":"measured","status":"ok","observed_at":"2026-07-25T06:58:00Z"},
				"request_count":60,
				"error_count":0,
				"usage_windows":[],
				"stale":false
			}]
		}}`)
	}))
	defer server.Close()

	projection, err := newTestReader(t, server.URL).ListAccountMonitors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || projection.SchemaVersion != 2 || projection.Settings.IntervalSeconds != 300 ||
		projection.ObservedAt.Format(time.RFC3339) != "2026-07-25T07:00:00Z" || projection.Stale {
		t.Fatalf("projection metadata = %#v", projection)
	}
	if projection.Settings.UpdatedBy != 7 || projection.Settings.UpdatedAt == nil ||
		projection.Settings.UpdatedAt.Format(time.RFC3339) != "2026-07-25T06:58:00Z" {
		t.Fatalf("settings = %#v", projection.Settings)
	}
	if len(projection.Accounts) != 2 || projection.Accounts[1].AccountID != 12 {
		t.Fatalf("accounts = %#v", projection.Accounts)
	}
	if projection.Accounts[0].AccountType != "oauth" || projection.Accounts[0].TodayStats == nil ||
		projection.Accounts[0].TodayStats.Tokens != 3400 {
		t.Fatalf("account usage projection = %#v", projection.Accounts[0])
	}
	if len(projection.Accounts[0].UsageWindows) != 1 {
		t.Fatalf("usage windows = %#v", projection.Accounts[0].UsageWindows)
	}
	window := projection.Accounts[0].UsageWindows[0]
	if window.Name != "daily" || window.Utilization != 0.42 || window.ResetsAt == nil || window.Requests != 12 || window.Tokens != 340 {
		t.Fatalf("usage window = %#v", window)
	}
	if projection.Accounts[0].Latest == nil || projection.Accounts[0].Latest.Status != "success" ||
		projection.Accounts[0].CheckedAt == nil || projection.Accounts[1].CheckedAt != nil {
		t.Fatalf("latest/check times = %#v / %#v", projection.Accounts[0], projection.Accounts[1])
	}
	if projection.Accounts[1].TTFTP95MS == nil || *projection.Accounts[1].TTFTP95MS != 120 {
		t.Fatalf("ttft = %#v", projection.Accounts[1].TTFTP95MS)
	}
	if projection.Accounts[0].Multiplier.Value == nil || *projection.Accounts[0].Multiplier.Value != 0.1 ||
		projection.Accounts[0].Multiplier.Source != "declared" ||
		projection.Accounts[1].Multiplier.Source != "measured" {
		t.Fatalf("multipliers = %#v / %#v", projection.Accounts[0].Multiplier, projection.Accounts[1].Multiplier)
	}
}

func TestHTTPReaderListAccountMonitorsRejectsSchemaDriftAndSecretKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing accounts",
			body: `{"data":{"schema_version":2,"observed_at":"2026-07-25T07:00:00Z","stale":false,"settings":{"interval_seconds":300}}}`,
		},
		{
			name: "wrong schema version",
			body: `{"data":{"schema_version":1,"observed_at":"2026-07-25T07:00:00Z","stale":false,"settings":{"interval_seconds":300},"accounts":[]}}`,
		},
		{
			name: "unknown field",
			body: `{"data":{"schema_version":2,"observed_at":"2026-07-25T07:00:00Z","stale":false,"settings":{"interval_seconds":300},"accounts":[],"unexpected":true}}`,
		},
		{
			name: "secret-shaped key",
			body: `{"data":{"schema_version":2,"observed_at":"2026-07-25T07:00:00Z","stale":false,"settings":{"interval_seconds":300},"accounts":[],"api_key":"must-not-leak"}}`,
		},
		{
			name: "ok multiplier without value",
			body: `{"data":{"schema_version":2,"observed_at":"2026-07-25T07:00:00Z","stale":false,"settings":{"interval_seconds":300},"accounts":[{
				"account_id":11,"name":"A","platform":"openai","account_type":"api_key","status":"active","schedulable":true,
				"group_ids":[],"group_names":[],"model_id":"gpt-5.4","latest_status":"success","sample_count":0,
				"success_rate":0,"multiplier":{"source":"declared","status":"ok","observed_at":"2026-07-25T06:58:00Z"},
				"request_count":0,"error_count":0,"usage_windows":[],"stale":false
			}]}}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			_, err := newTestReader(t, server.URL).ListAccountMonitors(context.Background())
			if !IsSchemaMismatch(err) {
				t.Fatalf("error = %v, want schema mismatch", err)
			}
		})
	}
}

func newTestReader(t *testing.T, baseURL string) *HTTPReader {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "admin-key")
	if err := os.WriteFile(keyFile, []byte("admin-test-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := NewHTTPReader(baseURL, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}
