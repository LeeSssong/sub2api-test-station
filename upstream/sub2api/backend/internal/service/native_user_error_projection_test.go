package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectNativeUserErrorCategories(t *testing.T) {
	const rid = "11111111-2222-4333-8444-555555555555"
	tests := []struct {
		name  string
		input NativeUserErrorInput
		want  string
	}{
		{"local balance", NativeUserErrorInput{Status: 403, Type: "billing_error", Message: "Insufficient balance"}, NativeUserCopyBalance},
		{"local balance code", NativeUserErrorInput{Status: 403, Type: "billing_error", Code: "INSUFFICIENT_BALANCE"}, NativeUserCopyBalance},
		{"local subscription", NativeUserErrorInput{Status: 403, Type: "subscription_error", Message: "No active subscription found for this group"}, NativeUserCopyQuota},
		{"authentication", NativeUserErrorInput{Status: 401, Type: "authentication_error", Message: "Invalid API key"}, NativeUserCopyAuth},
		{"selected unauthorized hides auth", NativeUserErrorInput{Status: 401, Type: "upstream_error", Message: "invalid secret token", AccountSelected: true}, NativeUserCopyAbnormal},
		{"rate", NativeUserErrorInput{Status: 429, Type: "rate_limit_error", Message: "Concurrency limit exceeded"}, NativeUserCopyRate},
		{"permission", NativeUserErrorInput{Status: 403, Type: "permission_error", Message: "model gpt-x not in whitelist"}, NativeUserCopyPermission},
		{"bad request", NativeUserErrorInput{Status: 400, Type: "invalid_request_error", Message: "Failed to parse request body"}, NativeUserCopyBadRequest},
		{"selected account bad request", NativeUserErrorInput{Status: 400, Type: "upstream_error", Message: "Invalid request parameters", Stage: "upstream", Ownership: "provider", AccountSelected: true}, NativeUserCopyBadRequest},
		{"too large", NativeUserErrorInput{Status: 413, Type: "invalid_request_error", Message: "request body too large"}, NativeUserCopyTooLarge},
		{"context window code", NativeUserErrorInput{Status: 400, Type: "invalid_request_error", Code: "context_length_exceeded", Message: "Your input exceeds the context window", AccountSelected: true}, NativeUserCopyTooLarge},
		{"selected rate limit type", NativeUserErrorInput{Type: "rate_limit_error", Code: "rate_limit_exceeded", Message: "Rate limit reached", AccountSelected: true}, NativeUserCopyBusy},
		{"selected overload code", NativeUserErrorInput{Type: "server_error", Code: "server_is_overloaded", Message: "The model is currently overloaded", AccountSelected: true}, NativeUserCopyBusy},
		{"selected account too large", NativeUserErrorInput{Status: 413, Type: "invalid_request_error", Message: "proxy limit secret=must-not-leak", Stage: "upstream", Ownership: "provider", AccountSelected: true}, NativeUserCopyTooLarge},
		{"selected payment required hides recharge", NativeUserErrorInput{Status: 402, Type: "payment_required", Message: "payment required", AccountSelected: true}, NativeUserCopyAbnormal},
		{"cloudflare unknown", NativeUserErrorInput{Status: 520, Type: "upstream_error", Message: "unknown web server returned an unknown error", AccountSelected: true}, NativeUserCopyAbnormal},
		{"cloudflare down", NativeUserErrorInput{Status: 521, Type: "upstream_error", Message: "Web server is down", AccountSelected: true}, NativeUserCopyAbnormal},
		{"client closed", NativeUserErrorInput{Status: 499, Type: "client_closed", Message: "client closed request"}, NativeUserCopyUpload},
		{"local capacity", NativeUserErrorInput{Status: 503, Type: "local_capacity_exhausted", Message: "No available accounts"}, NativeUserCopyBusy},
		{"selected account balance", NativeUserErrorInput{Status: 429, Type: "upstream_error", Message: "insufficient account balance", AccountSelected: true}, NativeUserCopyBusy},
		{"provider overload", NativeUserErrorInput{Status: 529, Type: "upstream_error", Message: "Upstream overloaded", AccountSelected: true}, NativeUserCopyBusy},
		{"provider failure", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "provider failed", AccountSelected: true}, NativeUserCopyAbnormal},
		{"stream payload too large", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "Request payload is too large", AccountSelected: true}, NativeUserCopyTooLarge},
		{"context window text", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "Your input exceeds the context window of this model", AccountSelected: true}, NativeUserCopyTooLarge},
		{"model not allowed", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Code: "upstream_error", Message: `Model "gpt-5.6-luna" is not allowed for this API key`, Model: "gpt-5.6-luna", AccountSelected: true}, nativeUserModelUnavailableCopy("gpt-5.6-luna")},
		{"model not available for group", NativeUserErrorInput{Status: http.StatusNotFound, Type: "upstream_error", Message: `Model "gpt-5.6-terra" is not available for this group`, Model: "gpt-5.6-terra", AccountSelected: true}, nativeUserModelUnavailableCopy("gpt-5.6-terra")},
		{"no channel hides distributor detail", NativeUserErrorInput{Status: http.StatusServiceUnavailable, Type: "upstream_error", Message: "No available channel for model gpt-5.6-luna under group secret-pool (request id: req_secret)", AccountSelected: true}, NativeUserCopyNoChannel},
		{"json object format", NativeUserErrorInput{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "messages must contain the word 'json' to use response_format of type 'json_object'", AccountSelected: true}, NativeUserCopyJSONFormat},
		{"unread body", NativeUserErrorInput{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "Failed to read request body"}, NativeUserCopyUpload},
		{"generic upstream fallback is busy", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "Upstream request failed", AccountSelected: true}, NativeUserCopyBusy},
		{"stream read is busy", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "stream_read_error", AccountSelected: true}, NativeUserCopyBusy},
		{"connection reset is busy", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "read tcp 10.0.0.1:1->10.0.0.2:443: read: connection reset by peer", AccountSelected: true}, NativeUserCopyBusy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.RequestID = rid
			got := ProjectNativeUserError(tt.input)
			require.Equal(t, AppendNativeUserErrorHelp(tt.want, rid), got.Message)
			require.NotEmpty(t, got.Type)
			require.Contains(t, got.Message, "Request ID: "+rid)
			require.NotContains(t, got.Message, "上游")
			require.NotContains(t, got.Message, "Upstream")
		})
	}
}

