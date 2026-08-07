package httpserver

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/acceptance"
	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"example.invalid/relay-ops-service/internal/upstreams"
)

var ErrQualityReportStale = errors.New("quality report is stale or mismatched")

//go:embed templates/*.html static/*.css static/*.js
var assets embed.FS

type PublicModel struct{ ModelID, Tier, Input, Output, CacheRead, CacheWrite string }
type PublicGroup struct {
	Name, UpdatedAt string
	Models          []PublicModel
}
type CandidateView struct {
	ID                                   int64
	Name, Status, LastCheck, PriceChange string
	Enabled                              bool
}
type QualityReportView struct {
	ReportID                         string
	ReportHash                       string
	Upstream                         string
	Status                           string
	QualityScore, TotalScore         int
	Direct, Gateway, Models, Pricing string
	Capacity                         string
}
type ProductionSourceView struct {
	ID                                   int64
	Name, Status, LastCheck, PriceChange string
	GroupIDs                             []int64
}
type OpsView struct {
	PublicGroups     []string
	NativeMonitorURL string
	Production       []ProductionSourceView
	Candidates       []CandidateView
	QualityReports   []QualityReportView
	Incidents        []string
	AgentReports     []string
	SiteRuntime      opsmetrics.Snapshot
	AccountQuality   accountquality.View
	RefreshedAt      string
}

type PricingSource interface {
	PublicPricing(context.Context) ([]PublicGroup, error)
}
type OpsSource interface {
	Snapshot(context.Context) (OpsView, error)
}
type CandidateService interface {
	List(context.Context, domain.AdminActor) ([]candidates.Candidate, error)
	CreateWithKey(context.Context, domain.AdminActor, candidates.CandidateIntakeInput) (candidates.Candidate, error)
	Disable(context.Context, domain.AdminActor, domain.UpstreamID) error
}
type ProductionUpstreamService interface {
	List(context.Context, domain.AdminActor) ([]upstreams.Source, error)
	CreateProduction(context.Context, domain.AdminActor, upstreams.ProductionInput) (upstreams.Source, error)
	Disable(context.Context, domain.AdminActor, domain.UpstreamID) error
}
type BillingSessionService interface {
	Configure(context.Context, domain.AdminActor, billing.SessionInput) (billing.SessionConfig, error)
}
type SyntheticAcceptanceService interface {
	Run(context.Context) (acceptance.Result, error)
}

type DailyReportAcceptanceService interface {
	Run(context.Context) (dailyreport.Result, error)
}

type QualityPreviewInput struct {
	ReportID   string `json:"report_id"`
	ReportHash string `json:"report_hash"`
}

type QualitySwitchPreview struct {
	ReportID   string `json:"report_id"`
	ReportHash string `json:"report_hash"`
	Status     string `json:"status"`
	Writes     int    `json:"writes"`
	Summary    string `json:"summary"`
}

type QualityReviewService interface {
	Preview(context.Context, domain.AdminActor, QualityPreviewInput) (QualitySwitchPreview, error)
}

type AccountingService interface {
	CreateCashEvent(context.Context, domain.AdminActor, accounting.CashEventInput, string) (accounting.CashEvent, bool, error)
	ReadDailySnapshot(context.Context, time.Time) (accounting.DailySnapshot, bool, error)
	ListCashEvents(context.Context, time.Time, time.Time, int) ([]accounting.CashEvent, error)
	RecomputeDate(context.Context, time.Time) (accounting.DailySnapshot, error)
}

type ReconciliationService interface {
	ReadReconciliationSummary(context.Context, int64, time.Time, time.Time, string) (reconciliation.Summary, error)
	ReadRequestCostDetail(context.Context, reconciliation.RequestCostQuery) (reconciliation.RequestCostDetail, error)
	ReadOperationsSummary(context.Context, reconciliation.OperationsScope) (reconciliation.OperationsSummary, error)
	ListOperationsDaily(context.Context, reconciliation.OperationsScope) ([]reconciliation.OperationsDailyRow, error)
	ListUpstreamCostExceptions(context.Context, int64, int) ([]reconciliation.Exception, error)
	CreateManualUpstreamCostForException(context.Context, int64, reconciliation.ManualAdjustmentInput) (reconciliation.Transaction, bool, error)
	RefreshReconciliation(context.Context, int64, time.Time, time.Time, string) (reconciliation.Summary, error)
}

