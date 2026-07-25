package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userUsageDetailRepo struct {
	service.UsageLogRepository
	record *service.UsageLog
}

func (r *userUsageDetailRepo) GetByID(_ context.Context, _ int64) (*service.UsageLog, error) {
	return r.record, nil
}

func TestUserUsageGetByID返回安全详情摘要(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(17)
	channelID := int64(23)
	accountMultiplier := 1.5
	accountCost := 0.42
	upstreamEndpoint := "/v1/responses"
	upstreamModel := "gpt-5.4-upstream"
	repo := &userUsageDetailRepo{record: &service.UsageLog{
		ID:                    42,
		UserID:                7,
		APIKeyID:              11,
		AccountID:             13,
		GroupID:               &groupID,
		RequestID:             "req-user-detail",
		Model:                 "gpt-5.4",
		UpstreamEndpoint:      &upstreamEndpoint,
		UpstreamModel:         &upstreamModel,
		ChannelID:             &channelID,
		AccountRateMultiplier: &accountMultiplier,
		AccountStatsCost:      &accountCost,
		APIKey: &service.APIKey{
			ID:   11,
			Name: "我的密钥",
			Key:  "sk-不应返回",
		},
		Group: &service.Group{
			ID:   groupID,
			Name: "默认分组",
		},
		Account: &service.Account{
			ID:          13,
			Name:        "不应返回的上游账号",
			Credentials: map[string]any{"api_key": "上游凭据"},
		},
	}}

	usageService := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageService, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
		c.Next()
	})
	router.GET("/usage/:id", handler.GetByID)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/42", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "req-user-detail", body.Data["request_id"])
	require.Equal(t, map[string]any{"id": float64(11), "name": "我的密钥"}, body.Data["api_key"])
	require.Equal(t, map[string]any{"id": float64(17), "name": "默认分组"}, body.Data["group"])

	for _, forbidden := range []string{
		"account_id",
		"account",
		"key",
		"credentials",
		"upstream_endpoint",
		"upstream_model",
		"channel_id",
		"account_rate_multiplier",
		"account_stats_cost",
	} {
		require.Falsef(t, jsonTreeContainsKey(body.Data, forbidden), "普通用户详情不应包含字段 %q", forbidden)
	}
}

func jsonTreeContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			if childKey == key || jsonTreeContainsKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonTreeContainsKey(child, key) {
				return true
			}
		}
	}
	return false
}
