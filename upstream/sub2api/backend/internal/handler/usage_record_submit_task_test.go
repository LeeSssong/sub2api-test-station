package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newUsageRecordTestPool(t *testing.T) *service.UsageRecordWorkerPool {
	t.Helper()
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &GatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

type handlerUsageRecordCaptureRepo struct {
	service.UsageLogRepository
	created chan *service.UsageLog
}

func (r *handlerUsageRecordCaptureRepo) CreateBestEffortWithResult(_ context.Context, log *service.UsageLog) (service.UsageLogBestEffortResult, error) {
	copy := *log
	r.created <- &copy
	return service.UsageLogBestEffortResult{Inserted: true, UsageLogID: 701}, nil
}

type handlerUsageRecordRegistrarSpy struct {
	registered chan int64
}

func (s *handlerUsageRecordRegistrarSpy) RegisterOnce(_ context.Context, usageLogID int64) error {
	s.registered <- usageLogID
	return nil
}

type handlerUsageRecordGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *handlerUsageRecordGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

func (r *handlerUsageRecordGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

type handlerUsageRecordAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *handlerUsageRecordAccountRepo) matching(platforms []string) []service.Account {
	wanted := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		wanted[platform] = struct{}{}
	}
	result := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if _, ok := wanted[account.Platform]; ok && account.IsSchedulable() {
			result = append(result, account)
		}
	}
	return result
}

func (r *handlerUsageRecordAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.matching([]string{platform}), nil
}

func (r *handlerUsageRecordAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.matching([]string{platform}), nil
}

func (r *handlerUsageRecordAccountRepo) ListSchedulableByPlatforms(_ context.Context, platforms []string) ([]service.Account, error) {
	return r.matching(platforms), nil
}

func (r *handlerUsageRecordAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, _ int64, platforms []string) ([]service.Account, error) {
	return r.matching(platforms), nil
}

func (r *handlerUsageRecordAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			copy := account
			return &copy, nil
		}
	}
	return nil, nil
}

type handlerUsageRecordAnthropicUpstream struct {
	service.HTTPUpstream
}

func (u *handlerUsageRecordAnthropicUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_nonstream","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":12}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"handler ok"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
		``,
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"upstream-nonstream-1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

type handlerUsageRecordOpenAIUpstream struct {
	service.HTTPUpstream
}

func (u *handlerUsageRecordOpenAIUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"ok"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_handler_stream","object":"response","model":"gpt-5.2","status":"completed","output":[{"type":"message","id":"msg_handler_stream","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		``,
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func positiveSubLedgerExtra() map[string]any {
	return map[string]any{
		service.UpstreamBillingProbeExtraKey: service.UpstreamBillingProbeSnapshot{Status: service.UpstreamBillingProbeStatusOK},
	}
}

func TestUsageRecordNonStreamSuccessfulChatCompletionsHandlerCallsGatewayRecordUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(6101)
	group := &service.Group{ID: groupID, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
	account := service.Account{
		ID: 6102, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "sk-handler-test", "base_url": "https://anthropic.example.test"},
		Extra:         positiveSubLedgerExtra(),
		AccountGroups: []service.AccountGroup{{AccountID: 6102, GroupID: groupID}},
	}
	usageRepo := &handlerUsageRecordCaptureRepo{created: make(chan *service.UsageLog, 1)}
	registrar := &handlerUsageRecordRegistrarSpy{registered: make(chan int64, 1)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	accountRepo := &handlerUsageRecordAccountRepo{accounts: []service.Account{account}}
	gatewayService := service.NewGatewayService(
		accountRepo, &handlerUsageRecordGroupRepo{group: group}, usageRepo, nil, nil, nil, nil, nil, cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, nil, nil, &handlerUsageRecordAnthropicUpstream{},
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	gatewayService.SetUsageCostEvidenceRegistrar(registrar)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	pool := newUsageRecordTestPool(t)
	h := NewGatewayHandler(
		gatewayService, nil, nil, nil, nil, service.NewConcurrencyService(nil), billingCache, nil,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), pool, nil, nil, nil, cfg, nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet-4.5","messages":[{"role":"user","content":"hello"}],"stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	apiKey := &service.APIKey{ID: 6103, UserID: 6104, GroupID: &groupID, Group: group, User: &service.User{ID: 6104, Status: service.StatusActive, Balance: 100}}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 0})

	h.ChatCompletions(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "handler ok", gjson.GetBytes(recorder.Body.Bytes(), "choices.0.message.content").String())
	select {
	case log := <-usageRepo.created:
		require.Equal(t, 12, log.InputTokens)
		require.Equal(t, 7, log.OutputTokens)
		require.Equal(t, int64(6102), log.AccountID)
		require.False(t, log.Stream)
		require.NotNil(t, log.InboundEndpoint)
		require.Equal(t, "/v1/chat/completions", *log.InboundEndpoint)
	case <-time.After(time.Second):
		t.Fatal("GatewayService.RecordUsage did not reach the official usage writer")
	}
	select {
	case usageLogID := <-registrar.registered:
		require.Equal(t, int64(701), usageLogID)
	case <-time.After(time.Second):
		t.Fatal("GatewayService.RecordUsage did not register evidence after the official writer")
	}
}

func TestUsageRecordStreamSuccessfulResponsesHandlerCallsOpenAIRecordUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(6201)
	usageRepo := &handlerUsageRecordCaptureRepo{created: make(chan *service.UsageLog, 1)}
	registrar := &handlerUsageRecordRegistrarSpy{registered: make(chan int64, 1)}
	accounts := []service.Account{
		{ID: 6202, Name: "healthy", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"api_key": "sk-healthy", "base_url": "https://api.example.test"}, Extra: map[string]any{
			"openai_responses_mode":              "force_responses",
			service.UpstreamBillingProbeExtraKey: service.UpstreamBillingProbeSnapshot{Status: service.UpstreamBillingProbeStatusOK},
		}},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxAccountSwitches = 1
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gatewayService := service.NewOpenAIGatewayService(
		&openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, &handlerUsageRecordOpenAIUpstream{}, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	gatewayService.SetUsageCostEvidenceRegistrar(registrar)
	pool := newUsageRecordTestPool(t)
	h := NewOpenAIGatewayHandler(gatewayService, service.NewConcurrencyService(nil), billingCache, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), pool, nil, nil, nil, cfg)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 6203, GroupID: &groupID,
		User:  &service.User{ID: 6204, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 6204, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"type":"response.completed"`)
	select {
	case log := <-usageRepo.created:
		require.Equal(t, 1, log.InputTokens)
		require.Equal(t, 1, log.OutputTokens)
		require.Equal(t, int64(6202), log.AccountID)
		require.True(t, log.Stream)
		require.NotNil(t, log.InboundEndpoint)
		require.Equal(t, "/v1/responses", *log.InboundEndpoint)
	case <-time.After(time.Second):
		t.Fatal("OpenAIGatewayService.RecordUsage did not reach the official usage writer")
	}
	select {
	case usageLogID := <-registrar.registered:
		require.Equal(t, int64(701), usageLogID)
	case <-time.After(time.Second):
		t.Fatal("OpenAIGatewayService.RecordUsage did not register evidence after the official writer")
	}
}

func TestOpenAIGatewayHandlerSubmitMandatoryUsageRecordTask_DroppedTaskSyncFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitMandatoryUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "mandatory usage task must run synchronously when async submit is dropped")
}

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_ImageResultUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{ImageCount: 1}, func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "image usage task must be mandatory when async submit is dropped")
}

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_SearchCountUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{SearchCount: 3}, func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "search surcharge usage task must be mandatory when async submit is dropped")
}
