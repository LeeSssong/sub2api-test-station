package controlplane

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type AdminIdentity struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	Status string `json:"status"`
}
type AuthClient interface {
	Me(context.Context, string, string, string) (AdminIdentity, error)
}

func RequireAdmin(client AuthClient, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := strings.Fields(r.Header.Get("Authorization"))
		if len(h) != 2 || !strings.EqualFold(h[0], "Bearer") {
			http.Error(w, "unauthorized", 401)
			return
		}
		identity, err := client.Me(r.Context(), h[1], r.RemoteAddr, r.Header.Get("Origin"))
		if err != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		if identity.UserID <= 0 || identity.Role != "admin" || identity.Status != "active" {
			http.Error(w, "forbidden", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var ErrUnauthorized = errors.New("unauthorized")
