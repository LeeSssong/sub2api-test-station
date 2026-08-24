package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectNativeErrorDiagnosisFourClasses(t *testing.T) {
	accountID := int64(17)
	groupID := int64(23)
	status429 := 429
	status503 := 503

	tests := []struct {
		name             string
		detail           OpsErrorLogDetail
		wantClass        string
		wantCode         string
		wantSelected     bool
		wantStage        string
		wantOwner        string
		wantMeaning      string
		wantSuggestion   string
		wantEvidenceCode *int
	}{
		{
			name: "local limit before selection",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "rate_limit_error", Owner: "client",
				Message:           "requests-per-minute limit exceeded",
				IsBusinessLimited: true,
			}},
			wantClass: "local_limit", wantCode: "LOCAL_LIMIT", wantStage: "request", wantOwner: "client",
			wantMeaning: "请求过于频繁", wantSuggestion: "请稍后重试或降低并发",
		},
		{
			name: "upstream overloaded after selection",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "upstream", Type: "upstream_error", Owner: "provider",
				AccountID: &accountID, AccountName: "provider-a", GroupID: &groupID, GroupName: "paid",
			}, UpstreamStatusCode: &status429, UpstreamErrorMessage: "upstream overloaded"},
			wantClass: "upstream_overloaded", wantCode: "UPSTREAM_OVERLOADED", wantSelected: true,
			wantStage: "upstream", wantOwner: "provider", wantMeaning: "上游服务繁忙", wantSuggestion: "请稍后重试",
			wantEvidenceCode: &status429,
		},
		{
			name: "generic upstream failure",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "network", Type: "upstream_error", Owner: "provider", AccountID: &accountID,
			}, UpstreamStatusCode: &status503, UpstreamErrorMessage: "connection reset"},
			wantClass: "upstream_failed", wantCode: "UPSTREAM_FAILED", wantSelected: true,
			wantStage: "network", wantOwner: "provider", wantMeaning: "上游请求失败",
			wantSuggestion: "请稍后重试；持续失败请联系管理员并提供请求 ID", wantEvidenceCode: &status503,
		},
		{
			name: "upload interrupted before selection",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "invalid_request_error", Owner: "client", Source: "client_request",
				Message: "Failed to read request body",
			}},
			wantClass: "upload_interrupted", wantCode: "UPLOAD_INTERRUPTED", wantStage: "request", wantOwner: "client",
			wantMeaning: "请求上传中断", wantSuggestion: "请检查网络后重试；大上下文请保持连接稳定",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectNativeErrorDiagnosis(&tt.detail)
			require.Equal(t, tt.wantClass, got.Class)
			require.Equal(t, tt.wantCode, got.Code)
			require.Equal(t, tt.wantSelected, got.UpstreamAccountSelected)
			require.Equal(t, tt.wantStage, got.Stage)
			require.Equal(t, tt.wantOwner, got.Ownership)
			require.Equal(t, tt.wantMeaning, got.UserMeaning)
			require.Equal(t, tt.wantSuggestion, got.UserSuggestion)
			require.Equal(t, tt.wantEvidenceCode, got.OriginalUpstreamStatus)
			if !tt.wantSelected {
				require.Nil(t, got.SelectedAccountID)
				require.Empty(t, got.SelectedAccountName)
			}
		})
	}
}

func TestProjectNativeErrorDiagnosisStatusAndClientDisconnectBoundaries(t *testing.T) {
	accountID := int64(91)
	for _, tt := range []struct {
		name, phase, owner, message string
		status                      int
		selected                    bool
		wantClass, wantMeaning      string
	}{
		{"client disconnect", "request", "client", "client closed connection", 499, false, NativeErrorClassUploadInterrupted, "请求上传中断"},
		{"payment before selection", "request", "client", "payment required", 402, false, NativeErrorClassLocalLimit, "额度或订阅不可用"},
		{"storage upstream", "upstream", "provider", "insufficient storage", 507, true, NativeErrorClassUpstreamFailed, "上游请求失败"},
		{"cloudflare timeout", "upstream", "provider", "connection timed out", 522, true, NativeErrorClassUpstreamFailed, "上游请求失败"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := (*int64)(nil)
			if tt.selected {
				id = &accountID
			}
			status := tt.status
			got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: tt.phase, Owner: tt.owner, Message: tt.message, AccountID: id,
				StatusCode: tt.status, Type: "api_error", IsBusinessLimited: !tt.selected,
			}, UpstreamStatusCode: &status})
			require.NotNil(t, got)
			require.Equal(t, tt.wantClass, got.Class)
			require.Equal(t, tt.wantMeaning, got.UserMeaning)
		})
	}
}

