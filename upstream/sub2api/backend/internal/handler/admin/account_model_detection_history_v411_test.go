package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type historyDetectionRepo struct {
	gotLimit                                  int
	gotCursor, gotStatus, gotProfile, gotMode string
}

func (r *historyDetectionRepo) LoadSettings(context.Context, int64) (service.AccountModelDetectionSettings, error) {
	return service.AccountModelDetectionSettings{}, nil
}
func (r *historyDetectionRepo) SaveSettings(context.Context, service.AccountModelDetectionSettings) error {
	return nil
}
func (r *historyDetectionRepo) Enqueue(context.Context, service.AccountModelDetectionRun) (service.AccountModelDetectionRun, bool, error) {
	return service.AccountModelDetectionRun{}, false, nil
}
func (r *historyDetectionRepo) ListQueued(context.Context, int) ([]string, error) { return nil, nil }
func (r *historyDetectionRepo) Claim(context.Context, string) (*service.AccountModelDetectionRun, error) {
	return nil, nil
}
func (r *historyDetectionRepo) Complete(context.Context, string, service.AccountModelDetectionResponse, string, string) error {
	return nil
}
func (r *historyDetectionRepo) ListRecent(_ context.Context, _ int64, limit int, cursor, status, profile, mode string) (service.AccountModelDetectionHistoryPage, error) {
	r.gotLimit, r.gotCursor, r.gotStatus, r.gotProfile, r.gotMode = limit, cursor, status, profile, mode
	finished := time.Date(2026, 8, 26, 2, 0, 0, 0, time.UTC)
	return service.AccountModelDetectionHistoryPage{Items: []service.AccountModelDetectionRun{{ID: "run-1", Status: service.AccountModelDetectionStatusAbnormal, Profile: profile, Mode: mode, TriggerReason: service.AccountModelDetectionTriggerModelConflict, PlannedRequests: 158, ValidSamples: 157, EvidenceState: service.AccountModelDetectionEvidenceComplete, FingerprintStatus: "strong_match", FinishedAt: &finished, QueuedAt: finished.Add(-time.Second)}}, NextCursor: "next-page"}, nil
}

type historyDetectionAccounts struct{}

func (historyDetectionAccounts) GetByID(context.Context, int64) (*service.Account, error) {
	return &service.Account{ID: 7, Type: service.AccountTypeAPIKey}, nil
}
func (historyDetectionAccounts) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]service.Account, error) {
	return nil, nil
}

type historyDetectionSidecar struct{}

func (historyDetectionSidecar) Catalog(context.Context) (service.AccountModelDetectionSidecarCatalog, error) {
	return service.AccountModelDetectionSidecarCatalog{}, nil
}
func (historyDetectionSidecar) Detect(context.Context, service.AccountModelDetectionRequest) (service.AccountModelDetectionResponse, error) {
	return service.AccountModelDetectionResponse{}, nil
}

func TestAccountModelDetectionHistoryParsesCursorAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &historyDetectionRepo{}
	detection := service.NewAccountModelDetectionService(repo, historyDetectionAccounts{}, historyDetectionSidecar{})
	h := NewAccountMonitorHandler(nil, nil, nil, nil)
	h.SetModelDetectionService(detection)
	router := gin.New()
	router.GET("/accounts/:account_id/detection", h.AccountModelDetectionHistory)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts/7/detection?limit=2&cursor=abc&status=abnormal&profile=high&mode=escalation", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.gotLimit != 2 || repo.gotCursor != "abc" || repo.gotStatus != "abnormal" || repo.gotProfile != "high" || repo.gotMode != "escalation" {
		t.Fatalf("repo args limit=%d cursor=%q status=%q profile=%q mode=%q", repo.gotLimit, repo.gotCursor, repo.gotStatus, repo.gotProfile, repo.gotMode)
	}
	body := recorder.Body.String()
	for _, fragment := range []string{`"next_cursor":"next-page"`, `"profile":"high"`, `"planned_requests":158`, `"fingerprint_status":"strong_match"`} {
		if !contains(body, fragment) {
			t.Fatalf("response missing %q: %s", fragment, body)
		}
	}
}

func TestAccountModelDetectionHistoryMarksLegacyRowsAsHistorical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	detection := service.NewAccountModelDetectionService(&historyDetectionRepo{}, historyDetectionAccounts{}, historyDetectionSidecar{})
	h := NewAccountMonitorHandler(nil, nil, nil, nil)
	h.SetModelDetectionService(detection)
	router := gin.New()
	router.GET("/accounts/:account_id/detection", h.AccountModelDetectionHistory)

	// The repository stub's default row is structured; replace it with a legacy row
	// through a small wrapper to assert the handler's source projection directly.
	legacyRepo := &legacyHistoryDetectionRepo{}
	h.SetModelDetectionService(service.NewAccountModelDetectionService(legacyRepo, historyDetectionAccounts{}, historyDetectionSidecar{}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts/7/detection", nil))
	if recorder.Code != http.StatusOK || !contains(recorder.Body.String(), `"source":"historical"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

type legacyHistoryDetectionRepo struct{ historyDetectionRepo }

func (r *legacyHistoryDetectionRepo) ListRecent(context.Context, int64, int, string, string, string, string) (service.AccountModelDetectionHistoryPage, error) {
	finished := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	return service.AccountModelDetectionHistoryPage{Items: []service.AccountModelDetectionRun{{ID: "legacy-1", Status: service.AccountModelDetectionStatusNormal, FinishedAt: &finished, QueuedAt: finished}}}, nil
}

func contains(value, fragment string) bool {
	return len(value) >= len(fragment) && stringIndex(value, fragment) >= 0
}

func stringIndex(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}
