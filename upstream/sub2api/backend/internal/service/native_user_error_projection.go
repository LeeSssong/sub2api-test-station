package service

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

type NativeUserErrorInput struct {
	Status          int
	Type            string
	Code            string
	Message         string
	Stage           string
	Ownership       string
	AccountSelected bool
}

type NativeUserErrorProjection struct {
	Type    string
	Code    string
	Message string
}

var nativeUserErrorSensitivePattern = regexp.MustCompile(`(?i)(?:https?://|cloudflare|ray\s*id|request[_ -]?id|req_[a-z0-9_-]+)`)

func ProjectNativeUserError(input NativeUserErrorInput) NativeUserErrorProjection {
	errType := strings.TrimSpace(input.Type)
	if errType == "" {
		errType = "api_error"
	}
	result := NativeUserErrorProjection{Type: errType, Code: strings.TrimSpace(input.Code)}
	text := strings.ToLower(strings.Join([]string{input.Type, input.Code, input.Message, input.Stage, input.Ownership}, " "))

	switch {
	case input.Status == http.StatusRequestEntityTooLarge || containsAnyNativeErrorMarker(text, "body too large", "request too large", "context length", "context window", "max bytes"):
		result.Message = "请求内容过大，请缩短内容后重试。"
	case input.Status == 499 || containsAnyNativeErrorMarker(text, "client closed", "client disconnected", "upload interrupted", "broken pipe"):
		result.Message = "请求上传中断，请检查网络后重试。"
	case input.Status == http.StatusPaymentRequired || containsAnyNativeErrorMarker(text, "payment required", "payment_required", "billing required"):
		result.Message = "余额或额度不足，请充值或检查额度后重试。"
	case input.Status == http.StatusInsufficientStorage || containsAnyNativeErrorMarker(text, "insufficient storage", "storage exhausted", "resource exhausted"):
		result.Message = "服务资源暂时不足，请稍后重试。"
	case input.Status == 521 || containsAnyNativeErrorMarker(text, "web server is down", "connection refused"):
		result.Message = "上游服务暂时不可用，请稍后重试。"
	case input.Status == 522 || containsAnyNativeErrorMarker(text, "connection timed out", "connection timeout", "origin connection timed out"):
		result.Message = "连接上游服务超时，请稍后重试。"
	case input.Status == 523 || containsAnyNativeErrorMarker(text, "origin is unreachable", "origin unreachable"):
		result.Message = "上游服务暂时不可达，请稍后重试。"
	case input.Status == 524 || containsAnyNativeErrorMarker(text, "a timeout occurred", "origin timeout", "upstream processing timeout"):
		result.Message = "上游服务处理超时，请稍后重试。"
	case input.Status == 525 || containsAnyNativeErrorMarker(text, "ssl handshake failed", "tls handshake failed"):
		result.Message = "上游安全连接失败，请稍后重试。"
	case input.Status == 520 || containsAnyNativeErrorMarker(text, "unknown web server error", "web server returned an unknown error"):
		result.Message = "服务暂时异常，请稍后重试。"
	case input.AccountSelected || containsAnyNativeErrorMarker(text, "upstream", "provider", "cloudflare", "ray id") ||
		containsAnyNativeErrorMarker(strings.ToLower(strings.TrimSpace(input.Stage)), "upstream", "network", "account_auth"):
		if containsAnyNativeErrorMarker(text, "insufficient balance", "account balance", "quota exhausted", "subscription") {
			result.Message = "服务暂时异常，请稍后重试。"
		} else if input.Status == http.StatusTooManyRequests || input.Status == 529 || containsAnyNativeErrorMarker(text, "overload", "busy", "capacity", "rate limit", "too many requests") {
			result.Message = "服务暂时繁忙，请稍后重试。"
		} else {
			result.Message = "服务暂时异常，请稍后重试。"
		}
	case containsAnyNativeErrorMarker(text, "insufficient balance", "余额不足"):
		result.Message = "余额不足，请充值后重试。"
	case containsAnyNativeErrorMarker(text, "subscription", "quota", "usage limit", "额度", "限额"):
		result.Message = "额度或订阅不可用，请检查当前套餐后重试。"
	case input.Status == http.StatusUnauthorized || containsAnyNativeErrorMarker(text, "authentication", "invalid api key", "api key required", "token expired"):
		result.Message = "认证失败，请检查 API Key 后重试。"
	case input.Status == http.StatusTooManyRequests || containsAnyNativeErrorMarker(text, "rate_limit", "rate limit", "concurrency", "pending requests", "rpm exceeded", "too many requests"):
		result.Message = "请求过于频繁，请稍后重试或降低并发。"
	case input.Type == NativeErrorClassLocalCapacity || containsAnyNativeErrorMarker(text, "local_capacity_exhausted", "no available account", "当前服务资源暂时不可用"):
		result.Message = "服务暂时繁忙，请稍后重试。"
	case input.Status == http.StatusForbidden || containsAnyNativeErrorMarker(text, "permission", "whitelist", "not allowed", "restricted", "unsupported", "not supported", "group access"):
		result.Message = "当前模型或分组不可用，请调整后重试。"
	case input.Status == http.StatusBadRequest || containsAnyNativeErrorMarker(text, "invalid_request", "invalid request", "parse request", "request body", "required field", "model is required"):
		result.Message = "请求参数或格式不正确，请检查后重试。"
	case input.Status >= 500 || containsAnyNativeErrorMarker(text, "server_error", "api_error", "service unavailable"):
		result.Message = "服务暂时异常，请稍后重试。"
	default:
		result.Message = "请求处理失败，请检查后重试。"
	}

	if !isSafeNativeUserMessage(result.Message) {
		result.Message = "服务暂时异常，请稍后重试。"
	}
	return result
}

func isSafeNativeUserMessage(message string) bool {
	if strings.TrimSpace(message) == "" || nativeUserErrorSensitivePattern.MatchString(message) {
		return false
	}
	for _, r := range message {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
