package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	ModeReadOnly = "read_only"
	ModeProbe    = "probe"
	ModeClosed   = "closed"

	FeishuCommandDisabled = "disabled"
	FeishuCommandDryRun   = "dry_run"
	FeishuCommandEnabled  = "enabled"
)

type Config struct {
	Mode                     string
	ListenAddress            string
	Timezone                 *time.Location
	TimezoneName             string
	DatabaseURLFile          string
	Sub2APIBaseURL           string
	Sub2APIAdminKeyFile      string
	AccountQualityResultFile string
	UpstreamGroupMappingFile string
	FeishuWebhookFile        string
	FeishuCommandMode        string
	FeishuAppIDFile          string
	FeishuAppSecretFile      string
	FeishuVerificationFile   string
	FeishuEncryptKeyFile     string
	FeishuRoutingFile        string
	FeishuAlertChatIDFile    string
	AgentBaseURL             string
	AgentAPIKeyFile          string
	AgentModel               string
	ProductionPageInterval   time.Duration
	CandidateInterval        time.Duration
	PublicBaseURL            string
	RubyPath                 string
	V2ScriptPath             string
	CandidateProfilePath     string
	FastProfilePath          string
	CandidateSecretDir       string
	QualificationProfilePath string
	AnalyzerCommandPath      string
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
	feishuCommandMode := get("RELAY_OPS_FEISHU_COMMAND_MODE", FeishuCommandDisabled)
	if feishuCommandMode != FeishuCommandDisabled && feishuCommandMode != FeishuCommandDryRun && feishuCommandMode != FeishuCommandEnabled {
		return Config{}, fmt.Errorf("RELAY_OPS_FEISHU_COMMAND_MODE must be disabled, dry_run, or enabled")
	}
	feishuCallbackFiles := map[string]string{
		"Feishu App ID":             get("RELAY_OPS_FEISHU_APP_ID_FILE", ""),
		"Feishu App Secret":         get("RELAY_OPS_FEISHU_APP_SECRET_FILE", ""),
		"Feishu verification token": get("RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE", ""),
		"Feishu Encrypt Key":        get("RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE", ""),
	}
	feishuRoutingFile := get("RELAY_OPS_FEISHU_ROUTING_FILE", "")
	feishuAlertChatIDFile := get("RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE", "")
	configuredCallbackFiles := 0
	for _, path := range feishuCallbackFiles {
		if path != "" {
			configuredCallbackFiles++
		}
	}
	if configuredCallbackFiles > 0 && configuredCallbackFiles != len(feishuCallbackFiles) {
		return Config{}, fmt.Errorf("Feishu callback files must be configured as a complete set")
	}
	if feishuCommandMode != FeishuCommandDisabled {
		if configuredCallbackFiles != len(feishuCallbackFiles) || feishuRoutingFile == "" {
			return Config{}, fmt.Errorf("Feishu command files must be configured as a complete set")
		}
	}
	if feishuRoutingFile != "" && configuredCallbackFiles != len(feishuCallbackFiles) {
		return Config{}, fmt.Errorf("Feishu routing config requires callback files")
	}
	if feishuAlertChatIDFile != "" && configuredCallbackFiles != len(feishuCallbackFiles) {
		return Config{}, fmt.Errorf("Feishu alert chat requires callback files")
	}
	if configuredCallbackFiles == len(feishuCallbackFiles) {
		for label, path := range feishuCallbackFiles {
			if err := validateSecretFile(path); err != nil {
				return Config{}, fmt.Errorf("%s file: %w", label, err)
			}
		}
	}
	if feishuRoutingFile != "" {
		if err := validateSecretFile(feishuRoutingFile); err != nil {
			return Config{}, fmt.Errorf("Feishu routing config file: %w", err)
		}
	}
	if feishuAlertChatIDFile != "" {
		if err := validateSecretFile(feishuAlertChatIDFile); err != nil {
			return Config{}, fmt.Errorf("Feishu alert chat ID file: %w", err)
		}
	}
	baseURL := strings.TrimRight(get("RELAY_OPS_SUB2API_URL", "http://sub2api:8080"), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("RELAY_OPS_SUB2API_URL is required")
	}
	accountQualityResultFile := get("RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE", "")
	if accountQualityResultFile != "" && !filepath.IsAbs(accountQualityResultFile) {
		return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE must be an absolute path")
	}
	upstreamGroupMappingFile := get("RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE", "")
	if upstreamGroupMappingFile != "" && !filepath.IsAbs(upstreamGroupMappingFile) {
		return Config{}, fmt.Errorf("RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE must be an absolute path")
	}
	candidateSecretDir := filepath.Clean(get("RELAY_OPS_CANDIDATE_SECRET_DIR", "/var/lib/relay-ops/candidate-keys"))
	if !filepath.IsAbs(candidateSecretDir) {
		return Config{}, fmt.Errorf("RELAY_OPS_CANDIDATE_SECRET_DIR must be an absolute path")
	}
	analyzerCommandPath := get("RELAY_OPS_ACCOUNT_MONITOR_ANALYZER_PATH", "/app/ops/analyze-account-monitor.rb")
	return Config{
		Mode:                     mode,
		ListenAddress:            get("RELAY_OPS_LISTEN_ADDRESS", ":8100"),
		Timezone:                 timezone,
		TimezoneName:             timezoneName,
		DatabaseURLFile:          databaseURLFile,
		Sub2APIBaseURL:           baseURL,
		Sub2APIAdminKeyFile:      adminKeyFile,
		AccountQualityResultFile: accountQualityResultFile,
		UpstreamGroupMappingFile: upstreamGroupMappingFile,
		FeishuWebhookFile:        feishuFile,
		FeishuCommandMode:        feishuCommandMode,
		FeishuAppIDFile:          feishuCallbackFiles["Feishu App ID"],
		FeishuAppSecretFile:      feishuCallbackFiles["Feishu App Secret"],
		FeishuVerificationFile:   feishuCallbackFiles["Feishu verification token"],
		FeishuEncryptKeyFile:     feishuCallbackFiles["Feishu Encrypt Key"],
		FeishuRoutingFile:        feishuRoutingFile,
		FeishuAlertChatIDFile:    feishuAlertChatIDFile,
		AgentBaseURL:             strings.TrimRight(get("RELAY_OPS_AGENT_BASE_URL", ""), "/"),
		AgentAPIKeyFile:          agentKeyFile,
		AgentModel:               get("RELAY_OPS_AGENT_MODEL", ""),
		ProductionPageInterval:   5 * time.Minute,
		CandidateInterval:        6 * time.Hour,
		PublicBaseURL:            strings.TrimRight(get("RELAY_OPS_PUBLIC_BASE_URL", "https://api.xingqiaolab.top"), "/"),
		RubyPath:                 get("RELAY_OPS_RUBY_PATH", "/usr/bin/ruby"),
		V2ScriptPath:             get("RELAY_OPS_V2_SCRIPT_PATH", "/app/ops/upstream-benchmark-v2.rb"),
		CandidateProfilePath:     get("RELAY_OPS_CANDIDATE_PROFILE_PATH", "/app/config/upstream-benchmarks/candidate-watch-v2.yaml"),
		FastProfilePath:          get("RELAY_OPS_FAST_PROFILE_PATH", "/app/config/upstream-benchmarks/quality-first-fast-v1.yaml"),
		CandidateSecretDir:       candidateSecretDir,
		QualificationProfilePath: get("RELAY_OPS_QUALIFICATION_PROFILE_PATH", "/app/config/upstream-benchmarks/mvp-text-v2.yaml"),
		AnalyzerCommandPath:      analyzerCommandPath,
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
	permissions := info.Mode().Perm()
	if permissions != 0o600 && permissions != 0o640 {
		return fmt.Errorf("permissions must be 0600 or 0640")
	}
	if info.Size() == 0 {
		return fmt.Errorf("must not be empty")
	}
	return nil
}
