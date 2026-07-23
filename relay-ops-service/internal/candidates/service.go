package candidates

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"example.invalid/relay-ops-service/internal/domain"
)

var ErrConflict = errors.New("candidate already exists")
var ErrNotFound = errors.New("candidate not found")
var ErrCreateFailed = errors.New("candidate create failed")

const MaxProbeKeyBytes = 8 << 10

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type CandidateInput struct {
	Name           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	ProbeKeyFile   string
}

type CandidateIntakeInput struct {
	Name           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	ProbeKey       []byte
}

type Candidate struct {
	ID             domain.UpstreamID
	Name           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	ProbeSecretRef string
	Enabled        bool
}

type SecretRef struct {
	SecretRef   string
	Kind        string
	OwnerScope  string
	Fingerprint string
	LastFour    string
}

type AuditEvent struct {
	ActorUserID  int64
	Action       string
	ObjectType   string
	AfterSummary map[string]string
}

type CreateRecord struct {
	Candidate Candidate
	SecretRef SecretRef
	Audit     AuditEvent
}

type Repository interface {
	CreateCandidate(context.Context, CreateRecord) (domain.UpstreamID, error)
	ListCandidates(context.Context) ([]Candidate, error)
	DisableCandidate(context.Context, domain.UpstreamID, AuditEvent) error
}

func (s Service) List(ctx context.Context, actor domain.AdminActor) ([]Candidate, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("candidate repository is required")
	}
	if actor.UserID <= 0 {
		return nil, fmt.Errorf("administrator identity is required")
	}
	return s.Repository.ListCandidates(ctx)
}

func (s Service) Disable(ctx context.Context, actor domain.AdminActor, upstreamID domain.UpstreamID) error {
	if s.Repository == nil {
		return fmt.Errorf("candidate repository is required")
	}
	if actor.UserID <= 0 {
		return fmt.Errorf("administrator identity is required")
	}
	if upstreamID <= 0 {
		return fmt.Errorf("candidate ID is required")
	}
	return s.Repository.DisableCandidate(ctx, upstreamID, AuditEvent{
		ActorUserID: actor.UserID,
		Action:      "candidate.disable",
		ObjectType:  "upstream",
		AfterSummary: map[string]string{
			"enabled": "false",
		},
	})
}

type Service struct {
	Repository  Repository
	Resolver    Resolver
	SecretStore SecretStore
}

type secretCleanupError struct{ cause error }

func (e secretCleanupError) Error() string { return "candidate secret cleanup failed" }
func (e secretCleanupError) Unwrap() error { return e.cause }

type createOperationError struct{ cause error }

func (e createOperationError) Error() string   { return ErrCreateFailed.Error() }
func (e createOperationError) Unwrap() []error { return []error{ErrCreateFailed, e.cause} }

func (s Service) CreateWithKey(ctx context.Context, actor domain.AdminActor, input CandidateIntakeInput) (Candidate, error) {
	defer clear(input.ProbeKey)
	if s.SecretStore == nil {
		return Candidate{}, fmt.Errorf("candidate secret store is required")
	}
	path, err := s.SecretStore.Install(input.Name, input.ProbeKey)
	if err != nil {
		return Candidate{}, err
	}
	created, err := s.Create(ctx, actor, CandidateInput{
		Name: input.Name, BaseURL: input.BaseURL, PricingURL: input.PricingURL,
		UsageURL: input.UsageURL, PerformanceURL: input.PerformanceURL, ProbeKeyFile: path,
	})
	if err == nil {
		return created, nil
	}
	if cleanupErr := s.SecretStore.Remove(path); cleanupErr != nil {
		return Candidate{}, secretCleanupError{cause: err}
	}
	return Candidate{}, err
}

