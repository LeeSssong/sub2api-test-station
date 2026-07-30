package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestRetiredOpsAndAcknowledgementRoutesAreNotMounted(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/ops"},
		{http.MethodGet, "/relay-ops/api/ops-view"},
		{http.MethodPost, "/relay-ops/api/incidents/ack"},
		{http.MethodPost, "/relay-ops/api/feishu/events"},
		{http.MethodGet, "/relay-ops/static/ops.js"},
		{http.MethodGet, "/relay-ops/static/ops-admin.js"},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d want=404", tt.method, tt.path, recorder.Code)
		}
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

func newTestServer(_ fakeOps) http.Handler {
	server, err := NewServer(Dependencies{
		BaseOrigin: "https://api.example.com", Pricing: fakePricing{},
	})
	if err != nil {
		panic(err)
	}
	return server
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
