package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMonitorHandlerRepoStub struct {
	service.AccountMonitorRepository
	weights service.AccountMonitorScoreWeights
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
