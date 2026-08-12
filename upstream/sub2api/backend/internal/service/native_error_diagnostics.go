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

var nativeErrorInlineSecretPattern = regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer\s+)?|bearer\s+|(?:sk|key)-)[^\s,"}]+`)

func ProjectNativeErrorDiagnosis(detail *OpsErrorLogDetail) *NativeErrorDiagnosis {
	if detail == nil {
		return nil
	}

	class := classifyNativeError(detail)
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
	text := strings.ToLower(strings.Join([]string{
		detail.Message, detail.Type, detail.Source, detail.UpstreamErrorMessage, detail.UpstreamErrorDetail,
	}, " "))
	if detail.AccountID == nil && detail.Phase == "request" &&
		(strings.Contains(text, "failed to read request body") ||
			strings.Contains(text, "request body read") ||
			strings.Contains(text, "unexpected eof") ||
			strings.Contains(text, "upload interrupted")) {
		return NativeErrorClassUploadInterrupted
	}
	if detail.AccountID == nil && (detail.IsBusinessLimited ||
		(detail.Phase == "request" && detail.Type == "rate_limit_error") ||
		isNativeLocalLimitText(text)) {
		return NativeErrorClassLocalLimit
	}
	status := 0
	if detail.UpstreamStatusCode != nil {
		status = *detail.UpstreamStatusCode
	} else if detail.AccountID != nil {
		// List queries expose COALESCE(upstream_status_code, status_code) as
		// StatusCode, while detail queries retain the original upstream field.
		status = detail.StatusCode
	}
	if status == 429 || status == 529 || strings.Contains(text, "overload") ||
		strings.Contains(text, "capacity") || strings.Contains(text, "high demand") ||
		(detail.AccountID != nil && strings.Contains(text, "rate limit")) {
		return NativeErrorClassUpstreamOverloaded
	}
	return NativeErrorClassUpstreamFailed
}

func isNativeLocalLimitText(text string) bool {
	for _, marker := range []string{
		"requests-per-minute limit exceeded",
		"too many pending requests",
		"concurrency limit exceeded",
		"usage limit exceeded",
		"quota exhausted",
		"insufficient balance",
		"subscription is invalid or expired",
		"no active subscription",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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
	value = nativeErrorInlineSecretPattern.ReplaceAllString(value, "[REDACTED]")
	return truncateString(value, maxBytes)
}
