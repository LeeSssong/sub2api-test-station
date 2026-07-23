package d04readiness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSourceReadsValidatedReadinessResult(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, []byte(`{"policy_id":"D04-LIGHTWEIGHT-LAUNCH-v3","snapshot_id":"snapshot-1","account_set_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","evaluated_at":"2026-07-22T10:00:00Z","decision":"no_go","blocking_reasons":["upstream_balance_below_minimum"],"required_actions":["replenish_active_upstream_balance"],"upstreams":[{"account_id":7,"display_name":"Account","group_ids":[2],"status":"active","schedulable":true,"runtime_available":true,"balance_usd":9.99,"financial_recorded_at":"2026-07-22T10:00:00Z","quality_recorded_at":"2026-07-22T10:00:00Z","sample_count":30,"success_rate":0.99,"error_rate":0.01,"ttft_p95_ms":2000,"total_latency_p95_ms":8000,"decision":"no_go","blocking_reasons":["upstream_balance_below_minimum"]}],"real_action_executed":false,"external_system_contacted":false}`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (FileSource{Path: path}).Read()
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != "no_go" || len(result.Upstreams) != 1 || result.Upstreams[0].AccountID != 7 {
		t.Fatalf("result = %#v", result)
	}
}

func TestFileSourceRejectsSecretsAndUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, []byte(`{"policy_id":"D04-LIGHTWEIGHT-LAUNCH-v3","api_key":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (FileSource{Path: path}).Read()
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v, want redacted validation error", err)
	}
}
