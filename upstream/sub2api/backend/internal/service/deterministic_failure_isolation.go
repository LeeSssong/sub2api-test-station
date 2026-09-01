package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
)

type DeterministicFailureDecision struct {
	Classified     bool
	FailureClass   string
	Scope          string
	CanonicalModel string
	EvidenceCode   string
	RecoveryPolicy string
}

const (
	deterministicFailureSource   = "deterministic_failure_isolation"
	deterministicBalanceClass    = "balance_exhausted"
	deterministicCredentialClass = "credential_invalid"
	deterministicModelClass      = "model_unsupported"
	deterministicAccountScope    = "account"
	deterministicModelScope      = "account_model"
	deterministicExpiresPolicy   = "expires"
	deterministicProbePolicy     = "probe_required"
)

type deterministicFailureReason struct {
	Source         string `json:"source"`
	FailureClass   string `json:"failure_class"`
	Scope          string `json:"scope"`
	CanonicalModel string `json:"canonical_model,omitempty"`
	EvidenceCode   string `json:"evidence_code"`
	EpisodeID      string `json:"episode_id"`
	OwnerRevision  string `json:"owner_revision"`
	RecoveryPolicy string `json:"recovery_policy"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

func classifyDeterministicUpstreamFailure(account *Account, statusCode int, responseBody []byte, requestedModel string) DeterministicFailureDecision {
	if account == nil {
		return DeterministicFailureDecision{}
	}
	text := strings.ToLower(strings.TrimSpace(string(responseBody)))
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "code").String()))
	}
	if statusCode == 402 && strings.EqualFold(strings.TrimSpace(gjson.GetBytes(responseBody, "detail.code").String()), "deactivated_workspace") {
		return DeterministicFailureDecision{}
	}
	errorType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.type").String()))
	if errorType == "" {
		errorType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "type").String()))
	}
	// An explicit balance decision is valid only for a client-side upstream
	// response. Status alone is never sufficient, and 429/5xx retain their
	// existing rate-limit/transient semantics.
	if account.Platform == PlatformOpenAI && statusCode >= 400 && statusCode < 500 && statusCode != http.StatusTooManyRequests && isDeterministicBalanceEvidence(code, errorType, text) {
		return DeterministicFailureDecision{Classified: true, FailureClass: deterministicBalanceClass, Scope: deterministicAccountScope, EvidenceCode: firstNonEmptyDeterministic(code, "insufficient_balance"), RecoveryPolicy: deterministicProbePolicy}
	}
	if statusCode == 401 && (account.Type == AccountTypeAPIKey || isCredentialFailureCode(code, text)) {
		return DeterministicFailureDecision{Classified: true, FailureClass: deterministicCredentialClass, Scope: deterministicAccountScope, EvidenceCode: firstNonEmptyDeterministic(code, "unauthorized"), RecoveryPolicy: deterministicProbePolicy}
	}
	model := strings.TrimSpace(requestedModel)
	if model != "" && isDeterministicModelUnsupported(statusCode, code, text) {
		canonical := strings.TrimSpace(account.GetMappedModel(model))
		if canonical == "" {
			canonical = model
		}
		return DeterministicFailureDecision{Classified: true, FailureClass: deterministicModelClass, Scope: deterministicModelScope, CanonicalModel: canonical, EvidenceCode: firstNonEmptyDeterministic(code, "model_not_found"), RecoveryPolicy: deterministicProbePolicy}
	}
	return DeterministicFailureDecision{}
}

func isDeterministicBalanceEvidence(code, errorType, text string) bool {
	for _, token := range []string{
		"insufficient_balance", "insufficient_quota", "insufficient_user_quota",
		"quota_exhausted", "balance_exhausted", "credits_exhausted", "e44001",
	} {
		if code == token || errorType == token || strings.Contains(text, token) {
			return true
		}
	}
	for _, phrase := range []string{
		"insufficient balance", "insufficient quota", "balance exhausted", "balance is exhausted",
		"quota exhausted", "quota is exhausted", "run out of credits", "out of credits",
		"credits have run out", "余额不足", "余额耗尽", "余额已耗尽", "额度不足", "额度耗尽",
		"额度已用完", "配额已用完", "spending limit reached", "spending limit exceeded",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func isCredentialFailureCode(code, text string) bool {
	if code == "invalid_api_key" || code == "invalid_token" || code == "token_revoked" || code == "unauthorized" {
		return true
	}
	return strings.Contains(text, "invalid api key") || strings.Contains(text, "token revoked")
}

func isDeterministicModelUnsupported(statusCode int, code, text string) bool {
	if code == "model_not_found" || code == "unsupported_model" || code == "model_unsupported" || code == "model_access_denied" {
		return true
	}
	return statusCode == 404 && (strings.Contains(text, "model not found") || strings.Contains(text, "model_not_found"))
}

func buildDeterministicFailureReason(decision DeterministicFailureDecision, message string, now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	clean := sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if len(clean) > 256 {
		clean = clean[:256]
	}
	payload := deterministicFailureReason{
		Source: deterministicFailureSource, FailureClass: decision.FailureClass, Scope: decision.Scope,
		CanonicalModel: decision.CanonicalModel, EvidenceCode: decision.EvidenceCode,
		EpisodeID: now.UTC().Format("20060102T150405.000000000Z"), OwnerRevision: now.UTC().Format(time.RFC3339Nano),
		RecoveryPolicy: decision.RecoveryPolicy, ErrorMessage: clean,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return deterministicFailureSource
	}
	return string(raw)
}

func deterministicBalanceIsolationDuration(cfg *config.Config) time.Duration {
	// Balance exhaustion remains isolated until a successful billing probe. The
	// persisted timestamp is intentionally far in the future; recovery clears it
	// explicitly instead of allowing a wall-clock expiry to re-admit the account.
	return 3650 * 24 * time.Hour
}

func firstNonEmptyDeterministic(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
