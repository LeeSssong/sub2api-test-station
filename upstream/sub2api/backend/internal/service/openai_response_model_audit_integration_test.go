package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAIResponseModelAuditRepo struct {
	UsageLogRepository
	requestID string
	model     string
	called    int
}

func (r *openAIResponseModelAuditRepo) UpdateActualResponseModelByRequestID(_ context.Context, requestID, model string) error {
	r.called++
	r.requestID = requestID
	r.model = model
	return nil
}

func TestOpenAIGatewayServiceUpdateActualResponseModelBestEffort(t *testing.T) {
	repo := &openAIResponseModelAuditRepo{}
	svc := &OpenAIGatewayService{usageLogRepo: repo}

	svc.UpdateActualResponseModel(context.Background(), &OpenAIForwardResult{
		RequestID:           "req-audit-1",
		ActualResponseModel: "gpt-5.6-terra",
	})

	require.Equal(t, 1, repo.called)
	require.Equal(t, "req-audit-1", repo.requestID)
	require.Equal(t, "gpt-5.6-terra", repo.model)
}

func TestOpenAIGatewayServiceUpdateActualResponseModelIgnoresMissingValues(t *testing.T) {
	repo := &openAIResponseModelAuditRepo{}
	svc := &OpenAIGatewayService{usageLogRepo: repo}

	svc.UpdateActualResponseModel(context.Background(), &OpenAIForwardResult{RequestID: "req-audit-1"})
	svc.UpdateActualResponseModel(context.Background(), &OpenAIForwardResult{ActualResponseModel: "gpt-5.6-terra"})

	require.Zero(t, repo.called)
}