func TestProjectNativeErrorDiagnosisLocalCapacityExhausted(t *testing.T) {
	groupID := int64(19)
	detail := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "routing", Type: "api_error", Owner: "platform", Source: "gateway",
		StatusCode: 503, Message: "当前服务资源暂时不可用，请稍后重试",
		GroupID: &groupID, GroupName: "GPT-特惠分组", IsBusinessLimited: true,
	}}

	got := ProjectNativeErrorDiagnosis(detail)
	require.NotNil(t, got)
	require.Equal(t, "local_capacity_exhausted", got.Class)
	require.Equal(t, "LOCAL_CAPACITY_EXHAUSTED", got.Code)
	require.Equal(t, "routing", got.Stage)
	require.Equal(t, "platform", got.Ownership)
	require.False(t, got.UpstreamAccountSelected)
	require.Nil(t, got.SelectedAccountID)
	require.Nil(t, got.OriginalUpstreamStatus)
	require.Empty(t, got.OriginalUpstreamMessage)
	require.Empty(t, got.OriginalUpstreamDetail)
	require.Equal(t, "当前分组暂无可用服务资源", got.UserMeaning)
	require.Equal(t, "请稍后重试；持续失败请联系管理员并提供请求 ID", got.UserSuggestion)
}

func TestProjectNativeErrorDiagnosisRealUpstream503RemainsUpstream(t *testing.T) {
	accountID := int64(23)
	status := 503
	detail := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "upstream", Type: "upstream_error", Owner: "provider", StatusCode: status,
		AccountID: &accountID, AccountName: "provider-a", Message: "upstream request failed",
	}, UpstreamStatusCode: &status, UpstreamErrorMessage: "provider unavailable"}

	got := ProjectNativeErrorDiagnosis(detail)
	require.NotNil(t, got)
	require.Equal(t, NativeErrorClassUpstreamFailed, got.Class)
	require.Equal(t, "UPSTREAM_FAILED", got.Code)
	require.True(t, got.UpstreamAccountSelected)
	require.Equal(t, &status, got.OriginalUpstreamStatus)
}

func TestProjectNativeErrorDiagnosisSanitizesAdminEvidence(t *testing.T) {
	accountID := int64(7)
	status := 503
	detail := &OpsErrorLogDetail{
		OpsErrorLog:          OpsErrorLog{Phase: "upstream", Owner: "provider", AccountID: &accountID},
		UpstreamStatusCode:   &status,
		UpstreamErrorMessage: "Authorization: Bearer sk-secret-token provider unavailable",
		UpstreamErrorDetail:  `{"api_key":"sk-secret-token","message":"failed"}`,
	}

	got := ProjectNativeErrorDiagnosis(detail)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "sk-secret-token")
	require.Contains(t, got.OriginalUpstreamMessage, "[REDACTED]")
	require.Contains(t, got.OriginalUpstreamDetail, "[REDACTED]")
}

func TestProjectNativeErrorDiagnosisSanitizesCommonNonJSONCredentials(t *testing.T) {
	accountID := int64(8)
	detail := &OpsErrorLogDetail{
		OpsErrorLog:          OpsErrorLog{Phase: "upstream", Owner: "provider", AccountID: &accountID},
		UpstreamErrorMessage: "x-api-key: x-secret-123 access_token=oauth-secret-456 Cookie: session=private-cookie",
		UpstreamErrorDetail:  `api_key=sk-body-secret&access_token=query-secret client_secret="quoted-secret" Authorization: Bearer bearer-secret`,
	}

	got := ProjectNativeErrorDiagnosis(detail)
	combined := got.OriginalUpstreamMessage + " " + got.OriginalUpstreamDetail
	for _, secret := range []string{
		"x-secret-123", "oauth-secret-456", "private-cookie", "sk-body-secret", "query-secret", "quoted-secret", "bearer-secret",
	} {
		require.NotContains(t, combined, secret)
	}
	require.Contains(t, combined, "[REDACTED]")
	require.LessOrEqual(t, len(got.OriginalUpstreamMessage), 2048)
	require.LessOrEqual(t, len(got.OriginalUpstreamDetail), opsMaxStoredErrorBodyBytes)
}

