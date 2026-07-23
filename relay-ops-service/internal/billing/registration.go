package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/pricing"
)

const sessionSecretRoot = "/run/secrets/upstream-sessions"

type SessionInput struct {
	UpstreamID domain.UpstreamID
	AuthMode   string
	SecretFile string
	LoginURL   string
}

type SessionSecretRef struct {
	SecretRef   string
	Kind        string
	OwnerScope  string
	Fingerprint string
	LastFour    string
}

type SessionAuditEvent struct {
	ActorUserID  int64
	Action       string
	ObjectType   string
	AfterSummary map[string]string
}

type SessionRecord struct {
	Config SessionConfig
	Secret SessionSecretRef
	Audit  SessionAuditEvent
}

type SessionRegistrationRepository interface {
	UpsertUsageSession(context.Context, SessionRecord) (SessionConfig, error)
}

type SessionRegistrationService struct {
	Repository SessionRegistrationRepository
	Resolver   pricing.Resolver
}

func (s SessionRegistrationService) Configure(ctx context.Context, actor domain.AdminActor, input SessionInput) (SessionConfig, error) {
	if s.Repository == nil {
		return SessionConfig{}, fmt.Errorf("usage session repository is required")
	}
	if actor.UserID <= 0 || input.UpstreamID <= 0 {
		return SessionConfig{}, fmt.Errorf("administrator and upstream are required")
	}
	authMode := strings.ToLower(strings.TrimSpace(input.AuthMode))
	if authMode != "cookie" && authMode != "bearer" {
		return SessionConfig{}, fmt.Errorf("usage auth mode must be cookie or bearer")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	loginURL := strings.TrimSpace(input.LoginURL)
	if err := pricing.ValidateRemoteURL(ctx, resolver, loginURL); err != nil {
		return SessionConfig{}, fmt.Errorf("login URL is unsafe")
	}
	secret, err := inspectSessionSecret(input.SecretFile, input.UpstreamID)
	if err != nil {
		return SessionConfig{}, err
	}
	record := SessionRecord{
		Config: SessionConfig{UpstreamID: input.UpstreamID, LoginURL: loginURL, AuthMode: authMode, SecretRef: secret.SecretRef},
		Secret: secret,
		Audit: SessionAuditEvent{
			ActorUserID: actor.UserID, Action: "upstream.usage_session.configure", ObjectType: "auth_session",
			AfterSummary: map[string]string{"upstream_id": fmt.Sprint(input.UpstreamID), "auth_mode": authMode, "login_url": loginURL},
		},
	}
	return s.Repository.UpsertUsageSession(ctx, record)
}

func inspectSessionSecret(rawPath string, upstreamID domain.UpstreamID) (SessionSecretRef, error) {
	path := filepath.Clean(strings.TrimSpace(rawPath))
	relative, err := filepath.Rel(sessionSecretRoot, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return SessionSecretRef{}, fmt.Errorf("usage session secret must be inside %s", sessionSecretRoot)
	}
	secret, err := readSessionSecret(path)
	if err != nil {
		return SessionSecretRef{}, err
	}
	defer clearSessionSecret(secret)
	if len(secret) < 4 {
		return SessionSecretRef{}, fmt.Errorf("usage session secret is too short")
	}
	sum := sha256.Sum256(secret)
	return SessionSecretRef{
		SecretRef: "file:" + path, Kind: "upstream_usage_session", OwnerScope: fmt.Sprint(upstreamID),
		Fingerprint: hex.EncodeToString(sum[:]), LastFour: string(secret[len(secret)-4:]),
	}, nil
}
