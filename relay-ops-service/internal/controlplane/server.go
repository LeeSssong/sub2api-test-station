package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type Server struct {
	reader    Reader
	refresher Refresher
}

func NewServer(reader Reader, refresher Refresher) http.Handler {
	s := &Server{reader: reader, refresher: refresher}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts/monitor", s.read("accounts/monitor"))
	mux.HandleFunc("GET /operations/profitability", s.read("operations/profitability"))
	mux.HandleFunc("GET /accounting/ledger", s.read("accounting/ledger"))
	mux.HandleFunc("GET /reconciliation", s.read("reconciliation"))
	mux.HandleFunc("POST /accounts/{id}/refresh", s.refresh)
	return mux
}
func (s *Server) read(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.reader == nil {
			http.Error(w, "control plane unavailable", http.StatusServiceUnavailable)
			return
		}
		q := map[string]string{}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				q[k] = v[0]
			}
		}
		model, err := s.reader.Read(name, q)
		if err != nil {
			http.Error(w, "control plane unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, model)
	}
}
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		http.Error(w, "Idempotency-Key is required", http.StatusBadRequest)
		return
	}
	if s.refresher == nil {
		http.Error(w, "control plane unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	if err := s.refresher.RefreshAccount(r.Context(), id, key); err != nil {
		if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", 499)
			return
		}
		http.Error(w, "refresh failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"account_id": id, "status": "accepted"})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
