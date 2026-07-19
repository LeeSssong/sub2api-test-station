package adminauth

import (
	"context"
	"net/http"
	"strings"

	"example.invalid/relay-ops-service/internal/domain"
)

type Identity struct {
	UserID int64
	Role   string
	Status string
}

type Session struct {
	Bearer       string
	UserAgent    string
	ForwardedFor string
	RealIP       string
}

type Verifier interface {
	VerifyAdminSession(context.Context, Session) (Identity, error)
}

type actorContextKey struct{}

func RequireAdmin(verifier Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		identity, err := verifier.VerifyAdminSession(r.Context(), Session{
			Bearer:       bearer,
			UserAgent:    r.UserAgent(),
			ForwardedFor: strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
			RealIP:       strings.TrimSpace(r.Header.Get("X-Real-IP")),
		})
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		if identity.UserID <= 0 || identity.Role != "admin" || identity.Status != "active" {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		actor := domain.AdminActor{UserID: identity.UserID}
		ctx := context.WithValue(r.Context(), actorContextKey{}, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ActorFromContext(ctx context.Context) (domain.AdminActor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(domain.AdminActor)
	return actor, ok
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
