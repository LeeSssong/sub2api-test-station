package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"github.com/shopspring/decimal"
)

func reconciliationWindow(request *http.Request) (int64, time.Time, time.Time, string, error) {
	accountID := int64(0)
	if raw := strings.TrimSpace(request.URL.Query().Get("account_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return 0, time.Time{}, time.Time{}, "", errors.New("invalid account_id")
		}
		accountID = parsed
	}
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if raw := strings.TrimSpace(request.URL.Query().Get("start")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return 0, time.Time{}, time.Time{}, "", err
		}
		start = parsed
	}
	if raw := strings.TrimSpace(request.URL.Query().Get("end")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return 0, time.Time{}, time.Time{}, "", err
		}
		end = parsed
	}
	if !start.Before(end) {
		return 0, time.Time{}, time.Time{}, "", errors.New("invalid time window")
	}
	currency := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("currency")))
	if currency == "" {
		currency = "USD"
	}
	return accountID, start, end, currency, nil
}

func (s *server) reconciliationSummary(w http.ResponseWriter, request *http.Request) {
	accountID, start, end, currency, err := reconciliationWindow(request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_RECONCILIATION_WINDOW")
		return
	}
	summary, err := s.dependencies.Reconciliation.ReadReconciliationSummary(request.Context(), accountID, start, end, currency)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "RECONCILIATION_SUMMARY_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) reconciliationExceptions(w http.ResponseWriter, request *http.Request) {
	accountID, _, _, _, err := reconciliationWindow(request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_RECONCILIATION_WINDOW")
		return
	}
	limit := 100
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_LIMIT")
			return
		}
		limit = parsed
	}
	items, err := s.dependencies.Reconciliation.ListUpstreamCostExceptions(request.Context(), accountID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "RECONCILIATION_EXCEPTIONS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) reconciliationRefresh(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	accountID, start, end, currency, err := reconciliationWindow(request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_RECONCILIATION_WINDOW")
		return
	}
	summary, err := s.dependencies.Reconciliation.RefreshReconciliation(request.Context(), accountID, start, end, currency)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "RECONCILIATION_REFRESH_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) reconciliationManualAdjust(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	exceptionID, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || exceptionID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EXCEPTION_ID")
		return
	}
	var body struct {
		Amount string `json:"amount"`
		Notes  string `json:"notes"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_MANUAL_ADJUSTMENT")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_MANUAL_ADJUSTMENT")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(body.Amount))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_MANUAL_ADJUSTMENT")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	// Scope a client retry key to the exception. A browser-provided key must
	// never be able to reuse a transaction from another account/exception.
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = amount.String()
	}
	idempotencyKey = "manual:exception:" + strconv.FormatInt(exceptionID, 10) + ":" + idempotencyKey
	transaction, created, err := s.dependencies.Reconciliation.CreateManualUpstreamCostForException(request.Context(), exceptionID, reconciliation.ManualAdjustmentInput{
		Amount: amount, Notes: body.Notes, ActorUserID: actor.UserID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeAPIError(w, http.StatusConflict, "MANUAL_ADJUSTMENT_REJECTED")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, transaction)
}
