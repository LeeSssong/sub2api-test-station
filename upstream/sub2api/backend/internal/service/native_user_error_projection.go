package service

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

const (
	NativeUserCopyTooLarge   = "当前对话上下文过大，请新开一个窗口，或去掉大附件后再试。"
	NativeUserCopyUpload     = "请求上传中断，请检查网络后重试。"
	NativeUserCopyBalance    = "余额不足，请充值后重试。"
	NativeUserCopyQuota      = "额度或订阅不可用，请检查当前套餐后重试。"
	NativeUserCopyAuth       = "认证失败，请检查 API Key 后重试。"
	NativeUserCopyRate       = "请求过于频繁，请稍后重试或降低并发。"
	NativeUserCopyPermission = "当前模型或分组不可用，请调整后重试。"
	NativeUserCopyNoChannel  = "当前模型暂时没有可用线路，请稍后重试，或切换模型。"
	NativeUserCopyJSONFormat = "当前请求要求 JSON 输出，输入内容里需要包含 json 相关说明，请修改提示词后再试。"
	NativeUserCopyBadRequest = "请求参数或格式不正确，请检查后重试。"
	NativeUserCopyBusy       = "服务暂时繁忙，请稍后重试。"
	NativeUserCopyAbnormal   = "服务暂时异常，请稍后重试。"
	NativeUserCopyFailed     = "请求处理失败，请检查后重试。"
	nativeUserErrorAdminHelp = "如需协助请联系管理员"
	nativeTrustedUserCopyKey = "native_trusted_user_copy"
	nativeRequestIDHeader    = "X-Request-ID"
	nativeUserModelKey       = "ops_model"
)

const NativeUserCopyFastUnavailable = "当前分组的 Fast 模式暂时异常或不支持 Fast。"

// NativeUserErrorContactAdminSuggestion 是站内错误页的统一建议，Request ID 由详情字段单独给出。
const NativeUserErrorContactAdminSuggestion = nativeUserErrorAdminHelp + "并提供 Request ID"

type NativeUserErrorInput struct {
	Status          int
	Type            string
	Code            string
	Message         string
	Stage           string
	Ownership       string
	AccountSelected bool
	RequestID       string
	TrustedCopy     bool
	Model           string
}

type NativeUserErrorProjection struct {
	Type    string
	Code    string
	Message string
}

var nativeUserErrorLeakPattern = regexp.MustCompile(`(?i)(?:https?://|cloudflare|ray\s*id|\bupstream\b|上游|req_[a-z0-9_-]+)`)

func ProjectNativeUserError(input NativeUserErrorInput) NativeUserErrorProjection {
	result := ProjectNativeUserErrorCore(input)
	result.Message = AppendNativeUserErrorHelp(result.Message, input.RequestID)
	return result
}

func ProjectNativeUserErrorCore(input NativeUserErrorInput) NativeUserErrorProjection {
	errType := strings.TrimSpace(input.Type)
	if errType == "" {
		errType = "api_error"
	}
	result := NativeUserErrorProjection{Type: errType, Code: strings.TrimSpace(input.Code)}
	// Preserve the scheduler's Fast verdict even when an older log already
	// contains generic Chinese copy or a previous failover selected an account.
	if strings.EqualFold(errType, "fast_unavailable") || strings.EqualFold(result.Code, "fast_unavailable") {
		result.Message = NativeUserCopyFastUnavailable
		return result
	}
	copy := strings.TrimSpace(input.Message)
	upstreamFacing := nativeUserErrorUpstreamFacing(input, errType)
	if copy != "" {
		if input.TrustedCopy && isSafeTrustedUserCopy(copy) {
			result.Message = copy
			return result
		}
		if !upstreamFacing && isSafeNativeUserMessage(copy) {
			result.Message = copy
			return result
		}
	}
	result.Message = classifyNativeUserErrorCopy(input, errType, upstreamFacing)
	if !isSafeNativeUserMessage(result.Message) {
		result.Message = NativeUserCopyAbnormal
	}
	applyNativeUserContextWireIdentity(&result, input)
	return result
}

func applyNativeUserContextWireIdentity(result *NativeUserErrorProjection, input NativeUserErrorInput) {
	if result == nil || result.Message != NativeUserCopyTooLarge {
		return
	}
	if isGenericNativeErrorCode(result.Code) {
		result.Code = "context_length_exceeded"
	}
	if isGenericNativeErrorType(result.Type) {
		result.Type = "invalid_request_error"
	}
}

func isGenericNativeErrorCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "null", "upstream_error", "upstream_unavailable", "server_error", "api_error":
		return true
	default:
		return false
	}
}

