package httpserver

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
)

//go:embed templates/*.html static/*.css
var assets embed.FS

type PublicModel struct{ ModelID, Tier, Input, Output, CacheRead, CacheWrite string }
type PublicGroup struct {
	Name, UpdatedAt string
	Models          []PublicModel
}
type CandidateView struct {
	ID                                   int64
	Name, Status, LastCheck, PriceChange string
}
type OpsView struct {
	PublicGroups     []string
	NativeMonitorURL string
	Candidates       []CandidateView
	Incidents        []string
	AgentReports     []string
}

type PricingSource interface {
	PublicPricing(context.Context) ([]PublicGroup, error)
}
type OpsSource interface {
	Snapshot(context.Context) (OpsView, error)
}
type CandidateService interface {
	List(context.Context, domain.AdminActor) ([]candidates.Candidate, error)
	Create(context.Context, domain.AdminActor, candidates.CandidateInput) (candidates.Candidate, error)
	Disable(context.Context, domain.AdminActor, domain.UpstreamID) error
}

type Dependencies struct {
	BaseOrigin string
	Auth       adminauth.Verifier
	Pricing    PricingSource
	Ops        OpsSource
	Candidates CandidateService
}

type server struct {
	dependencies Dependencies
	templates    *template.Template
	css          []byte
}

func NewServer(dependencies Dependencies) (http.Handler, error) {
	if dependencies.Auth == nil || dependencies.Pricing == nil || dependencies.Ops == nil || dependencies.Candidates == nil {
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
	s := &server{dependencies: dependencies, templates: templates, css: css}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /relay-ops/static/app.css", s.styles)
	mux.HandleFunc("GET /pricing", s.pricing)
	mux.Handle("GET /ops", adminauth.RequireAdmin(dependencies.Auth, http.HandlerFunc(s.ops)))
	mux.Handle("GET /relay-ops/api/candidates", adminauth.RequireAdmin(dependencies.Auth, http.HandlerFunc(s.listCandidates)))
	mux.Handle("POST /relay-ops/api/candidates", adminauth.RequireAdmin(dependencies.Auth, http.HandlerFunc(s.createCandidate)))
	mux.Handle("POST /relay-ops/api/candidates/{id}/disable", adminauth.RequireAdmin(dependencies.Auth, http.HandlerFunc(s.disableCandidate)))
	return mux, nil
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

func (s *server) ops(w http.ResponseWriter, request *http.Request) {
	view, err := s.dependencies.Ops.Snapshot(request.Context())
	if err != nil {
		http.Error(w, "运维数据暂时不可用", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "ops.html", view)
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
		Name           string `json:"name"`
		BaseURL        string `json:"base_url"`
		PricingURL     string `json:"pricing_url"`
		UsageURL       string `json:"usage_url"`
		PerformanceURL string `json:"performance_url"`
		ProbeKeyFile   string `json:"probe_key_file"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CANDIDATE")
		return
	}
	actor, _ := adminauth.ActorFromContext(request.Context())
	created, err := s.dependencies.Candidates.Create(request.Context(), actor, candidates.CandidateInput{Name: input.Name, BaseURL: input.BaseURL, PricingURL: input.PricingURL, UsageURL: input.UsageURL, PerformanceURL: input.PerformanceURL, ProbeKeyFile: input.ProbeKeyFile})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, candidates.ErrConflict) {
			status = http.StatusConflict
		}
		writeAPIError(w, status, "CANDIDATE_REJECTED")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": created.ID, "name": created.Name, "base_url": created.BaseURL, "enabled": created.Enabled})
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
	return request.Header.Get("Origin") == s.dependencies.BaseOrigin && strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json")
}
func writeAPIError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
