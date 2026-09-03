package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMonitorHandlerRepoStub struct {
	service.AccountMonitorRepository
	weights               service.AccountMonitorScoreWeights
	globalWeights         service.AccountMonitorScoreWeights
	globalWeightsErr      error
	globalWeightsSaveErr  error
	globalWeightsResetErr error
	savedGlobalWeights    []service.AccountMonitorScoreWeights
	latest                map[int64]service.AccountMonitorLatest
	timelines             map[int64][]service.AccountMonitorTimelinePoint
	aggregates            map[int64]service.AccountMonitorAggregate
	windowAggregates      map[int64]service.AccountMonitorWindowAggregate
	groupWindowAggregates map[int64]map[int64]service.AccountMonitorWindowAggregate
	groups                []service.AccountMonitorGroup
}

func monitorTimePtr(value time.Time) *time.Time { return &value }
func monitorIntPtr(value int) *int              { return &value }

func (*accountMonitorHandlerRepoStub) LoadSettings(context.Context) (service.AccountMonitorSettings, error) {
	return service.AccountMonitorSettings{IntervalSeconds: 300}, nil
}

func (s *accountMonitorHandlerRepoStub) ListGroups(context.Context) ([]service.AccountMonitorGroup, error) {
	return append([]service.AccountMonitorGroup(nil), s.groups...), nil
}

func (s *accountMonitorHandlerRepoStub) ListGroupWindowAggregates(_ context.Context, groupID int64, _ []int64, _, _ time.Time) (map[int64]service.AccountMonitorWindowAggregate, error) {
	if s.groupWindowAggregates == nil {
		return s.windowAggregates, nil
	}
	return s.groupWindowAggregates[groupID], nil
}

func (s *accountMonitorHandlerRepoStub) ListLatest(context.Context, []int64) (map[int64]service.AccountMonitorLatest, error) {
	return s.latest, nil
}

func (s *accountMonitorHandlerRepoStub) ListTimelines(context.Context, []int64, int) (map[int64][]service.AccountMonitorTimelinePoint, error) {
	return s.timelines, nil
}

func (s *accountMonitorHandlerRepoStub) ListAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorAggregate, error) {
	return s.aggregates, nil
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

func (s *accountMonitorHandlerRepoStub) LoadGlobalScoreWeights(context.Context) (service.AccountMonitorScoreWeights, error) {
	if s.globalWeightsErr != nil {
		return service.AccountMonitorScoreWeights{}, s.globalWeightsErr
	}
	return s.globalWeights, nil
}

func (s *accountMonitorHandlerRepoStub) SaveGlobalScoreWeights(_ context.Context, actorID int64, weights service.AccountMonitorScoreWeights) (service.AccountMonitorScoreWeights, error) {
	if s.globalWeightsSaveErr != nil {
		return service.AccountMonitorScoreWeights{}, s.globalWeightsSaveErr
	}
	weights.UpdatedBy = actorID
	s.savedGlobalWeights = append(s.savedGlobalWeights, weights)
	s.globalWeights = weights
	return weights, nil
}

func (s *accountMonitorHandlerRepoStub) ResetGlobalScoreWeights(context.Context) error {
	if s.globalWeightsResetErr != nil {
		return s.globalWeightsResetErr
	}
	s.globalWeights = service.AccountMonitorScoreWeights{}
	s.globalWeightsErr = sql.ErrNoRows
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

func TestAccountMonitorHandlerGlobalScoreWeightsCRUD(t *testing.T) {
	repo := &accountMonitorHandlerRepoStub{globalWeightsErr: sql.ErrNoRows}
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	})
	router.GET("/global-score-weights", h.GetGlobalScoreWeights)
	router.PUT("/global-score-weights", h.UpdateGlobalScoreWeights)
	router.DELETE("/global-score-weights", h.ResetGlobalScoreWeights)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/global-score-weights", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"is_default":true`) {
		t.Fatalf("GET response = %d %s", get.Code, get.Body.String())
	}

	body := strings.NewReader(`{"cost":25,"success":35,"ttft":20,"latency":20,"ttft_target_ms":1}`)
	put := httptest.NewRecorder()
	router.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/global-score-weights", body))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT response = %d %s", put.Code, put.Body.String())
	}
	if saved := repo.savedGlobalWeights[len(repo.savedGlobalWeights)-1]; saved.TTFTTargetMS != 0 {
		t.Fatalf("threshold field leaked into global save: %#v", saved)
	}

	del := httptest.NewRecorder()
	router.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/global-score-weights", nil))
	if del.Code != http.StatusOK || !strings.Contains(del.Body.String(), `"is_default":true`) {
		t.Fatalf("DELETE response = %d %s", del.Code, del.Body.String())
	}
}

