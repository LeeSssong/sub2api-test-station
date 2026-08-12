package service

import (
	"regexp"
	"strings"
)

const (
	NativeErrorClassLocalLimit         = "local_limit"
	NativeErrorClassUpstreamOverloaded = "upstream_overloaded"
	NativeErrorClassUpstreamFailed     = "upstream_failed"
	NativeErrorClassUploadInterrupted  = "upload_interrupted"
)

type NativeErrorDiagnosis struct {
	Class                   string `json:"class"`
	Code                    string `json:"code"`
	Stage                   string `json:"stage"`
	Ownership               string `json:"ownership"`
	UpstreamAccountSelected bool   `json:"upstream_account_selected"`
	SelectedAccountID       *int64 `json:"selected_account_id,omitempty"`
	SelectedAccountName     string `json:"selected_account_name,omitempty"`
	GroupID                 *int64 `json:"group_id,omitempty"`
	GroupName               string `json:"group_name,omitempty"`
	OriginalUpstreamStatus  *int   `json:"original_upstream_status,omitempty"`
	OriginalUpstreamMessage string `json:"original_upstream_message,omitempty"`
	OriginalUpstreamDetail  string `json:"original_upstream_detail,omitempty"`
	UserMeaning             string `json:"-"`
	UserSuggestion          string `json:"-"`
}

var (
	nativeErrorNamedSecretPattern  = regexp.MustCompile(`(?i)((?:authorization|x-api-key|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret)\s*[:=]\s*(?:bearer\s+)?)(?:"[^"]*"|'[^']*'|[^&\s,;"'}]+)`)
	nativeErrorBearerSecretPattern = regexp.MustCompile(`(?i)(bearer\s+)[^&\s,;"'}]+`)
	nativeErrorCookieSecretPattern = regexp.MustCompile(`(?i)((?:set-)?cookie\s*[:=]\s*)[^\r\n]+`)
	nativeErrorKeyPrefixPattern    = regexp.MustCompile(`(?i)(?:sk|key)-[^&\s,;"'}]+`)
)

func ProjectNativeErrorDiagnosis(detail *OpsErrorLogDetail) *NativeErrorDiagnosis {
	if detail == nil {
		return nil
	}

	class := classifyNativeError(detail)
	if class == "" {
		return nil
	}
	diagnosis := &NativeErrorDiagnosis{
		Class:                   class,
		Stage:                   normalizedNativeErrorStage(detail, class),
		Ownership:               normalizedNativeErrorOwner(detail, class),
		OriginalUpstreamStatus:  positiveStatus(detail.UpstreamStatusCode),
		OriginalUpstreamMessage: sanitizeNativeDiagnosticEvidence(detail.UpstreamErrorMessage, 2048),
		OriginalUpstreamDetail:  sanitizeNativeDiagnosticEvidence(detail.UpstreamErrorDetail, opsMaxStoredErrorBodyBytes),
	}
	diagnosis.Code, diagnosis.UserMeaning, diagnosis.UserSuggestion = nativeErrorExplanation(class)

	if detail.AccountID != nil && *detail.AccountID > 0 {
		diagnosis.UpstreamAccountSelected = true
		diagnosis.SelectedAccountID = detail.AccountID
		diagnosis.SelectedAccountName = strings.TrimSpace(detail.AccountName)
		diagnosis.GroupID = detail.GroupID
		diagnosis.GroupName = strings.TrimSpace(detail.GroupName)
	}
	return diagnosis
}

func AttachNativeErrorDiagnosis(detail *OpsErrorLogDetail) *OpsErrorLogDetail {
	if detail != nil {
		detail.Diagnosis = ProjectNativeErrorDiagnosis(detail)
	}
	return detail
}

func classifyNativeError(detail *OpsErrorLogDetail) string {
	accountSelected := hasSelectedNativeUpstreamAccount(detail)
	text := strings.ToLower(strings.Join([]string{
		detail.Message, detail.Type, detail.Source, detail.UpstreamErrorMessage, detail.UpstreamErrorDetail,
	}, " "))
	if !accountSelected && detail.Phase == "request" &&
		(strings.Contains(text, "failed to read request body") ||
			strings.Contains(text, "request body read") ||
			strings.Contains(text, "unexpected eof") ||
			strings.Contains(text, "upload interrupted")) {
		return NativeErrorClassUploadInterrupted
	}
	localLimitEvidence := (detail.Phase == "request" && (detail.Type == "rate_limit_error" ||
		detail.Type == "billing_error" || detail.Type == "subscription_error" || detail.Type == "cyber_policy")) ||
		isNativeLocalLimitText(text)
	if !accountSelected && localLimitEvidence {
		return NativeErrorClassLocalLimit
	}
	status := 0
	if detail.UpstreamStatusCode != nil {
		status = *detail.UpstreamStatusCode
	} else if accountSelected {
		// List queries expose COALESCE(upstream_status_code, status_code) as
		// StatusCode, while detail queries retain the original upstream field.
		status = detail.StatusCode
	}
	if accountSelected && (status == 429 || status == 529 || strings.Contains(text, "overload") ||
		strings.Contains(text, "capacity") || strings.Contains(text, "high demand") ||
		strings.Contains(text, "rate limit")) {
		return NativeErrorClassUpstreamOverloaded
	}
	if hasNativeUpstreamFailureEvidence(detail, accountSelected) {
		return NativeErrorClassUpstreamFailed
	}
	return ""
}

