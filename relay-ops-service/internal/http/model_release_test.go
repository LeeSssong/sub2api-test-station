package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/accountquality"
)

func TestOpsShowsReadOnlyAccountPoolQuality(t *testing.T) {
	t.Parallel()

	server := newTestServer(fakeOps{view: OpsView{
		NativeMonitorURL: "/monitor",
		AccountQuality: accountquality.View{
			Available: true, ObservedAt: "2026-07-23T00:00:00Z", AccountSetSHA256: strings.Repeat("a", 64),
			Accounts: []accountquality.AccountView{{AccountID: "21", ModelID: "gpt-5.6-sol", Stability: "成功 3/4", SuccessRate: "75.0%", TTFTP50: "150ms", TTFTP95: "210ms", LastResult: "通过"}},
		},
	}})
	request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, required := range []string{"上游账号质量", "稳定性", "成功 3/4", "TTFT P95", "210ms", "账号 21", strings.Repeat("a", 64)} {
		if !strings.Contains(body, required) {
			t.Fatalf("ops body missing %q", required)
		}
	}
	for _, prohibited := range []string{"倍率", "模型版本", "<form", "<input", "<select", "<textarea", "<button", "Base URL", "API Key", "确认替换", "立即升级"} {
		if strings.Contains(body, prohibited) {
			t.Fatalf("ops body contains forbidden value %q", prohibited)
		}
	}
}

func TestOpsShowsUnavailableAndStaleAccountQualityWithoutWriteControls(t *testing.T) {
	t.Parallel()

	for _, view := range []accountquality.View{{}, {Available: true, Stale: true, ObservedAt: "2026-07-22T23:00:00Z"}} {
		server := newTestServer(fakeOps{view: OpsView{NativeMonitorURL: "/monitor", AccountQuality: view}})
		request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
		request.Header.Set("Authorization", "Bearer admin")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "上游账号质量") {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
}
