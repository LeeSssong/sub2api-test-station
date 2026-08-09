package adminauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestRequireAdminAuthenticatesThroughSub2API(t *testing.T) {
	t.Parallel()

	verifier := &fakeVerifier{identity: Identity{UserID: 42, Role: "admin", Status: "active"}}
	handler := RequireAdmin(verifier, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFromContext(r.Context())
		if !ok || actor.UserID != 42 {
			t.Fatalf("actor = %#v, %v", actor, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ops", nil)
	req.Header.Set("Authorization", "Bearer browser-token")
	req.Header.Set("User-Agent", "Mozilla/5.0 session-browser")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "203.0.113.9")
	req.Header.Set("Cookie", "must-not-be-forwarded=1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d", recorder.Code)
	}
	if verifier.session.Bearer != "browser-token" || verifier.session.UserAgent != "Mozilla/5.0 session-browser" || verifier.session.ForwardedFor != "203.0.113.9" || verifier.session.RealIP != "203.0.113.9" {
		t.Fatalf("session = %#v", verifier.session)
	}
}

func TestRequireAdminRejectsMissingAndNonAdminSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity Identity
		bearer   string
		want     int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "user", bearer: "user-token", identity: Identity{UserID: 7, Role: "user", Status: "active"}, want: http.StatusForbidden},
		{name: "disabled admin", bearer: "admin-token", identity: Identity{UserID: 8, Role: "admin", Status: "disabled"}, want: http.StatusUnauthorized},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			verifier := &fakeVerifier{identity: test.identity}
			handler := RequireAdmin(verifier, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next called") }))
			req := httptest.NewRequest(http.MethodGet, "/ops", nil)
			if test.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+test.bearer)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.want {
				t.Fatalf("status=%d want=%d", recorder.Code, test.want)
			}
		})
	}
}

func TestRequireHiddenAdminMasksEveryUnauthorizedStateAsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity Identity
		bearer   string
		err      error
	}{
		{name: "missing"},
		{name: "invalid", bearer: "invalid", err: context.Canceled},
		{name: "user", bearer: "user", identity: Identity{UserID: 7, Role: "user", Status: "active"}},
		{name: "disabled admin", bearer: "disabled", identity: Identity{UserID: 8, Role: "admin", Status: "disabled"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			verifier := &fakeVerifier{identity: test.identity, err: test.err}
			handler := RequireHiddenAdmin(verifier, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next called") }))
			req := httptest.NewRequest(http.MethodGet, "/ops", nil)
			if test.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+test.bearer)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status=%d want=%d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

type fakeVerifier struct {
	identity Identity
	session  Session
	err      error
}

func (f *fakeVerifier) VerifyAdminSession(_ context.Context, session Session) (Identity, error) {
	f.session = session
	return f.identity, f.err
}

var _ domain.AdminActor
