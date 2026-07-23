package httpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/publicsettings"
	"example.invalid/internal-test-service/internal/registration"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
	"example.invalid/internal-test-service/internal/testsupport"
)

type httpFixture struct {
	server *Server
	store  *store.Store
	fake   *testsupport.Fake
}

func newHTTPTest(t *testing.T) *httpFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake := testsupport.NewFake()
	fake.AddUser(sub2api.User{ID: 1})
	if err := st.RegisterUser(ctx, store.InternalUser{UserID: 1, JoinedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	credit := &credits.Service{
		Store: st, Provider: fake, Timezone: loc, TotalBudget: 100_000_000,
		DailyLoginCredit: domain.DailyLoginCredit, CostMultiplierBPS: 700,
		CostPolicyID: "test-policy", CostPolicyQualified: true, Mode: "write",
	}
	forward := func(_ context.Context, endpoint string, body []byte, headers http.Header) (authproxy.Response, error) {
		userID := int64(1)
		status := http.StatusOK
		if endpoint == authproxy.RegisterEndpoint {
			userID = 2
			status = http.StatusCreated
			fake.AddUser(sub2api.User{ID: userID})
		}
		return authproxy.Response{
			Status: status,
			Header: http.Header{
				"Content-Type":  {"application/json"},
				"Set-Cookie":    {"session=fixture; HttpOnly"},
				"Authorization": {"Bearer replacement"},
				"Cache-Control": {"public, max-age=3600"},
			},
			Body: []byte(`{"code":0,"data":{"user":{"id":` + string(rune('0'+userID)) + `},"access_token":"fixture"}}`),
		}, nil
	}
	grant := func(ctx context.Context, userID int64, now time.Time) error {
		_, err := credit.GrantDailyLogin(ctx, userID, now)
		return err
	}
	reg := &registration.Service{
		Store: st, MaxUsers: 15, Mode: "write", RegistrationOpen: true,
		AuthForward: forward, GrantDailyLogin: grant, CanGrantDaily: func(context.Context) (bool, error) { return true, nil },
		Now: func() time.Time { return time.Date(2026, 7, 21, 2, 0, 0, 0, time.UTC) },
	}
	auth := &authproxy.Service{
		Forward: forward,
		IsLaunchUser: func(ctx context.Context, userID int64) (bool, error) {
			_, err := st.GetInternalUser(ctx, userID)
			return err == nil, nil
		},
		GrantDailyLogin: grant,
		Now:             func() time.Time { return time.Date(2026, 7, 21, 2, 0, 0, 0, time.UTC) },
	}
	settings := &publicsettings.Service{
		Forward: func(context.Context, http.Header) (authproxy.Response, error) {
			return authproxy.Response{
				Status: http.StatusOK,
				Header: http.Header{"Content-Type": {"application/json"}},
				Body:   []byte(`{"code":0,"data":{"registration_enabled":true,"invitation_code_enabled":true,"affiliate_enabled":true,"site_name":"Relay"}}`),
			}, nil
		},
		EffectiveRegistrationOpen: reg.EffectiveRegistrationOpen,
	}
	srv, err := NewServer(reg, auth, settings)
	if err != nil {
		t.Fatal(err)
	}
	return &httpFixture{server: srv, store: st, fake: fake}
}

func TestNativeAuthRoutesPreserveStatusBodyAndAuthHeaders(t *testing.T) {
	fx := newHTTPTest(t)
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: authproxy.RegisterEndpoint, wantStatus: http.StatusCreated},
		{path: authproxy.LoginEndpoint, wantStatus: http.StatusOK},
		{path: authproxy.Login2FAEndpoint, wantStatus: http.StatusOK},
	} {
		t.Run(test.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"email":"user@example.com","password":"fixture"}`))
			req.Header.Set("Origin", "http://example.com")
			req.Header.Set("Content-Type", "application/json")
			fx.server.ServeHTTP(rr, req)
			if rr.Code != test.wantStatus || !strings.Contains(rr.Body.String(), `"access_token":"fixture"`) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Set-Cookie") == "" || rr.Header().Get("Authorization") != "Bearer replacement" {
				t.Fatalf("headers=%v", rr.Header())
			}
			if values := rr.Header().Values("Cache-Control"); len(values) != 1 || values[0] != "no-store" {
				t.Fatalf("authentication response is cacheable: %v", values)
			}
		})
	}
	balance, err := fx.fake.GetBalance(context.Background(), 2)
	if err != nil || balance.Balance != domain.DailyLoginCredit {
		t.Fatalf("registration credit=%+v err=%v", balance, err)
	}
}

func TestPublicSettingsOverlayUsesEffectiveRegistrationState(t *testing.T) {
	fx := newHTTPTest(t)
	rr := httptest.NewRecorder()
	fx.server.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"registration_enabled":true`) ||
		!strings.Contains(rr.Body.String(), `"invitation_code_enabled":false`) ||
		!strings.Contains(rr.Body.String(), `"affiliate_enabled":false`) ||
		!strings.Contains(rr.Body.String(), `"site_name":"Relay"`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAuthRequestBodyIsLimitedToOneMiB(t *testing.T) {
	fx := newHTTPTest(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, authproxy.LoginEndpoint, bytes.NewReader(bytes.Repeat([]byte("x"), (1<<20)+1)))
	req.Header.Set("Origin", "http://example.com")
	fx.server.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), "REQUEST_TOO_LARGE") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRetiredLaunchEndpointsAreGone(t *testing.T) {
	fx := newHTTPTest(t)
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/internal-test/join/legacy"},
		{http.MethodGet, "/internal-test/api/join/legacy"},
		{http.MethodPost, "/internal-test/api/invitations"},
		{http.MethodPost, "/internal-test/api/checkin"},
	} {
		rr := httptest.NewRecorder()
		fx.server.ServeHTTP(rr, httptest.NewRequest(test.method, test.path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s %s=%d body=%s", test.method, test.path, rr.Code, rr.Body.String())
		}
	}
}

func TestAuthRoutesRejectUnexpectedMethods(t *testing.T) {
	fx := newHTTPTest(t)
	for _, path := range []string{authproxy.RegisterEndpoint, authproxy.LoginEndpoint, authproxy.Login2FAEndpoint} {
		rr := httptest.NewRecorder()
		fx.server.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusMethodNotAllowed || rr.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("GET %s=%d headers=%v", path, rr.Code, rr.Header())
		}
	}
}

func TestHealthReportsSchedulerCompletionWithoutErrorDetails(t *testing.T) {
	fx := newHTTPTest(t)
	fx.server.SchedulerStatus = func() (time.Time, bool) {
		return time.Date(2026, 7, 21, 10, 2, 0, 0, time.UTC), true
	}
	rr := httptest.NewRecorder()
	fx.server.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"scheduler_status":"ok"`) || !strings.Contains(rr.Body.String(), `"scheduler_last_tick":"2026-07-21T10:02:00Z"`) {
		t.Fatalf("health %d %s", rr.Code, rr.Body.String())
	}
}
