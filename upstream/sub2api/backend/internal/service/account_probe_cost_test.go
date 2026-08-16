package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type probeCostRepositoryStub struct {
	logs []AccountProbeCostLog
	err  error
}

func (r *probeCostRepositoryStub) Append(_ context.Context, log AccountProbeCostLog) error {
	r.logs = append(r.logs, log)
	return r.err
}

func (r *probeCostRepositoryStub) ReadWindow(context.Context, time.Time, time.Time) ([]AccountProbeCostAggregate, error) {
	return nil, nil
}

type probeAccountRepositoryStub struct {
	AccountRepository
	account *Account
}

func (r probeAccountRepositoryStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

type probeRecorderStub struct {
	inputs []ProbeRecordInput
	err    error
}

type probeModelHTTPStub struct {
	requests []*http.Request
}

func (s *probeModelHTTPStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, nil
}

func (s *probeModelHTTPStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.requests = append(s.requests, req)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")),
	}, nil
}

func newProbeModelTestService(account *Account, upstream HTTPUpstream, recorder *probeRecorderStub) *AccountTestService {
	svc := NewAccountTestService(probeAccountRepositoryStub{account: account}, nil, nil, nil, nil, upstream, &config.Config{}, nil)
	svc.SetProbeCostRecorder(recorder)
	return svc
}

func assertProbeRequestModel(t *testing.T, req *http.Request, want string) {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	req.Body = io.NopCloser(bytes.NewReader(body))
	require.Contains(t, string(body), `"model":"`+want+`"`)
}

func TestAccountProbeRecordsResolvedDefaultUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}, Extra: map[string]any{"openai_responses_supported": false}}
	upstream := &probeModelHTTPStub{}
	recorder := &probeRecorderStub{}
	svc := newProbeModelTestService(account, upstream, recorder)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/17/test", nil)

	err := svc.TestAccountConnectionWithProbeKind(c, 17, "", "", AccountTestModeDefault, ProbeKindManual)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Len(t, recorder.inputs, 1)
	assertProbeRequestModel(t, upstream.requests[0], openai.DefaultTestModel)
	require.Equal(t, openai.DefaultTestModel, recorder.inputs[0].Model)
}

func TestAccountProbeRecordsMappedUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":       "key",
		"model_mapping": map[string]any{"gpt-5": "gpt-5-upstream"},
	}, Extra: map[string]any{"openai_responses_supported": false}}
	upstream := &probeModelHTTPStub{}
	recorder := &probeRecorderStub{}
	svc := newProbeModelTestService(account, upstream, recorder)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/17/test", nil)

	err := svc.TestAccountConnectionWithProbeKind(c, 17, "gpt-5", "", AccountTestModeDefault, ProbeKindManual)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Len(t, recorder.inputs, 1)
	assertProbeRequestModel(t, upstream.requests[0], "gpt-5-upstream")
	require.Equal(t, "gpt-5-upstream", recorder.inputs[0].Model)
}

func (r *probeRecorderStub) Record(_ context.Context, input ProbeRecordInput) error {
	r.inputs = append(r.inputs, input)
	return r.err
}

