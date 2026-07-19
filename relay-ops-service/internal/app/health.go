package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
)

type Pinger interface{ Ping(context.Context) error }

type Readiness struct {
	Database      Pinger
	nativeSuccess atomic.Bool
}

func (r *Readiness) MarkNativeSuccess() { r.nativeSuccess.Store(true) }

func BootstrapNativeReadiness(ctx context.Context, sync func(context.Context) error, readiness *Readiness) error {
	if err := sync(ctx); err != nil {
		return err
	}
	readiness.MarkNativeSuccess()
	return nil
}

func (r *Readiness) Ready(ctx context.Context) bool {
	return r != nil && r.Database != nil && r.nativeSuccess.Load() && r.Database.Ping(ctx) == nil
}

func HealthHandler(readiness *Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeStatus(w, http.StatusOK, "alive") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, request *http.Request) {
		if !readiness.Ready(request.Context()) {
			writeStatus(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeStatus(w, http.StatusOK, "ready")
	})
	return mux
}

func writeStatus(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": value})
}
