package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminUsageDetailRepo struct {
	service.UsageLogRepository
	record *service.UsageLog
	err    error
	gotID  int64
	calls  int
}

func (r *adminUsageDetailRepo) GetByID(_ context.Context, id int64) (*service.UsageLog, error) {
	r.calls++
	r.gotID = id
	return r.record, r.err
}

func newAdminUsageDetailTestRouter(repo *adminUsageDetailRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageService := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageService, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/usage/:id", handler.GetByID)
	return router
}

func TestAdminUsageGetByIDReturnsAdminProjection(t *testing.T) {
	upstreamModel := "claude-sonnet-4-20250514"
	channelID := int64(7)
	billingTier := "priority"
	accountRateMultiplier := 1.25
	inboundEndpoint := "/v1/messages"
	upstreamEndpoint := "/v1/responses"
	repo := &adminUsageDetailRepo{
		record: &service.UsageLog{
			ID:                    42,
			RequestID:             "req-admin-detail",
			Model:                 "claude-sonnet-4",
			InboundEndpoint:       &inboundEndpoint,
			UpstreamEndpoint:      &upstreamEndpoint,
			UpstreamModel:         &upstreamModel,
			ChannelID:             &channelID,
			BillingTier:           &billingTier,
			AccountRateMultiplier: &accountRateMultiplier,
			Account: &service.Account{
				ID:   9,
				Name: "admin-visible-account",
			},
		},
	}
	router := newAdminUsageDetailTestRouter(repo)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/usage/42", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    dto.AdminUsageLog `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "success", body.Message)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, int64(42), repo.gotID)
	require.Equal(t, "req-admin-detail", body.Data.RequestID)
	require.NotNil(t, body.Data.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *body.Data.InboundEndpoint)
	require.NotNil(t, body.Data.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *body.Data.UpstreamEndpoint)
	require.NotNil(t, body.Data.UpstreamModel)
	require.Equal(t, upstreamModel, *body.Data.UpstreamModel)
	require.NotNil(t, body.Data.ChannelID)
	require.Equal(t, channelID, *body.Data.ChannelID)
	require.NotNil(t, body.Data.BillingTier)
	require.Equal(t, billingTier, *body.Data.BillingTier)
	require.NotNil(t, body.Data.AccountRateMultiplier)
	require.Equal(t, accountRateMultiplier, *body.Data.AccountRateMultiplier)
	require.Equal(t, &dto.AccountSummary{ID: 9, Name: "admin-visible-account"}, body.Data.Account)
}

func TestAdminUsageGetByIDRejectsInvalidID(t *testing.T) {
	repo := &adminUsageDetailRepo{}
	router := newAdminUsageDetailTestRouter(repo)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/usage/not-an-id", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, repo.calls)
}

func TestAdminUsageGetByIDRejectsNonPositiveID(t *testing.T) {
	for _, id := range []string{"0", "-1"} {
		t.Run(id, func(t *testing.T) {
			repo := &adminUsageDetailRepo{}
			router := newAdminUsageDetailTestRouter(repo)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/admin/usage/"+id, nil)
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, repo.calls)
		})
	}
}

func TestAdminUsageGetByIDPreservesNotFound(t *testing.T) {
	repo := &adminUsageDetailRepo{err: service.ErrUsageLogNotFound}
	router := newAdminUsageDetailTestRouter(repo)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/usage/404", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, int64(404), repo.gotID)
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, http.StatusNotFound, body.Code)
	require.Equal(t, "USAGE_LOG_NOT_FOUND", body.Reason)
}

func TestAdminUsageGetUpstreamCostReturnsComparison(t *testing.T) {
	repo := &adminUsageDetailRepo{record: &service.UsageLog{
		ID: 42, RequestID: "local-req", ActualCost: 0.00688,
		Account: &service.Account{Credentials: map[string]any{}},
	}}
	usageService := service.NewUsageService(repo, nil, nil, nil)
	upstreamCostService := service.NewSubUpstreamCostService(usageService)
	handler := NewUsageHandler(usageService, nil, nil, nil, upstreamCostService)
	router := gin.New()
	router.GET("/admin/usage/:id/upstream-cost", handler.GetUpstreamCost)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/usage/42/upstream-cost", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Code int                           `json:"code"`
		Data service.SubUpstreamCostDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, int64(42), body.Data.UsageID)
	require.Equal(t, "unavailable", body.Data.Status)
}

func TestAdminUsageGetUpstreamCostReturnsConfirmedZeroAndProfit(t *testing.T) {
	upstreamID := "upstream-blank-cost"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
			"request_id":          "local-blank-cost",
			"upstream_request_id": upstreamID,
			"actual_cost":         "",
		}}})
	}))
	defer upstream.Close()

	repo := &adminUsageDetailRepo{record: &service.UsageLog{
		ID: 42, RequestID: "local-blank-cost", UpstreamRequestID: &upstreamID,
		ActualCost: 0.00688, CreatedAt: time.Now(),
		Account: &service.Account{Credentials: map[string]any{
			"base_url": upstream.URL,
			"api_key":  "stored-upstream-key",
		}},
	}}
	usageService := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageService, nil, nil, nil, service.NewSubUpstreamCostService(usageService))
	router := gin.New()
	router.GET("/admin/usage/:id/upstream-cost", handler.GetUpstreamCost)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/usage/42/upstream-cost", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Code int                           `json:"code"`
		Data service.SubUpstreamCostDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "confirmed", body.Data.Status)
	require.NotNil(t, body.Data.UpstreamActualCost)
	require.Zero(t, *body.Data.UpstreamActualCost)
	require.NotNil(t, body.Data.Profit)
	require.InDelta(t, 0.00688, *body.Data.Profit, 1e-9)
}

func TestAdminUsageGetUpstreamCostRejectsInvalidID(t *testing.T) {
	handler := NewUsageHandler(nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/usage/:id/upstream-cost", handler.GetUpstreamCost)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/usage/not-an-id/upstream-cost", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