func TestAccountMonitorHandlerGlobalScoreWeightsStorageErrorsAreNotBadRequest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		errField string
		method   string
		body     string
	}{
		{name: "get load error", errField: "load", method: http.MethodGet},
		{name: "put save error", errField: "save", method: http.MethodPut, body: `{"cost":25,"success":35,"ttft":20,"latency":20}`},
		{name: "delete reset error", errField: "reset", method: http.MethodDelete},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountMonitorHandlerRepoStub{}
			switch tt.errField {
			case "load":
				repo.globalWeightsErr = errors.New("database unavailable")
			case "save":
				repo.globalWeightsSaveErr = errors.New("database unavailable")
			case "reset":
				repo.globalWeightsResetErr = errors.New("database unavailable")
			}
			h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			})
			router.GET("/global-score-weights", h.GetGlobalScoreWeights)
			router.PUT("/global-score-weights", h.UpdateGlobalScoreWeights)
			router.DELETE("/global-score-weights", h.ResetGlobalScoreWeights)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, "/global-score-weights", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusBadRequest {
				t.Fatalf("storage error returned 400: %s", recorder.Body.String())
			}
		})
	}
}

func TestAccountMonitorHandlerRejectsOverflowSizedGlobalScoreWeights(t *testing.T) {
	repo := &accountMonitorHandlerRepoStub{}
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	})
	router.PUT("/global-score-weights", h.UpdateGlobalScoreWeights)

	maxInt := strconv.Itoa(int(^uint(0) >> 1))
	body := `{"cost":` + maxInt + `,"success":` + maxInt + `,"ttft":101,"latency":1}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/global-score-weights", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_SCORE_WEIGHTS") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(repo.savedGlobalWeights) != 0 {
		t.Fatalf("invalid weights reached repository save: %#v", repo.savedGlobalWeights)
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

func TestAccountMonitorHandlerPublicJSONHidesProbeSourceAndPreservesSchedulerAndAccounting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorHandlerRepoStub{
		aggregates: map[int64]service.AccountMonitorAggregate{
			7: {SampleCount: 1, SuccessSampleCount: 1, SuccessRate: 1, LastCheckedAt: &now},
		},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{
			7: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now},
		},
		latest: map[int64]service.AccountMonitorLatest{
			7: {Status: "success", CheckedAt: now},
		},
		groups: []service.AccountMonitorGroup{{
			ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true,
			RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights,
		}},
	}
	accounts := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{
		ID: 7, Name: "probe-only", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42},
	}}}
	svc := service.NewAccountMonitorService(repo, accounts, nil, nil, &accountMonitorHandlerMultiplierStub{})
	probeRank := 2
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{projection: &service.OpenAIAccountSchedulerProjection{
		SnapshotAt: now, CandidateCount: 2,
		Candidates: []service.OpenAIAccountSchedulerProjectionCandidate{{AccountID: 7, Rank: &probeRank, Eligible: true}},
	}}
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Groups []struct {
				Accounts []map[string]json.RawMessage `json:"accounts"`
			} `json:"groups"`
			Accounts []map[string]json.RawMessage `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if len(envelope.Data.Accounts) != 1 || len(envelope.Data.Groups) != 1 || len(envelope.Data.Groups[0].Accounts) != 1 {
		t.Fatalf("accounts = %s", res.Body.String())
	}
	row := envelope.Data.Groups[0].Accounts[0]
	fullSiteRow := envelope.Data.Accounts[0]
	for _, field := range []string{"probe_count", "probe_success_count", "probe_failure_count", "probe_sample_count", "source", "real_request_sample_count"} {
		if _, exposed := row[field]; exposed {
			t.Fatalf("public monitor row exposed %q: %s", field, res.Body.String())
		}
		if _, exposed := fullSiteRow[field]; exposed {
			t.Fatalf("public full-site row exposed %q: %s", field, res.Body.String())
		}
	}
	var requestCount int64
	if err := json.Unmarshal(row["request_count"], &requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("request_count = %d, want 1", requestCount)
	}
	var schedulerRank int
	if err := json.Unmarshal(row["scheduler_rank"], &schedulerRank); err != nil {
		t.Fatal(err)
	}
	if schedulerRank != 2 {
		t.Fatalf("scheduler_rank = %d, want 2", schedulerRank)
	}
	var profitability service.AccountMonitorGroupProfitability
	if err := json.Unmarshal(row["group_profitability"], &profitability); err != nil {
		t.Fatal(err)
	}
	if profitability.Revenue != 0 || profitability.AccountCost != 0 {
		t.Fatalf("probe-only accounting leaked into profitability: %#v", profitability)
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
			RequestCount: 3, SuccessCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: &rate, LatencyP95MS: &rate, LastObservedAt: &now,
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
				ScoreStatus  string                                `json:"score_status"`
			} `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Accounts) != 1 || len(payload.Data.Accounts[0].Timeline) != 2 || payload.Data.Accounts[0].Timeline[1].Status != "failed" || payload.Data.Accounts[0].QualityScore != nil || payload.Data.Accounts[0].GroupRank != nil || payload.Data.Accounts[0].ScoreStatus != "ineligible" {
		t.Fatalf("window payload missing full account monitor fields: %s", res.Body.String())
	}
}

func TestAccountMonitorHandlerReturnsUnavailableAndStaleRowsWithRetainedNativeScores(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 0.5
	repo := &accountMonitorHandlerRepoStub{
		globalWeights: service.AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20},
		aggregates: map[int64]service.AccountMonitorAggregate{
			8: {SampleCount: 24, SuccessCount: 21, SuccessSampleCount: 21, ErrorCount: 3, SuccessRate: 21.0 / 24.0, ConsecutiveFailed: 3, LastCheckedAt: &now},
			9: {SampleCount: 24, SuccessCount: 24, SuccessSampleCount: 24, SuccessRate: 1, LastCheckedAt: monitorTimePtr(now.Add(-20 * time.Minute))},
		},
		latest: map[int64]service.AccountMonitorLatest{
			8: {Status: "failed", HTTPStatus: monitorIntPtr(http.StatusBadGateway), CheckedAt: now},
			9: {Status: "success", CheckedAt: now.Add(-20 * time.Minute)},
		},
	}
	accounts := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{
		{ID: 8, Name: "unavailable-with-score", Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate},
		{ID: 9, Name: "stale-with-score", Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate},
	}}
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
				AvailabilityStatus string   `json:"availability_status"`
				Stale              bool     `json:"stale"`
				QualityScore       *float64 `json:"quality_score"`
				GroupRank          *int     `json:"group_rank"`
				ScoreStatus        string   `json:"score_status"`
			} `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Accounts) != 2 {
		t.Fatalf("accounts = %s", res.Body.String())
	}
	byStatus := map[string]struct {
		stale bool
		score *float64
		rank  *int
	}{
		payload.Data.Accounts[0].AvailabilityStatus: {payload.Data.Accounts[0].Stale, payload.Data.Accounts[0].QualityScore, payload.Data.Accounts[0].GroupRank},
		payload.Data.Accounts[1].AvailabilityStatus: {payload.Data.Accounts[1].Stale, payload.Data.Accounts[1].QualityScore, payload.Data.Accounts[1].GroupRank},
	}
	if row := byStatus["unavailable"]; row.score == nil || row.rank == nil {
		t.Fatalf("unavailable row lost score/rank: %#v", byStatus)
	}
	if row := byStatus["stale"]; !row.stale || row.score == nil || row.rank == nil {
		t.Fatalf("stale row lost truthful state or score/rank: %#v", byStatus)
	}
}