func TestProjectNativeUserErrorFastUnavailable(t *testing.T) {
	const requestID = "fast-request-id"
	inputs := []struct {
		name  string
		input NativeUserErrorInput
	}{
		{"scheduler failure", NativeUserErrorInput{Type: "fast_unavailable", Message: ErrOpenAIFastUnavailable.Error()}},
		{"code only", NativeUserErrorInput{Type: "api_error", Code: " FAST_UNAVAILABLE "}},
		{"previous generic copy", NativeUserErrorInput{Type: "fast_unavailable", Message: "服务暂时繁忙，请稍后重试。"}},
		{"selected account after failover", NativeUserErrorInput{Type: "fast_unavailable", Message: ErrOpenAIFastUnavailable.Error(), AccountSelected: true, Stage: "upstream"}},
		{"unsafe diagnostic", NativeUserErrorInput{Type: " FAST_UNAVAILABLE ", Message: "no available channel upstream https://secret.example req_secret"}},
	}
	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Status = http.StatusServiceUnavailable
			tt.input.RequestID = requestID
			got := ProjectNativeUserError(tt.input)
			require.Equal(t, "当前分组的 Fast 模式暂时异常或不支持 Fast。"+
				"如需协助请联系管理员并提供 Request ID: fast-request-id。", got.Message)
			require.Equal(t, strings.TrimSpace(tt.input.Type), got.Type)
			require.Equal(t, strings.TrimSpace(tt.input.Code), got.Code)
		})
	}
}

func TestProjectNativeUserErrorContextLimitUsesStableCode(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{
		Status:          http.StatusBadGateway,
		Type:            "upstream_error",
		Code:            "upstream_error",
		Message:         "Request payload is too large",
		AccountSelected: true,
	})
	require.Equal(t, "invalid_request_error", got.Type)
	require.Equal(t, "context_length_exceeded", got.Code)
	require.Equal(t, AppendNativeUserErrorHelp(NativeUserCopyTooLarge, ""), got.Message)
	require.NotContains(t, got.Message, "payload")
}

func TestProjectNativeUserErrorModelCopyOmitsUnsafeModel(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{
		Type:            "upstream_error",
		Message:         "Model is not allowed for this API key",
		Model:           "https://secret.example/gpt",
		AccountSelected: true,
	})
	require.Equal(t, AppendNativeUserErrorHelp("当前分组不支持这个模型，请切换模型后再试。", ""), got.Message)
	require.NotContains(t, got.Message, "secret.example")
}

