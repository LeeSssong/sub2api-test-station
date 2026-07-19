package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
)

func TestPricingIsAnonymousFilteredAndDoesNotLeakUpstreamCosts(t *testing.T) {
	t.Parallel()
	server := newTestServer()
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

func TestOpsAndCandidateAPIsRequireAdminAndCSRF(t *testing.T) {
	t.Parallel()
	server := newTestServer()
	request := httptest.NewRequest(http.MethodGet, "/ops", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "/relay-ops/static/ops.js") {
		t.Fatalf("ops bootstrap=%d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "GPT-Pro") || strings.Contains(recorder.Body.String(), "公开分组") {
		t.Fatalf("ops bootstrap leaked protected data: %s", recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous ops view=%d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/relay-ops/api/ops-view", nil)
	request.Header.Set("Authorization", "Bearer admin")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "公开分组") || !strings.Contains(recorder.Body.String(), "/monitor") {
		t.Fatalf("ops=%d %s", recorder.Code, recorder.Body.String())
	}

	payload := `{"name":"candidate","base_url":"https://candidate.example/v1","pricing_url":"https://candidate.example/pricing","usage_url":"https://candidate.example/usage","probe_key_file":"/run/secrets/candidate"}`
	request = httptest.NewRequest(http.MethodPost, "/relay-ops/api/candidates", strings.NewReader(payload))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing origin=%d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/relay-ops/api/candidates", strings.NewReader(payload))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://api.example.com")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("candidate create=%d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "probe_key") || strings.Contains(recorder.Body.String(), "/run/secrets") {
		t.Fatalf("secret ref leaked: %s", recorder.Body.String())
	}
}

func TestOpsBootstrapUsesExistingSub2APITokenWithoutExposingIt(t *testing.T) {
	t.Parallel()
	server := newTestServer()
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/relay-ops/static/ops.js", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	script := recorder.Body.String()
	for _, required := range []string{`localStorage.getItem('auth_token')`, `Authorization`, `Bearer`, `/relay-ops/api/ops-view`} {
		if !strings.Contains(script, required) {
			t.Fatalf("missing %q in ops bootstrap", required)
		}
	}
	for _, leak := range []string{"console.log", "document.cookie", "location.search"} {
		if strings.Contains(script, leak) {
			t.Fatalf("unsafe token handling %q", leak)
		}
	}
}

func TestNoPerformanceRouteAndResponsiveStatesExist(t *testing.T) {
	t.Parallel()
	server := newTestServer()
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

func newTestServer() http.Handler {
	verifier := fakeVerifier{}
	candidatesService := &fakeCandidates{}
	server, err := NewServer(Dependencies{BaseOrigin: "https://api.example.com", Auth: verifier, Pricing: fakePricing{}, Ops: fakeOps{}, Candidates: candidatesService})
	if err != nil {
		panic(err)
	}
	return server
}

type fakeVerifier struct{}

func (fakeVerifier) VerifyAdminSession(_ context.Context, bearer string) (adminauth.Identity, error) {
	if bearer == "admin" {
		return adminauth.Identity{UserID: 42, Role: "admin", Status: "active"}, nil
	}
	return adminauth.Identity{}, nil
}

type fakePricing struct{}

func (fakePricing) PublicPricing(context.Context) ([]PublicGroup, error) {
	return []PublicGroup{{Name: "GPT-Pro", UpdatedAt: "2026-07-19 12:00", Models: []PublicModel{{ModelID: "gpt-5.6-sol", Tier: ">272k", Input: "1.25", Output: "10.00", CacheRead: "0.125"}}}}, nil
}

type fakeOps struct{}

func (fakeOps) Snapshot(context.Context) (OpsView, error) {
	return OpsView{PublicGroups: []string{"GPT-Pro"}, NativeMonitorURL: "/monitor", Candidates: []CandidateView{}}, nil
}

type fakeCandidates struct{ created bool }

func (f *fakeCandidates) List(context.Context, domain.AdminActor) ([]candidates.Candidate, error) {
	return nil, nil
}
func (f *fakeCandidates) Create(_ context.Context, _ domain.AdminActor, input candidates.CandidateInput) (candidates.Candidate, error) {
	f.created = true
	return candidates.Candidate{ID: 17, Name: input.Name, BaseURL: input.BaseURL, Enabled: true}, nil
}
func (f *fakeCandidates) Disable(context.Context, domain.AdminActor, domain.UpstreamID) error {
	return nil
}

var _ = json.Valid
