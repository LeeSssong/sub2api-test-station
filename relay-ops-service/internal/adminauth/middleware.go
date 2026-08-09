package adminauth

import (
	"context"
	"net"
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
	ClientIP     string
	Origin       string
}

type Verifier interface {
	VerifyAdminSession(context.Context, Session) (Identity, error)
}

type actorContextKey struct{}

func RequireAdmin(verifier Verifier, next http.Handler) http.Handler {
	return requireAdmin(verifier, next, false)
}

func RequireHiddenAdmin(verifier Verifier, next http.Handler) http.Handler {
	return requireAdmin(verifier, next, true)
}

func requireAdmin(verifier Verifier, next http.Handler, hidden bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reject := func(status int) {
			if hidden {
				http.NotFound(w, r)
				return
			}
			http.Error(w, http.StatusText(status), status)
		}
		bearer, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			reject(http.StatusUnauthorized)
			return
		}
		identity, err := verifier.VerifyAdminSession(r.Context(), Session{
			Bearer:       bearer,
			UserAgent:    r.UserAgent(),
			ForwardedFor: strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
			RealIP:       strings.TrimSpace(r.Header.Get("X-Real-IP")),
			ClientIP:     requestClientIP(r),
			Origin:       strings.TrimSpace(r.Header.Get("Origin")),
		})
		if err != nil {
			reject(http.StatusUnauthorized)
			return
		}
		if identity.Status != "active" || identity.UserID <= 0 {
			reject(http.StatusUnauthorized)
			return
		}
		if identity.Role != "admin" {
			reject(http.StatusForbidden)
			return
		}
		actor := domain.AdminActor{UserID: identity.UserID}
		ctx := context.WithValue(r.Context(), actorContextKey{}, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if trustedProxyIP(host) {
			if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
				return forwarded
			}
			if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
				return realIP
			}
		}
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func trustedProxyIP(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func firstForwardedIP(header string) string {
	for _, value := range strings.Split(header, ",") {
		candidate := strings.TrimSpace(value)
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return ""
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
