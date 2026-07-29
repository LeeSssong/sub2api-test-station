package updater

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	codeConfirmationRequired = "UPDATE_CONFIRMATION_REQUIRED"
	codeAuthRequired         = "UPDATE_AUTH_REQUIRED"
	codeForbidden            = "UPDATE_FORBIDDEN"
	codeAlreadyScheduled     = "UPDATE_ALREADY_SCHEDULED"
	codeInProgress           = "UPDATE_IN_PROGRESS"
	codeInvalidTime          = "UPDATE_INVALID_TIME"
	codeTargetChanged        = "UPDATE_TARGET_CHANGED"
	codeCandidateNotReady    = "UPDATE_CANDIDATE_NOT_READY"
	codeServiceError         = "UPDATE_SERVICE_ERROR"
)

type updateHTTP struct {
	service        *Service
	identity       IdentityVerifier
	expectedOrigin string
	traceDir       string
	now            func() time.Time
}

// NewHTTP exposes only the host-controlled update endpoints. traceDir is where
// the executor writes per-operation step traces; "" disables event reporting.
func NewHTTP(service *Service, identity IdentityVerifier, expectedOrigin, traceDir string, clocks ...func() time.Time) http.Handler {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	h := &updateHTTP{service: service, identity: identity, expectedOrigin: expectedOrigin, traceDir: traceDir, now: now}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/admin/system/update", h.update)
	mux.HandleFunc("GET /api/v1/admin/system/host-update/status", h.status)
	mux.HandleFunc("GET /api/v1/admin/system/host-update/readiness", h.readiness)
	mux.HandleFunc("DELETE /api/v1/admin/system/host-update/schedule", h.cancel)
	return mux
}

func (h *updateHTTP) update(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var request struct {
		Mode          string    `json:"mode"`
		TargetVersion string    `json:"target_version"`
		ScheduledAt   time.Time `json:"scheduled_at"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.More() {
		writeError(w, http.StatusBadRequest, codeConfirmationRequired)
		return
	}
	if request.Mode != "now" && request.Mode != "schedule" || strings.TrimSpace(request.TargetVersion) == "" {
		writeError(w, http.StatusBadRequest, codeConfirmationRequired)
		return
	}
	if request.Mode == "schedule" {
		now := h.now().UTC()
		if request.ScheduledAt.IsZero() || request.ScheduledAt.Before(now.Add(2*time.Minute)) || request.ScheduledAt.After(now.Add(30*24*time.Hour)) {
			writeError(w, http.StatusBadRequest, codeInvalidTime)
			return
		}
	}
	op, err := h.service.Schedule(r.Context(), identity.ID, request.Mode, request.TargetVersion, request.ScheduledAt)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"operation_id": op.OperationID, "stage": op.Stage})
}

func (h *updateHTTP) status(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r, false); !ok {
		return
	}
	op, err := h.service.Status()
	if errors.Is(err, ErrNoOperation) {
		writeData(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, struct {
		Operation
		Events []string `json:"events,omitempty"`
	}{op, readTraceEvents(h.traceDir, op.OperationID)})
}

func (h *updateHTTP) readiness(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r, false); !ok {
		return
	}
	targetVersions := r.URL.Query()["target_version"]
	if len(targetVersions) != 1 || strings.TrimSpace(targetVersions[0]) == "" {
		writeError(w, http.StatusBadRequest, codeConfirmationRequired)
		return
	}
	readiness, err := h.service.Readiness(r.Context(), targetVersions[0])
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, readiness)
}

// readTraceEvents returns the executor's step trace for the operation. The
// trace is advisory progress data, so read failures simply yield no events.
func readTraceEvents(traceDir, operationID string) []string {
	tracePath := TracePath(traceDir, operationID)
	if tracePath == "" {
		return nil
	}
	raw, err := os.ReadFile(tracePath)
	if err != nil || len(raw) > 64<<10 {
		return nil
	}
	var events []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			events = append(events, line)
		}
	}
	if len(events) > 256 {
		events = events[len(events)-256:]
	}
	return events
}

func (h *updateHTTP) cancel(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r, true); !ok {
		return
	}
	if err := h.service.Cancel(); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"stage": "cancelled"})
}

func (h *updateHTTP) authorize(w http.ResponseWriter, r *http.Request, mutation bool) (Identity, bool) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeError(w, http.StatusUnauthorized, codeAuthRequired)
		return Identity{}, false
	}
	if r.Header.Get("X-Admin-UI-Request") != "1" {
		writeError(w, http.StatusBadRequest, codeConfirmationRequired)
		return Identity{}, false
	}
	if mutation && !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusBadRequest, codeConfirmationRequired)
		return Identity{}, false
	}
	origin := r.Header.Get("Origin")
	if mutation && origin != h.expectedOrigin || !mutation && origin != "" && origin != h.expectedOrigin {
		writeError(w, http.StatusForbidden, codeForbidden)
		return Identity{}, false
	}
	if fetchSite := r.Header.Get("Sec-Fetch-Site"); fetchSite != "" && fetchSite != "same-origin" {
		writeError(w, http.StatusForbidden, codeForbidden)
		return Identity{}, false
	}
	identity, err := h.identity.Verify(r.Context(), token, r.Header)
	if err != nil {
		writeError(w, http.StatusUnauthorized, codeAuthRequired)
		return Identity{}, false
	}
	if !isActiveAdmin(identity) {
		writeError(w, http.StatusForbidden, codeForbidden)
		return Identity{}, false
	}
	return identity, true
}

func (h *updateHTTP) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOperationRunning):
		writeError(w, http.StatusConflict, codeInProgress)
	case errors.Is(err, ErrOperationExists):
		writeError(w, http.StatusConflict, codeAlreadyScheduled)
	case errors.Is(err, ErrTargetChanged):
		writeError(w, http.StatusConflict, codeTargetChanged)
	case errors.Is(err, ErrCandidateNotReady):
		writeError(w, http.StatusConflict, codeCandidateNotReady)
	case errors.Is(err, ErrNoOperation):
		writeError(w, http.StatusNotFound, codeServiceError)
	default:
		writeError(w, http.StatusInternalServerError, codeServiceError)
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
