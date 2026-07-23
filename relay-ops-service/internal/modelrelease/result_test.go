package modelrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAcceptsSecretFreeReadyResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC)
	path := writeResult(t, validDocument())
	result, err := Load(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "可升级" || len(result.Accounts) != 1 || result.Accounts[0].AccountID != 10 {
		t.Fatalf("result = %#v", result)
	}
	view := result.View(now)
	if !view.Available || view.Stale || view.Status != "可升级" || view.CandidateModels != "gpt-5.7, gpt-5.7-sol" {
		t.Fatalf("view = %#v", view)
	}
}

func TestLoadRejectsUnknownFieldsSecretsAndOversizeInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC)
	for _, key := range []string{"unknown", "api_key", "model_output"} {
		document := validDocument()
		document[key] = "must-not-leak"
		_, err := Load(writeResult(t, document), now)
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

func TestLoadRejectsHashMismatchDuplicateModelsAndFutureTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC)
	document := validDocument()
	document["proposal_id"] = strings.Repeat("0", 64)
	if _, err := Load(writeRawDocument(t, document), now); err == nil {
		t.Fatal("proposal hash mismatch was accepted")
	}

	document = validDocument()
	candidate := document["candidate"].(map[string]any)
	candidate["models"] = []any{"gpt-5.7", "gpt-5.7"}
	if _, err := Load(writeResult(t, document), now); err == nil {
		t.Fatal("duplicate candidate model was accepted")
	}

	document = validDocument()
	document["evaluated_at"] = "2026-07-22T12:10:01Z"
	if _, err := Load(writeResult(t, document), now); err == nil {
		t.Fatal("future result was accepted")
	}
}

func TestViewMarksResultStaleAfterTwentyMinutes(t *testing.T) {
	t.Parallel()

	evaluatedAt := time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC)
	result, err := Load(writeResult(t, validDocument()), evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.View(evaluatedAt.Add(20 * time.Minute)).Stale {
		t.Fatal("result became stale at inclusive twenty-minute boundary")
	}
	if !result.View(evaluatedAt.Add(20*time.Minute + time.Second)).Stale {
		t.Fatal("result did not become stale after twenty minutes")
	}
}

func TestFileSourceReadsThroughTheBoundedLoader(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC)
	result, err := (FileSource{Path: writeResult(t, validDocument())}).Read(now)
	if err != nil || result.Status != "可升级" {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func TestViewTranslatesStableBlockerCodesForOperators(t *testing.T) {
	t.Parallel()

	view := (Result{Blockers: []string{
		"bootstrap_qualification_required", "financial_evidence_missing",
		"quality_evidence_missing", "group_model_coverage_incomplete",
	}}).View(time.Date(2026, 7, 22, 12, 10, 0, 0, time.UTC))

	got := strings.Join(view.Blockers, "|")
	for _, label := range []string{"首版模型尚未完成兼容测试", "缺少上游余额证据", "缺少自然质量证据", "公开分组模型覆盖不完整"} {
		if !strings.Contains(got, label) {
			t.Fatalf("blockers = %q, missing %q", got, label)
		}
	}
}

func validDocument() map[string]any {
	return map[string]any{
		"schema_version":     float64(1),
		"proposal_id":        strings.Repeat("0", 64),
		"snapshot_id":        "MODEL-RELEASE-1",
		"evaluated_at":       "2026-07-22T12:10:00Z",
		"status":             "可升级",
		"account_set_sha256": strings.Repeat("a", 64),
		"base_config_sha256": strings.Repeat("b", 64),
		"published": map[string]any{
			"families": []any{"5.5", "5.6"},
			"models":   []any{"gpt-5.5", "gpt-5.6"},
		},
		"candidate": map[string]any{
			"family":        "5.7",
			"families":      []any{"5.7"},
			"models":        []any{"gpt-5.7", "gpt-5.7-sol"},
			"review_models": []any{},
		},
		"groups": []any{
			map[string]any{"group_id": float64(2), "name": "GPT-Plus", "covered": true, "covered_models": []any{"gpt-5.7", "gpt-5.7-sol"}, "missing_models": []any{}},
		},
		"accounts": []any{
			map[string]any{"account_id": float64(10), "group_ids": []any{float64(2)}, "qualified_models": []any{"gpt-5.7", "gpt-5.7-sol"}},
		},
		"evidence": map[string]any{
			"captured_at":         "2026-07-22T12:05:00Z",
			"freshness_minutes":   float64(20),
			"balance_min_usd":     float64(5),
			"quality_samples_min": float64(20),
		},
		"blockers": []any{},
	}
}

func writeResult(t *testing.T, document map[string]any) string {
	t.Helper()
	copyDocument := cloneDocument(t, document)
	delete(copyDocument, "proposal_id")
	data, err := json.Marshal(copyDocument)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	document["proposal_id"] = hex.EncodeToString(digest[:])
	return writeRawDocument(t, document)
}

func writeRawDocument(t *testing.T, document map[string]any) string {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cloneDocument(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