func TestAccountProbeObservationRecordsOpenAIUsage(t *testing.T) {
	observer := &accountProbeUsageObserver{}
	observer.observeJSON([]byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":3}}}`))

	got := observer.observation("gpt-5", ProbeOutcomeSuccess, "")
	require.Equal(t, ProbeUsageComplete, got.Completeness)
	require.Equal(t, UsageTokens{InputTokens: 11, OutputTokens: 7, CacheReadTokens: 3}, got.Tokens)
}

func TestAccountProbeObservationRecordsNestedResponsesUsage(t *testing.T) {
	observer := &accountProbeUsageObserver{}
	observer.observeJSON([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":13,"output_tokens":5,"cached_tokens":2}}}`))

	got := observer.observation("gpt-5", ProbeOutcomeSuccess, "")
	require.Equal(t, ProbeUsageComplete, got.Completeness)
	require.Equal(t, UsageTokens{InputTokens: 13, OutputTokens: 5, CacheReadTokens: 2}, got.Tokens)
}

func TestAccountProbeCostServicePersistsUnknownCostAsNull(t *testing.T) {
	repo := &probeCostRepositoryStub{}
	svc := NewAccountProbeCostService(nil, repo)

	err := svc.Record(context.Background(), ProbeRecordInput{
		AccountID:    17,
		Kind:         ProbeKindManual,
		RunID:        "run-unknown",
		Model:        "gpt-5",
		Completeness: ProbeUsageUnknown,
		Outcome:      ProbeOutcomeFailure,
		ErrorCode:    "account_test_error",
	})
	require.NoError(t, err)
	require.Len(t, repo.logs, 1)
	require.Nil(t, repo.logs[0].AccountCost)
	require.Equal(t, ProbeUsageUnknown, repo.logs[0].UsageCompleteness)
	require.Equal(t, ProbeOutcomeFailure, repo.logs[0].ProbeOutcome)
	require.NotNil(t, repo.logs[0].ErrorCode)
	require.Equal(t, "account_test_error", *repo.logs[0].ErrorCode)
}

func TestAccountProbeRecordFailureIsFailOpen(t *testing.T) {
	repo := &probeCostRepositoryStub{err: context.DeadlineExceeded}
	svc := NewAccountProbeCostService(nil, repo)

	err := svc.Record(context.Background(), ProbeRecordInput{
		AccountID:    17,
		Kind:         ProbeKindMonitor,
		RunID:        "run-fail-open",
		Model:        "gpt-5",
		Completeness: ProbeUsageUnknown,
		Outcome:      ProbeOutcomeSuccess,
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAccountProbeManualWrapperRecordsOneClassifiedRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 17, Extra: map[string]any{"synthetic_ui_test": true}}
	svc := NewAccountTestService(probeAccountRepositoryStub{account: account}, nil, nil, nil, nil, nil, nil, nil)
	recorder := &probeRecorderStub{}
	svc.SetProbeCostRecorder(recorder)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/17/test", nil)
	err := svc.TestAccountConnectionWithProbeKind(c, 17, "gpt-5", "", AccountTestModeDefault, ProbeKindManual)

	require.NoError(t, err)
	require.Len(t, recorder.inputs, 1)
	require.Equal(t, ProbeKindManual, recorder.inputs[0].Kind)
	require.Equal(t, ProbeUsageUnknown, recorder.inputs[0].Completeness)
	require.Equal(t, ProbeOutcomeSuccess, recorder.inputs[0].Outcome)
}

func TestAccountProbeMonitorAndScheduledWrappersClassifyTheirRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 17, Extra: map[string]any{"synthetic_ui_test": true}}
	svc := NewAccountTestService(probeAccountRepositoryStub{account: account}, nil, nil, nil, nil, nil, nil, nil)
	recorder := &probeRecorderStub{}
	svc.SetProbeCostRecorder(recorder)

	monitor, err := svc.ProbeAccountConnection(context.Background(), 17, "gpt-5", "", AccountTestModeDefault)
	require.NoError(t, err)
	require.Equal(t, "success", monitor.Status)
	_, err = svc.RunTestBackgroundWithProbeKind(context.Background(), 17, "gpt-5", ProbeKindScheduled)
	require.NoError(t, err)

	require.Len(t, recorder.inputs, 2)
	require.Equal(t, ProbeKindMonitor, recorder.inputs[0].Kind)
	require.Equal(t, ProbeKindScheduled, recorder.inputs[1].Kind)
}

func TestAccountProbeAppendFailureDoesNotChangeSuccessfulTest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 17, Extra: map[string]any{"synthetic_ui_test": true}}
	svc := NewAccountTestService(probeAccountRepositoryStub{account: account}, nil, nil, nil, nil, nil, nil, nil)
	svc.SetProbeCostRecorder(&probeRecorderStub{err: context.DeadlineExceeded})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/17/test", nil)

	err := svc.TestAccountConnectionWithProbeKind(c, 17, "gpt-5", "", AccountTestModeDefault, ProbeKindManual)
	require.NoError(t, err)
}