func isGenericNativeErrorType(errType string) bool {
	switch strings.ToLower(strings.TrimSpace(errType)) {
	case "", "null", "upstream_error", "upstream_unavailable", "server_error", "api_error":
		return true
	default:
		return false
	}
}

func nativeUserErrorUpstreamFacing(input NativeUserErrorInput, errType string) bool {
	typeLower := strings.ToLower(strings.TrimSpace(errType))
	stage := strings.ToLower(strings.TrimSpace(input.Stage))
	return input.AccountSelected ||
		typeLower == "upstream_error" ||
		stage == "upstream" || stage == "network" || stage == "account_auth"
}

func classifyNativeUserErrorCopy(input NativeUserErrorInput, errType string, upstreamFacing bool) string {
	code := strings.ToLower(strings.TrimSpace(input.Code))
	typeLower := strings.ToLower(strings.TrimSpace(errType))

	switch {
	case isNativeUserContextTooLarge(code, input.Message, input.Status):
		return NativeUserCopyTooLarge
	case isNativeUserModelUnavailable(code, input.Message):
		return nativeUserModelUnavailableCopy(input.Model)
	case isNativeUserNoChannel(input.Message) || code == "no_available_channel":
		return NativeUserCopyNoChannel
	case isNativeUserJSONFormat(input.Message) || code == "json_format_required":
		return NativeUserCopyJSONFormat
	case strings.EqualFold(strings.TrimSpace(input.Message), "Failed to read request body"):
		return NativeUserCopyUpload
	case input.Status == 499 || typeLower == "client_closed":
		return NativeUserCopyUpload
	case !upstreamFacing && (typeLower == "billing_error" || code == "insufficient_balance" || input.Status == http.StatusPaymentRequired):
		return NativeUserCopyBalance
	case !upstreamFacing && isNativeUserQuotaIdentity(typeLower, code):
		return NativeUserCopyQuota
	case !upstreamFacing && (input.Status == http.StatusUnauthorized || typeLower == "authentication_error" ||
		code == "invalid_api_key" || code == "api_key_required" || code == "api_key_disabled" ||
		code == "user_not_found" || code == "user_inactive"):
		return NativeUserCopyAuth
	case !upstreamFacing && (input.Status == http.StatusTooManyRequests || typeLower == "rate_limit_error"):
		return NativeUserCopyRate
	case typeLower == NativeErrorClassLocalCapacity || typeLower == "compact_not_supported" ||
		(!upstreamFacing && input.Status == http.StatusServiceUnavailable):
		return NativeUserCopyBusy
	case typeLower == "permission_error" || typeLower == "cyber_policy" || typeLower == "unsupported_model" ||
		(!upstreamFacing && input.Status == http.StatusForbidden):
		return NativeUserCopyPermission
	case input.Status == http.StatusBadRequest || typeLower == "invalid_request_error":
		return NativeUserCopyBadRequest
	case upstreamFacing && (input.Status == http.StatusTooManyRequests || input.Status == 529 ||
		typeLower == "overloaded_error" || typeLower == "rate_limit_error" || code == "server_is_overloaded"):
		return NativeUserCopyBusy
	case isNativeUserTransientUpstream(input.Message):
		return NativeUserCopyBusy
	case upstreamFacing || input.Status >= 500:
		return NativeUserCopyAbnormal
	default:
		return NativeUserCopyFailed
	}
}

func nativeUserModelUnavailableCopy(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 128 || strings.ContainsAny(model, "\"\n\r") || nativeUserErrorLeakPattern.MatchString(model) {
		return "当前分组不支持这个模型，请切换模型后再试。"
	}
	return fmt.Sprintf("当前分组不支持模型 %q，请切换模型后再试。", model)
}

func nativeUserActionableUpstreamMessage(code, message string, status int) bool {
	return isNativeUserContextTooLarge(code, message, status) ||
		isNativeUserModelUnavailable(code, message) ||
		isNativeUserNoChannel(message) ||
		isNativeUserJSONFormat(message)
}

func isNativeUserContextTooLarge(code, message string, status int) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "context_length_exceeded", "context_too_large", "request_too_large":
		return true
	}
	if status == http.StatusRequestEntityTooLarge {
		return true
	}
	if isOpenAIContextWindowError(message, nil) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "payload is too large") ||
		strings.Contains(lower, "payload too large") ||
		strings.Contains(lower, "request body too large") ||
		strings.Contains(lower, "request body is too large") ||
		strings.Contains(lower, "request entity too large")
}

