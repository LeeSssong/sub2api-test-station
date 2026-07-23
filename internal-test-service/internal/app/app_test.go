package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/config"
)

func TestNewWiresConfiguredAppBotAlerter(t *testing.T) {
	dir := t.TempDir()
	key := writeAppTestFile(t, dir, "admin-key", "admin")
	appID := writeAppTestFile(t, dir, "app-id", "cli-test")
	appSecret := writeAppTestFile(t, dir, "app-secret", "secret")
	chatID := writeAppTestFile(t, dir, "chat-id", "oc-test")
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cfg := config.Config{
		MaxUsers: 15, TotalBudget: 100_000_000, Timezone: loc, TimezoneName: "Asia/Shanghai",
		Sub2APIURL: "http://127.0.0.1:1", AdminAPIKeyFile: key, Mode: "read_only",
		DataPath: filepath.Join(dir, "state.db"), ListenAddress: ":8090",
		FeishuBaseURL: "https://open.feishu.cn", FeishuAppIDFile: appID,
		FeishuAppSecretFile: appSecret, FeishuAlertChatIDFile: chatID,
	}
	instance, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if instance.Scheduler.Alerter == nil {
		t.Fatal("configured App Bot alerter was not wired")
	}
	if instance.Scheduler.Reporter != nil {
		t.Fatal("unqualified read-only cost policy must not emit budget reports")
	}
}

func TestNewWiresBoundedNativeAuthAndPublicSettingsProxy(t *testing.T) {
	var paths []string
	forwardedHeaders := http.Header{
		"X-Forwarded-For":   {"203.0.113.24"},
		"X-Forwarded-Proto": {"https"},
		"X-Forwarded-Host":  {"api.example.test"},
	}
	incomingHeaders := http.Header{}
	for key, values := range forwardedHeaders {
		incomingHeaders[key] = slices.Clone(values)
	}
	incomingHeaders.Set("X-Real-IP", "203.0.113.24")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=native; HttpOnly")
		w.Header().Set("Authorization", "Bearer native")
		switch r.URL.Path {
		case "/api/v1/auth/login", "/api/v1/auth/login/2fa":
			for key, want := range forwardedHeaders {
				if got := r.Header.Values(key); !slices.Equal(got, want) {
					t.Fatalf("%s = %v, want %v", key, got, want)
				}
			}
			if got := r.Header.Values("X-Real-IP"); len(got) != 0 {
				t.Fatalf("X-Real-IP = %v, want absent", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "fixture-password") {
				t.Fatalf("auth body changed")
			}
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"code":0,"data":{"user":{"id":7}}}`)
		case "/api/v1/settings/public":
			fmt.Fprint(w, `{"code":0,"data":{"registration_enabled":true,"invitation_code_enabled":true,"affiliate_enabled":true,"site_name":"Relay"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	dir := t.TempDir()
	key := writeAppTestFile(t, dir, "admin-key", "admin")
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cfg := config.Config{
		MaxUsers: 15, TotalBudget: 100_000_000, RegistrationOpen: true,
		DailyLoginCredit: 20_000_000, Timezone: loc, TimezoneName: "Asia/Shanghai",
		Sub2APIURL: upstream.URL, AdminAPIKeyFile: key, Mode: "read_only",
		DataPath: filepath.Join(dir, "state.db"), ListenAddress: ":8090",
	}
	instance, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()

	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/login/2fa"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"email":"user@example.com","password":"fixture-password"}`))
		req.Header.Set("Origin", "https://example.com")
		for key, values := range incomingHeaders {
			req.Header.Del(key)
			for _, value := range slices.Clone(values) {
				req.Header.Add(key, value)
			}
		}
		instance.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted || rr.Header().Get("Set-Cookie") == "" || rr.Header().Get("Authorization") != "Bearer native" {
			t.Fatalf("%s status=%d headers=%v body=%s", path, rr.Code, rr.Header(), rr.Body.String())
		}
	}
	rr := httptest.NewRecorder()
	instance.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"registration_enabled":false`) || !strings.Contains(rr.Body.String(), `"site_name":"Relay"`) {
		t.Fatalf("settings status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(paths) != 3 {
		t.Fatalf("paths=%v", paths)
	}
}

func TestNativeForwarderReturnsRedirectWithoutReplayingCredentials(t *testing.T) {
	redirected := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("credentials were replayed to a redirect target")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()

	response, err := forwardRequest(context.Background(), upstream.URL, http.MethodPost, authproxy.LoginEndpoint,
		[]byte(`{"password":"fixture"}`), http.Header{"Authorization": {"Bearer fixture"}, "Cookie": {"session=fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusTemporaryRedirect || redirected != 0 {
		t.Fatalf("status=%d redirected=%d", response.Status, redirected)
	}
}

func writeAppTestFile(t *testing.T, dir, name, value string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHealthcheckNormalizesListenAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := Healthcheck(t.Context(), ":"+port); err != nil {
		t.Fatal(err)
	}
}