type CostGuardService interface {
	ReadCostGuard(context.Context, reconciliation.CostGuardQuery) (reconciliation.CostGuard, error)
}

type Dependencies struct {
	BaseOrigin     string
	Auth           adminauth.Verifier
	Pricing        PricingSource
	Candidates     CandidateService
	Upstreams      ProductionUpstreamService
	Billing        BillingSessionService
	Acceptance     SyntheticAcceptanceService
	DailyReport    DailyReportAcceptanceService
	QualityReview  QualityReviewService
	Accounting     AccountingService
	Reconciliation ReconciliationService
	CostGuard      CostGuardService
}

type server struct {
	dependencies Dependencies
	templates    *template.Template
	css          []byte
	accountingJS []byte
}

func NewServer(dependencies Dependencies) (http.Handler, error) {
	if dependencies.Pricing == nil {
		return nil, fmt.Errorf("HTTP dependencies are incomplete")
	}
	templates, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	css, err := assets.ReadFile("static/app.css")
	if err != nil {
		return nil, fmt.Errorf("read CSS: %w", err)
	}
	accountingJS, err := assets.ReadFile("static/accounting.js")
	if err != nil {
		return nil, fmt.Errorf("read accounting JavaScript: %w", err)
	}
	s := &server{dependencies: dependencies, templates: templates, css: css, accountingJS: accountingJS}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /relay-ops/static/app.css", s.styles)
	mux.HandleFunc("GET /pricing", s.pricing)
	if dependencies.Accounting != nil {
		accountingMux := http.NewServeMux()
		accountingMux.HandleFunc("GET /relay-ops/accounting", s.accountingPage)
		accountingMux.HandleFunc("GET /relay-ops/api/accounting/daily", s.accountingDaily)
		accountingMux.HandleFunc("POST /relay-ops/api/accounting/cash-events", s.createAccountingCashEvent)
		mux.HandleFunc("GET /relay-ops/static/accounting.js", s.accountingScript)
		mux.Handle("/relay-ops/", adminauth.RequireAdmin(dependencies.Auth, accountingMux))
	}
	if dependencies.Reconciliation != nil {
		reconciliationMux := http.NewServeMux()
		reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/summary", s.reconciliationSummary)
		reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/request-cost", s.reconciliationRequestCost)
		if dependencies.CostGuard != nil {
			reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/cost-guard", s.reconciliationCostGuard)
		}
		reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/operations", s.reconciliationOperations)
		reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/operations/history", s.reconciliationOperationsHistory)
		reconciliationMux.HandleFunc("GET /relay-ops/api/reconciliation/exceptions", s.reconciliationExceptions)
		reconciliationMux.HandleFunc("POST /relay-ops/api/reconciliation/refresh", s.reconciliationRefresh)
		reconciliationMux.HandleFunc("POST /relay-ops/api/reconciliation/exceptions/{id}/adjust", s.reconciliationManualAdjust)
		mux.Handle("/relay-ops/api/reconciliation/", adminauth.RequireAdmin(dependencies.Auth, reconciliationMux))
	}
	return mux, nil
}

