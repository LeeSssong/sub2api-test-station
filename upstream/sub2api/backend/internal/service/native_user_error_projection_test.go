package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectNativeUserErrorCategories(t *testing.T) {
	tests := []struct {
		name  string
		input NativeUserErrorInput
		want  string
	}{
		{"local balance", NativeUserErrorInput{Status: 403, Type: "billing_error", Message: "Insufficient balance"}, "余额不足，请充值后重试。"},
		{"local subscription", NativeUserErrorInput{Status: 403, Type: "subscription_error", Message: "No active subscription found for this group"}, "额度或订阅不可用，请检查当前套餐后重试。"},
		{"authentication", NativeUserErrorInput{Status: 401, Type: "authentication_error", Message: "Invalid API key"}, "认证失败，请检查 API Key 后重试。"},
		{"rate", NativeUserErrorInput{Status: 429, Type: "rate_limit_error", Message: "Concurrency limit exceeded"}, "请求过于频繁，请稍后重试或降低并发。"},
		{"permission", NativeUserErrorInput{Status: 403, Type: "permission_error", Message: "model gpt-x not in whitelist"}, "当前模型或分组不可用，请调整后重试。"},
		{"bad request", NativeUserErrorInput{Status: 400, Type: "invalid_request_error", Message: "Failed to parse request body"}, "请求参数或格式不正确，请检查后重试。"},
		{"too large", NativeUserErrorInput{Status: 413, Type: "invalid_request_error", Message: "request body too large"}, "请求内容过大，请缩短内容后重试。"},
		{"local capacity", NativeUserErrorInput{Status: 503, Type: "local_capacity_exhausted", Message: "No available accounts"}, "服务暂时繁忙，请稍后重试。"},
		{"selected account balance", NativeUserErrorInput{Status: 429, Type: "upstream_error", Message: "insufficient account balance", AccountSelected: true}, "服务暂时异常，请稍后重试。"},
		{"provider overload", NativeUserErrorInput{Status: 529, Type: "upstream_error", Message: "Upstream overloaded", AccountSelected: true}, "服务暂时繁忙，请稍后重试。"},
		{"provider failure", NativeUserErrorInput{Status: http.StatusBadGateway, Type: "upstream_error", Message: "provider failed", AccountSelected: true}, "服务暂时异常，请稍后重试。"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectNativeUserError(tt.input)
			require.Equal(t, tt.want, got.Message)
			require.NotEmpty(t, got.Type)
		})
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
	})
	for _, forbidden := range []string{"Upstream", "upstream", "provider.example", "Cloudflare", "Ray", "request_id", "req_123", "internal-provider", "上游"} {
		require.NotContains(t, got.Message, forbidden)
	}
	require.Equal(t, "服务暂时异常，请稍后重试。", got.Message)
}

func TestProjectNativeUserErrorPreservesMachineClassification(t *testing.T) {
	got := ProjectNativeUserError(NativeUserErrorInput{Status: 429, Type: "rate_limit_error", Code: "rate_limit_exceeded", Message: "too many requests"})
	require.Equal(t, "rate_limit_error", got.Type)
	require.Equal(t, "rate_limit_exceeded", got.Code)
}
