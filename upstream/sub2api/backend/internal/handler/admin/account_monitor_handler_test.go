package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMonitorHandlerRepoStub struct {
	service.AccountMonitorRepository
	weights          service.AccountMonitorScoreWeights
	latest           map[int64]service.AccountMonitorLatest
	timelines        map[int64][]service.AccountMonitorTimelinePoint
	windowAggregates map[int64]service.AccountMonitorWindowAggregate
}

func (*accountMonitorHandlerRepoStub) LoadSettings(context.Context) (service.AccountMonitorSettings, error) {
	return service.AccountMonitorSettings{IntervalSeconds: 300}, nil
}

func (*accountMonitorHandlerRepoStub) ListGroups(context.Context) ([]service.AccountMonitorGroup, error) {
	return nil, nil
}

func (s *accountMonitorHandlerRepoStub) ListLatest(context.Context, []int64) (map[int64]service.AccountMonitorLatest, error) {
	return s.latest, nil
}

func (s *accountMonitorHandlerRepoStub) ListTimelines(context.Context, []int64, int) (map[int64][]service.AccountMonitorTimelinePoint, error) {
	return s.timelines, nil
}

func (*accountMonitorHandlerRepoStub) ListAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorAggregate, error) {
	return nil, nil
}

func (s *accountMonitorHandlerRepoStub) ListWindowAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorWindowAggregate, error) {
	return s.windowAggregates, nil
}

func (s *accountMonitorHandlerRepoStub) LoadGroupScoreWeights(context.Context, int64) (service.AccountMonitorScoreWeights, error) {
	return s.weights, nil
}

func (s *accountMonitorHandlerRepoStub) SaveGroupScoreWeights(_ context.Context, _, actorID int64, weights service.AccountMonitorScoreWeights) error {
	weights.UpdatedBy = actorID
	s.weights = weights
	return nil
}

func TestAccountMonitorHandlerUpdatesGroupScoreWeights(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &accountMonitorHandlerRepoStub{}
	svc := service.NewAccountMonitorService(repo, nil, nil, nil, nil)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/account-monitors/groups/7/score-weights", strings.NewReader(`{"cost":20,"success":40,"ttft":20,"latency":20,"ttft_target_ms":1200,"ttft_limit_ms":6000,"latency_target_ms":12000,"latency_limit_ms":65000}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = req
	c.Params = gin.Params{{Key: "group_id", Value: "7"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3})

	h.UpdateGroupScoreWeights(c)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if repo.weights != (service.AccountMonitorScoreWeights{Cost: 20, Success: 40, TTFT: 20, Latency: 20, TTFTTargetMS: 1200, TTFTLimitMS: 6000, LatencyTargetMS: 12000, LatencyLimitMS: 65000, UpdatedBy: 3}) {
		t.Fatalf("weights = %#v", repo.weights)
	}
}

func TestAccountMonitorHandlerPreservesThresholdsForLegacyWeightOnlyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &accountMonitorHandlerRepoStub{weights: service.AccountMonitorScoreWeights{
		Cost: 15, Success: 45, TTFT: 20, Latency: 20,
		TTFTTargetMS: 1400, TTFTLimitMS: 7000, LatencyTargetMS: 18000, LatencyLimitMS: 70000,
	}}
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/account-monitors/groups/7/score-weights", strings.NewReader(`{"cost":20,"success":40,"ttft":20,"latency":20}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = req
	c.Params = gin.Params{{Key: "group_id", Value: "7"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3})

	h.UpdateGroupScoreWeights(c)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if repo.weights.TTFTTargetMS != 1400 || repo.weights.TTFTLimitMS != 7000 || repo.weights.LatencyTargetMS != 18000 || repo.weights.LatencyLimitMS != 70000 {
		t.Fatalf("legacy request overwrote thresholds: %#v", repo.weights)
	}
}

func TestAccountMonitorHandlerDefaultsAndValidatesWindowRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(&accountMonitorHandlerRepoStub{}, &accountMonitorHandlerAccountRepoStub{}, nil, nil, nil), nil, nil, nil)

	for _, tt := range []struct {
		name string
		url  string
		want int
	}{
		{name: "default", url: "/api/v1/admin/accounts/monitor", want: http.StatusOK},
		{name: "valid", url: "/api/v1/admin/accounts/monitor?range=7d", want: http.StatusOK},
		{name: "invalid", url: "/api/v1/admin/accounts/monitor?range=48h", want: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)
			h.List(c)
			if res.Code != tt.want {
				t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
			}
			if tt.want == http.StatusOK && !strings.Contains(res.Body.String(), `"range":"`+map[string]string{"default": "24h", "valid": "7d"}[tt.name]+`"`) {
				t.Fatalf("window response = %s", res.Body.String())
			}
		})
	}
}

