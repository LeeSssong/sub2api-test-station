package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestD04ReadinessOpsSectionIsAuthenticatedAndReadOnly(t *testing.T) {
	t.Parallel()

	source := readinessOpsSource{view: OpsView{
		NativeMonitorURL: "/monitor",
		RefreshedAt:      "2026-07-22 10:01 UTC",
		D04LaunchReadiness: D04LaunchReadinessView{
			Available: true, Decision: "NO-GO", SnapshotID: "snapshot-1",
			AccountSetSHA256: strings.Repeat("a", 64), EvaluatedAt: "2026-07-22 10:00 UTC",
			Upstreams: []D04LaunchReadinessUpstreamView{{
				AccountID: "7", DisplayName: "Account", Groups: "2", Runtime: "可用",
				Balance: "9.99 USD", FinancialAge: "2026-07-22 10:00 UTC",
				Quality: "成功 99.0%", Samples: 30, Blockers: "余额低于最低门槛",
				BlockerCodes: "upstream_balance_below_minimum",
			}},
		},
	}}
	server, err := NewServer(Dependencies{
		BaseOrigin: "https://api.example.com", Auth: fakeVerifier{}, Pricing: fakePricing{}, Ops: source,
	})
	if err != nil {
		t.Fatal(err)
	}

	anonymous := httptest.NewRecorder()
	server.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/ops", nil))
	if strings.Contains(anonymous.Body.String(), "upstream_balance_below_minimum") || strings.Contains(anonymous.Body.String(), "snapshot-1") {
		t.Fatal("bootstrap leaked readiness evidence")
	}

	request := httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, required := range []string{"内测开放状态", "暂不可开放", "当前活动上游", "账号", "余额", "质量", "余额低于最低门槛", "技术详情", `class="d04-readiness-table"`, `class="d04-blockers"`, `title="upstream_balance_below_minimum"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing %q", required)
		}
	}
	start := strings.Index(body, "内测开放状态")
	section := body[start:]
	for _, prohibited := range []string{"<button", "<form", "POST"} {
		if strings.Contains(section, prohibited) {
			t.Fatalf("D04 section contains %q", prohibited)
		}
	}
}

type readinessOpsSource struct{ view OpsView }

func (s readinessOpsSource) Snapshot(_ context.Context) (OpsView, error) { return s.view, nil }