func (s Service) Create(ctx context.Context, actor domain.AdminActor, input CandidateInput) (Candidate, error) {
	if s.Repository == nil {
		return Candidate{}, fmt.Errorf("candidate repository is required")
	}
	if actor.UserID <= 0 {
		return Candidate{}, fmt.Errorf("administrator identity is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 100 {
		return Candidate{}, fmt.Errorf("candidate name must be 1-100 characters")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	baseURL, err := validateURL(ctx, resolver, input.BaseURL, true)
	if err != nil {
		return Candidate{}, fmt.Errorf("base URL: %w", err)
	}
	pricingURL, err := validateURL(ctx, resolver, input.PricingURL, false)
	if err != nil {
		return Candidate{}, fmt.Errorf("pricing URL: %w", err)
	}
	usageURL, err := validateURL(ctx, resolver, input.UsageURL, false)
	if err != nil {
		return Candidate{}, fmt.Errorf("usage URL: %w", err)
	}
	performanceURL := ""
	if strings.TrimSpace(input.PerformanceURL) != "" {
		performanceURL, err = validateURL(ctx, resolver, input.PerformanceURL, false)
		if err != nil {
			return Candidate{}, fmt.Errorf("performance URL: %w", err)
		}
	}
	secretRef, err := inspectSecretFile(input.ProbeKeyFile, name)
	if err != nil {
		return Candidate{}, err
	}
	candidate := Candidate{
		Name: name, BaseURL: baseURL, PricingURL: pricingURL, UsageURL: usageURL,
		PerformanceURL: performanceURL, ProbeSecretRef: secretRef.SecretRef, Enabled: true,
	}
	record := CreateRecord{
		Candidate: candidate,
		SecretRef: secretRef,
		Audit: AuditEvent{
			ActorUserID: actor.UserID, Action: "candidate.create", ObjectType: "upstream",
			AfterSummary: map[string]string{"name": name, "base_url": baseURL},
		},
	}
	id, err := s.Repository.CreateCandidate(ctx, record)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Candidate{}, ErrConflict
		}
		return Candidate{}, createOperationError{cause: err}
	}
	candidate.ID = id
	return candidate, nil
}

func validateURL(ctx context.Context, resolver Resolver, raw string, trimTrailingSlash bool) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("must be an HTTPS URL")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("userinfo is forbidden")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("fragment is forbidden")
	}
	host := parsed.Hostname()
	addresses := make([]net.IPAddr, 0, 1)
	if ip := net.ParseIP(host); ip != nil {
		addresses = append(addresses, net.IPAddr{IP: ip})
	} else {
		addresses, err = resolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return "", fmt.Errorf("hostname cannot be resolved")
		}
	}
	for _, address := range addresses {
		if unsafeIP(address.IP) {
			return "", fmt.Errorf("private or local address is forbidden")
		}
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	result := parsed.String()
	if trimTrailingSlash {
		result = strings.TrimRight(result, "/")
	}
	return result, nil
}

func unsafeIP(ip net.IP) bool {
	return ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func inspectSecretFile(path, owner string) (SecretRef, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	info, err := os.Stat(path)
	if err != nil {
		return SecretRef{}, fmt.Errorf("probe key file: %w", err)
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) {
		return SecretRef{}, fmt.Errorf("probe key file must be regular with permissions 0600 or 0640")
	}
	if info.Size() <= 0 || info.Size() > MaxProbeKeyBytes {
		return SecretRef{}, fmt.Errorf("probe key file size is invalid")
	}
	rawKey, err := os.ReadFile(path)
	if err != nil {
		return SecretRef{}, fmt.Errorf("read probe key file: %w", err)
	}
	defer func() {
		for index := range rawKey {
			rawKey[index] = 0
		}
	}()
	key := bytes.TrimSpace(rawKey)
	if len(key) < 4 {
		return SecretRef{}, fmt.Errorf("probe key is too short")
	}
	sum := sha256.Sum256(key)
	return SecretRef{
		SecretRef: "file:" + path, Kind: "candidate_probe_key", OwnerScope: owner,
		Fingerprint: hex.EncodeToString(sum[:]), LastFour: string(key[len(key)-4:]),
	}, nil
}