func TestProjectNativeErrorDiagnosisNonTargetErrorsReturnNil(t *testing.T) {
	for _, detail := range []*OpsErrorLogDetail{
		{OpsErrorLog: OpsErrorLog{Phase: "auth", Type: "authentication_error", Owner: "client", Message: "Invalid API key"}},
		{OpsErrorLog: OpsErrorLog{Phase: "routing", Type: "api_error", Owner: "platform", Message: "No available accounts"}},
		{OpsErrorLog: OpsErrorLog{Phase: "request", Type: "invalid_request_error", Owner: "client", Message: "model is required"}},
		{OpsErrorLog: OpsErrorLog{Phase: "auth", Type: "invalid_request_error", Owner: "client", Message: "custom policy rejection", IsBusinessLimited: true}},
	} {
		require.Nil(t, ProjectNativeErrorDiagnosis(detail))
	}
	detail := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "auth", Type: "authentication_error", Owner: "client", Message: "Invalid API key",
	}}
	AttachNativeErrorDiagnosis(detail)
	require.Nil(t, detail.Diagnosis)
}

func TestProjectNativeErrorDiagnosisUsesEffectiveListStatusOnlyAfterSelection(t *testing.T) {
	accountID := int64(17)
	selected := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "upstream", Owner: "provider", AccountID: &accountID, StatusCode: 429,
	}})
	require.Equal(t, NativeErrorClassUpstreamOverloaded, selected.Class)

	preselection := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Owner: "client", StatusCode: 429,
	}})
	require.Nil(t, preselection)
}

func TestProjectNativeErrorDiagnosisListAndDetailShareCanonicalBusinessLimit(t *testing.T) {
	base := OpsErrorLog{
		Phase: "request", Type: "billing_error", Owner: "client",
		Message: "API key 额度已用完", IsBusinessLimited: true,
	}
	listDiagnosis := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: base})
	detailDiagnosis := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: base})

	require.Equal(t, NativeErrorClassLocalLimit, listDiagnosis.Class)
	require.Equal(t, listDiagnosis.Class, detailDiagnosis.Class)
	require.False(t, listDiagnosis.UpstreamAccountSelected)
}

func TestProjectNativeErrorDiagnosisMatchesCanonicalBusinessLimitEvidence(t *testing.T) {
	for _, message := range []string{
		"API key 额度已用完",
		"daily usage limit exceeded",
		"This group does not allow /v1/messages dispatch",
		"model gpt-private not in whitelist",
	} {
		t.Run(message, func(t *testing.T) {
			got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Owner: "client", Message: message,
			}})
			require.Equal(t, NativeErrorClassLocalLimit, got.Class)
		})
	}
}

func TestProjectNativeErrorDiagnosisDoesNotCallPreselectionCapacityUpstreamOverload(t *testing.T) {
	got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "routing", Owner: "platform", Message: "No upstream capacity available",
	}})
	require.Nil(t, got)
}

func TestProjectNativeErrorDiagnosisDoesNotTreatOversizedBodyAsInterruptedUpload(t *testing.T) {
	got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "invalid_request_error", Owner: "client",
		Message: "request body too large",
	}})
	require.Nil(t, got)
}

func TestProjectNativeErrorDiagnosisHTTPAndSSEStoredRecordsShareSemantics(t *testing.T) {
	accountID := int64(31)
	httpRecord := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "rate_limit_error", Owner: "client", Message: "concurrency limit exceeded", Stream: false,
		AccountID:         &accountID,
		IsBusinessLimited: true,
	}}
	sseRecord := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "rate_limit_error", Owner: "client", Message: "concurrency limit exceeded", Stream: true,
		AccountID:         &accountID,
		IsBusinessLimited: true,
	}}

	httpDiagnosis := ProjectNativeErrorDiagnosis(httpRecord)
	sseDiagnosis := ProjectNativeErrorDiagnosis(sseRecord)
	require.Equal(t, httpDiagnosis.Class, sseDiagnosis.Class)
	require.Equal(t, httpDiagnosis.Code, sseDiagnosis.Code)
	require.Equal(t, httpDiagnosis.UserMeaning, sseDiagnosis.UserMeaning)
	require.Equal(t, httpDiagnosis.UserSuggestion, sseDiagnosis.UserSuggestion)
	require.Equal(t, NativeErrorClassLocalLimit, httpDiagnosis.Class)
	require.True(t, httpDiagnosis.UpstreamAccountSelected)
}