func (s *server) qualityReportPreview(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	reportID := strings.TrimSpace(request.PathValue("id"))
	if reportID == "" || len(reportID) > 128 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_QUALITY_REPORT")
		return
	}
	var input struct {
		ReportHash string `json:"report_hash"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 512))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !lowerHex(input.ReportHash, 64) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_QUALITY_REPORT")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	preview, err := s.dependencies.QualityReview.Preview(request.Context(), actor, QualityPreviewInput{
		ReportID: reportID, ReportHash: input.ReportHash,
	})
	if err != nil {
		if errors.Is(err, ErrQualityReportStale) {
			writeAPIError(w, http.StatusConflict, "QUALITY_REPORT_STALE")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "QUALITY_PREVIEW_REJECTED")
		return
	}
	if preview.ReportID != reportID || preview.ReportHash != input.ReportHash || preview.Status != "dry_run" || preview.Writes != 0 {
		writeAPIError(w, http.StatusInternalServerError, "QUALITY_PREVIEW_INVALID")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func (s *server) syntheticAcceptance(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	// The endpoint accepts no user-controlled event data. An empty JSON object
	// is allowed so browser clients can use the same mutation helper.
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 256))
	var input map[string]any
	if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SYNTHETIC_ACCEPTANCE")
		return
	}
	if len(input) > 0 {
		writeAPIError(w, http.StatusBadRequest, "SYNTHETIC_INPUT_NOT_ALLOWED")
		return
	}
	result, err := s.dependencies.Acceptance.Run(request.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "SYNTHETIC_ACCEPTANCE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) dailyReportAcceptance(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 256))
	var input map[string]any
	if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_DAILY_REPORT_ACCEPTANCE")
		return
	}
	if len(input) > 0 {
		writeAPIError(w, http.StatusBadRequest, "DAILY_REPORT_INPUT_NOT_ALLOWED")
		return
	}
	result, err := s.dependencies.DailyReport.Run(request.Context())
	if err != nil {
		log.Printf("daily report acceptance failed: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "DAILY_REPORT_ACCEPTANCE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) configureBillingSession(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_UPSTREAM_ID")
		return
	}
	var input struct {
		AuthMode         string `json:"auth_mode"`
		SecretFile       string `json:"secret_file"`
		LoginURL         string `json:"login_url"`
		BillingAccountID int64  `json:"billing_account_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_BILLING_SESSION")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	configured, err := s.dependencies.Billing.Configure(request.Context(), actor, billing.SessionInput{
		UpstreamID: domain.UpstreamID(id), AuthMode: input.AuthMode, SecretFile: input.SecretFile, LoginURL: input.LoginURL, BillingAccountID: input.BillingAccountID,
	})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "BILLING_SESSION_REJECTED")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"upstream_id": configured.UpstreamID, "auth_mode": configured.AuthMode, "login_url": configured.LoginURL, "billing_account_id": configured.BillingAccountID, "status": "active"})
}

