package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/adminauth"
	"github.com/shopspring/decimal"
)

const (
	accountingDateLayout = "2006-01-02"
	cashEventListLimit   = 100
	cashEventBodyLimit   = 4 << 10
)

type accountingPageView struct {
	SelectedDate       string
	DefaultPaidAt      string
	Snapshot           accountingSnapshotView
	HasSnapshot        bool
	Events             []accountingEventView
	HasUnlinkedOutflow bool
}

type accountingSnapshotView struct {
	ReportDate              string
	ExternalRevenueCNY      string
	ExternalRequests        int64
	InternalRequests        int64
	CustomerResourceCostCNY string
	InternalResourceCostCNY string
	ResourceCostCNY         string
	OperatingGrossProfitCNY string
	CashOutflowCNY          string
	CashNetResultCNY        string
	UnlinkedCashOutflowCNY  string
	CashEventCount          int64
	OwnedOAuthCostCNY       string
	UpstreamAPIKeyCostCNY   string
}

type accountingEventView struct {
	ID         int64
	EventType  string
	PaidAt     string
	AmountCNY  string
	SourceKind string
	AccountID  string
	Notes      string
}

type accountingCashEventRequest struct {
	EventType  string                `json:"event_type"`
	PaidAt     string                `json:"paid_at"`
	AmountCNY  string                `json:"amount_cny"`
	SourceKind accounting.SourceKind `json:"source_kind"`
	AccountID  *int64                `json:"account_id"`
	Notes      string                `json:"notes"`
}

func (s *server) accountingPage(w http.ResponseWriter, request *http.Request) {
	selectedDate, err := requestedAccountingDate(request, true)
	if err != nil {
		http.Error(w, "invalid accounting date", http.StatusBadRequest)
		return
	}
	snapshot, found, err := s.dependencies.Accounting.ReadDailySnapshot(request.Context(), selectedDate)
	if err != nil {
		log.Printf("read accounting snapshot: %v", err)
		http.Error(w, "accounting report unavailable", http.StatusServiceUnavailable)
		return
	}
	window := accounting.NewDayWindow(selectedDate)
	events, err := s.dependencies.Accounting.ListCashEvents(request.Context(), window.Start, window.End, cashEventListLimit)
	if err != nil {
		log.Printf("list accounting cash events: %v", err)
		http.Error(w, "accounting events unavailable", http.StatusServiceUnavailable)
		return
	}
	eventViews := make([]accountingEventView, 0, len(events))
	for _, event := range events {
		accountID := ""
		if event.AccountID != nil {
			accountID = strconv.FormatInt(*event.AccountID, 10)
		}
		eventViews = append(eventViews, accountingEventView{
			ID:         event.ID,
			EventType:  event.EventType,
			PaidAt:     event.PaidAt.In(accounting.LocalDay(event.PaidAt).Location()).Format("2006-01-02 15:04"),
			AmountCNY:  cny(event.AmountCNY),
			SourceKind: string(event.SourceKind),
			AccountID:  accountID,
			Notes:      event.Notes,
		})
	}
	now := time.Now().In(accounting.LocalDay(time.Now()).Location())
	view := accountingPageView{
		SelectedDate:  selectedDate.Format(accountingDateLayout),
		DefaultPaidAt: now.Format("2006-01-02T15:04"),
		HasSnapshot:   found,
		Events:        eventViews,
	}
	if found {
		view.Snapshot = newAccountingSnapshotView(snapshot)
		view.HasUnlinkedOutflow = !snapshot.UnlinkedCashOutflowCNY.IsZero()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "accounting.html", view); err != nil {
		log.Printf("render accounting page: %v", err)
	}
}