func TestProjectNativeErrorDiagnosisSelectedAccountLocalQueueLimitWinsOver429(t *testing.T) {
	accountID := int64(81)
	status429 := 429
	detail := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			Phase: "request", Type: "rate_limit_error", Owner: "client",
			Message: "too many pending requests", AccountID: &accountID,
			StatusCode: 429, IsBusinessLimited: true,
		},
		UpstreamStatusCode: &status429,
	}

	got := ProjectNativeErrorDiagnosis(detail)
	require.NotNil(t, got)
	require.Equal(t, NativeErrorClassLocalLimit, got.Class)
	require.Equal(t, "请求过于频繁", got.UserMeaning)
	require.True(t, got.UpstreamAccountSelected)
}

func TestProjectNativeErrorDiagnosisLocalLimitUsesAccurateSubreasonCopy(t *testing.T) {
	tests := []struct {
		name       string
		detail     OpsErrorLogDetail
		meaning    string
		suggestion string
	}{
		{
			name: "frequency and concurrency",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "rate_limit_error", Owner: "client",
				Message: "concurrency limit exceeded", IsBusinessLimited: true,
			}},
			meaning: "请求过于频繁", suggestion: "请稍后重试或降低并发",
		},
		{
			name: "quota and subscription",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "billing_error", Owner: "client",
				Message: "insufficient balance", IsBusinessLimited: true,
			}},
			meaning: "额度或订阅不可用", suggestion: "请检查余额、额度或订阅状态",
		},
		{
			name: "policy and model whitelist",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "cyber_policy", Owner: "client",
				Message: "model gpt-private not in whitelist", IsBusinessLimited: true,
			}},
			meaning: "请求不符合当前使用规则", suggestion: "请更换可用模型或按当前分组规则调整请求",
		},
		{
			name: "deprecated api key transport rule",
			detail: OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
				Phase: "request", Type: "invalid_request_error", Owner: "client",
				Message: "API key in query parameter is deprecated", IsBusinessLimited: true,
			}},
			meaning: "请求不符合当前使用规则", suggestion: "请更换可用模型或按当前分组规则调整请求",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectNativeErrorDiagnosis(&tt.detail)
			require.NotNil(t, got)
			require.Equal(t, NativeErrorClassLocalLimit, got.Class)
			require.Equal(t, tt.meaning, got.UserMeaning)
			require.Equal(t, tt.suggestion, got.UserSuggestion)
		})
	}
}

func TestAttachNativeErrorDiagnosisSecondPassRedactsAllAdminResponseEvidence(t *testing.T) {
	accountID := int64(20)
	detail := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			Phase: "upstream", Owner: "provider", AccountID: &accountID,
			Message: `failed X-Goog-Api-Key: goog-secret apikey=query-secret`,
		},
		ErrorBody:            "Authorization: Bearer bearer-secret\nCookie: sid=cookie-secret; session=second-cookie-secret\nservice_api_key=body-secret",
		UpstreamErrorMessage: `x-provider-token: token-secret customSecret="quoted-secret"`,
		UpstreamErrorDetail:  `https://provider.test/path?apikey=url-secret&access_token=access-secret`,
		UpstreamErrors:       `[{"message":"x-extra-secret=event-secret"}]`,
	}

	got := AttachNativeErrorDiagnosis(detail)
	require.NotNil(t, got.Diagnosis)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	for _, secret := range []string{
		"goog-secret", "query-secret", "bearer-secret", "cookie-secret", "second-cookie-secret", "body-secret",
		"token-secret", "quoted-secret", "url-secret", "access-secret", "event-secret",
	} {
		require.NotContains(t, string(raw), secret)
	}
	require.Contains(t, string(raw), "[REDACTED]")
}

func TestAttachNativeErrorDiagnosisAddsAdminProjection(t *testing.T) {
	detail := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "invalid_request_error", Owner: "client", Message: "Failed to read request body",
	}}

	AttachNativeErrorDiagnosis(detail)
	require.NotNil(t, detail.Diagnosis)
	require.Equal(t, "upload_interrupted", detail.Diagnosis.Class)
}