func (s *server) listUpstreams(w http.ResponseWriter, request *http.Request) {
	actor, _ := adminauth.ActorFromContext(request.Context())
	items, err := s.dependencies.Upstreams.List(request.Context(), actor)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "UPSTREAM_LIST_FAILED")
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, item := range items {
		views = append(views, map[string]any{"id": item.ID, "name": item.Name, "role": item.Role, "base_url": item.BaseURL, "adapter_type": item.AdapterType, "group_ids": item.GroupIDs, "monitor_id": item.MonitorID, "enabled": item.Enabled})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (s *server) createUpstream(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	var input struct {
		Name           string   `json:"name"`
		BaseURL        string   `json:"base_url"`
		AdapterType    string   `json:"adapter_type"`
		PricingURL     string   `json:"pricing_url"`
		UsageURL       string   `json:"usage_url"`
		PerformanceURL string   `json:"performance_url"`
		GroupIDs       []int64  `json:"group_ids"`
		GroupNames     []string `json:"group_names"`
		MonitorID      int64    `json:"monitor_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_UPSTREAM")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	created, err := s.dependencies.Upstreams.CreateProduction(request.Context(), actor, upstreams.ProductionInput{
		Name: input.Name, BaseURL: input.BaseURL, AdapterType: input.AdapterType, PricingURL: input.PricingURL, UsageURL: input.UsageURL,
		PerformanceURL: input.PerformanceURL, GroupIDs: input.GroupIDs, GroupNames: input.GroupNames, MonitorID: input.MonitorID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, upstreams.ErrConflict) {
			status = http.StatusConflict
		}
		writeAPIError(w, status, "UPSTREAM_REJECTED")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": created.ID, "name": created.Name, "role": created.Role, "base_url": created.BaseURL, "adapter_type": created.AdapterType, "group_ids": created.GroupIDs, "monitor_id": created.MonitorID, "enabled": created.Enabled})
}

func (s *server) disableUpstream(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_UPSTREAM_ID")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	if err := s.dependencies.Upstreams.Disable(request.Context(), actor, domain.UpstreamID(id)); err != nil {
		writeAPIError(w, http.StatusBadRequest, "UPSTREAM_DISABLE_FAILED")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) styles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(s.css)
}

func (s *server) pricing(w http.ResponseWriter, request *http.Request) {
	groups, err := s.dependencies.Pricing.PublicPricing(request.Context())
	if err != nil {
		http.Error(w, "价格数据暂时不可用", http.StatusServiceUnavailable)
		return
	}
	query := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("q")))
	rows := make([]pricingRow, 0)
	for _, group := range groups {
		for _, model := range group.Models {
			if query != "" && !strings.Contains(strings.ToLower(model.ModelID+" "+group.Name+" "+model.Tier), query) {
				continue
			}
			rows = append(rows, pricingRow{Group: group.Name, UpdatedAt: group.UpdatedAt, PublicModel: model})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "pricing.html", map[string]any{"Rows": rows, "Query": request.URL.Query().Get("q")})
}

type pricingRow struct {
	Group, UpdatedAt string
	PublicModel
}

func (s *server) listCandidates(w http.ResponseWriter, request *http.Request) {
	actor, _ := adminauth.ActorFromContext(request.Context())
	items, err := s.dependencies.Candidates.List(request.Context(), actor)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "CANDIDATE_LIST_FAILED")
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, item := range items {
		views = append(views, map[string]any{"id": item.ID, "name": item.Name, "base_url": item.BaseURL, "enabled": item.Enabled})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (s *server) createCandidate(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	var input struct {
		Name           string     `json:"name"`
		BaseURL        string     `json:"base_url"`
		PricingURL     string     `json:"pricing_url"`
		UsageURL       string     `json:"usage_url"`
		PerformanceURL string     `json:"performance_url"`
		ProbeKey       secretJSON `json:"probe_key"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "CANDIDATE_REJECTED")
		return
	}
	defer clear(input.ProbeKey)
	if len(input.ProbeKey) < 4 || len(input.ProbeKey) > candidates.MaxProbeKeyBytes {
		writeAPIError(w, http.StatusBadRequest, "CANDIDATE_REJECTED")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	created, err := s.dependencies.Candidates.CreateWithKey(request.Context(), actor, candidates.CandidateIntakeInput{
		Name: input.Name, BaseURL: input.BaseURL, PricingURL: input.PricingURL,
		UsageURL: input.UsageURL, PerformanceURL: input.PerformanceURL, ProbeKey: input.ProbeKey,
	})
	if err != nil {
		status := http.StatusBadRequest
		code := "CANDIDATE_REJECTED"
		if errors.Is(err, candidates.ErrConflict) || errors.Is(err, candidates.ErrSecretConflict) {
			status = http.StatusConflict
			code = "CANDIDATE_CONFLICT"
		} else if errors.Is(err, candidates.ErrCreateFailed) {
			status = http.StatusInternalServerError
			code = "CANDIDATE_CREATE_FAILED"
		}
		writeAPIError(w, status, code)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": created.ID, "name": created.Name, "base_url": created.BaseURL, "enabled": created.Enabled})
}

type secretJSON []byte

func (value *secretJSON) UnmarshalJSON(raw []byte) error {
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("secret must be a string")
	}
	*value = append((*value)[:0], decoded...)
	return nil
}

func (s *server) disableCandidate(w http.ResponseWriter, request *http.Request) {
	if !s.validMutation(request) {
		writeAPIError(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CANDIDATE_ID")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	if err := s.dependencies.Candidates.Disable(request.Context(), actor, domain.UpstreamID(id)); err != nil {
		writeAPIError(w, http.StatusBadRequest, "CANDIDATE_DISABLE_FAILED")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) validMutation(request *http.Request) bool {
	if request.Header.Get("Origin") != s.dependencies.BaseOrigin {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}
func writeAPIError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
