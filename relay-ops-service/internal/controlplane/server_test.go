package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
