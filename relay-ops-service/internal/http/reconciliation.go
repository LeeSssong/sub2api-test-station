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

func operationsScope(request *http.Request, history bool) (reconciliation.OperationsScope, error) {
	query := request.URL.Query()
	timezone := strings.TrimSpace(query.Get("timezone"))
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return reconciliation.OperationsScope{}, errors.New("invalid timezone")
	}
	parseOptionalID := func(name string) (*int64, error) {
		raw := strings.TrimSpace(query.Get(name))
		if raw == "" {
			return nil, nil
		}
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			return nil, errors.New("invalid " + name)
		}
		return &value, nil
	}
	groupID, err := parseOptionalID("group_id")
	if err != nil {
		return reconciliation.OperationsScope{}, err
	}
	accountID, err := parseOptionalID("account_id")
	if err != nil {
		return reconciliation.OperationsScope{}, err
	}
	now := time.Now().In(location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	start, end := dayStart, now
	if history {
		start = dayStart.AddDate(0, 0, -29)
	}
	if raw := strings.TrimSpace(query.Get("start")); raw != "" {
		start, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return reconciliation.OperationsScope{}, err
		}
	}
	if raw := strings.TrimSpace(query.Get("end")); raw != "" {
		end, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return reconciliation.OperationsScope{}, err
		}
	}
	currency := strings.ToUpper(strings.TrimSpace(query.Get("currency")))
	if currency == "" {
		currency = "USD"
	}
	scope, err := reconciliation.ValidateOperationsScope(reconciliation.OperationsScope{
		GroupID: groupID, AccountID: accountID, Start: start, End: end, Currency: currency, Timezone: location.String(),
	})
	if err != nil {
		return reconciliation.OperationsScope{}, err
	}
	if history && scope.End.Sub(scope.Start) > 366*24*time.Hour {
		return reconciliation.OperationsScope{}, errors.New("history window exceeds 366 days")
	}
	return scope, nil
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

func (s *server) reconciliationCostGuard(w http.ResponseWriter, request *http.Request) {
	accountID, err := positiveQueryID(request, "account_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ACCOUNT_ID")
		return
	}
	groupID, err := positiveQueryID(request, "group_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_GROUP_ID")
		return
	}
	rawMultiplier := strings.TrimSpace(request.URL.Query().Get("group_multiplier"))
	if rawMultiplier == "" {
		writeAPIError(w, http.StatusBadRequest, "INVALID_GROUP_MULTIPLIER")
		return
	}
	groupMultiplier, err := decimal.NewFromString(rawMultiplier)
	if err != nil || groupMultiplier.IsNegative() {
		writeAPIError(w, http.StatusBadRequest, "INVALID_GROUP_MULTIPLIER")
		return
	}
	costGuard, err := s.dependencies.CostGuard.ReadCostGuard(request.Context(), reconciliation.CostGuardQuery{
		AccountID: accountID, GroupID: groupID, GroupMultiplier: groupMultiplier,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "COST_GUARD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, costGuard)
}

func positiveQueryID(request *http.Request, name string) (int64, error) {
	raw := strings.TrimSpace(request.URL.Query().Get(name))
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid " + name)
	}
	return parsed, nil
}

func (s *server) reconciliationOperations(w http.ResponseWriter, request *http.Request) {
	scope, err := operationsScope(request, false)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_OPERATIONS_SCOPE")
		return
	}
	summary, err := s.dependencies.Reconciliation.ReadOperationsSummary(request.Context(), scope)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "OPERATIONS_SUMMARY_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) reconciliationOperationsHistory(w http.ResponseWriter, request *http.Request) {
	scope, err := operationsScope(request, true)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_OPERATIONS_SCOPE")
		return
	}
	items, err := s.dependencies.Reconciliation.ListOperationsDaily(request.Context(), scope)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "OPERATIONS_HISTORY_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
