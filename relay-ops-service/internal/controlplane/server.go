package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"example.invalid/relay-ops-service/internal/billing"
)

type Server struct {
	reader    Reader
	refresher Refresher
	updater   billing.OfficialAccountWriter
	audit     billing.AccountUpdateCommandAudit
}

func NewServer(reader Reader, refresher Refresher) http.Handler {
	return NewServerWithAccountUpdates(reader, refresher, nil, nil)
}

func NewServerWithAccountUpdates(reader Reader, refresher Refresher, updater billing.OfficialAccountWriter, audit billing.AccountUpdateCommandAudit) http.Handler {
	s := &Server{reader: reader, refresher: refresher, updater: updater, audit: audit}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts/monitor", s.read("accounts/monitor"))
	mux.HandleFunc("GET /operations/profitability", s.read("operations/profitability"))
	mux.HandleFunc("GET /accounting/ledger", s.read("accounting/ledger"))
	mux.HandleFunc("GET /reconciliation", s.read("reconciliation"))
	mux.HandleFunc("POST /accounts/{id}/refresh", s.refresh)
	mux.HandleFunc("POST /accounts/{id}/commands/v1", s.updateAccount)
	return mux
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || identity.UserID <= 0 || identity.Role != "admin" || identity.Status != "active" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.updater == nil || s.audit == nil {
		http.Error(w, "control plane unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	var command billing.AccountUpdateCommand
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&command); err != nil {
		http.Error(w, "invalid account command", http.StatusBadRequest)
		return
	}
	command.ActorID = identity.UserID
	command.AccountID = id
	command.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if err := billing.ExecuteAccountUpdate(r.Context(), s.updater, s.audit, command); err != nil {
		switch {
		case errors.Is(err, billing.ErrAccountUpdatePending):
			http.Error(w, "command pending", http.StatusConflict)
		case errors.Is(err, billing.ErrAccountUpdateFailed):
			http.Error(w, "command failed", http.StatusBadGateway)
		default:
			http.Error(w, "account update failed", http.StatusBadGateway)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"account_id": id, "status": "accepted"})
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
		model, err := s.reader.Read(r.Context(), name, q)
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
