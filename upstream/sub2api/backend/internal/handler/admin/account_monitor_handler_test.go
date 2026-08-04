package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMonitorHandlerRepoStub struct {
	service.AccountMonitorRepository
	weights service.AccountMonitorScoreWeights
}

func (*accountMonitorHandlerRepoStub) LoadSettings(context.Context) (service.AccountMonitorSettings, error) {
	return service.AccountMonitorSettings{IntervalSeconds: 300}, nil
}

func (*accountMonitorHandlerRepoStub) ListGroups(context.Context) ([]service.AccountMonitorGroup, error) {
	return nil, nil
}

func (*accountMonitorHandlerRepoStub) ListLatest(context.Context, []int64) (map[int64]service.AccountMonitorLatest, error) {
	return nil, nil
}

func (*accountMonitorHandlerRepoStub) ListTimelines(context.Context, []int64, int) (map[int64][]service.AccountMonitorTimelinePoint, error) {
	return nil, nil
}

func (*accountMonitorHandlerRepoStub) ListAggregates(context.Context, []int64, time.Time) (map[int64]service.AccountMonitorAggregate, error) {
	return nil, nil
}

func (*accountMonitorHandlerRepoStub) ListWindowAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorWindowAggregate, error) {
	return nil, nil
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
	h := NewAccountMonitorHandler(svc, nil)

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
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil)
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
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(&accountMonitorHandlerRepoStub{}, &accountMonitorHandlerAccountRepoStub{}, nil, nil, nil), nil)

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

type accountMonitorHandlerAccountRepoStub struct{}

func (*accountMonitorHandlerAccountRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]service.Account, error) {
	return nil, nil
}
