package sub2api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// NewHTTPReader takes an admin-key FILE path (not the key itself) and returns
// an error, so tests must materialise a key file first.
func newHistoryTestReader(t *testing.T, baseURL string) *HTTPReader {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "admin-key")
	if err := os.WriteFile(keyFile, []byte("test-admin-key\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	reader, err := NewHTTPReader(baseURL, keyFile)
	if err != nil {
		t.Fatalf("NewHTTPReader: %v", err)
	}
	return reader
}

func TestListAccountMonitorHistoryDecodesProductionShape(t *testing.T) {
	const body = `{"code":0,"message":"success","data":{"items":[
		{"account_id":21,"model_id":"gpt-5.6-terra","status":"success","ttft_ms":1255.694,"latency_ms":1453.25,"checked_at":"2026-07-26T11:23:50Z"},
		{"account_id":21,"model_id":"gpt-5.6-terra","status":"failed","error_code":"balance_exhausted","latency_ms":542.088,"checked_at":"2026-07-26T11:28:55Z"}
	]}}`
	var gotPath, gotQuery, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	reader := newHistoryTestReader(t, server.URL)
	entries, err := reader.ListAccountMonitorHistory(context.Background(), 21, 692)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/account-monitors/21/history" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "limit=692" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotKey != "test-admin-key" {
		t.Fatalf("x-api-key = %q", gotKey)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].TTFTMS == nil || *entries[0].TTFTMS != 1255.694 {
		t.Fatalf("entries[0].TTFTMS = %v", entries[0].TTFTMS)
	}
	if entries[1].ErrorCode != "balance_exhausted" {
		t.Fatalf("entries[1].ErrorCode = %q", entries[1].ErrorCode)
	}
	if entries[1].TTFTMS != nil {
		t.Fatalf("失败记录不应有 ttft_ms: %v", entries[1].TTFTMS)
	}
	if entries[0].CheckedAt.IsZero() {
		t.Fatal("CheckedAt not parsed")
	}
}

func TestListAccountMonitorHistoryRejectsBadInput(t *testing.T) {
	reader := newHistoryTestReader(t, "http://127.0.0.1:1")
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 0, 10); err == nil {
		t.Fatal("accountID 0 must be rejected")
	}
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 1, 0); err == nil {
		t.Fatal("limit 0 must be rejected")
	}
}

func TestListAccountMonitorHistoryRejectsUnknownField(t *testing.T) {
	const body = `{"data":{"items":[{"account_id":1,"model_id":"m","status":"success","checked_at":"2026-07-26T11:23:50Z","surprise":1}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	reader := newHistoryTestReader(t, server.URL)
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 1, 10); err == nil {
		t.Fatal("未知字段必须触发 schema mismatch")
	}
}
