package sub2api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
			fmt.Fprint(w, `{"data":{"total_requests":100,"total_input_tokens":1000,"total_output_tokens":500,"total_cost":1.5,"total_actual_cost":0.15,"total_account_cost":0.1,"average_duration_ms":1600}}`)
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
	if err != nil || usage.TotalActualCost != 0.15 || usage.TotalAccountCost != 0.1 {
		t.Fatalf("GetUsageStats = %#v, %v", usage, err)
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
