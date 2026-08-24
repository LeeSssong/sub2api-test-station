package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func nativeUserErrorTestContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return context, recorder
}

func TestGatewayErrorResponseProjectsLocalBalanceToChinese(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/messages")
	(&GatewayHandler{}).errorResponse(c, http.StatusForbidden, "billing_error", "Insufficient balance")
	require.Contains(t, recorder.Body.String(), "余额不足，请充值后重试。")
	require.NotContains(t, recorder.Body.String(), "Insufficient balance")
}

func TestGatewayErrorResponseProjectsApplication402WithoutUpstreamLeak(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/messages")
	c.Set(opsAccountIDKey, int64(23))
	(&GatewayHandler{}).errorResponse(c, http.StatusPaymentRequired, "payment_required", "payment required provider=https://secret.example")
	require.Contains(t, recorder.Body.String(), "余额或额度不足，请充值或检查额度后重试。")
	require.NotContains(t, recorder.Body.String(), "secret.example")
}

func TestResponsesStreamErrorProjectsApplication524WithoutUpstreamLeak(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/responses")
	c.Set(opsAccountIDKey, int64(23))
	(&OpenAIGatewayHandler{}).handleStreamingAwareError(c, 524, "upstream_error", "A timeout occurred request_id=req_secret", true)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), "上游服务处理超时，请稍后重试。")
	require.NotContains(t, recorder.Body.String(), "req_secret")
}

func TestOpenAIErrorResponseHidesSelectedAccountEvidence(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/chat/completions")
	c.Set(opsAccountIDKey, int64(23))
	(&OpenAIGatewayHandler{}).errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream https://provider.example Cloudflare Ray ID abc request_id=req_1")
	require.Contains(t, recorder.Body.String(), "服务暂时异常，请稍后重试。")
	for _, forbidden := range []string{"Upstream", "provider.example", "Cloudflare", "Ray", "request_id", "req_1", "上游"} {
		require.NotContains(t, recorder.Body.String(), forbidden)
	}
}

func TestResponsesStreamErrorProjectsChineseTerminalMessage(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/responses")
	c.Set(opsAccountIDKey, int64(9))
	(&OpenAIGatewayHandler{}).handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream provider unavailable", true)
	require.Contains(t, recorder.Body.String(), "event: response.failed")
	require.Contains(t, recorder.Body.String(), "服务暂时异常，请稍后重试。")
	require.NotContains(t, recorder.Body.String(), "Upstream provider unavailable")
}

func TestAnthropicStreamErrorProjectsChineseMessage(t *testing.T) {
	c, recorder := nativeUserErrorTestContext(t, "/v1/messages")
	c.Set(opsAccountIDKey, int64(9))
	(&OpenAIGatewayHandler{}).anthropicStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream provider unavailable", true)
	require.Contains(t, recorder.Body.String(), "event: error")
	require.Contains(t, recorder.Body.String(), "服务暂时异常，请稍后重试。")
	require.NotContains(t, recorder.Body.String(), "Upstream provider unavailable")
}
