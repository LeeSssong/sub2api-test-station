package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
