package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func qualityUnsupportedModelKey(model string) string {
	return fmt.Sprintf("quality_unsupported_model_%x", sha256.Sum256([]byte(model)))
}

const qualityUnsupportedModelRetryDelay = 5 * time.Minute

// Legacy string markers have no expiry and cannot establish current access.
// A runtime rejection only suppresses probes briefly before rediscovery.
func qualityUnsupportedModelActive(extra map[string]any, model string, now time.Time) bool {
	marker, ok := extra[qualityUnsupportedModelKey(model)].(map[string]any)
	if !ok || marker["model"] != model {
		return false
	}
	raw, _ := marker["retry_at"].(string)
	retryAt, err := time.Parse(time.RFC3339, raw)
	return err == nil && now.Before(retryAt)
}

// Discovery is shared with the account test picker. An unavailable catalog is
// an error, never evidence that the account does not support a model.
func (s *ScheduledTestService) accountQualityModels(ctx context.Context, id int64) ([]string, error) {
	account, err := s.accountTests.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	var models []string
	if account.IsOpenAI() {
		catalog, err := s.accountTests.FetchOpenAIAccountModels(ctx, account)
		if err != nil {
			return nil, err
		}
		for _, model := range catalog {
			models = append(models, model.ID)
		}
	} else if account.IsTypeSafe() {
		models = []string{typesafe.JevLatestModel}
	} else if mapping := account.GetModelMapping(); len(mapping) > 0 {
		for model := range mapping {
			models = append(models, model)
		}
	} else {
		switch account.Platform {
		case PlatformGemini:
			catalog := geminicli.DefaultModels
			if account.IsGeminiGoogleOne() {
				catalog = geminicli.GoogleOneModels
			}
			for _, model := range catalog {
				models = append(models, model.ID)
			}
		case PlatformGrok:
			for _, model := range xai.DefaultModels() {
				models = append(models, model.ID)
			}
		case PlatformAntigravity:
			for _, model := range antigravity.DefaultModels() {
				models = append(models, model.ID)
			}
		case PlatformDeepseek:
			models = defaultModelsListCandidateIDs(PlatformDeepseek)
		default:
			for _, model := range claude.DefaultModels {
				models = append(models, model.ID)
			}
		}
	}
	supported := models[:0]
	for _, model := range models {
		if !qualityUnsupportedModelActive(account.Extra, model, time.Now()) {
			supported = append(supported, model)
		}
	}
	return supported, nil
}

func qualityPlanModels(plan *ScheduledTestPlan) []string {
	if plan.PelicanConfig != nil && len(plan.PelicanConfig.ModelIDs) > 0 {
		return plan.PelicanConfig.ModelIDs
	}
	return []string{plan.ModelID}
}

func qualityCatalogSupportsModel(catalog []string, model string) bool {
	for _, pattern := range catalog {
		if pattern == model || strings.Contains(pattern, "*") && matchWildcard(pattern, model) {
			return true
		}
	}
	return false
}