func isNativeLocalLimitText(text string) bool {
	for _, marker := range []string{
		"api key in query parameter is deprecated",
		"query parameter api_key is deprecated",
		"no active subscription found for this group",
		"requests-per-minute limit exceeded",
		"too many pending requests",
		"concurrency limit exceeded",
		"image generation concurrency limit exceeded",
		"usage limit exceeded",
		"daily usage limit exceeded",
		"weekly usage limit exceeded",
		"monthly usage limit exceeded",
		"usage quota exhausted for this platform",
		"quota exhausted",
		"insufficient balance",
		"insufficient account balance",
		"subscription is invalid or expired",
		"no active subscription",
		"api key 额度已用完",
		"api key 5小时限额已用完",
		"api key 日限额已用完",
		"api key 7天限额已用完",
		"api key group platform is not gemini",
		"this group is restricted to claude code clients",
		"this group does not allow /v1/messages dispatch",
		"image generation is not enabled for this group",
		"token counting is not supported for this platform",
		"images api is not supported for this platform",
		"this account only allows codex official clients",
		"openai wsv1 is temporarily unsupported",
		"openai codex passthrough requires a non-empty instructions field",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return (strings.Contains(text, "model ") && strings.Contains(text, " not in whitelist")) ||
		(strings.Contains(text, "beta feature ") && strings.Contains(text, " is not allowed")) ||
		(strings.Contains(text, "openai service_tier=") && strings.Contains(text, " is not allowed for model"))
}

func hasSelectedNativeUpstreamAccount(detail *OpsErrorLogDetail) bool {
	return detail != nil && detail.AccountID != nil && *detail.AccountID > 0
}

func hasNativeUpstreamFailureEvidence(detail *OpsErrorLogDetail, accountSelected bool) bool {
	if detail == nil {
		return false
	}
	switch strings.TrimSpace(detail.Phase) {
	case "upstream", "network", "account_auth":
		return true
	}
	return accountSelected || positiveStatus(detail.UpstreamStatusCode) != nil ||
		strings.TrimSpace(detail.UpstreamErrorMessage) != "" ||
		strings.TrimSpace(detail.UpstreamErrorDetail) != ""
}

func nativeErrorExplanation(class string) (code, meaning, suggestion string) {
	switch class {
	case NativeErrorClassLocalLimit:
		return "LOCAL_LIMIT", "请求过于频繁", "请稍后重试或降低并发"
	case NativeErrorClassUpstreamOverloaded:
		return "UPSTREAM_OVERLOADED", "上游服务繁忙", "请稍后重试"
	case NativeErrorClassUploadInterrupted:
		return "UPLOAD_INTERRUPTED", "请求上传中断", "请检查网络后重试；大上下文请保持连接稳定"
	default:
		return "UPSTREAM_FAILED", "上游请求失败", "请稍后重试；持续失败请联系管理员并提供请求 ID"
	}
}

func normalizedNativeErrorStage(detail *OpsErrorLogDetail, class string) string {
	if stage := strings.TrimSpace(detail.Phase); stage != "" {
		return stage
	}
	if class == NativeErrorClassLocalLimit || class == NativeErrorClassUploadInterrupted {
		return "request"
	}
	return "upstream"
}

func normalizedNativeErrorOwner(detail *OpsErrorLogDetail, class string) string {
	if owner := strings.TrimSpace(detail.Owner); owner != "" {
		return owner
	}
	if class == NativeErrorClassLocalLimit || class == NativeErrorClassUploadInterrupted {
		return "client"
	}
	return "provider"
}

func positiveStatus(status *int) *int {
	if status == nil || *status <= 0 {
		return nil
	}
	value := *status
	return &value
}

func sanitizeNativeDiagnosticEvidence(raw string, maxBytes int) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if sanitized, _ := sanitizeErrorBodyForStorage(value, maxBytes); sanitized != "" {
		value = sanitized
	}
	value = sanitizeUpstreamErrorMessage(value)
	value = nativeErrorCookieSecretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = nativeErrorNamedSecretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = nativeErrorBearerSecretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = nativeErrorKeyPrefixPattern.ReplaceAllString(value, "[REDACTED]")
	return truncateString(value, maxBytes)
}
