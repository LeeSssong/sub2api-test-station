package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpsSchedulerExperienceTestRouter(handler *OpsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/scheduler-experience", handler.GetOpenAISchedulerExperience)
	return router
}

func TestOpsSchedulerExperienceHandlerReturnsFilteredMetrics(t *testing.T) {
	start := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	groupID := int64(987650)
	service.RecordOpenAIResilienceOutcome(service.OpenAIResilienceEvent{
		At: start.Add(time.Minute), Platform: service.PlatformOpenAI, GroupID: &groupID,
		CorrelationID: "handler-sample", Name: service.OpenAIEventSchedulerSelection,
		EligibleCount: 2, EffectiveTopK: 1,
	})
	service.RecordOpenAIResilienceOutcome(service.OpenAIResilienceEvent{
		At: start.Add(2 * time.Minute), Platform: service.PlatformOpenAI, GroupID: &groupID,
		CorrelationID: "handler-sample", Name: service.OpenAIEventSchedulerRequestOutcome,
		FinalOutcome: "success",
	})

	handler := NewOpsHandler(service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	router := newOpsSchedulerExperienceTestRouter(handler)
	url := fmt.Sprintf(
		"/scheduler-experience?start_time=%s&end_time=%s&platform=%s&group_id=%d",
		start.Format(time.RFC3339), end.Format(time.RFC3339), service.PlatformOpenAI, groupID,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			SampleSize int64 `json:"sample_size"`
			Metrics    struct {
				TopKFilteredRate service.OpsSchedulerRateMetric `json:"top_k_filtered_rate"`
			} `json:"metrics"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(1), envelope.Data.SampleSize)
	require.Equal(t, int64(1), envelope.Data.Metrics.TopKFilteredRate.Numerator)
	require.Equal(t, int64(2), envelope.Data.Metrics.TopKFilteredRate.Denominator)
}

func TestOpsSchedulerExperienceHandlerRejectsInvalidFilters(t *testing.T) {
	handler := NewOpsHandler(service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	router := newOpsSchedulerExperienceTestRouter(handler)

	tests := []string{
		"/scheduler-experience?group_id=bad",
		"/scheduler-experience?group_id=0",
		"/scheduler-experience?start_time=2026-08-17T10:00:00Z&end_time=2026-08-17T09:00:00Z",
	}
	for _, url := range tests {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusBadRequest, recorder.Code, url)
	}
}

func TestOpsSchedulerExperienceHandlerUnavailableAndDisabled(t *testing.T) {
	t.Run("service unavailable", func(t *testing.T) {
		router := newOpsSchedulerExperienceTestRouter(NewOpsHandler(nil))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scheduler-experience", nil))
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	})

	t.Run("monitoring disabled", func(t *testing.T) {
		svc := service.NewOpsService(nil, nil, &config.Config{Ops: config.OpsConfig{Enabled: false}}, nil, nil, nil, nil, nil, nil, nil, nil)
		router := newOpsSchedulerExperienceTestRouter(NewOpsHandler(svc))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/scheduler-experience", nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
}