type accountMonitorHandlerSchedulerProjectionStub struct {
	projection *service.OpenAIAccountSchedulerProjection
	err        error
	calls      int
	requests   []service.OpenAIAccountSchedulerProjectionRequest
}

func (s *accountMonitorHandlerSchedulerProjectionStub) Project(_ context.Context, request service.OpenAIAccountSchedulerProjectionRequest) (*service.OpenAIAccountSchedulerProjection, error) {
	s.calls++
	s.requests = append(s.requests, request)
	return s.projection, s.err
}

type accountMonitorHandlerMultiplierStub struct{}

func (*accountMonitorHandlerMultiplierStub) Resolve(*service.Account, time.Time) service.AccountMonitorMultiplier {
	value := 1.0
	return service.AccountMonitorMultiplier{Value: &value, Source: "declared", Status: "ok", SampleCount: 1}
}

func (*accountMonitorHandlerMultiplierStub) Refresh(context.Context, *service.Account, service.AccountMonitorRefreshOptions) error {
	return nil
}

func TestAccountMonitorHandlerGroupResponseIncludesRankingContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	accounts := []*service.Account{
		{ID: 7, Name: "quality-first", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}},
		{ID: 8, Name: "scheduler-first", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}},
	}
	repo := &accountMonitorHandlerRepoStub{
		globalWeights: service.DefaultAccountMonitorScoreWeights,
		aggregates: map[int64]service.AccountMonitorAggregate{
			7: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now},
			8: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now},
		},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{
			7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now},
			8: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now},
		},
		groupWindowAggregates: map[int64]map[int64]service.AccountMonitorWindowAggregate{
			42: {
				7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now},
				8: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now},
			},
		},
		latest: map[int64]service.AccountMonitorLatest{
			7: {Status: "success", CheckedAt: now},
			8: {Status: "success", CheckedAt: now},
		},
		groups: []service.AccountMonitorGroup{{ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true, RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights}},
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: accounts}
	schedulerRank1, schedulerRank2 := 1, 2
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{projection: &service.OpenAIAccountSchedulerProjection{
		SnapshotAt: now, PolicyKey: "group_policy", PolicyLabel: "利润优先", CandidateCount: 2,
		EffectiveWeights: map[string]float64{"upstream_cost": 3},
		EffectiveFacts:   []service.AccountMonitorSchedulerFact{{Label: "上游成本权重", Value: "3"}},
		Candidates: []service.OpenAIAccountSchedulerProjectionCandidate{
			{AccountID: 8, Rank: &schedulerRank1, Eligible: true, PrimaryReasonCode: service.AccountMonitorReasonStrategy},
			{AccountID: 7, Rank: &schedulerRank2, Eligible: true},
		},
	}}
	svc := service.NewAccountMonitorService(repo, accountRepo, nil, nil, &accountMonitorHandlerMultiplierStub{})
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Groups []struct {
				Accounts []map[string]json.RawMessage `json:"accounts"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if envelope.Code != 0 || envelope.Message != "success" || len(envelope.Data.Groups) != 1 || len(envelope.Data.Groups[0].Accounts) != 2 {
		t.Fatalf("unexpected response envelope: %s", res.Body.String())
	}
	byID := make(map[int64]map[string]json.RawMessage, len(envelope.Data.Groups[0].Accounts))
	for _, row := range envelope.Data.Groups[0].Accounts {
		var accountID int64
		if err := json.Unmarshal(row["account_id"], &accountID); err != nil {
			t.Fatalf("decode account_id: %v", err)
		}
		byID[accountID] = row
	}
	var qualityRank, qualityRankTotal, groupRank, schedulerRank, schedulerRankTotal int
	if err := json.Unmarshal(byID[7]["quality_rank"], &qualityRank); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[7]["quality_rank_total"], &qualityRankTotal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[7]["group_rank"], &groupRank); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[7]["scheduler_rank"], &schedulerRank); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[7]["scheduler_rank_total"], &schedulerRankTotal); err != nil {
		t.Fatal(err)
	}
	var schedulerRank8, schedulerRankTotal8 int
	if err := json.Unmarshal(byID[8]["scheduler_rank"], &schedulerRank8); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[8]["scheduler_rank_total"], &schedulerRankTotal8); err != nil {
		t.Fatal(err)
	}
	if qualityRank != groupRank || qualityRankTotal != 2 || schedulerRank != 2 || schedulerRankTotal != 2 || schedulerRank8 != 1 || schedulerRankTotal8 != 2 {
		t.Fatalf("ranking contract for account 7 = quality %d/%d group %d scheduler %d/%d", qualityRank, qualityRankTotal, groupRank, schedulerRank, schedulerRankTotal)
	}
	var explanation struct {
		Rank           *int   `json:"rank"`
		RankTotal      int    `json:"rank_total"`
		CandidateTotal int    `json:"candidate_total"`
		Eligible       bool   `json:"eligible"`
		PolicyKey      string `json:"policy_key"`
		PolicyLabel    string `json:"policy_label"`
		EffectiveFacts []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"effective_facts"`
		CandidateScope     string    `json:"candidate_scope"`
		SnapshotAt         time.Time `json:"snapshot_at"`
		PrimaryReasonCode  string    `json:"primary_reason_code"`
		PrimaryReasonLabel string    `json:"primary_reason_label"`
	}
	if err := json.Unmarshal(byID[8]["scheduler_explanation"], &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Rank == nil || *explanation.Rank != 1 || explanation.RankTotal != 2 || explanation.CandidateTotal != 2 || !explanation.Eligible || explanation.PolicyKey != "group_policy" || explanation.PolicyLabel != "利润优先" || len(explanation.EffectiveFacts) != 1 || explanation.EffectiveFacts[0].Label != "上游成本权重" || explanation.EffectiveFacts[0].Value != "3" || explanation.CandidateScope != "group" || !explanation.SnapshotAt.Equal(now) || explanation.PrimaryReasonCode != string(service.AccountMonitorReasonStrategy) || explanation.PrimaryReasonLabel == "" {
		t.Fatalf("scheduler explanation = %#v", explanation)
	}
	if _, exposed := byID[8]["effective_weights"]; exposed {
		t.Fatal("scheduler explanation exposed raw effective weight keys")
	}
	var secondExplanation struct {
		Rank      *int `json:"rank"`
		RankTotal int  `json:"rank_total"`
		Eligible  bool `json:"eligible"`
	}
	if err := json.Unmarshal(byID[7]["scheduler_explanation"], &secondExplanation); err != nil {
		t.Fatal(err)
	}
	if secondExplanation.Rank == nil || *secondExplanation.Rank != 2 || secondExplanation.RankTotal != 2 || !secondExplanation.Eligible {
		t.Fatalf("account 7 scheduler explanation = %#v", secondExplanation)
	}
	if scheduler.calls != 1 {
		t.Fatalf("scheduler projection calls = %d, want 1", scheduler.calls)
	}
	if len(scheduler.requests) != 1 || scheduler.requests[0].GroupID != 42 || scheduler.requests[0].Platform != service.PlatformOpenAI || scheduler.requests[0].RequiredTransport != service.OpenAIUpstreamTransportAny || scheduler.requests[0].RequestedModel != "" || scheduler.requests[0].SnapshotAt.IsZero() {
		t.Fatalf("scheduler projection request = %#v", scheduler.requests)
	}
}

