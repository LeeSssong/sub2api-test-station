package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type transientCooldownAccountRepo struct {
	AccountRepository
}

func (transientCooldownAccountRepo) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}

func TestHandleOpenAITransientError_BlocksOnlyRequestedModel(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5105,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	firstShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"Upstream request failed","type":"upstream_error"}}`), "gpt-5.5")
	secondShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"Upstream request failed","type":"upstream_error"}}`), "gpt-5.5")

	require.False(t, firstShouldDisable)
	require.False(t, secondShouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.6-terra"))
}

func TestHandleOpenAITransientError_TransientStatusesUseModelScope(t *testing.T) {
	for _, statusCode := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 520, 521, 522, 523, 524} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
			account := &Account{
				ID:       int64(5100 + statusCode),
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			}

			firstShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, statusCode, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), "gpt-5.5")
			secondShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, statusCode, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), "gpt-5.5")

			require.False(t, firstShouldDisable)
			require.False(t, secondShouldDisable)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account), "status %d must not block the whole account", statusCode)
			require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"), "status %d should block the failing model", statusCode)
		})
	}
}

func TestHandleOpenAITransientError_529RemainsOverloadOnly(t *testing.T) {
	require.False(t, shouldCooldownOpenAITransientUpstreamError(529, []byte(`{"error":{"message":"overloaded"}}`)))
}

func TestHandleOpenAITransientError_CanonicalModelIsNotMappedTwice(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5107,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"public-alias": "upstream-a",
				"upstream-a":   "upstream-b",
			},
		},
	}
	canonicalModel := account.GetMappedModel("public-alias")
	require.Equal(t, "upstream-a", canonicalModel)

	for range 2 {
		svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), canonicalModel)
	}

	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "public-alias"))
	svc.ReportOpenAIAccountScheduleResult(account, canonicalModel, true, nil)
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "public-alias"))
}

func TestHandleOpenAITransientError_AttemptMetadataDefersFailureToHandler(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 5108, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	ctx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{
		LogicalRequestID: "request-5108", AttemptID: "request-5108:1", AttemptNumber: 1,
		AccountID: account.ID, CanonicalModel: "gpt-5.5",
	})

	shouldDisable := svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), "gpt-5.5")

	require.False(t, shouldDisable)
	require.Empty(t, svc.SnapshotOpenAIAccountModelRuntime(time.Now()), "the handler records this attempt once with replay safety")
}

func TestHandleOpenAITransientError_DoesNotBlockParameter400(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5103,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	shouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadRequest, http.Header{}, []byte(`{"error":{"message":"Invalid type for input[0].arguments"}}`), "gpt-5.5")

	require.False(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"))
}

func TestHandleOpenAITransientError_HardDisableStillBlocksWholeAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 5106, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "upstream_disable")

	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.5"))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.6-sol"))
}

func TestRecordOpenAIAccountModelFailure_CurrentRequestRetryMatrix(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                                       string
		outputStarted, safeToReplay, hasSideEffect bool
		secondFailure                              bool
		want                                       bool
	}{
		{name: "safe first failure", safeToReplay: true, want: true},
		{name: "output started", outputStarted: true, safeToReplay: true},
		{name: "unsafe replay", safeToReplay: false},
		{name: "side effect", safeToReplay: true, hasSideEffect: true},
		{name: "second failure", safeToReplay: true, secondFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
			event := OpenAIAccountModelFailureEvent{AccountID: 31, CanonicalModel: "gpt-5.5", StatusCode: http.StatusBadGateway, OutputStarted: tc.outputStarted, SafeToReplay: tc.safeToReplay, HasSideEffect: tc.hasSideEffect, Now: now}
			if tc.secondFailure {
				svc.RecordOpenAIAccountModelFailure(context.Background(), event)
			}
			decision := svc.RecordOpenAIAccountModelFailure(context.Background(), event)
			require.Equal(t, tc.want, decision.CurrentRequestRetry)
		})
	}
}

func TestRecordOpenAIAccountModelFailure_ExplicitClassification(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		status     int
		errorType  string
		wantStreak int
	}{
		{name: "processing 400 is transient", status: http.StatusBadRequest, errorType: "transient_processing", wantStreak: 1},
		{name: "zero status transient", errorType: "transient_network", wantStreak: 1},
		{name: "zero model missing hard", errorType: "transient model_not_found", wantStreak: 0},
		{name: "zero insufficient balance hard", errorType: "transient insufficient_balance", wantStreak: 0},
		{name: "zero permission hard", errorType: "transient permission_denied", wantStreak: 0},
		{name: "zero forbidden hard", errorType: "transient forbidden", wantStreak: 0},
		{name: "zero unauthorized hard", errorType: "transient unauthorized", wantStreak: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{openaiModelTransient: newOpenAIAccountModelTransientState(16)}
			decision := svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{AccountID: 31, CanonicalModel: "gpt-5.5", StatusCode: tc.status, ErrorType: tc.errorType, Now: now})
			require.Equal(t, tc.wantStreak, decision.FailureStreak)
			require.True(t, decision.ExcludeFromRequest)
		})
	}
}
