package upstreams

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/pricing"
)

const RoleProduction = "production"

var (
	ErrConflict         = errors.New("upstream already exists")
	ErrGroupRequired    = errors.New("at least one public group is required")
	ErrGroupUnavailable = errors.New("public group is unavailable")
	ErrNotFound         = errors.New("production upstream not found")
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type ProductionInput struct {
	Name           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	GroupIDs       []int64
	GroupNames     []string
	MonitorID      int64
}

type Source struct {
	ID             domain.UpstreamID
	Name           string
	Role           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	AdapterType    string
	MonitorID      int64
	GroupIDs       []int64
	Enabled        bool
}

type AuditEvent struct {
	ActorUserID  int64
	Action       string
	ObjectType   string
	AfterSummary map[string]string
}

type ProductionRecord struct {
	Source Source
	Audit  AuditEvent
}

type Repository interface {
	CreateProduction(context.Context, ProductionRecord) (domain.UpstreamID, error)
	ResolvePublicGroupIDs(context.Context, []string) ([]int64, error)
	ListProduction(context.Context) ([]Source, error)
	DisableProduction(context.Context, domain.UpstreamID, AuditEvent) error
}

type Service struct {
	Repository Repository
	Resolver   Resolver
}

func (s Service) CreateProduction(ctx context.Context, actor domain.AdminActor, input ProductionInput) (Source, error) {
	if s.Repository == nil {
		return Source{}, fmt.Errorf("upstream repository is required")
	}
	if actor.UserID <= 0 {
		return Source{}, fmt.Errorf("administrator identity is required")
	}
	var err error
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 100 {
		return Source{}, fmt.Errorf("upstream name must be 1-100 characters")
	}
	groupIDs := input.GroupIDs
	if len(groupIDs) == 0 && len(input.GroupNames) > 0 {
		groupIDs, err = s.Repository.ResolvePublicGroupIDs(ctx, input.GroupNames)
		if err != nil {
			return Source{}, err
		}
	}
	groups, err := normalizeGroupIDs(groupIDs)
	if err != nil {
		return Source{}, err
	}
	if input.MonitorID < 0 {
		return Source{}, fmt.Errorf("monitor ID must not be negative")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	baseURL, err := normalizeURL(ctx, resolver, input.BaseURL, true, true)
	if err != nil {
		return Source{}, fmt.Errorf("base URL: %w", err)
	}
	pricingURL, err := normalizeURL(ctx, resolver, input.PricingURL, true, false)
	if err != nil {
		return Source{}, fmt.Errorf("pricing URL: %w", err)
	}
	usageURL, err := normalizeURL(ctx, resolver, input.UsageURL, false, false)
	if err != nil {
		return Source{}, fmt.Errorf("usage URL: %w", err)
	}
	performanceURL, err := normalizeURL(ctx, resolver, input.PerformanceURL, false, false)
	if err != nil {
		return Source{}, fmt.Errorf("performance URL: %w", err)
	}
	source := Source{
		Name: name, Role: RoleProduction, BaseURL: baseURL, PricingURL: pricingURL,
		UsageURL: usageURL, PerformanceURL: performanceURL, AdapterType: "unknown",
		MonitorID: input.MonitorID, GroupIDs: groups, Enabled: true,
	}
	record := ProductionRecord{
		Source: source,
		Audit: AuditEvent{
			ActorUserID: actor.UserID, Action: "upstream.production.create", ObjectType: "upstream",
			AfterSummary: map[string]string{"name": name, "base_url": baseURL, "group_count": fmt.Sprint(len(groups))},
		},
	}
	id, err := s.Repository.CreateProduction(ctx, record)
	if err != nil {
		return Source{}, err
	}
	source.ID = id
	return source, nil
}

func (s Service) List(ctx context.Context, actor domain.AdminActor) ([]Source, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("upstream repository is required")
	}
	if actor.UserID <= 0 {
		return nil, fmt.Errorf("administrator identity is required")
	}
	return s.Repository.ListProduction(ctx)
}

func (s Service) Disable(ctx context.Context, actor domain.AdminActor, id domain.UpstreamID) error {
	if s.Repository == nil {
		return fmt.Errorf("upstream repository is required")
	}
	if actor.UserID <= 0 {
		return fmt.Errorf("administrator identity is required")
	}
	if id <= 0 {
		return fmt.Errorf("upstream ID is required")
	}
	return s.Repository.DisableProduction(ctx, id, AuditEvent{
		ActorUserID: actor.UserID, Action: "upstream.production.disable", ObjectType: "upstream",
		AfterSummary: map[string]string{"enabled": "false"},
	})
}

func normalizeGroupIDs(values []int64) ([]int64, error) {
	unique := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, fmt.Errorf("public group IDs must be positive")
		}
		unique[value] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, ErrGroupRequired
	}
	result := make([]int64, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func normalizeURL(ctx context.Context, resolver Resolver, raw string, required, trimTrailingSlash bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" && !required {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("must be an HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("userinfo and fragments are forbidden")
	}
	if err := pricing.ValidateRemoteURL(ctx, resolver, parsed.String()); err != nil {
		return "", fmt.Errorf("remote address is unsafe")
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	result := parsed.String()
	if trimTrailingSlash {
		result = strings.TrimRight(result, "/")
	}
	return result, nil
}
