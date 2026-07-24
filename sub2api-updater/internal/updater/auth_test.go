package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOfficialAuthenticatorForwardsSessionContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "updater-test" {
			t.Fatalf("user agent = %q", got)
		}
		if got := r.Header.Get("X-Forwarded-For"); got != "203.0.113.8" {
			t.Fatalf("forwarded for = %q", got)
		}
		if got := r.Header.Get("X-Real-IP"); got != "203.0.113.9" {
			t.Fatalf("real ip = %q", got)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":42,"role":"admin","status":"active"}}`))
	}))
	defer server.Close()

	auth, err := NewOfficialAuthenticator(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("User-Agent", "updater-test")
	headers.Set("X-Forwarded-For", "203.0.113.8")
	headers.Set("X-Real-IP", "203.0.113.9")
	identity, err := auth.Verify(context.Background(), "session-token", headers)
	if err != nil {
		t.Fatal(err)
	}
	if identity != (Identity{ID: 42, Role: "admin", Status: "active"}) {
		t.Fatalf("identity = %#v", identity)
	}
}
