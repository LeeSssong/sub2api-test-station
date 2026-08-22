package lab

import "testing"

func validConfig() Config {
	return Config{
		LabOnly: "1", DatabaseHost: "admin-lab-postgres", DatabaseName: "sub2api_lab",
		RedisHost: "admin-lab-redis", ServiceName: "admin-lab-api", CookieName: "sub2api_lab_session",
		JWTSecret: "lab-jwt-secret-unique", CSRFSecret: "lab-csrf-secret-unique",
		PaymentProvider: "mock", UpstreamProvider: "mock-upstream", FrontendBasePath: "/admin/lab/",
	}
}

func TestValidateConfigAcceptsIndependentLabResources(t *testing.T) {
	if err := ValidateConfig(validConfig()); err != nil {
		t.Fatalf("valid lab config rejected: %v", err)
	}
}

func TestValidateConfigRejectsProductionDatabaseAndRedis(t *testing.T) {
	for field := range map[string]bool{"database": true, "redis": true} {
		cfg := validConfig()
		if field == "database" {
			cfg.DatabaseHost = "postgres"
		} else {
			cfg.RedisHost = "redis"
		}
		if err := ValidateConfig(cfg); err == nil {
			t.Fatalf("expected %s production resource to be rejected", field)
		}
	}
}

func TestValidateConfigRejectsSharedSecretsAndProductionCookie(t *testing.T) {
	cases := []func(*Config){
		func(c *Config) { c.CookieName = "sub2api_session" },
		func(c *Config) { c.JWTSecret = "prod-shared-secret" },
		func(c *Config) { c.CSRFSecret = "" },
	}
	for i, mutate := range cases {
		cfg := validConfig()
		mutate(&cfg)
		if err := ValidateConfig(cfg); err == nil {
			t.Fatalf("case %d unexpectedly accepted", i)
		}
	}
}

func TestValidateConfigRejectsRealPaymentUpstreamAndWrongBase(t *testing.T) {
	cases := []func(*Config){
		func(c *Config) { c.PaymentProvider = "stripe" },
		func(c *Config) { c.UpstreamProvider = "openai" },
		func(c *Config) { c.FrontendBasePath = "/" },
	}
	for i, mutate := range cases {
		cfg := validConfig()
		mutate(&cfg)
		if err := ValidateConfig(cfg); err == nil {
			t.Fatalf("case %d unexpectedly accepted", i)
		}
	}
}

func TestValidateConfigRejectsNonLabAllowlist(t *testing.T) {
	cfg := validConfig()
	cfg.ExternalAllowlist = "https://api.openai.com"
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected real external endpoint to be rejected")
	}
}