func TestAccountMonitorHandlerFullSiteRowsUseBestGroupSchedulerRanking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorHandlerRepoStub{
		globalWeights:    service.DefaultAccountMonitorScoreWeights,
		aggregates:       map[int64]service.AccountMonitorAggregate{7: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now}},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		groupWindowAggregates: map[int64]map[int64]service.AccountMonitorWindowAggregate{
			42: {7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		},
		latest: map[int64]service.AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		groups: []service.AccountMonitorGroup{{ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true, RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights}},
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{ID: 7, Name: "full-site-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}}}}
	rank := 1
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{projection: &service.OpenAIAccountSchedulerProjection{
		SnapshotAt: now, PolicyLabel: "利润优先", CandidateCount: 1,
		Candidates: []service.OpenAIAccountSchedulerProjectionCandidate{{AccountID: 7, Rank: &rank, Eligible: true, QualityScore: func() *float64 { v := 88.0; return &v }()}},
	}}
	svc := service.NewAccountMonitorService(repo, accountRepo, nil, nil, &accountMonitorHandlerMultiplierStub{})
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Accounts []map[string]json.RawMessage `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if len(envelope.Data.Accounts) != 1 {
		t.Fatalf("full-site accounts = %s", res.Body.String())
	}
	row := envelope.Data.Accounts[0]
	if _, ok := row["quality_score"]; !ok {
		t.Fatalf("full-site row missing quality_score: %s", res.Body.String())
	}
	if _, ok := row["scheduler_rank"]; ok {
		t.Fatalf("full-site row must not expose scheduler rank: %s", res.Body.String())
	}
}

func TestAccountMonitorHandlerProjectionFailureLeavesSchedulerStateUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorHandlerRepoStub{
		globalWeights:    service.DefaultAccountMonitorScoreWeights,
		aggregates:       map[int64]service.AccountMonitorAggregate{7: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now}},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		groupWindowAggregates: map[int64]map[int64]service.AccountMonitorWindowAggregate{
			42: {7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		},
		latest: map[int64]service.AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		groups: []service.AccountMonitorGroup{{ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true, RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights}},
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{ID: 7, Name: "projection-failure", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}}}}
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{err: errors.New("scheduler projection unavailable")}
	svc := service.NewAccountMonitorService(repo, accountRepo, nil, nil, &accountMonitorHandlerMultiplierStub{})
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Groups []struct {
				Accounts []map[string]json.RawMessage `json:"accounts"`
			} `json:"groups"`
			Accounts []map[string]json.RawMessage `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if envelope.Code != 0 || len(envelope.Data.Groups) != 1 || len(envelope.Data.Groups[0].Accounts) != 1 || len(envelope.Data.Accounts) != 1 {
		t.Fatalf("projection failure changed monitor envelope: %s", res.Body.String())
	}
	groupRow := envelope.Data.Groups[0].Accounts[0]
	fullSiteRow := envelope.Data.Accounts[0]
	if _, ok := groupRow["quality_rank"]; !ok {
		t.Fatalf("projection failure discarded quality state: %s", res.Body.String())
	}
	for _, field := range []string{"scheduler_rank", "scheduler_rank_total", "scheduler_explanation"} {
		if _, ok := groupRow[field]; ok {
			t.Fatalf("projection failure fabricated %s: %s", field, res.Body.String())
		}
		if _, ok := fullSiteRow[field]; ok {
			t.Fatalf("full-site row fabricated %s after projection failure: %s", field, res.Body.String())
		}
	}
	var schedulerUnavailable bool
	if err := json.Unmarshal(groupRow["scheduler_unavailable"], &schedulerUnavailable); err != nil {
		t.Fatalf("decode scheduler_unavailable: %v", err)
	}
	if !schedulerUnavailable {
		t.Fatalf("projection failure did not expose scheduler unavailability: %s", res.Body.String())
	}
	if scheduler.calls != 1 {
		t.Fatalf("scheduler projection calls = %d, want 1", scheduler.calls)
	}
}

func TestAccountMonitorHandlerNilProjectionExposesSchedulerUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorHandlerRepoStub{
		globalWeights:    service.DefaultAccountMonitorScoreWeights,
		aggregates:       map[int64]service.AccountMonitorAggregate{7: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now}},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		groupWindowAggregates: map[int64]map[int64]service.AccountMonitorWindowAggregate{
			42: {7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		},
		latest: map[int64]service.AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		groups: []service.AccountMonitorGroup{{ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true, RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights}},
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{ID: 7, Name: "nil-projection", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}}}}
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{}
	svc := service.NewAccountMonitorService(repo, accountRepo, nil, nil, &accountMonitorHandlerMultiplierStub{})
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	h := NewAccountMonitorHandler(svc, nil, nil, nil)
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Groups []struct {
				Accounts []map[string]json.RawMessage `json:"accounts"`
			} `json:"groups"`
			Accounts []map[string]json.RawMessage `json:"accounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if envelope.Code != 0 || len(envelope.Data.Groups) != 1 || len(envelope.Data.Groups[0].Accounts) != 1 || len(envelope.Data.Accounts) != 1 {
		t.Fatalf("nil projection changed monitor envelope: %s", res.Body.String())
	}
	groupRow := envelope.Data.Groups[0].Accounts[0]
	fullSiteRow := envelope.Data.Accounts[0]
	if _, ok := groupRow["quality_rank"]; !ok {
		t.Fatalf("nil projection discarded quality state: %s", res.Body.String())
	}
	var schedulerUnavailable bool
	if err := json.Unmarshal(groupRow["scheduler_unavailable"], &schedulerUnavailable); err != nil {
		t.Fatalf("decode scheduler_unavailable: %v", err)
	}
	if !schedulerUnavailable {
		t.Fatalf("nil projection did not expose scheduler unavailability: %s", res.Body.String())
	}
	for _, field := range []string{"scheduler_rank", "scheduler_rank_total", "scheduler_explanation"} {
		if _, ok := groupRow[field]; ok {
			t.Fatalf("nil projection fabricated %s: %s", field, res.Body.String())
		}
		if _, ok := fullSiteRow[field]; ok {
			t.Fatalf("full-site row fabricated %s after nil projection: %s", field, res.Body.String())
		}
	}
	if scheduler.calls != 1 {
		t.Fatalf("scheduler projection calls = %d, want 1", scheduler.calls)
	}
}

func TestAccountMonitorHandlerNilLoadSnapshotExposesSchedulerUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorHandlerRepoStub{
		globalWeights:    service.DefaultAccountMonitorScoreWeights,
		aggregates:       map[int64]service.AccountMonitorAggregate{7: {SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1, LastCheckedAt: &now}},
		windowAggregates: map[int64]service.AccountMonitorWindowAggregate{7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		groupWindowAggregates: map[int64]map[int64]service.AccountMonitorWindowAggregate{
			42: {7: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, LastObservedAt: &now}},
		},
		latest: map[int64]service.AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		groups: []service.AccountMonitorGroup{{ID: 42, Name: "GPT-Pro", Platform: service.PlatformOpenAI, CustomerVisible: true, RateMultiplier: 1, ScoreWeights: service.DefaultAccountMonitorScoreWeights}},
	}
	accountRepo := &accountMonitorHandlerAccountRepoStub{accounts: []*service.Account{{ID: 7, Name: "nil-load-snapshot", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}}}}
	scheduler := &accountMonitorHandlerSchedulerProjectionStub{projection: &service.OpenAIAccountSchedulerProjection{CandidateCount: 1}}
	svc := service.NewAccountMonitorService(repo, accountRepo, nil, nil, &accountMonitorHandlerMultiplierStub{})
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)
	svc.SetAccountMonitorConcurrencyService(service.NewConcurrencyService(nil))
	h := NewAccountMonitorHandler(svc, nil, nil, nil)
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/monitor?range=24h", nil)
	h.List(c)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Groups []struct {
				Accounts []map[string]json.RawMessage `json:"accounts"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
	if len(envelope.Data.Groups) != 1 || len(envelope.Data.Groups[0].Accounts) != 1 {
		t.Fatalf("nil load snapshot changed monitor envelope: %s", res.Body.String())
	}
	row := envelope.Data.Groups[0].Accounts[0]
	var unavailable bool
	if err := json.Unmarshal(row["scheduler_unavailable"], &unavailable); err != nil {
		t.Fatalf("decode scheduler_unavailable: %v", err)
	}
	if !unavailable {
		t.Fatalf("nil load snapshot did not expose scheduler unavailability: %s", res.Body.String())
	}
	for _, field := range []string{"scheduler_rank", "scheduler_rank_total", "scheduler_explanation"} {
		if _, ok := row[field]; ok {
			t.Fatalf("nil load snapshot fabricated %s: %s", field, res.Body.String())
		}
	}
	if scheduler.calls != 0 {
		t.Fatalf("scheduler projection calls = %d, want 0 when load snapshot is unavailable", scheduler.calls)
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