func isNativeUserModelUnavailable(code, message string) bool {
	if strings.EqualFold(strings.TrimSpace(code), "group_model_unavailable") {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "not allowed for this api key") ||
		strings.Contains(lower, "not available for this group") ||
		strings.Contains(lower, "not supported by any configured account")
}

func isNativeUserNoChannel(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "无可用渠道") ||
		strings.Contains(lower, "no available channel") ||
		strings.Contains(lower, "account selection timed out")
}

func isNativeUserJSONFormat(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "must contain the word 'json'") ||
		strings.Contains(lower, "must contain the word \"json\"") ||
		(strings.Contains(lower, "json_object") && strings.Contains(lower, "json"))
}

func isNativeUserTransientUpstream(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch lower {
	case "upstream request failed", "upstream service temporarily unavailable", "service temporarily unavailable":
		return true
	}
	return strings.Contains(lower, "stream_read_error") ||
		strings.Contains(lower, "stream_timeout") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "stream transport failed")
}

func isNativeUserQuotaIdentity(typeLower, code string) bool {
	switch typeLower {
	case "subscription_error":
		return true
	}
	switch code {
	case "subscription_not_found", "subscription_invalid", "api_key_expired", "api_key_quota_exhausted":
		return true
	}
	return strings.Contains(code, "usage_limit") || strings.Contains(code, "quota")
}

func AppendNativeUserErrorHelp(message, requestID string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = NativeUserCopyAbnormal
	}
	if strings.Contains(message, nativeUserErrorAdminHelp) {
		return message
	}
	if !strings.HasSuffix(message, "。") && !strings.HasSuffix(message, ".") {
		message += "。"
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return message + nativeUserErrorAdminHelp + "。"
	}
	return message + nativeUserErrorAdminHelp + "并提供 Request ID: " + requestID + "。"
}

func NativeUserErrorRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if c.Request != nil {
		if rid, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok {
			if rid = strings.TrimSpace(rid); rid != "" {
				return rid
			}
		}
	}
	if c.Writer != nil {
		return strings.TrimSpace(c.Writer.Header().Get(nativeRequestIDHeader))
	}
	return ""
}

func projectSelectedAccountUserError(c *gin.Context, status int, errType, code, message string) NativeUserErrorProjection {
	return ProjectNativeUserErrorFromGin(c, status, errType, code, message, true, "upstream", "provider")
}

func writeProjectedOpenAIUserError(c *gin.Context, status int, errType, message string) {
	writeProjectedOpenAIUserErrorClassified(c, status, status, errType, "", message)
}

func writeProjectedOpenAIUserErrorClassified(c *gin.Context, writeStatus, classifyStatus int, errType, code, message string) {
	projected := projectSelectedAccountUserError(c, classifyStatus, errType, code, message)
	c.JSON(writeStatus, gin.H{"error": gin.H{"type": projected.Type, "message": projected.Message}})
}

func writeProjectedAnthropicUserError(c *gin.Context, status int, errType, message string) {
	projected := projectSelectedAccountUserError(c, status, errType, "", message)
	c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": projected.Type, "message": projected.Message}})
}

func ProjectNativeUserErrorFromGin(c *gin.Context, status int, errType, code, message string, accountSelected bool, stage, owner string) NativeUserErrorProjection {
	input := NativeUserErrorInput{
		Status:          status,
		Type:            errType,
		Code:            code,
		Message:         message,
		Stage:           stage,
		Ownership:       owner,
		AccountSelected: accountSelected,
		RequestID:       NativeUserErrorRequestID(c),
		Model:           requestedNativeUserModel(c),
	}
	if trusted, ok := NativeTrustedUserCopy(c); ok {
		input.TrustedCopy = true
		input.Message = trusted
	}
	return ProjectNativeUserError(input)
}

func requestedNativeUserModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.GetString(nativeUserModelKey))
}

func SetNativeTrustedUserCopy(c *gin.Context, message string) {
	if c == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	c.Set(nativeTrustedUserCopyKey, message)
}

func NativeTrustedUserCopy(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	value, ok := c.Get(nativeTrustedUserCopyKey)
	if !ok {
		return "", false
	}
	message, ok := value.(string)
	if !ok {
		return "", false
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", false
	}
	return message, true
}

func isSafeTrustedUserCopy(message string) bool {
	return strings.TrimSpace(message) != "" && !nativeUserErrorLeakPattern.MatchString(message)
}

func isSafeNativeUserMessage(message string) bool {
	if !isSafeTrustedUserCopy(message) {
		return false
	}
	for _, r := range message {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