func TestAccountMonitorHandlerReturnsCompleteWindowTimelineAndGlobalRanking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 0.5
	repo := &accountMonitorHandlerRepoStub{
		latest: map[int64]service.AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		timelines: map[int64][]service.AccountMonitorTimelinePoint{7: {
			{Status: "success", CheckedAt: now.Add(-time.Minute)},
			{Status: "failed", CheckedAt: now},
		}},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{7: {
			RequestCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: &rate, LatencyP95MS: &rate, LastObservedAt: &now,
		}},
	}
	accounts := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{
		ID: 7, Name: "handler-account", Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate,
	}}}
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, accounts, nil, nil, nil), nil, nil, nil)
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data struct {
			Accounts []struct {
				Timeline     []service.AccountMonitorTimelinePoint `json:"timeline"`
				QualityScore *float64                              `json:"quality_score"`
				GroupRank    *int                                  `json:"group_rank"`
			} `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Accounts) != 1 || len(payload.Data.Accounts[0].Timeline) != 2 || payload.Data.Accounts[0].Timeline[1].Status != "failed" || payload.Data.Accounts[0].QualityScore == nil || payload.Data.Accounts[0].GroupRank == nil || *payload.Data.Accounts[0].GroupRank != 1 {
		t.Fatalf("window payload missing full account monitor fields: %s", res.Body.String())
	}
}

type accountMonitorHandlerAccountRepoStub struct {
	service.AccountRepository
	accounts []*service.Account
	err      error
	calls    int
	gotIDs   []int64
}

func (s *accountMonitorHandlerAccountRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]service.Account, error) {
	accounts := make([]service.Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account != nil {
			accounts = append(accounts, *account)
		}
	}
	return accounts, nil
}

func (s *accountMonitorHandlerAccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.calls++
	s.gotIDs = append([]int64(nil), ids...)
	return s.accounts, s.err
}

type accountMonitorHandlerConcurrencyCacheStub struct {
	service.ConcurrencyCache
	current map[int64]int
	err     error
	calls   int
	gotIDs  []int64
}

func (s *accountMonitorHandlerConcurrencyCacheStub) GetAccountConcurrencyBatch(_ context.Context, ids []int64) (map[int64]int, error) {
	s.calls++
	s.gotIDs = append([]int64(nil), ids...)
	return s.current, s.err
}

type accountMonitorConcurrencyEnvelope struct {
	Code int `json:"code"`
	Data struct {
		Items []struct {
			AccountID int64 `json:"account_id"`
			Current   int   `json:"current"`
			Limit     int   `json:"limit"`
		} `json:"items"`
	} `json:"data"`
}

func newAccountMonitorConcurrencyHandler(
	accountRepo *accountMonitorHandlerAccountRepoStub,
	cache *accountMonitorHandlerConcurrencyCacheStub,
) *AccountMonitorHandler {
	return NewAccountMonitorHandler(nil, nil, accountRepo, service.NewConcurrencyService(cache))
}

func runAccountMonitorConcurrencyRequest(t *testing.T, h *AccountMonitorHandler, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/monitor/concurrency", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Concurrency(c)
	return res
}

func TestAccountMonitorHandlerConcurrencyBatchesDeduplicatesAndPreservesOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{
		{ID: 1, Concurrency: 10},
		{ID: 2, Concurrency: 7},
	}}
	cache := &accountMonitorHandlerConcurrencyCacheStub{current: map[int64]int{1: 3, 2: 4}}
	h := newAccountMonitorConcurrencyHandler(accountRepo, cache)

	res := runAccountMonitorConcurrencyRequest(t, h, map[string]any{"account_ids": []int64{2, 1, 2}})

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if accountRepo.calls != 1 || !reflect.DeepEqual(accountRepo.gotIDs, []int64{2, 1}) {
		t.Fatalf("account lookup calls=%d ids=%v", accountRepo.calls, accountRepo.gotIDs)
	}
	if cache.calls != 1 || !reflect.DeepEqual(cache.gotIDs, []int64{2, 1}) {
		t.Fatalf("concurrency batch calls=%d ids=%v", cache.calls, cache.gotIDs)
	}
	var envelope accountMonitorConcurrencyEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	want := [][3]int64{{2, 4, 7}, {1, 3, 10}}
	if len(envelope.Data.Items) != len(want) {
		t.Fatalf("items = %#v", envelope.Data.Items)
	}
	for i, item := range envelope.Data.Items {
		got := [3]int64{item.AccountID, int64(item.Current), int64(item.Limit)}
		if got != want[i] {
			t.Fatalf("item[%d] = %v, want %v", i, got, want[i])
		}
	}
}

func TestAccountMonitorHandlerConcurrencyValidatesBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tooMany := make([]int64, 201)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for _, tt := range []struct {
		name string
		ids  []int64
	}{
		{name: "empty", ids: []int64{}},
		{name: "zero", ids: []int64{1, 0}},
		{name: "negative", ids: []int64{-1}},
		{name: "more than 200 unique", ids: tooMany},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accountRepo := &accountMonitorHandlerAccountRepoStub{}
			cache := &accountMonitorHandlerConcurrencyCacheStub{}
			res := runAccountMonitorConcurrencyRequest(t, newAccountMonitorConcurrencyHandler(accountRepo, cache), map[string]any{"account_ids": tt.ids})

			if res.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
			}
			if accountRepo.calls != 0 || cache.calls != 0 {
				t.Fatalf("invalid request reached dependencies: account=%d concurrency=%d", accountRepo.calls, cache.calls)
			}
		})
	}
}

func TestAccountMonitorHandlerConcurrencyAllowsTwoHundredUniqueIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := make([]int64, 200)
	accounts := make([]*service.Account, 200)
	current := make(map[int64]int, 200)
	for i := range ids {
		ids[i] = int64(i + 1)
		accounts[i] = &service.Account{ID: ids[i], Concurrency: 10}
		current[ids[i]] = i
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: accounts}
	cache := &accountMonitorHandlerConcurrencyCacheStub{current: current}

	res := runAccountMonitorConcurrencyRequest(t, newAccountMonitorConcurrencyHandler(accountRepo, cache), map[string]any{"account_ids": ids})

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if accountRepo.calls != 1 || len(accountRepo.gotIDs) != 200 || cache.calls != 1 || len(cache.gotIDs) != 200 {
		t.Fatalf("calls/account counts: account=%d/%d concurrency=%d/%d", accountRepo.calls, len(accountRepo.gotIDs), cache.calls, len(cache.gotIDs))
	}
}

func TestAccountMonitorHandlerConcurrencyRejectsUnknownBeforeRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{ID: 1, Concurrency: 10}}}
	cache := &accountMonitorHandlerConcurrencyCacheStub{}

	res := runAccountMonitorConcurrencyRequest(t, newAccountMonitorConcurrencyHandler(accountRepo, cache), map[string]any{"account_ids": []int64{1, 2}})

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if accountRepo.calls != 1 || cache.calls != 0 {
		t.Fatalf("calls: account=%d concurrency=%d", accountRepo.calls, cache.calls)
	}
}

func TestAccountMonitorHandlerConcurrencyReturnsInternalFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name         string
		accountError error
		redisError   error
		wantRedis    int
	}{
		{name: "account lookup", accountError: errors.New("database unavailable"), wantRedis: 0},
		{name: "redis batch", redisError: errors.New("redis unavailable"), wantRedis: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accountRepo := &accountMonitorHandlerAccountRepoStub{
				accounts: []*service.Account{{ID: 1, Concurrency: 10}},
				err:      tt.accountError,
			}
			cache := &accountMonitorHandlerConcurrencyCacheStub{err: tt.redisError}
			res := runAccountMonitorConcurrencyRequest(t, newAccountMonitorConcurrencyHandler(accountRepo, cache), map[string]any{"account_ids": []int64{1}})

			if res.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
			}
			if accountRepo.calls != 1 || cache.calls != tt.wantRedis {
				t.Fatalf("calls: account=%d concurrency=%d", accountRepo.calls, cache.calls)
			}
		})
	}
}
