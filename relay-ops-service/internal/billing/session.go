package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/pricing"
	"github.com/PuerkitoBio/goquery"
)

var ErrUsageSchema = errors.New("upstream usage page schema changed")

type SessionConfig struct {
	UpstreamID domain.UpstreamID
	UsageURL   string
	LoginURL   string
	AuthMode   string
	SecretRef  string
}

type UsageEvidence struct {
	UpstreamID          domain.UpstreamID
	ObservedAt          time.Time
	StandardCost        domain.MicroUSD
	ActualCost          domain.MicroUSD
	HasActualCost       bool
	EffectiveMultiplier domain.MultiplierBPS
	Note                string
}

type SessionExpiredError struct {
	UpstreamID domain.UpstreamID
	LoginURL   string
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("upstream usage session expired; log in again at %s", e.LoginURL)
}

type SessionReporter interface {
	RecordExpired(context.Context, domain.UpstreamID, string, time.Time) (bool, error)
	RecordHealthy(context.Context, domain.UpstreamID, time.Time) error
}

type SessionReader struct {
	Client   *http.Client
	Resolver pricing.Resolver
	Reporter SessionReporter
	Now      func() time.Time
}

func (s SessionReader) ReadUsage(ctx context.Context, cfg SessionConfig) (UsageEvidence, error) {
	if cfg.UpstreamID <= 0 || cfg.UsageURL == "" || cfg.LoginURL == "" {
		return UsageEvidence{}, fmt.Errorf("usage session configuration is incomplete")
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := pricing.ValidateRemoteURL(ctx, resolver, cfg.UsageURL); err != nil {
		return UsageEvidence{}, err
	}
	if err := pricing.ValidateRemoteURL(ctx, resolver, cfg.LoginURL); err != nil {
		return UsageEvidence{}, err
	}
	secretPath, err := sessionSecretPath(cfg.SecretRef)
	if err != nil {
		return UsageEvidence{}, err
	}
	secret, err := readSessionSecret(secretPath)
	if err != nil {
		return UsageEvidence{}, err
	}
	defer clearSessionSecret(secret)
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}

	var responseBody []byte
	for attempt := 0; attempt < 2; attempt++ {
		status, body, err := s.request(ctx, resolver, cfg, secret)
		if err != nil {
			return UsageEvidence{}, err
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			if attempt == 0 {
				continue
			}
			if s.Reporter != nil {
				_, _ = s.Reporter.RecordExpired(ctx, cfg.UpstreamID, cfg.LoginURL, now)
			}
			return UsageEvidence{}, &SessionExpiredError{UpstreamID: cfg.UpstreamID, LoginURL: cfg.LoginURL}
		}
		if status < 200 || status >= 300 {
			return UsageEvidence{}, fmt.Errorf("upstream usage page returned HTTP %d", status)
		}
		responseBody = body
		break
	}
	standard, actual, hasActual, err := parseUsage(responseBody)
	if err != nil {
		return UsageEvidence{}, err
	}
	evidence := UsageEvidence{UpstreamID: cfg.UpstreamID, ObservedAt: now, StandardCost: standard, ActualCost: actual, HasActualCost: hasActual}
	if hasActual {
		evidence.EffectiveMultiplier, err = EstimateEffectiveMultiplier(standard, actual)
		if err != nil {
			return UsageEvidence{}, err
		}
		evidence.Note = "上游用量页辅助证据"
	} else {
		evidence.Note = "按公开定价估算"
	}
	if s.Reporter != nil {
		_ = s.Reporter.RecordHealthy(ctx, cfg.UpstreamID, now)
	}
	return evidence, nil
}

func (s SessionReader) request(ctx context.Context, resolver pricing.Resolver, cfg SessionConfig, secret []byte) (int, []byte, error) {
	base := s.Client
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("usage redirect limit exceeded")
		}
		return pricing.ValidateRemoteURL(req.Context(), resolver, req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UsageURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build usage request: %w", err)
	}
	switch cfg.AuthMode {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+string(secret))
	case "cookie":
		req.Header.Set("Cookie", string(secret))
	default:
		return 0, nil, fmt.Errorf("unsupported usage auth mode")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("read upstream usage page")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return 0, nil, fmt.Errorf("read upstream usage page")
	}
	return resp.StatusCode, body, nil
}

func parseUsage(body []byte) (domain.MicroUSD, domain.MicroUSD, bool, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err == nil {
		standard, standardOK := costField(document, "standard_cost")
		actual, actualOK := costField(document, "actual_cost")
		if standardOK {
			return standard, actual, actualOK, nil
		}
	}
	html, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err == nil {
		selection := html.Find("[data-standard-cost]").First()
		if selection.Length() > 0 {
			standard, standardErr := domain.ParseMicroUSD(selection.AttrOr("data-standard-cost", ""))
			actualRaw, actualExists := selection.Attr("data-actual-cost")
			actual, actualErr := domain.ParseMicroUSD(actualRaw)
			if standardErr == nil && (!actualExists || actualErr == nil) {
				return standard, actual, actualExists, nil
			}
		}
	}
	return 0, 0, false, ErrUsageSchema
}

func costField(document map[string]any, key string) (domain.MicroUSD, bool) {
	value, ok := document[key]
	if !ok {
		if data, nested := document["data"].(map[string]any); nested {
			value, ok = data[key]
		}
	}
	if !ok {
		return 0, false
	}
	parsed, err := domain.ParseMicroUSD(fmt.Sprint(value))
	return parsed, err == nil
}

func sessionSecretPath(reference string) (string, error) {
	if !strings.HasPrefix(reference, "file:") {
		return "", fmt.Errorf("usage session secret must use file scheme")
	}
	path := filepath.Clean(strings.TrimPrefix(reference, "file:"))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("usage session secret path must be absolute")
	}
	return path, nil
}

func readSessionSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("usage session secret is unavailable or unsafe")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) || info.Size() <= 0 || info.Size() > 64<<10 {
		return nil, fmt.Errorf("usage session secret is unavailable or unsafe")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("usage session secret is unavailable")
	}
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return nil, fmt.Errorf("usage session secret is empty")
	}
	return value, nil
}

func clearSessionSecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