func (s *server) accountingDaily(w http.ResponseWriter, request *http.Request) {
	date, err := requestedAccountingDate(request, false)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ACCOUNTING_DATE")
		return
	}
	snapshot, found, err := s.dependencies.Accounting.ReadDailySnapshot(request.Context(), date)
	if err != nil {
		log.Printf("read accounting snapshot: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "ACCOUNTING_DAILY_FAILED")
		return
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, "ACCOUNTING_DAILY_NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, accountingSnapshotJSON(snapshot))
}

func (s *server) createAccountingCashEvent(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if !validAccountingIdempotencyKey(idempotencyKey) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY")
		return
	}
	var requestBody accountingCashEventRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, cashEventBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&requestBody); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CASH_EVENT")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CASH_EVENT")
		return
	}
	paidAt, err := time.Parse(time.RFC3339Nano, requestBody.PaidAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CASH_EVENT")
		return
	}
	amount, err := decimal.NewFromString(requestBody.AmountCNY)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CASH_EVENT")
		return
	}
	input, err := accounting.ValidateCashEvent(accounting.CashEventInput{
		EventType:  requestBody.EventType,
		PaidAt:     paidAt,
		AmountCNY:  amount,
		SourceKind: requestBody.SourceKind,
		AccountID:  requestBody.AccountID,
		Notes:      requestBody.Notes,
	})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CASH_EVENT")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	event, created, err := s.dependencies.Accounting.CreateCashEvent(request.Context(), actor, input, idempotencyKey)
	if err != nil {
		log.Printf("create accounting cash event: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "CASH_EVENT_CREATE_FAILED")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, accountingEventJSON(event))
}

func (s *server) accountingScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(s.accountingJS)
}

func requestedAccountingDate(request *http.Request, defaultYesterday bool) (time.Time, error) {
	location := accounting.LocalDay(time.Now()).Location()
	value := request.URL.Query().Get("date")
	if value == "" && defaultYesterday {
		return accounting.LocalDay(time.Now()).AddDate(0, 0, -1), nil
	}
	if value == "" {
		return time.Time{}, errors.New("date is required")
	}
	return time.ParseInLocation(accountingDateLayout, value, location)
}

func validAccountingIdempotencyKey(value string) bool {
	if len(value) < 16 || len(value) > 128 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func newAccountingSnapshotView(snapshot accounting.DailySnapshot) accountingSnapshotView {
	return accountingSnapshotView{
		ReportDate:              snapshot.ReportDate.Format(accountingDateLayout),
		ExternalRevenueCNY:      cny(snapshot.ExternalRevenueCNY),
		ExternalRequests:        snapshot.ExternalRequests,
		InternalRequests:        snapshot.InternalRequests,
		CustomerResourceCostCNY: cny(snapshot.CustomerResourceCostCNY),
		InternalResourceCostCNY: cny(snapshot.InternalResourceCostCNY),
		ResourceCostCNY:         cny(snapshot.ResourceCostCNY),
		OperatingGrossProfitCNY: cny(snapshot.OperatingGrossProfitCNY),
		CashOutflowCNY:          cny(snapshot.CashOutflowCNY),
		CashNetResultCNY:        cny(snapshot.CashNetResultCNY),
		UnlinkedCashOutflowCNY:  cny(snapshot.UnlinkedCashOutflowCNY),
		CashEventCount:          snapshot.CashEventCount,
		OwnedOAuthCostCNY:       cny(snapshot.OwnedOAuthCostCNY),
		UpstreamAPIKeyCostCNY:   cny(snapshot.UpstreamAPIKeyCostCNY),
	}
}

func accountingSnapshotJSON(snapshot accounting.DailySnapshot) map[string]any {
	view := newAccountingSnapshotView(snapshot)
	return map[string]any{
		"report_date":                view.ReportDate,
		"external_revenue_cny":       view.ExternalRevenueCNY,
		"external_requests":          view.ExternalRequests,
		"internal_requests":          view.InternalRequests,
		"customer_resource_cost_cny": view.CustomerResourceCostCNY,
		"internal_resource_cost_cny": view.InternalResourceCostCNY,
		"resource_cost_cny":          view.ResourceCostCNY,
		"operating_gross_profit_cny": view.OperatingGrossProfitCNY,
		"cash_outflow_cny":           view.CashOutflowCNY,
		"cash_net_result_cny":        view.CashNetResultCNY,
		"unlinked_cash_outflow_cny":  view.UnlinkedCashOutflowCNY,
		"cash_event_count":           view.CashEventCount,
		"owned_oauth_cost_cny":       view.OwnedOAuthCostCNY,
		"upstream_apikey_cost_cny":   view.UpstreamAPIKeyCostCNY,
	}
}

func accountingEventJSON(event accounting.CashEvent) map[string]any {
	return map[string]any{
		"id":                 event.ID,
		"event_type":         event.EventType,
		"paid_at":            formatAccountingTime(event.PaidAt),
		"amount_cny":         cny(event.AmountCNY),
		"source_kind":        event.SourceKind,
		"account_id":         event.AccountID,
		"notes":              event.Notes,
		"created_by_user_id": event.CreatedByUserID,
		"created_at":         formatAccountingTime(event.CreatedAt),
	}
}

func formatAccountingTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func cny(value decimal.Decimal) string {
	return value.StringFixed(2)
}
