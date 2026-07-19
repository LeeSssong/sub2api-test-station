package config

import (
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	ModeReadOnly = "read_only"
	ModeProbe    = "probe"
	ModeClosed   = "closed"
)

type Config struct {
	Mode                   string
	ListenAddress          string
	Timezone               *time.Location
	TimezoneName           string
	DatabaseURLFile        string
	Sub2APIBaseURL         string
	Sub2APIAdminKeyFile    string
	FeishuWebhookFile      string
	AgentBaseURL           string
	AgentAPIKeyFile        string
	AgentModel             string
	ProductionPageInterval time.Duration
	CandidateInterval      time.Duration
}

func Load(env func(string) string) (Config, error) {
	get := func(key, fallback string) string {
		if value := strings.TrimSpace(env(key)); value != "" {
			return value
		}
		return fallback
	}
	mode := get("RELAY_OPS_MODE", ModeReadOnly)
	if mode != ModeReadOnly && mode != ModeProbe && mode != ModeClosed {
		return Config{}, fmt.Errorf("RELAY_OPS_MODE must be read_only, probe, or closed")
	}
	timezoneName := get("RELAY_OPS_TIMEZONE", "Asia/Shanghai")
	if timezoneName != "Asia/Shanghai" {
		return Config{}, fmt.Errorf("RELAY_OPS_TIMEZONE must be Asia/Shanghai")
	}
	timezone, err := time.LoadLocation(timezoneName)
	if err != nil {
		return Config{}, fmt.Errorf("load timezone: %w", err)
	}
	databaseURLFile := get("RELAY_OPS_DATABASE_URL_FILE", "/run/secrets/relay-ops-database-url")
	adminKeyFile := get("RELAY_OPS_SUB2API_ADMIN_KEY_FILE", "/run/secrets/sub2api-admin-api-key")
	for label, path := range map[string]string{
		"database URL":      databaseURLFile,
		"Sub2API admin key": adminKeyFile,
	} {
		if err := validateSecretFile(path); err != nil {
			return Config{}, fmt.Errorf("%s file: %w", label, err)
		}
	}
	feishuFile := get("RELAY_OPS_FEISHU_WEBHOOK_FILE", "")
	agentKeyFile := get("RELAY_OPS_AGENT_API_KEY_FILE", "")
	for label, path := range map[string]string{"Feishu webhook": feishuFile, "Agent API key": agentKeyFile} {
		if path != "" {
			if err := validateSecretFile(path); err != nil {
				return Config{}, fmt.Errorf("%s file: %w", label, err)
			}
		}
	}
	baseURL := strings.TrimRight(get("RELAY_OPS_SUB2API_URL", "http://sub2api:8080"), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("RELAY_OPS_SUB2API_URL is required")
	}
	return Config{
		Mode:                   mode,
		ListenAddress:          get("RELAY_OPS_LISTEN_ADDRESS", ":8100"),
		Timezone:               timezone,
		TimezoneName:           timezoneName,
		DatabaseURLFile:        databaseURLFile,
		Sub2APIBaseURL:         baseURL,
		Sub2APIAdminKeyFile:    adminKeyFile,
		FeishuWebhookFile:      feishuFile,
		AgentBaseURL:           strings.TrimRight(get("RELAY_OPS_AGENT_BASE_URL", ""), "/"),
		AgentAPIKeyFile:        agentKeyFile,
		AgentModel:             get("RELAY_OPS_AGENT_MODEL", ""),
		ProductionPageInterval: 5 * time.Minute,
		CandidateInterval:      6 * time.Hour,
	}, nil
}

func validateSecretFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("must be a regular file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("must not be writable by group or other")
	}
	if info.Size() == 0 {
		return fmt.Errorf("must not be empty")
	}
	return nil
}
