package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"example.invalid/relay-ops-service/internal/notificationpolicy"
)

const (
	ModeReadOnly = "read_only"
	ModeProbe    = "probe"
	ModeClosed   = "closed"
)

type Config struct {
	Mode                            string
	ListenAddress                   string
	Timezone                        *time.Location
	TimezoneName                    string
	DatabaseURLFile                 string
	Sub2APIBaseURL                  string
	Sub2APIAdminKeyFile             string
	Sub2APIAlertReadDatabaseURLFile string
	AccountQualityResultFile        string
	UpstreamGroupMappingFile        string
	FeishuWebhookFile               string
	FeishuAppIDFile                 string
	FeishuAppSecretFile             string
	FeishuAlertChatIDFile           string
	FeishuAlertRecipientsFile       string
	NotificationPolicyFile          string
	NotificationPolicy              notificationpolicy.Policy
	AgentBaseURL                    string
	AgentAPIKeyFile                 string
	AgentModel                      string
	ProductionPageInterval          time.Duration
	CandidateInterval               time.Duration
	PublicBaseURL                   string
	RubyPath                        string
	V2ScriptPath                    string
	CandidateProfilePath            string
	FastProfilePath                 string
	CandidateSecretDir              string
	QualificationProfilePath        string
	AnalyzerCommandPath             string
	AccountingEnabled               bool
	AccountingLedgerStartDate       time.Time
	AccountingInternalUserIDs       []int64
	AccountingInternalAPIKeyIDs     []int64
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
	accountingEnabled, err := parseBool(get("RELAY_OPS_ACCOUNTING_ENABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_ENABLED must be true or false")
	}
	accountingStartDateText := get("RELAY_OPS_ACCOUNTING_LEDGER_START_DATE", "")
	var accountingStartDate time.Time
	if accountingEnabled {
		if accountingStartDateText == "" {
			return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_LEDGER_START_DATE is required when accounting is enabled")
		}
		parsed, parseErr := time.ParseInLocation("2006-01-02", accountingStartDateText, timezone)
		if parseErr != nil || parsed.Format("2006-01-02") != accountingStartDateText {
			return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_LEDGER_START_DATE must be YYYY-MM-DD")
		}
		accountingStartDate = parsed
	} else if accountingStartDateText != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", accountingStartDateText, timezone)
		if parseErr != nil || parsed.Format("2006-01-02") != accountingStartDateText {
			return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_LEDGER_START_DATE must be YYYY-MM-DD")
		}
		accountingStartDate = parsed
	}
	accountingInternalUserIDs, err := parsePositiveIDList(get("RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS", ""))
	if err != nil {
		return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS: %w", err)
	}
	accountingInternalAPIKeyIDs, err := parsePositiveIDList(get("RELAY_OPS_ACCOUNTING_INTERNAL_API_KEY_IDS", ""))
	if err != nil {
		return Config{}, fmt.Errorf("RELAY_OPS_ACCOUNTING_INTERNAL_API_KEY_IDS: %w", err)
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
	feishuAppIDFile := get("RELAY_OPS_FEISHU_APP_ID_FILE", "")
	feishuAppSecretFile := get("RELAY_OPS_FEISHU_APP_SECRET_FILE", "")
	feishuAlertChatIDFile := get("RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE", "")
	feishuAlertRecipientsFile := get("RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE", "")
	notificationPolicyFile := get("RELAY_OPS_NOTIFICATION_POLICY_FILE", "")
	if (feishuAppIDFile == "") != (feishuAppSecretFile == "") {
		return Config{}, fmt.Errorf("Feishu App ID and App Secret files must be configured together")
	}
	if feishuAlertChatIDFile != "" && feishuAppIDFile == "" {
		return Config{}, fmt.Errorf("Feishu alert chat requires App ID and App Secret files")
	}
	if feishuAlertChatIDFile != "" && feishuAlertRecipientsFile == "" {
		return Config{}, fmt.Errorf("Feishu alert chat requires an alert recipients file")
	}
	if feishuAlertRecipientsFile != "" && feishuAlertChatIDFile == "" {
		return Config{}, fmt.Errorf("Feishu alert recipients require an alert chat")
	}
	if feishuAppIDFile != "" {
		for label, path := range map[string]string{
			"Feishu App ID":     feishuAppIDFile,
			"Feishu App Secret": feishuAppSecretFile,
		} {
			if err := validateSecretFile(path); err != nil {
				return Config{}, fmt.Errorf("%s file: %w", label, err)
			}
		}
	}
	if feishuAlertChatIDFile != "" {
		if err := validateSecretFile(feishuAlertChatIDFile); err != nil {
			return Config{}, fmt.Errorf("Feishu alert chat ID file: %w", err)
		}
	}
	if feishuAlertRecipientsFile != "" {
		if err := validateSecretFile(feishuAlertRecipientsFile); err != nil {
			return Config{}, fmt.Errorf("Feishu alert recipients file: %w", err)
		}
	}
	alertTransportConfigured := feishuFile != "" || feishuAlertChatIDFile != ""
	if alertTransportConfigured && notificationPolicyFile == "" {
		return Config{}, fmt.Errorf("Feishu alert transport requires a notification policy file")
	}
	var notificationPolicy notificationpolicy.Policy
	if notificationPolicyFile != "" {
		if err := validateSecretFile(notificationPolicyFile); err != nil {
			return Config{}, fmt.Errorf("notification policy file: %w", err)
		}
		notificationPolicy, err = notificationpolicy.Load(notificationPolicyFile)
		if err != nil {
			return Config{}, fmt.Errorf("notification policy file: %w", err)
		}
	}
	sub2APIAlertReadDatabaseURLFile := get("RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE", "")
	if notificationPolicy.Feishu.NativeOpsAlertsEnabled {
		if sub2APIAlertReadDatabaseURLFile == "" {
			return Config{}, fmt.Errorf("RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE is required when native ops alerts are enabled")
		}
		if !filepath.IsAbs(sub2APIAlertReadDatabaseURLFile) {
			return Config{}, fmt.Errorf("RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE must be an absolute path")
		}
		if err := validateSecretFile(sub2APIAlertReadDatabaseURLFile); err != nil {
			return Config{}, fmt.Errorf("Sub2API alert read database URL file: %w", err)
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
		Mode:                            mode,
		ListenAddress:                   get("RELAY_OPS_LISTEN_ADDRESS", ":8100"),
		Timezone:                        timezone,
		TimezoneName:                    timezoneName,
		DatabaseURLFile:                 databaseURLFile,
		Sub2APIBaseURL:                  baseURL,
		Sub2APIAdminKeyFile:             adminKeyFile,
		Sub2APIAlertReadDatabaseURLFile: sub2APIAlertReadDatabaseURLFile,
		AccountQualityResultFile:        accountQualityResultFile,
		UpstreamGroupMappingFile:        upstreamGroupMappingFile,
		FeishuWebhookFile:               feishuFile,
		FeishuAppIDFile:                 feishuAppIDFile,
		FeishuAppSecretFile:             feishuAppSecretFile,
		FeishuAlertChatIDFile:           feishuAlertChatIDFile,
		FeishuAlertRecipientsFile:       feishuAlertRecipientsFile,
		NotificationPolicyFile:          notificationPolicyFile,
		NotificationPolicy:              notificationPolicy,
		AgentBaseURL:                    strings.TrimRight(get("RELAY_OPS_AGENT_BASE_URL", ""), "/"),
		AgentAPIKeyFile:                 agentKeyFile,
		AgentModel:                      get("RELAY_OPS_AGENT_MODEL", ""),
		ProductionPageInterval:          5 * time.Minute,
		CandidateInterval:               6 * time.Hour,
		PublicBaseURL:                   strings.TrimRight(get("RELAY_OPS_PUBLIC_BASE_URL", "https://api.xingqiaolab.top"), "/"),
		RubyPath:                        get("RELAY_OPS_RUBY_PATH", "/usr/bin/ruby"),
		V2ScriptPath:                    get("RELAY_OPS_V2_SCRIPT_PATH", "/app/ops/upstream-benchmark-v2.rb"),
		CandidateProfilePath:            get("RELAY_OPS_CANDIDATE_PROFILE_PATH", "/app/config/upstream-benchmarks/candidate-watch-v2.yaml"),
		FastProfilePath:                 get("RELAY_OPS_FAST_PROFILE_PATH", "/app/config/upstream-benchmarks/quality-first-fast-v1.yaml"),
		CandidateSecretDir:              candidateSecretDir,
		QualificationProfilePath:        get("RELAY_OPS_QUALIFICATION_PROFILE_PATH", "/app/config/upstream-benchmarks/mvp-text-v2.yaml"),
		AnalyzerCommandPath:             analyzerCommandPath,
		AccountingEnabled:               accountingEnabled,
		AccountingLedgerStartDate:       accountingStartDate,
		AccountingInternalUserIDs:       accountingInternalUserIDs,
		AccountingInternalAPIKeyIDs:     accountingInternalAPIKeyIDs,
	}, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

func parsePositiveIDList(value string) ([]int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("IDs must be positive comma-separated integers")
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("IDs must be positive comma-separated integers")
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate ID %d", id)
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
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
