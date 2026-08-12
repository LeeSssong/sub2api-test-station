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
				Message: "requests-per-minute limit exceeded",
			}, IsBusinessLimited: true},
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

func TestProjectNativeErrorDiagnosisUnknownFallsBackWithoutInventingSelection(t *testing.T) {
	got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{})
	require.Equal(t, "upstream_failed", got.Class)
	require.Equal(t, "UPSTREAM_FAILED", got.Code)
	require.Equal(t, "upstream", got.Stage)
	require.Equal(t, "provider", got.Ownership)
	require.False(t, got.UpstreamAccountSelected)
	require.Nil(t, got.SelectedAccountID)
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
	require.Equal(t, NativeErrorClassUpstreamFailed, preselection.Class)
}

func TestProjectNativeErrorDiagnosisDoesNotTreatOversizedBodyAsInterruptedUpload(t *testing.T) {
	got := ProjectNativeErrorDiagnosis(&OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "invalid_request_error", Owner: "client",
		Message: "request body too large",
	}})
	require.Equal(t, NativeErrorClassUpstreamFailed, got.Class)
}

func TestProjectNativeErrorDiagnosisHTTPAndSSEStoredRecordsShareSemantics(t *testing.T) {
	httpRecord := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "rate_limit_error", Owner: "client", Message: "concurrency limit exceeded", Stream: false,
	}, IsBusinessLimited: true}
	sseRecord := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "rate_limit_error", Owner: "client", Message: "concurrency limit exceeded", Stream: true,
	}, IsBusinessLimited: true}

	httpDiagnosis := ProjectNativeErrorDiagnosis(httpRecord)
	sseDiagnosis := ProjectNativeErrorDiagnosis(sseRecord)
	require.Equal(t, httpDiagnosis.Class, sseDiagnosis.Class)
	require.Equal(t, httpDiagnosis.Code, sseDiagnosis.Code)
	require.Equal(t, httpDiagnosis.UserMeaning, sseDiagnosis.UserMeaning)
	require.Equal(t, httpDiagnosis.UserSuggestion, sseDiagnosis.UserSuggestion)
}

func TestAttachNativeErrorDiagnosisAddsAdminProjection(t *testing.T) {
	detail := &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{
		Phase: "request", Type: "invalid_request_error", Owner: "client", Message: "Failed to read request body",
	}}

	AttachNativeErrorDiagnosis(detail)
	require.NotNil(t, detail.Diagnosis)
	require.Equal(t, "upload_interrupted", detail.Diagnosis.Class)
}
