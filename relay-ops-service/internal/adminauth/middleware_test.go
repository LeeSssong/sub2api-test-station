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
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent || verifier.bearer != "browser-token" {
		t.Fatalf("status=%d bearer=%q", recorder.Code, verifier.bearer)
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
		{name: "disabled admin", bearer: "admin-token", identity: Identity{UserID: 8, Role: "admin", Status: "disabled"}, want: http.StatusForbidden},
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

type fakeVerifier struct {
	identity Identity
	bearer   string
	err      error
}

func (f *fakeVerifier) VerifyAdminSession(_ context.Context, bearer string) (Identity, error) {
	f.bearer = bearer
	return f.identity, f.err
}

var _ domain.AdminActor
