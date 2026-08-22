// Package lab contains fail-closed safety checks for the isolated admin lab.
package lab

import (
	"fmt"
	"net/url"
	"strings"
)

// Config is the minimal environment contract required before a lab process may start.
type Config struct {
	LabOnly               string
	DatabaseHost          string
	DatabaseName          string
	RedisHost             string
	ServiceName           string
	CookieName            string
	JWTSecret             string
	CSRFSecret            string
	PaymentProvider       string
	UpstreamProvider      string
	ExternalAllowlist     string
	FrontendBasePath      string
	NotificationTransport string
}

// ValidateConfig rejects ambiguous or production-like resource configuration.
func ValidateConfig(cfg Config) error {
	checks := []struct {
		field string
		value string
		bad   func(string) bool
	}{
		{"LAB_ONLY", cfg.LabOnly, func(v string) bool { return v != "1" }},
		{"DATABASE_HOST", cfg.DatabaseHost, func(v string) bool {
			return v == "" || v == "postgres" || v == "sub2api-postgres" || strings.Contains(v, "prod")
		}},
		{"DATABASE_DBNAME", cfg.DatabaseName, func(v string) bool { return v == "" || v == "sub2api" || strings.Contains(v, "prod") }},
		{"REDIS_HOST", cfg.RedisHost, func(v string) bool {
			return v == "" || v == "redis" || v == "sub2api-redis" || strings.Contains(v, "prod")
		}},
		{"SERVICE_NAME", cfg.ServiceName, func(v string) bool { return v == "" || v == "sub2api" || v == "sub2api-blue" || v == "sub2api-green" }},
		{"COOKIE_NAME", cfg.CookieName, func(v string) bool { return v == "" || v == "sub2api_session" || !strings.HasPrefix(v, "sub2api_lab_") }},
		{"JWT_SECRET", cfg.JWTSecret, func(v string) bool { return v == "" || v == "lab-shared-secret" || v == "prod-shared-secret" }},
		{"CSRF_SECRET", cfg.CSRFSecret, func(v string) bool { return v == "" || v == "lab-shared-secret" || v == "prod-shared-secret" }},
		{"PAYMENT_PROVIDER", cfg.PaymentProvider, func(v string) bool { return v != "mock" }},
		{"UPSTREAM_PROVIDER", cfg.UpstreamProvider, func(v string) bool { return v != "mock-upstream" }},
		{"FRONTEND_BASE_PATH", cfg.FrontendBasePath, func(v string) bool { return v != "/admin/lab/" }},
		{"NOTIFICATION_TRANSPORT", cfg.NotificationTransport, func(v string) bool { return v != "lab-outbox" }},
	}
	for _, check := range checks {
		if check.bad(strings.TrimSpace(check.value)) {
			return fmt.Errorf("lab config rejected: %s is missing or not lab-scoped", check.field)
		}
	}
	if cfg.ExternalAllowlist != "" {
		for _, raw := range strings.Split(cfg.ExternalAllowlist, ",") {
			candidate := strings.TrimSpace(raw)
			if candidate == "" {
				continue
			}
			u, err := url.Parse(candidate)
			if err != nil || u.Host == "" || !strings.HasPrefix(u.Host, "admin-lab-") {
				return fmt.Errorf("lab config rejected: EXTERNAL_ALLOWLIST contains non-lab endpoint")
			}
		}
	}
	return nil
}
