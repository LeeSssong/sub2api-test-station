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
	UpstreamID       domain.UpstreamID
	AuthMode         string
	SecretFile       string
	LoginURL         string
	BillingAccountID int64
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
	record, err := s.Prepare(ctx, actor, input)
	if err != nil {
		return SessionConfig{}, err
	}
	return s.Repository.UpsertUsageSession(ctx, record)
}

// Prepare validates a session configuration without writing it.
func (s SessionRegistrationService) Prepare(ctx context.Context, actor domain.AdminActor, input SessionInput) (SessionRecord, error) {
	if actor.UserID <= 0 || input.UpstreamID <= 0 {
		return SessionRecord{}, fmt.Errorf("administrator and upstream are required")
	}
	authMode := strings.ToLower(strings.TrimSpace(input.AuthMode))
	if authMode != "cookie" && authMode != "bearer" {
		return SessionRecord{}, fmt.Errorf("usage auth mode must be cookie or bearer")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	loginURL := strings.TrimSpace(input.LoginURL)
	if err := pricing.ValidateRemoteURL(ctx, resolver, loginURL); err != nil {
		return SessionRecord{}, fmt.Errorf("login URL is unsafe")
	}
	secret, err := inspectSessionSecret(input.SecretFile, input.UpstreamID)
	if err != nil {
		return SessionRecord{}, err
	}
	scope := "usage_read"
	if authMode == "bearer" && input.BillingAccountID > 0 {
		scope = "billing_read"
	}
	return SessionRecord{
		Config: SessionConfig{UpstreamID: input.UpstreamID, LoginURL: loginURL, AuthMode: authMode, SecretRef: secret.SecretRef, Scope: scope, BillingAccountID: input.BillingAccountID},
		Secret: secret,
		Audit: SessionAuditEvent{
			ActorUserID: actor.UserID, Action: "upstream.usage_session.configure", ObjectType: "auth_session",
			AfterSummary: map[string]string{"upstream_id": fmt.Sprint(input.UpstreamID), "auth_mode": authMode, "login_url": loginURL, "scope": scope},
		},
	}, nil
}

// BillingReadSessionRecord is a prepared bearer credential reference for the
// atomic billing provisioner. Credential bytes are never retained.
type BillingReadSessionRecord struct {
	LoginURL         string
	BillingAccountID int64
	Secret           SessionSecretRef
	Audit            SessionAuditEvent
}

// PrepareBillingRead validates an active bearer billing session before the
// provisioner opens its database transaction. The upstream identity is bound
// by the store transaction, so no partial upstream/session state is possible.
func (s SessionRegistrationService) PrepareBillingRead(ctx context.Context, actor domain.AdminActor, secretFile, loginURL string, billingAccountID int64) (BillingReadSessionRecord, error) {
	if actor.UserID <= 0 || billingAccountID <= 0 {
		return BillingReadSessionRecord{}, fmt.Errorf("administrator and billing account are required")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	loginURL = strings.TrimSpace(loginURL)
	if err := pricing.ValidateRemoteURL(ctx, resolver, loginURL); err != nil {
		return BillingReadSessionRecord{}, fmt.Errorf("login URL is unsafe")
	}
	secret, err := inspectSessionSecret(secretFile, 0)
	if err != nil {
		return BillingReadSessionRecord{}, err
	}
	secret.OwnerScope = ""
	return BillingReadSessionRecord{
		LoginURL: loginURL, BillingAccountID: billingAccountID, Secret: secret,
		Audit: SessionAuditEvent{ActorUserID: actor.UserID, Action: "upstream.billing_session.provision", ObjectType: "auth_session",
			AfterSummary: map[string]string{"auth_mode": "bearer", "login_url": loginURL, "scope": "billing_read", "billing_account_id": fmt.Sprint(billingAccountID)}},
	}, nil
}

func inspectSessionSecret(rawPath string, upstreamID domain.UpstreamID) (SessionSecretRef, error) {
	path := filepath.Clean(strings.TrimSpace(rawPath))
	relative, err := filepath.Rel(sessionSecretRoot, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return SessionSecretRef{}, fmt.Errorf("usage session secret must be inside %s", sessionSecretRoot)
	}
	secret, err := readBillingSecret("file:"+path, sessionSecretRoot)
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
