package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var ErrAuthenticationFailed = errors.New("updater authentication failed")

// IdentityVerifier verifies a browser session against the official Sub2API API.
type IdentityVerifier interface {
	Verify(ctx context.Context, bearer string, requestHeaders http.Header) (Identity, error)
}

// OfficialAuthenticator forwards an existing bearer session to /api/v1/auth/me.
type OfficialAuthenticator struct {
	baseURL *url.URL
	client  *http.Client
}

func NewOfficialAuthenticator(baseURL string, client *http.Client) (*OfficialAuthenticator, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid official API base URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &OfficialAuthenticator{baseURL: u, client: client}, nil
}

func (a *OfficialAuthenticator) Verify(ctx context.Context, bearer string, requestHeaders http.Header) (Identity, error) {
	if a == nil || a.baseURL == nil || a.client == nil {
		return Identity{}, ErrAuthenticationFailed
	}
	u := a.baseURL.JoinPath("api", "v1", "auth", "me")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Identity{}, fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	for _, name := range []string{"User-Agent", "X-Forwarded-For", "X-Real-IP"} {
		if value := requestHeaders.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}

	res, err := a.client.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("verify official session: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Identity{}, ErrAuthenticationFailed
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID     int64  `json:"id"`
			Role   string `json:"role"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload); err != nil || payload.Code != 0 {
		return Identity{}, ErrAuthenticationFailed
	}
	return Identity{ID: payload.Data.ID, Role: payload.Data.Role, Status: payload.Data.Status}, nil
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func isActiveAdmin(identity Identity) bool {
	return identity.ID > 0 && identity.Role == "admin" && identity.Status == "active"
}
