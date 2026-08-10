package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminTreatsInactiveSessionAsLocalUnauthorized(t *testing.T) {
	t.Parallel()
	client := authClientFunc(func(_ context.Context, bearer, clientIP, origin string) (AdminIdentity, error) {
		if bearer != "browser-session" || clientIP != "192.0.2.9" || origin != "https://api.example.test" {
			t.Fatalf("Me arguments = %q %q %q", bearer, clientIP, origin)
		}
		return AdminIdentity{UserID: 9, Role: "admin", Status: "inactive"}, nil
	})
	h := RequireAdmin(client, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next called") }))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/xingqiao/accounts/monitor", nil)
	req.RemoteAddr = "192.0.2.9:443"
	req.Header.Set("Authorization", "Bearer browser-session")
	req.Header.Set("Origin", "https://api.example.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("control-plane rejection must not set logout cookie: %q", rec.Header().Get("Set-Cookie"))
	}
}

type authClientFunc func(context.Context, string, string, string) (AdminIdentity, error)

func (f authClientFunc) Me(ctx context.Context, bearer, clientIP, origin string) (AdminIdentity, error) {
	return f(ctx, bearer, clientIP, origin)
}