func TestProjectNativeUserErrorIgnoresUpstreamEnglishMarkers(t *testing.T) {
	for _, message := range []string{
		"payment required by upstream",
		"insufficient storage capacity",
		"origin connection timed out",
		"SSL handshake failed",
	} {
		got := ProjectNativeUserError(NativeUserErrorInput{Type: "upstream_error", Message: message, AccountSelected: true})
		require.Equal(t, AppendNativeUserErrorHelp(NativeUserCopyAbnormal, ""), got.Message)
		require.NotContains(t, got.Message, "充值")
		require.NotContains(t, got.Message, "上游")
	}
}

func TestProjectNativeUserErrorNeverExposesSensitiveEvidence(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{
		Status: http.StatusBadGateway,
		Type:   "upstream_error",
		Code:   "server_error",
		Message: "Upstream https://provider.example failed; Cloudflare Ray ID: abc; " +
			"request_id=req_123 account=internal-provider",
		AccountSelected: true,
		RequestID:       "11111111-2222-4333-8444-555555555555",
	})
	for _, forbidden := range []string{"Upstream", "upstream", "provider.example", "Cloudflare", "Ray", "req_123", "internal-provider", "上游"} {
		require.NotContains(t, got.Message, forbidden)
	}
	require.Equal(t, AppendNativeUserErrorHelp(NativeUserCopyAbnormal, "11111111-2222-4333-8444-555555555555"), got.Message)
}

func TestAppendNativeUserErrorHelpIsIdempotent(t *testing.T) {
	once := AppendNativeUserErrorHelp(NativeUserCopyAbnormal, "rid-1")
	require.True(t, strings.HasSuffix(once, "。"))
	require.Equal(t, once, AppendNativeUserErrorHelp(once, "rid-2"))
	require.Equal(t, 1, strings.Count(once, nativeUserErrorAdminHelp))
	require.Contains(t, once, "Request ID: rid-1")
	require.NotContains(t, once, "rid-2")

	emptyRID := AppendNativeUserErrorHelp(NativeUserCopyAbnormal, "")
	require.Equal(t, NativeUserCopyAbnormal+nativeUserErrorAdminHelp+"。", emptyRID)
	require.Equal(t, emptyRID, AppendNativeUserErrorHelp(emptyRID, "rid-3"))
	require.NotContains(t, emptyRID, "Request ID")
	require.NotContains(t, once, nativeUserErrorAdminHelp+"。")
}

func TestProjectNativeUserErrorPreservesMachineClassification(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{Status: 429, Type: "rate_limit_error", Code: "rate_limit_exceeded", Message: "too many requests"})
	require.Equal(t, "rate_limit_error", got.Type)
	require.Equal(t, "rate_limit_exceeded", got.Code)
}

func TestProjectNativeUserErrorKeepsSafeLocalChinese(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{
		Status: 503, Type: NativeErrorClassLocalCapacity, Message: "当前服务资源暂时不可用，请稍后重试",
	})
	require.Equal(t, AppendNativeUserErrorHelp("当前服务资源暂时不可用，请稍后重试", ""), got.Message)

	got = ProjectNativeUserError(NativeUserErrorInput{
		Status: 503, Type: NativeErrorClassLocalCapacity, Message: "上游服务繁忙，请稍后重试",
	})
	require.Equal(t, AppendNativeUserErrorHelp(NativeUserCopyBusy, ""), got.Message)
	require.NotContains(t, got.Message, "上游")
}

func TestProjectNativeUserErrorTrustedCopy(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{
		Type:            "upstream_error",
		AccountSelected: true,
		TrustedCopy:     true,
		Message:         "模型今日额度已用完，请明天再试",
		RequestID:       "abc-id",
	})
	require.Equal(t, AppendNativeUserErrorHelp("模型今日额度已用完，请明天再试", "abc-id"), got.Message)

	got = ProjectNativeUserError(NativeUserErrorInput{
		Type:            "upstream_error",
		AccountSelected: true,
		TrustedCopy:     true,
		Message:         "Upstream https://secret.example failed",
		RequestID:       "abc-id",
	})
	require.Equal(t, AppendNativeUserErrorHelp(NativeUserCopyAbnormal, "abc-id"), got.Message)
	require.NotContains(t, got.Message, "secret.example")
}
