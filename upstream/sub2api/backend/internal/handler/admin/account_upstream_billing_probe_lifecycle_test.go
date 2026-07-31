package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamBillingProbeLifecycleSpy struct {
	lifecycleCalls chan upstreamBillingProbeLifecycleCall
}

type upstreamBillingProbeLifecycleCall struct {
	accountID int64
	ctx       context.Context
}

func (s *upstreamBillingProbeLifecycleSpy) GetSettings(context.Context) (*service.UpstreamBillingProbeSettings, error) {
	return &service.UpstreamBillingProbeSettings{}, nil
}

func (s *upstreamBillingProbeLifecycleSpy) UpdateSettings(context.Context, *service.UpstreamBillingProbeSettings) error {
	return nil
}

func (s *upstreamBillingProbeLifecycleSpy) SetAccountEnabled(context.Context, int64, bool) error {
	return nil
}

func (s *upstreamBillingProbeLifecycleSpy) ProbeAccount(context.Context, int64) (*service.UpstreamBillingProbeSnapshot, error) {
	return nil, nil
}

func (s *upstreamBillingProbeLifecycleSpy) ProbeAccounts(context.Context, []int64) []service.UpstreamBillingProbeResult {
	return nil
}

func (s *upstreamBillingProbeLifecycleSpy) ProbeLifecycleAccount(ctx context.Context, accountID int64) (*service.UpstreamBillingProbeSnapshot, error) {
	s.lifecycleCalls <- upstreamBillingProbeLifecycleCall{accountID: accountID, ctx: ctx}
	return &service.UpstreamBillingProbeSnapshot{Status: service.UpstreamBillingProbeStatusOK}, nil
}

func awaitLifecycleProbe(t *testing.T, calls <-chan upstreamBillingProbeLifecycleCall) upstreamBillingProbeLifecycleCall {
	t.Helper()
	select {
	case ctx := <-calls:
		return ctx
	case <-time.After(time.Second):
		t.Fatal("eligible lifecycle transition did not force a billing probe")
		return upstreamBillingProbeLifecycleCall{}
	}
}

func TestAccountHandlerCreateForcesLifecycleBillingProbeForEligibleAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.createAccountResult = &service.Account{
		ID:       701,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
	}
	probe := &upstreamBillingProbeLifecycleSpy{lifecycleCalls: make(chan upstreamBillingProbeLifecycleCall, 1)}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.upstreamBillingProbe = probe
	router := gin.New()
	router.POST("/admin/accounts", handler.Create)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/accounts", bytes.NewBufferString(`{"name":"upstream","platform":"openai","type":"apikey","credentials":{"api_key":"sk-test"}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	call := awaitLifecycleProbe(t, probe.lifecycleCalls)
	require.Equal(t, int64(701), call.accountID)
	require.Equal(t, service.UpstreamBillingRateMultiplierSyncTriggerLifecycle, service.UpstreamBillingRateMultiplierSyncTriggerFromContext(call.ctx))
}

func TestAccountHandlerUpdateForcesLifecycleBillingProbeForEligibleAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.updateAccountResult = &service.Account{
		ID:       702,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
	}
	probe := &upstreamBillingProbeLifecycleSpy{lifecycleCalls: make(chan upstreamBillingProbeLifecycleCall, 1)}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.upstreamBillingProbe = probe
	router := gin.New()
	router.PUT("/admin/accounts/:id", handler.Update)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/accounts/702", bytes.NewBufferString(`{"name":"changed"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	call := awaitLifecycleProbe(t, probe.lifecycleCalls)
	require.Equal(t, int64(702), call.accountID)
	require.Equal(t, service.UpstreamBillingRateMultiplierSyncTriggerLifecycle, service.UpstreamBillingRateMultiplierSyncTriggerFromContext(call.ctx))
}

func TestAccountHandlerEnableForcesLifecycleBillingProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	probe := &upstreamBillingProbeLifecycleSpy{lifecycleCalls: make(chan upstreamBillingProbeLifecycleCall, 1)}
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.upstreamBillingProbe = probe
	router := gin.New()
	router.PUT("/admin/accounts/:id/upstream-billing-probe", handler.SetUpstreamBillingProbeEnabled)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/accounts/703/upstream-billing-probe", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	call := awaitLifecycleProbe(t, probe.lifecycleCalls)
	require.Equal(t, int64(703), call.accountID)
	require.Equal(t, service.UpstreamBillingRateMultiplierSyncTriggerLifecycle, service.UpstreamBillingRateMultiplierSyncTriggerFromContext(call.ctx))
}
