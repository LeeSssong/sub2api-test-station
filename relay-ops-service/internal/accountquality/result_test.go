package accountquality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAcceptsStrictSecretFreeAccountQualityResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 10, 0, 0, time.UTC)
	result, err := Load(writeDocument(t, validDocument()), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 2 || result.Accounts[0].AccountID != 21 || result.Accounts[1].LastResult != "balance_exhausted" {
		t.Fatalf("result = %#v", result)
	}
	view := result.View(now)
	if !view.Available || view.Stale || len(view.Accounts) != 2 {
		t.Fatalf("view = %#v", view)
	}
	if view.Accounts[0].Stability != "成功 3/4" || view.Accounts[0].TTFTP95 != "210ms" || view.Accounts[0].Multiplier != "0.05x" {
		t.Fatalf("account = %#v", view.Accounts[0])
	}
	if view.Accounts[1].LastResult != "余额不足" || view.Accounts[1].TTFTP95 != "无成功样本" {
		t.Fatalf("account = %#v", view.Accounts[1])
	}
}

func TestLoadRejectsUnknownForbiddenMalformedAndOversizedDocuments(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 10, 0, 0, time.UTC)
	for _, key := range []string{"unknown", "response_text", "base_url", "api_key", "raw_response"} {
		document := validDocument()
		document[key] = "must-not-leak"
		_, err := Load(writeDocument(t, document), now)
		if err == nil || strings.Contains(err.Error(), "must-not-leak") {
			t.Fatalf("key %q error = %v", key, err)
		}
	}

	path := filepath.Join(t.TempDir(), "oversize.json")
	if err := os.WriteFile(path, []byte(`{"padding":"`+strings.Repeat("x", (2<<20)+1)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, now); err == nil {
		t.Fatal("oversize result was accepted")
	}
}

func TestLoadRejectsFutureDuplicateAndInvalidMetrics(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 10, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "future", mutate: func(document map[string]any) { document["observed_at"] = "2026-07-23T00:10:01Z" }},
		{name: "bad hash", mutate: func(document map[string]any) { document["account_set_sha256"] = "A" + strings.Repeat("a", 63) }},
		{name: "duplicate", mutate: func(document map[string]any) {
			accounts := document["accounts"].([]any)
			accounts[1].(map[string]any)["account_id"] = float64(21)
		}},
		{name: "count", mutate: func(document map[string]any) {
			document["accounts"].([]any)[0].(map[string]any)["success_count"] = float64(5)
		}},
		{name: "rate", mutate: func(document map[string]any) {
			document["accounts"].([]any)[0].(map[string]any)["success_rate"] = 1.1
		}},
		{name: "result", mutate: func(document map[string]any) {
			document["accounts"].([]any)[0].(map[string]any)["last_result"] = "unknown"
		}},
	}
	for _, test := range tests {
		document := validDocument()
		test.mutate(document)
		if _, err := Load(writeDocument(t, document), now); err == nil {
			t.Fatalf("%s document was accepted", test.name)
		}
	}
}

func TestViewMarksEvidenceStaleAfterTwentyMinutes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 21, 0, 0, time.UTC)
	document := validDocument()
	document["observed_at"] = "2026-07-23T00:00:00Z"
	result, err := Load(writeDocument(t, document), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.View(now.Add(-time.Minute)).Stale {
		t.Fatal("evidence became stale at the inclusive twenty-minute boundary")
	}
	if !result.View(now).Stale {
		t.Fatal("evidence did not become stale after twenty minutes")
	}
}

func TestViewPreservesNumericAccountOrder(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 10, 0, 0, time.UTC)
	document := validDocument()
	accounts := document["accounts"].([]any)
	accounts[0].(map[string]any)["account_id"] = float64(2)
	accounts[1].(map[string]any)["account_id"] = float64(10)
	result, err := Load(writeDocument(t, document), now)
	if err != nil {
		t.Fatal(err)
	}
	view := result.View(now)
	if view.Accounts[0].AccountID != "2" || view.Accounts[1].AccountID != "10" {
		t.Fatalf("accounts = %#v", view.Accounts)
	}
}

func TestFileSourceReadsThroughBoundedLoader(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 0, 10, 0, 0, time.UTC)
	result, err := (FileSource{Path: writeDocument(t, validDocument())}).Read(now)
	if err != nil || result.SnapshotID != "ACCOUNT-QUALITY-20260723T000000Z" {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func validDocument() map[string]any {
	return map[string]any{
		"schema_version":     float64(1),
		"snapshot_id":        "ACCOUNT-QUALITY-20260723T000000Z",
		"observed_at":        "2026-07-23T00:00:00Z",
		"account_set_sha256": strings.Repeat("a", 64),
		"accounts": []any{
			map[string]any{
				"account_id": float64(21), "model_id": "gpt-5.6-sol", "rate_multiplier": 0.05,
				"sample_count": float64(4), "success_count": float64(3), "success_rate": 0.75,
				"ttft_p50_ms": 150.0, "ttft_p95_ms": 210.0, "last_result": "passed", "last_error_code": "",
				"last_observed_at": "2026-07-23T00:00:00Z",
			},
			map[string]any{
				"account_id": float64(22), "model_id": "gpt-5.6-terra", "rate_multiplier": 0.10,
				"sample_count": float64(4), "success_count": float64(0), "success_rate": 0.0,
				"ttft_p50_ms": nil, "ttft_p95_ms": nil, "last_result": "balance_exhausted", "last_error_code": "balance_exhausted",
				"last_observed_at": "2026-07-23T00:00:00Z",
			},
		},
	}
}

func writeDocument(t *testing.T, document map[string]any) string {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "account-quality-result.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
