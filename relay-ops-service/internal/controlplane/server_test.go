package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadModelsExposeFreshnessAndRefreshRequiresIdempotency(t *testing.T) {
	r := NewMemoryReader()
	r.Set("accounts/monitor", ReadModel{Items: []string{"ok"}, Total: 1, Freshness: Freshness{Completeness: "complete", CalculationVersion: "v1"}})
	h := NewServer(r, nil)
	req := httptest.NewRequest(http.MethodGet, "/accounts/monitor", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/accounts/1/refresh", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestReadModelResponseHasTopLevelFreshnessAndRefreshAuditsActor(t *testing.T) {
	r := NewMemoryReader()
	r.Set("accounts/monitor", ReadModel{Items: []string{"ok"}, Total: 1, Freshness: Freshness{GeneratedAt: time.Unix(1, 0).UTC(), SourceWatermark: "event-1", FreshnessSeconds: 4, Completeness: "complete", CalculationVersion: "accounts-v1"}})
	audit := &recordingAudit{}
	h := RequireAdmin(authClientFunc(func(context.Context, string, string, string) (AdminIdentity, error) {
		return AdminIdentity{UserID: 42, Role: "admin", Status: "active"}, nil
	}), NewServer(r, CommandRefresher{Sender: commandSenderFunc(func(context.Context, int64, string) error { return nil }), Audit: audit}))
	req := httptest.NewRequest(http.MethodGet, "/accounts/monitor", nil)
	req.Header.Set("Authorization", "Bearer browser-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"generated_at", "source_watermark", "freshness_seconds", "completeness", "calculation_version"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response omitted %s: %#v", key, body)
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/accounts/7/refresh", nil)
	req.Header.Set("Authorization", "Bearer browser-token")
	req.Header.Set("Idempotency-Key", "account:7:refresh:1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status=%d body=%s", rec.Code, rec.Body.String())
	}
	if audit.actorID != 42 || audit.accountID != 7 || audit.key != "account:7:refresh:1" || audit.result != "accepted" || audit.contract != 1 {
		t.Fatalf("audit=%+v", audit)
	}
	if strings.Contains(rec.Body.String(), "browser-token") {
		t.Fatalf("response leaked bearer: %s", rec.Body.String())
	}
}

type commandSenderFunc func(context.Context, int64, string) error

func (f commandSenderFunc) SendAccountUpdate(ctx context.Context, id int64, key string) error {
	return f(ctx, id, key)
}

type recordingAudit struct {
	actorID, accountID int64
	key, result        string
	contract           int
}

func (a *recordingAudit) RecordExternalizationCommand(_ context.Context, actorID, accountID int64, key, result string, contract int) error {
	a.actorID, a.accountID, a.key, a.result, a.contract = actorID, accountID, key, result, contract
	return nil
}
