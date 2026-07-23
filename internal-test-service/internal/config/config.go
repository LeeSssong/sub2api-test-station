package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"example.invalid/internal-test-service/internal/domain"
)

type Config struct {
	MaxUsers              int
	TotalBudget           domain.MicroUSD
	RegistrationOpen      bool
	DailyLoginCredit      domain.MicroUSD
	Timezone              *time.Location
	TimezoneName          string
	Sub2APIURL            string
	AdminAPIKeyFile       string
	InvitationKeyFile     string
	FeishuBaseURL         string
	FeishuAppIDFile       string
	FeishuAppSecretFile   string
	FeishuAlertChatIDFile string
	CostPolicyID          string
	BudgetCostBPS         int64
	CostPolicyQualified   bool
	Mode                  string
	DataPath              string
	ListenAddress         string
}

func Load(env func(string) string) (Config, error) {
	get := func(key, fallback string) string {
		if v := strings.TrimSpace(env(key)); v != "" {
			return v
		}
		return fallback
	}
	maxUsers, err := strconv.Atoi(get("D04_MAX_USERS", "15"))
	if err != nil || maxUsers < 1 || maxUsers > 15 {
		return Config{}, fmt.Errorf("D04_MAX_USERS must be between 1 and 15")
	}
	explicitBudget := strings.TrimSpace(env("D04_TOTAL_BUDGET_USD"))
	budgetValue := explicitBudget
	if budgetValue == "" {
		budgetValue = "100"
	}
	budget, err := domain.ParseMicroUSD(budgetValue)
	if err != nil || budget <= 0 {
		return Config{}, fmt.Errorf("D04_TOTAL_BUDGET_USD must be positive")
	}
	registrationOpen, err := strconv.ParseBool(get("D04_REGISTRATION_OPEN", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("D04_REGISTRATION_OPEN must be true or false")
	}
	dailyLoginCredit, err := domain.ParseMicroUSD(get("D04_DAILY_LOGIN_CREDIT_USD", "20"))
	if err != nil || dailyLoginCredit != domain.DailyLoginCredit {
		return Config{}, fmt.Errorf("D04_DAILY_LOGIN_CREDIT_USD must be 20")
	}
	tzName := get("D04_TIMEZONE", "Asia/Shanghai")
	if tzName != "Asia/Shanghai" {
		return Config{}, fmt.Errorf("D04_TIMEZONE must be Asia/Shanghai")
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("load timezone: %w", err)
	}
	baseURL := strings.TrimRight(get("D04_SUB2API_URL", "http://sub2api:8080"), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("D04_SUB2API_URL is required")
	}
	keyFile := get("D04_ADMIN_API_KEY_FILE", "/run/secrets/sub2api-admin-api-key")
	if st, statErr := os.Stat(keyFile); statErr != nil {
		return Config{}, fmt.Errorf("admin API key file: %w", statErr)
	} else if st.Mode().Perm()&0o022 != 0 {
		return Config{}, fmt.Errorf("admin API key file must not be writable by group/other")
	}
	invitationKeyFile := strings.TrimSpace(env("D04_INVITATION_KEY_FILE"))
	if invitationKeyFile != "" {
		if err := validateSecretFile(invitationKeyFile); err != nil {
			return Config{}, fmt.Errorf("invitation encryption key file: %w", err)
		}
	}
	feishuBaseURL := get("D04_FEISHU_BASE_URL", "https://open.feishu.cn")
	feishuAppIDFile := get("D04_FEISHU_APP_ID_FILE", "")
	feishuAppSecretFile := get("D04_FEISHU_APP_SECRET_FILE", "")
	feishuAlertChatIDFile := get("D04_FEISHU_ALERT_CHAT_ID_FILE", "")
	mode := get("D04_MODE", "read_only")
	if mode != "read_only" && mode != "write" && mode != "closed" {
		return Config{}, fmt.Errorf("D04_MODE must be read_only, write, or closed")
	}
	if mode == "write" && explicitBudget == "" {
		return Config{}, fmt.Errorf("write mode requires an explicit D04_TOTAL_BUDGET_USD")
	}
	costPolicyID := get("D04_COST_POLICY_ID", "")
	budgetCostBPS := int64(0)
	if raw := get("D04_BUDGET_COST_BPS", ""); raw != "" {
		budgetCostBPS, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || budgetCostBPS < 1 || budgetCostBPS > 100_000 {
			return Config{}, fmt.Errorf("D04_BUDGET_COST_BPS must be between 1 and 100000")
		}
	}
	costPolicyQualified, err := strconv.ParseBool(get("D04_COST_POLICY_QUALIFIED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("D04_COST_POLICY_QUALIFIED must be true or false")
	}
	if mode == "write" && (costPolicyID == "" || budgetCostBPS == 0 || !costPolicyQualified) {
		return Config{}, fmt.Errorf("write mode requires an explicit qualified D04 cost policy")
	}
	feishuFiles := []string{feishuAppIDFile, feishuAppSecretFile, feishuAlertChatIDFile}
	configuredFeishuFiles := 0
	for _, path := range feishuFiles {
		if path == "" {
			continue
		}
		configuredFeishuFiles++
		if err := validateSecretFile(path); err != nil {
			return Config{}, fmt.Errorf("Feishu App Bot secret file: %w", err)
		}
	}
	if configuredFeishuFiles != 0 && configuredFeishuFiles != len(feishuFiles) {
		return Config{}, fmt.Errorf("all D04 Feishu App Bot files must be configured together")
	}
	if mode == "write" && configuredFeishuFiles != len(feishuFiles) {
		return Config{}, fmt.Errorf("write mode requires the controlled Feishu App Bot alert target")
	}
	return Config{
		MaxUsers: maxUsers, TotalBudget: budget, RegistrationOpen: registrationOpen,
		DailyLoginCredit: dailyLoginCredit, Timezone: loc, TimezoneName: tzName,
		Sub2APIURL: baseURL, AdminAPIKeyFile: keyFile, InvitationKeyFile: invitationKeyFile, FeishuBaseURL: feishuBaseURL,
		FeishuAppIDFile: feishuAppIDFile, FeishuAppSecretFile: feishuAppSecretFile,
		FeishuAlertChatIDFile: feishuAlertChatIDFile,
		CostPolicyID:          costPolicyID, BudgetCostBPS: budgetCostBPS, CostPolicyQualified: costPolicyQualified,
		Mode: mode, DataPath: get("D04_DATA_PATH", "/var/lib/internal-test/internal-test.db"),
		ListenAddress: get("D04_LISTEN_ADDRESS", ":8090"),
	}, nil
}

func validateSecretFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("file must not be writable by group/other")
	}
	return nil
}
