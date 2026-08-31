package billing

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/pricing"
)

func TestSessionReaderExtractsBearerUsageWithoutLeakingCredential(t *testing.T) {
	t.Parallel()
	secret := writeSessionSecret(t, 0o600, "billing-token-secret")
	client := &http.Client{Transport: billingTransport(func(request *http.Request) *http.Response {
		if request.Header.Get("Authorization") != "Bearer billing-token-secret" {
			t.Errorf("authorization missing")
		}
		return billingResponse(http.StatusOK, `{"standard_cost":"10.000000","actual_cost":"1.000000"}`)
	})}
	reader := SessionReader{Client: client, Resolver: billingResolver{}, SecretRoot: filepath.Dir(secret)}
	evidence, err := reader.ReadUsage(context.Background(), SessionConfig{UpstreamID: 7, UsageURL: "https://usage.example/usage", LoginURL: "https://usage.example/login", AuthMode: "bearer", SecretRef: "file:" + secret})
	if err != nil || evidence.StandardCost != 10_000_000 || evidence.ActualCost != 1_000_000 || evidence.EffectiveMultiplier != 1_000 {
		t.Fatalf("evidence=%#v err=%v", evidence, err)
	}
	if strings.Contains(evidence.Note, "billing-token-secret") {
		t.Fatal("evidence leaked credential")
	}
}

func TestSessionReaderClassifiesExpiredSessionAfterOneRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	client := &http.Client{Transport: billingTransport(func(*http.Request) *http.Response {
		calls++
		return billingResponse(http.StatusUnauthorized, `{"error":"cookie-secret-must-not-leak"}`)
	})}
	reporter := &fakeSessionReporter{}
	secret := writeSessionSecret(t, 0o600, "session=cookie-secret")
	reader := SessionReader{Client: client, Resolver: billingResolver{}, Reporter: reporter, Now: func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }, SecretRoot: filepath.Dir(secret)}
	_, err := reader.ReadUsage(context.Background(), SessionConfig{UpstreamID: 8, UsageURL: "https://usage.example/usage", LoginURL: "https://usage.example/login", AuthMode: "cookie", SecretRef: "file:" + secret})
	var expired *SessionExpiredError
	if !errors.As(err, &expired) || expired.LoginURL != "https://usage.example/login" || calls != 2 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	if strings.Contains(err.Error(), "cookie-secret") || reporter.loginURL != expired.LoginURL || !reporter.expired {
		t.Fatalf("expired=%#v reporter=%#v", expired, reporter)
	}
}

func TestSessionReaderClassifiesLoginHTMLAsExpired(t *testing.T) {
	t.Parallel()
	calls := 0
	client := &http.Client{Transport: billingTransport(func(*http.Request) *http.Response {
		calls++
		return billingResponse(http.StatusOK, `<form action="/login"><input type="password"></form>`)
	})}
	reporter := &fakeSessionReporter{}
	secret := writeSessionSecret(t, 0o600, "session=opaque")
	reader := SessionReader{Client: client, Resolver: billingResolver{}, Reporter: reporter, Now: func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }, SecretRoot: filepath.Dir(secret)}
	_, err := reader.ReadUsage(context.Background(), SessionConfig{UpstreamID: 10, UsageURL: "https://usage.example/usage", LoginURL: "https://usage.example/login", AuthMode: "cookie", SecretRef: "file:" + secret})
	var expired *SessionExpiredError
	if !errors.As(err, &expired) || calls != 2 || !reporter.expired {
		t.Fatalf("error=%v calls=%d expired=%#v", err, calls, expired)
	}
}

func TestSessionReaderRejectsUnsafeURLsWritableSecretsAndSchemaChanges(t *testing.T) {
	t.Parallel()
	secret := writeSessionSecret(t, 0o600, "token-value")
	base := SessionConfig{UpstreamID: 9, UsageURL: "https://127.0.0.1/usage", LoginURL: "https://usage.example/login", AuthMode: "bearer", SecretRef: "file:" + secret}
	if _, err := (SessionReader{Resolver: billingResolver{}, SecretRoot: filepath.Dir(secret)}).ReadUsage(context.Background(), base); !pricing.IsUnsafeURL(err) {
		t.Fatalf("unsafe error=%v", err)
	}
	base.UsageURL = "https://usage.example/usage"
	secret = writeSessionSecret(t, 0o666, "token-value")
	base.SecretRef = "file:" + secret
	if _, err := (SessionReader{Resolver: billingResolver{}, SecretRoot: filepath.Dir(secret)}).ReadUsage(context.Background(), base); err == nil {
		t.Fatal("writable session secret accepted")
	}
	secret = writeSessionSecret(t, 0o600, "token-value")
	base.SecretRef = "file:" + secret
	client := &http.Client{Transport: billingTransport(func(*http.Request) *http.Response { return billingResponse(http.StatusOK, `{"balance":"9"}`) })}
	if _, err := (SessionReader{Client: client, Resolver: billingResolver{}, SecretRoot: filepath.Dir(secret)}).ReadUsage(context.Background(), base); !errors.Is(err, ErrUsageSchema) {
		t.Fatalf("schema error=%v", err)
	}
}

func TestEstimateEffectiveMultiplier(t *testing.T) {
	t.Parallel()
	result, err := EstimateEffectiveMultiplier(domain.MicroUSD(10_000_000), domain.MicroUSD(700_000))
	if err != nil || result != 700 {
		t.Fatalf("result=%d err=%v", result, err)
	}
	if _, err := EstimateEffectiveMultiplier(0, 1); err == nil {
		t.Fatal("zero standard cost accepted")
	}
}

type fakeSessionReporter struct {
	expired  bool
	loginURL string
}

func (r *fakeSessionReporter) RecordExpired(_ context.Context, _ domain.UpstreamID, loginURL string, _ time.Time) error {
	r.expired = true
	r.loginURL = loginURL
	return nil
}

func (r *fakeSessionReporter) RecordHealthy(context.Context, domain.UpstreamID, time.Time) error {
	return nil
}

type billingResolver struct{}

func (billingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.30")}}, nil
}

type billingTransport func(*http.Request) *http.Response

func (fn billingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request), nil
}

func billingResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func writeSessionSecret(t *testing.T, mode os.FileMode, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session-secret")
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}
