package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/projection"
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

func TestStoreReaderRecomputesFreshnessAndNormalizesEmptyMetadata(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	reader := StoreReader{Now: func() time.Time { return now }, Store: projectionStoreStub{accounts: []projection.AccountRow{{AccountID: 7, Metadata: projection.Metadata{GeneratedAt: now.Add(-90 * time.Second), SourceWatermark: "event-7", Completeness: "complete", CalculationVersion: "accounts-v1"}}}}}
	model, err := reader.Read(context.Background(), "accounts/monitor", nil)
	if err != nil {
		t.Fatal(err)
	}
	if model.Freshness.FreshnessSeconds != 90 || model.Freshness.GeneratedAt != now.Add(-90*time.Second) {
		t.Fatalf("freshness=%+v", model.Freshness)
	}
	empty, err := reader.Read(context.Background(), "accounting/ledger", nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Freshness.GeneratedAt != now || empty.Freshness.Completeness != "empty" || empty.Freshness.CalculationVersion != "accounting-v1" {
		t.Fatalf("empty=%+v", empty.Freshness)
	}
}

type projectionStoreStub struct{ accounts []projection.AccountRow }

func (s projectionStoreStub) LoadAccountReadModels(context.Context) ([]projection.AccountRow, error) {
	return s.accounts, nil
}
func (projectionStoreStub) LoadProfitabilityReadModels(context.Context) ([]projection.ProfitabilityRow, error) {
	return nil, nil
}
func (projectionStoreStub) LoadAccountingReadModel(context.Context) (projection.Accounting, bool, error) {
	return projection.Accounting{}, false, nil
}
func (projectionStoreStub) LoadReconciliationReadModel(context.Context) (projection.Reconciliation, bool, error) {
	return projection.Reconciliation{}, false, nil
}

func TestRefreshIdempotencyDispatchesOnceForConcurrentRequests(t *testing.T) {
	audit := &idempotencyAudit{claimed: map[string]bool{}}
	var mu sync.Mutex
	calls := 0
	h := RequireAdmin(authClientFunc(func(context.Context, string, string, string) (AdminIdentity, error) {
		return AdminIdentity{UserID: 42, Role: "admin", Status: "active"}, nil
	}), NewServer(NewMemoryReader(), CommandRefresher{Sender: commandSenderFunc(func(context.Context, int64, string) error { mu.Lock(); calls++; mu.Unlock(); return nil }), Audit: audit}))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/accounts/7/refresh", nil)
			req.Header.Set("Authorization", "Bearer session")
			req.Header.Set("Idempotency-Key", "refresh:7:one")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusAccepted {
				t.Errorf("status=%d", rec.Code)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("dispatches=%d want=1", calls)
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

type idempotencyAudit struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func (a *idempotencyAudit) ClaimExternalizationCommand(_ context.Context, _, _ int64, key, _ string, _ int) (bool, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.claimed[key] {
		return false, "accepted", nil
	}
	a.claimed[key] = true
	return true, "pending", nil
}
func (a *idempotencyAudit) CompleteExternalizationCommand(context.Context, int64, int64, string, string, int) error {
	return nil
}
func (a *idempotencyAudit) RecordExternalizationCommand(context.Context, int64, int64, string, string, int) error {
	return nil
}