func (s *ScheduledTestService) supportedQualityModels(ctx context.Context, plan *ScheduledTestPlan) ([]string, []string, error) {
	requested := qualityPlanModels(plan)
	if plan.PelicanConfig == nil || plan.PelicanConfig.Quality == nil || s.qualityModels == nil {
		return requested, nil, nil
	}
	var catalog []string
	var blocked map[string]any
	if s.accountTests != nil {
		account, err := s.accountTests.accountRepo.GetByID(ctx, plan.AccountID)
		if err != nil {
			return nil, nil, err
		}
		if account != nil {
			blocked = account.Extra
			// Native candy tests can exercise explicitly configured public model
			// names even when an upstream /models catalog omits them. This applies
			// to every account type; it does not claim upstream availability.
			// State probes and BPS retain their route-specific discovery checks.
			if isQualityAnswerQuestion(plan.PelicanConfig.QuestionKind) && plan.PelicanConfig.TestChannel != "bps" &&
				!account.IsExcelBPSEnabled() && !account.IsOpenAIPassthroughEnabled() {
				for model := range account.GetModelMapping() {
					catalog = append(catalog, model)
				}
			}
		}
	}
	for _, model := range requested {
		if !qualityCatalogSupportsModel(catalog, model) {
			discovered, err := s.qualityModels(ctx, plan.AccountID)
			if err != nil {
				return nil, nil, fmt.Errorf("无法读取账号 #%d 的模型列表：%w", plan.AccountID, err)
			}
			catalog = append(catalog, discovered...)
			break
		}
	}
	var supported, unsupported []string
	for _, model := range requested {
		found := qualityCatalogSupportsModel(catalog, model)
		if qualityUnsupportedModelActive(blocked, model, time.Now()) {
			found = false
		}
		if found {
			supported = append(supported, model)
		} else {
			unsupported = append(unsupported, model)
		}
	}
	return supported, unsupported, nil
}

func (s *ScheduledTestService) validateQualityAccountModels(ctx context.Context, plan *ScheduledTestPlan) error {
	_, unsupported, err := s.supportedQualityModels(ctx, plan)
	if err != nil {
		return err
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("账号 #%d 不支持检测模型：%s", plan.AccountID, strings.Join(unsupported, ", "))
	}
	return nil
}

// Work on a private snapshot: the group template retains the complete selection.
func (s *ScheduledTestService) filterQualityTemplatePlan(ctx context.Context, plan *ScheduledTestPlan) (bool, error) {
	supported, _, err := s.supportedQualityModels(ctx, plan)
	if err != nil {
		return false, err
	}
	if len(supported) == 0 {
		return false, nil
	}
	plan.PelicanConfig = cloneQualityPelicanConfig(plan.PelicanConfig)
	plan.PelicanConfig.ModelIDs = append([]string(nil), supported...)
	plan.ModelID = supported[0]
	return true, nil
}

// Match explicit model errors only; invalid tools, transient failures and
// authorization errors must never permanently suppress a model.
var qualityUnsupportedModelMessage = regexp.MustCompile(`(?i)\bmodel\b.{0,160}\b(?:not supported|not found|does not exist)\b`)

func qualityModelUnsupported(message string) bool {
	lower := strings.ToLower(message)
	for _, code := range []string{"model_not_found", "model_not_supported", "unsupported_model", "model_unsupported"} {
		if strings.Contains(lower, code) {
			return true
		}
	}
	// Invalid request options do not establish that a model is inaccessible.
	for _, option := range []string{"reasoning", "tool", "temperature", "top_p", "response_format", "max_output_tokens"} {
		if strings.Contains(lower, option) {
			return false
		}
	}
	if !strings.Contains(lower, "model") && !strings.Contains(lower, "模型") {
		return false
	}
	return qualityUnsupportedModelMessage.MatchString(lower) || strings.Contains(lower, "model is not supported") || strings.Contains(lower, "model not supported") ||
		strings.Contains(lower, "does not support this model") ||
		strings.Contains(lower, "not supported by any configured account") || strings.Contains(lower, "model") && strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "不支持该模型") || strings.Contains(lower, "模型不支持") || strings.Contains(lower, "模型不存在")
}

func (s *ScheduledTestService) rememberUnsupportedQualityModel(ctx context.Context, id int64, model string) error {
	if s.accountTests == nil {
		return nil
	}
	// Independent keys use the native atomic Extra merge, so concurrent rules
	// cannot overwrite each other's observations or unrelated account settings.
	return s.accountTests.accountRepo.UpdateExtra(ctx, id, map[string]any{qualityUnsupportedModelKey(model): map[string]any{
		"model": model, "retry_at": time.Now().Add(qualityUnsupportedModelRetryDelay).UTC().Format(time.RFC3339),
	}})
}
