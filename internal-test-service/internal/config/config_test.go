package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsUnsafeKeyFile(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "admin.key")
	if err := os.WriteFile(key, []byte("redacted"), 0o664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(key, 0o664); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return key
		case "D04_TOTAL_BUDGET_USD":
			return "100"
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected unsafe key file rejection")
	}
}

func TestLoadAcceptsReadOnlyKeyFile(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "admin.key")
	if err := os.WriteFile(key, []byte("redacted"), 0o600); err != nil {
		t.Fatal(err)
	}
	invitationKey := writeConfigTestFile(t, dir, "invitation.key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	env := func(k string) string {
		if k == "D04_ADMIN_API_KEY_FILE" {
			return key
		}
		if k == "D04_INVITATION_KEY_FILE" {
			return invitationKey
		}
		if k == "D04_TOTAL_BUDGET_USD" {
			return "100.25"
		}
		if k == "D04_MODE" {
			return "read_only"
		}
		return ""
	}
	c, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxUsers != 15 || c.TotalBudget.String() != "100.250000" || c.TimezoneName != "Asia/Shanghai" {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoadAcceptsPublicRegistrationPolicyWithoutInvitationKey(t *testing.T) {
	dir := t.TempDir()
	adminKey := writeConfigTestFile(t, dir, "admin.key", "redacted")
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return adminKey
		case "D04_TOTAL_BUDGET_USD":
			return "300"
		case "D04_REGISTRATION_OPEN":
			return "true"
		case "D04_DAILY_LOGIN_CREDIT_USD":
			return "20"
		case "D04_MODE":
			return "read_only"
		}
		return ""
	}

	cfg, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RegistrationOpen || cfg.DailyLoginCredit != 20_000_000 || cfg.InvitationKeyFile != "" {
		t.Fatalf("unexpected public registration policy: %+v", cfg)
	}
}

func TestLoadRejectsChangedDailyLoginCredit(t *testing.T) {
	dir := t.TempDir()
	adminKey := writeConfigTestFile(t, dir, "admin.key", "redacted")
	env := func(k string) string {
		if k == "D04_ADMIN_API_KEY_FILE" {
			return adminKey
		}
		if k == "D04_DAILY_LOGIN_CREDIT_USD" {
			return "21"
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected fixed daily login credit rejection")
	}
}

func TestLoadRejectsMaximumUsersAboveLaunchHardCap(t *testing.T) {
	dir := t.TempDir()
	adminKey := writeConfigTestFile(t, dir, "admin.key", "redacted")
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return adminKey
		case "D04_MAX_USERS":
			return "16"
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected the launch hard cap to reject D04_MAX_USERS=16")
	}
}

func TestLoadWriteModeRequiresExplicitTotalBudget(t *testing.T) {
	dir := t.TempDir()
	adminKey := writeConfigTestFile(t, dir, "admin.key", "redacted")
	appID := writeConfigTestFile(t, dir, "app-id", "cli-test")
	appSecret := writeConfigTestFile(t, dir, "app-secret", "secret")
	chatID := writeConfigTestFile(t, dir, "chat-id", "oc-test")
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return adminKey
		case "D04_MODE":
			return "write"
		case "D04_COST_POLICY_ID":
			return "qualified-test-policy"
		case "D04_BUDGET_COST_BPS":
			return "700"
		case "D04_COST_POLICY_QUALIFIED":
			return "true"
		case "D04_FEISHU_APP_ID_FILE":
			return appID
		case "D04_FEISHU_APP_SECRET_FILE":
			return appSecret
		case "D04_FEISHU_ALERT_CHAT_ID_FILE":
			return chatID
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected write mode to reject an implicit total budget")
	}
}

func TestLoadRequiresSafeInvitationKeyFile(t *testing.T) {
	dir := t.TempDir()
	adminKey := writeConfigTestFile(t, dir, "admin.key", "redacted")
	unsafe := filepath.Join(dir, "invitation.key")
	if err := os.WriteFile(unsafe, []byte("0123456789abcdef0123456789abcdef"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0o666); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return adminKey
		case "D04_INVITATION_KEY_FILE":
			return unsafe
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected unsafe invitation key rejection")
	}
}

func TestLoadWriteModeRequiresQualifiedCostPolicy(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "admin.key")
	if err := os.WriteFile(key, []byte("redacted"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return key
		case "D04_MODE":
			return "write"
		case "D04_TOTAL_BUDGET_USD":
			return "300"
		}
		return ""
	}
	if _, err := Load(env); err == nil {
		t.Fatal("expected write mode to reject an absent cost policy")
	}
}

func TestLoadWriteModeAcceptsExplicitQualifiedCostPolicy(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "admin.key")
	if err := os.WriteFile(key, []byte("redacted"), 0o600); err != nil {
		t.Fatal(err)
	}
	appID := writeConfigTestFile(t, dir, "app-id", "cli-test")
	appSecret := writeConfigTestFile(t, dir, "app-secret", "secret")
	chatID := writeConfigTestFile(t, dir, "chat-id", "oc-test")
	invitationKey := writeConfigTestFile(t, dir, "invitation-key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	env := func(k string) string {
		switch k {
		case "D04_ADMIN_API_KEY_FILE":
			return key
		case "D04_MODE":
			return "write"
		case "D04_TOTAL_BUDGET_USD":
			return "300"
		case "D04_COST_POLICY_ID":
			return "qualified-test-policy"
		case "D04_BUDGET_COST_BPS":
			return "700"
		case "D04_COST_POLICY_QUALIFIED":
			return "true"
		case "D04_FEISHU_APP_ID_FILE":
			return appID
		case "D04_FEISHU_APP_SECRET_FILE":
			return appSecret
		case "D04_FEISHU_ALERT_CHAT_ID_FILE":
			return chatID
		case "D04_INVITATION_KEY_FILE":
			return invitationKey
		}
		return ""
	}
	cfg, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CostPolicyID != "qualified-test-policy" || cfg.BudgetCostBPS != 700 || !cfg.CostPolicyQualified {
		t.Fatalf("unexpected cost policy: %+v", cfg)
	}
}

func writeConfigTestFile(t *testing.T, dir, name, value string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
