package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/opsmetrics"
)

func TestPricingIsAnonymousFilteredAndDoesNotLeakUpstreamCosts(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, value := range []string{"GPT-Pro", "gpt-5.6-sol", "$1.25", "$10.00", "272k"} {
		if !strings.Contains(body, value) {
			t.Fatalf("missing %q", value)
		}
	}
	for _, leak := range []string{"Neko", "0.10x", "actual_cost", "secret"} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %q", leak)
		}
	}
}

func TestOpsProjectionIsHiddenAndRetiredMutationRoutesAreNotFound(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})

	bootstrap := httptest.NewRecorder()
	server.ServeHTTP(bootstrap, httptest.NewRequest(http.MethodGet, "/ops", nil))
	if bootstrap.Code != http.StatusOK || !strings.Contains(bootstrap.Body.String(), "/relay-ops/static/ops.js") {
		t.Fatalf("ops bootstrap=%d %s", bootstrap.Code, bootstrap.Body.String())
	}
	for _, protected := range []string{"内测开放状态", "GPT-Pro", "当前活动上游", "account_set_sha256"} {
		if strings.Contains(bootstrap.Body.String(), protected) {
			t.Fatalf("bootstrap leaked %q", protected)
		}
	}

	for _, bearer := range []string{"", "invalid", "user", "disabled"} {
		request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("bearer=%q status=%d want=404", bearer, recorder.Code)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin ops=%d %s", recorder.Code, recorder.Body.String())
	}

	retired := []string{
		"/relay-ops/api/candidates",
		"/relay-ops/api/candidates/17/disable",
		"/relay-ops/api/upstreams",
		"/relay-ops/api/upstreams/18/disable",
		"/relay-ops/api/upstreams/18/billing-session",
		"/relay-ops/api/acceptance/synthetic",
		"/relay-ops/api/acceptance/daily-report",
		"/relay-ops/api/quality-reports/report-1/preview",
	}
	for _, path := range retired {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer admin")
		request.Header.Set("Origin", "https://api.example.com")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("retired route %s status=%d want=404", path, recorder.Code)
		}
	}
}

func TestOpsPageIsReadOnlyPlainLanguageAndAutoRefreshes(t *testing.T) {
	t.Parallel()
	view := OpsView{
		NativeMonitorURL: "/monitor",
		RefreshedAt:      "2026-07-22 16:30 UTC",
		SiteRuntime: opsmetrics.Snapshot{
			Groups: []opsmetrics.GroupRuntime{
				{ID: 2, Name: "公开分组 A", RequestCount: 42, ErrorRate: 0.075, SLA: 97.5, TTFTP95MS: 220, DurationP95MS: 780, Status: opsmetrics.StatusOK},
				{ID: 4, Name: "公开分组 B", Status: opsmetrics.StatusReadFailed, ErrorCode: opsmetrics.ErrorCodeOpsSnapshotUnavailable},
			},
			Accounts: []opsmetrics.AccountRuntime{{ID: 10, Name: "当前账号 A", PublicGroupNames: []string{"公开分组 A", "公开分组 B"}, RequestCount: 8, ErrorRate: 0.2, SLA: 99.5, TTFTP95MS: 350, DurationP95MS: 1_200, Status: opsmetrics.StatusSampleInsufficient}},
		},
	}
	server := newTestServer(fakeOps{view: view})
	request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ops status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, required := range []string{"站内运行", "Sub2API 原生聚合，最近 15 分钟", "公开分组", "当前调度账号", "错误率", "TTFT P95", "总耗时 P95", "读取失败", "样本不足", "公开分组 A", "公开分组 B", "当前账号 A", "7.50%", "97.50%", "20.00%", "99.50%", "/monitor"} {
		if !strings.Contains(body, required) {
			t.Fatalf("ops missing %q", required)
		}
	}
	for _, required := range []string{`id="modeloc-reminder"`, "MODELOC 真实性报告尚未配置", "/home-assets/site-config.json"} {
		if !strings.Contains(body, required) {
			t.Fatalf("ops missing MODELOC reminder %q", required)
		}
	}
	for _, prohibited := range []string{"<form", "<input", "<select", "<textarea", "<button", "录入生产上游", "配置用量读取会话", "录入候选上游", "独立低额度监测 Key", "Base URL", "API Key", "切换上游", "确认切换", "发送测试告警", "预览变更"} {
		if strings.Contains(body, prohibited) {
			t.Fatalf("ops contains retired control %q", prohibited)
		}
	}

	adminScript := httptest.NewRecorder()
	server.ServeHTTP(adminScript, httptest.NewRequest(http.MethodGet, "/relay-ops/static/ops-admin.js", nil))
	script := adminScript.Body.String()
	for _, required := range []string{"30000", "/relay-ops/api/ops-view", "Authorization", "Bearer", "/404", "cache: 'no-store'", "/home-assets/site-config.json", "thirdPartyReports", "MODELOC", "https:"} {
		if !strings.Contains(script, required) {
			t.Fatalf("admin script missing %q", required)
		}
	}
	for _, required := range []string{"config.version !== 1", "report.id", "report.title", "['verified', 'reference', 'archived']", "report.status"} {
		if !strings.Contains(script, required) {
			t.Fatalf("admin script is missing the MODELOC schema guard %q", required)
		}
	}
	for _, prohibited := range []string{"/relay-ops/api/candidates", "/relay-ops/api/upstreams", "/relay-ops/api/acceptance", "/preview", "probe_key", "window.confirm", "console.log"} {
		if strings.Contains(script, prohibited) {
			t.Fatalf("admin script contains retired behavior %q", prohibited)
		}
	}

	bootstrapScript := httptest.NewRecorder()
	server.ServeHTTP(bootstrapScript, httptest.NewRequest(http.MethodGet, "/relay-ops/static/ops.js", nil))
	bootstrapJS := bootstrapScript.Body.String()
	for _, required := range []string{`localStorage.getItem('auth_token')`, "/relay-ops/api/ops-view", "Authorization", "Bearer", "/404"} {
		if !strings.Contains(bootstrapJS, required) {
			t.Fatalf("bootstrap script missing %q", required)
		}
	}
	for _, prohibited := range []string{"/login?", "document.cookie", "location.search", "console.log"} {
		if strings.Contains(bootstrapJS, prohibited) {
			t.Fatalf("bootstrap script contains %q", prohibited)
		}
	}
}

