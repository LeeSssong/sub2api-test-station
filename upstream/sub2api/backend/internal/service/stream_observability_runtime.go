package service

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const streamObservationKey = "openai_stream_observation"

func BeginStreamObservation(c *gin.Context, model, mappedModel, platform string, account *Account) *StreamObservation {
	if c == nil {
		return nil
	}
	input := StreamObservationInput{Model: model, MappedModel: mappedModel, Platform: platform}
	input.Environment = strings.TrimSpace(os.Getenv("SUB2API_ENVIRONMENT"))
	input.DeploymentCommit = strings.TrimSpace(os.Getenv("SUB2API_DEPLOYMENT_COMMIT"))
	input.ContainerSlot = strings.TrimSpace(os.Getenv("SUB2API_CONTAINER_SLOT"))
	input.ContainerID = strings.TrimSpace(os.Getenv("SUB2API_CONTAINER_ID"))
	if c.Request != nil {
		ctx := c.Request.Context()
		input.RequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		input.ClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		input.ThreadID, _ = ctx.Value(ctxkey.ThreadID).(string)
		input.WindowID, _ = ctx.Value(ctxkey.WindowID).(string)
		input.SessionID, _ = ctx.Value(ctxkey.SessionID).(string)
		input.LogicalRequestID, _ = ctx.Value(ctxkey.LogicalRequestID).(string)
	}
	if account != nil {
		input.AccountID = account.ID
		input.AccountName = account.Name
	}
	obs := NewStreamObservation(input)
	c.Set(streamObservationKey, obs)
	emitStreamObservation(c, obs)
	return obs
}

func StreamObservationFromContext(c *gin.Context) *StreamObservation {
	if c == nil {
		return nil
	}
	value, _ := c.Get(streamObservationKey)
	obs, _ := value.(*StreamObservation)
	return obs
}

func ObserveUpstreamHeaders(c *gin.Context, resp *http.Response) {
	obs := StreamObservationFromContext(c)
	if obs == nil || resp == nil {
		return
	}
	obs.RecordHeaders(StreamHeaders{
		HTTPStatus:       resp.StatusCode,
		ContentType:      resp.Header.Get("Content-Type"),
		ContentEncoding:  resp.Header.Get("Content-Encoding"),
		TransferEncoding: strings.Join(resp.TransferEncoding, ","),
		Protocol:         resp.Proto,
	})
	emitStreamObservation(c, obs)
}

func ObserveSSEEvent(c *gin.Context, eventType string, index int, bytesRead int64) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	obs.RecordEvent(eventType, index, bytesRead)
	if index == 1 {
		emitStreamObservation(c, obs)
	}
}

func ObserveVisibleOutput(c *gin.Context, bytesForwarded int64) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	snapshot := obs.Snapshot()
	if snapshot.SemanticOutputSeen {
		return
	}
	obs.RecordVisibleOutput(bytesForwarded)
	emitStreamObservation(c, obs)
}

func ObserveTerminal(c *gin.Context, eventType, responseID string, inputTokens, outputTokens, totalTokens int, bytesForwarded int64) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	obs.RecordTerminal(eventType, responseID, inputTokens, outputTokens, totalTokens, bytesForwarded)
	emitStreamObservation(c, obs)
}

func ObserveReadFailure(c *gin.Context, stage StreamFailureStage, err error) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	obs.RecordFailure(stage, err, false)
	emitStreamObservation(c, obs)
}

func ObserveClientWriteFailure(c *gin.Context, err error) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	obs.RecordFailure(StreamFailureStageClientWrite, err, true)
	emitStreamObservation(c, obs)
}

func FinishStreamObservation(c *gin.Context, failed bool) {
	obs := StreamObservationFromContext(c)
	if obs == nil {
		return
	}
	snapshot := obs.Snapshot()
	if !failed && snapshot.FailureStage == "" {
		obs.mu.Lock()
		obs.snapshot.Stage = StreamLifecycleStageCompleted
		obs.mu.Unlock()
	}
	emitStreamObservation(c, obs)
}

func emitStreamObservation(c *gin.Context, obs *StreamObservation) {
	if c == nil || obs == nil {
		return
	}
	snapshot := obs.Snapshot()
	fields := []zap.Field{
		zap.String("event", snapshot.Event), zap.String("stage", string(snapshot.Stage)),
		zap.String("request_id", snapshot.RequestID), zap.String("logical_request_id", snapshot.LogicalRequestID),
		zap.String("attempt_id", snapshot.AttemptID), zap.String("client_request_id", snapshot.ClientRequestID),
		zap.String("thread_id", snapshot.ThreadID), zap.String("window_id", snapshot.WindowID), zap.String("session_id", snapshot.SessionID),
		zap.String("error_class", string(snapshot.ErrorClass)), zap.String("failure_stage", string(snapshot.FailureStage)),
		zap.String("last_event_type", snapshot.LastEventType), zap.Int("event_index", snapshot.EventIndex),
		zap.Bool("semantic_output_seen", snapshot.SemanticOutputSeen), zap.Bool("usage_known", snapshot.UsageKnown),
		zap.Bool("client_disconnected", snapshot.ClientDisconnected), zap.Int64("bytes_read", snapshot.BytesRead),
		zap.Int64("response_bytes_forwarded", snapshot.ResponseBytesForwarded), zap.Int64("elapsed_ms", snapshot.ElapsedMS),
		zap.Bool("correlation_degraded", snapshot.CorrelationDegraded),
	}
	if snapshot.AccountID > 0 {
		fields = append(fields, zap.Int64("account_id", snapshot.AccountID))
	}
	if snapshot.Model != "" {
		fields = append(fields, zap.String("model", snapshot.Model))
	}
	if snapshot.MappedModel != "" {
		fields = append(fields, zap.String("mapped_model", snapshot.MappedModel))
	}
	if snapshot.ResponseID != "" {
		fields = append(fields, zap.String("response_id", snapshot.ResponseID))
	}
	log := logger.FromContext(contextOrBackground(c)).With(fields...)
	log.Info("openai.stream.lifecycle")
}

func contextOrBackground(c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	return context.Background()
}
