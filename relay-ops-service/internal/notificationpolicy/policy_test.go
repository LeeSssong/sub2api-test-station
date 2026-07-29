package notificationpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

const allEnabledPolicy = `{
  "version": 1,
  "delivery_mode": "enabled",
  "feishu_notifications": {
    "group_runtime_enabled": true,
    "group_capacity_enabled": true,
    "account_impact_enabled": true,
    "native_monitor_evidence_enabled": true,
    "pricing_notice_enabled": true,
    "daily_digest_enabled": true,
    "incident_escalation_enabled": true
  }
}`

func TestLoadAcceptsCompleteStrictPolicy(t *testing.T) {
	t.Parallel()

	policy, err := Load(writePolicy(t, allEnabledPolicy))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if policy.Version != 1 || policy.Mode != ModeEnabled {
		t.Fatalf("policy header = %#v", policy)
	}
	for _, family := range ApprovedFamilies() {
		if !policy.Enabled(family) || !policy.ShouldDeliver(family) {
			t.Fatalf("family %q is not deliverable", family)
		}
	}
}

func TestLoadRejectsUnknownAndMissingFamilies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "unknown",
			body: `{"version":1,"delivery_mode":"enabled","feishu_notifications":{
				"group_runtime_enabled":true,
				"group_capacity_enabled":true,
				"account_impact_enabled":true,
				"native_monitor_evidence_enabled":true,
				"pricing_notice_enabled":true,
				"daily_digest_enabled":true,
				"incident_escalation_enabled":true,
				"candidate_enabled":true
			}}`,
		},
		{
			name: "missing",
			body: `{"version":1,"delivery_mode":"enabled","feishu_notifications":{
				"group_runtime_enabled":true,
				"group_capacity_enabled":true,
				"account_impact_enabled":true,
				"native_monitor_evidence_enabled":true,
				"pricing_notice_enabled":true,
				"daily_digest_enabled":true
			}}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(writePolicy(t, test.body)); err == nil {
				t.Fatal("invalid family set was accepted")
			}
		})
	}
}

func TestDeliveryModeRejectsUnknownValueAndShadowNeverDelivers(t *testing.T) {
	t.Parallel()

	unknown := replaceMode(allEnabledPolicy, "live")
	if _, err := Load(writePolicy(t, unknown)); err == nil {
		t.Fatal("unknown delivery mode was accepted")
	}

	policy, err := Load(writePolicy(t, replaceMode(allEnabledPolicy, "shadow")))
	if err != nil {
		t.Fatalf("Load shadow: %v", err)
	}
	if !policy.Enabled(FamilyGroupRuntime) {
		t.Fatal("shadow policy lost the family flag")
	}
	if policy.ShouldDeliver(FamilyGroupRuntime) {
		t.Fatal("shadow policy permits real delivery")
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	if _, err := Load(writePolicy(t, allEnabledPolicy+` {}`)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func writePolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notification-policy.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func replaceMode(body, mode string) string {
	const marker = `"delivery_mode": "enabled"`
	return stringReplace(body, marker, `"delivery_mode": "`+mode+`"`)
}

func stringReplace(value, old, replacement string) string {
	for index := 0; index+len(old) <= len(value); index++ {
		if value[index:index+len(old)] == old {
			return value[:index] + replacement + value[index+len(old):]
		}
	}
	return value
}