func TestOpsPageDoesNotFabricateMetricsForZeroRuntimeSamples(t *testing.T) {
	t.Parallel()

	view := OpsView{SiteRuntime: opsmetrics.Snapshot{
		Groups: []opsmetrics.GroupRuntime{{
			ID: 2, Name: "空分组", RequestCount: 0, SuccessCount: 0, Status: opsmetrics.StatusSampleInsufficient,
		}, {
			ID: 3, Name: "全失败分组", RequestCount: 3, SuccessCount: 0, ErrorRate: 1, SLA: 50,
			DurationP95MS: 500, Status: opsmetrics.StatusSampleInsufficient,
		}},
	}}
	server := newTestServer(fakeOps{view: view})
	request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, required := range []string{"空分组", "全失败分组", "未知", "无成功样本"} {
		if !strings.Contains(body, required) {
			t.Fatalf("ops missing %q: %s", required, body)
		}
	}
	if strings.Contains(body, ">0.00%<") || strings.Contains(body, ">0ms<") {
		t.Fatalf("ops fabricated zero metrics: %s", body)
	}
}

func TestNoPerformanceRouteAndResponsivePricingStateExist(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/performance", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("performance status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing?q=missing", nil))
	if !strings.Contains(recorder.Body.String(), "没有匹配的模型") {
		t.Fatalf("empty state missing: %s", recorder.Body.String())
	}
}

func newTestServer(ops fakeOps) http.Handler {
	server, err := NewServer(Dependencies{BaseOrigin: "https://api.example.com", Auth: fakeVerifier{}, Pricing: fakePricing{}, Ops: ops})
	if err != nil {
		panic(err)
	}
	return server
}

type fakeVerifier struct{}

func (fakeVerifier) VerifyAdminSession(_ context.Context, session adminauth.Session) (adminauth.Identity, error) {
	switch session.Bearer {
	case "admin":
		return adminauth.Identity{UserID: 42, Role: "admin", Status: "active"}, nil
	case "user":
		return adminauth.Identity{UserID: 43, Role: "user", Status: "active"}, nil
	case "disabled":
		return adminauth.Identity{UserID: 44, Role: "admin", Status: "disabled"}, nil
	default:
		return adminauth.Identity{}, context.Canceled
	}
}

type fakePricing struct{}

func (fakePricing) PublicPricing(context.Context) ([]PublicGroup, error) {
	return []PublicGroup{{Name: "GPT-Pro", UpdatedAt: "2026-07-19 12:00", Models: []PublicModel{{ModelID: "gpt-5.6-sol", Tier: ">272k", Input: "1.25", Output: "10.00", CacheRead: "0.125"}}}}, nil
}

type fakeOps struct{ view OpsView }

func (source fakeOps) Snapshot(context.Context) (OpsView, error) {
	if source.view.NativeMonitorURL == "" {
		source.view.NativeMonitorURL = "/monitor"
	}
	return source.view, nil
}
