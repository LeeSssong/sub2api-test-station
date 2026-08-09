package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayUsageRecordsRepoStub struct {
	service.UsageLogRepository
	logs []service.UsageLog
}

func (s *gatewayUsageRecordsRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return s.logs, &pagination.PaginationResult{
		Total:    int64(len(s.logs)),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    1,
	}, nil
}

func TestGatewayHandlerUsageRecordsIncludesUpstreamRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamRequestID := "provider-req-789"
	repo := &gatewayUsageRecordsRepoStub{logs: []service.UsageLog{{
		ID:                42,
		APIKeyID:          7,
		AccountID:         11,
		RequestID:         "client:req-123",
		UpstreamRequestID: &upstreamRequestID,
		Model:             "gpt-5.1",
		ActualCost:        0.42,
		CreatedAt:         time.Date(2026, time.August, 9, 1, 2, 3, 0, time.UTC),
	}}}

	handler := &GatewayHandler{usageService: service.NewUsageService(repo, nil, nil, nil)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 7})
		c.Next()
	})
	router.GET("/v1/usage/records", handler.UsageRecords)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage/records", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	require.Equal(t, "provider-req-789", response.Data[0]["upstream_request_id"])
	require.Equal(t, 0.42, response.Data[0]["actual_cost"])
}
