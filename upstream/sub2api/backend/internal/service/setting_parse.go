package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
	}
	if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
	}

	oidcUsePKCEDefault := true
	oidcValidateIDTokenDefault := true
	if s != nil && s.cfg != nil {
		if s.cfg.OIDC.UsePKCEExplicit {
			oidcUsePKCEDefault = s.cfg.OIDC.UsePKCE
		}
		if s.cfg.OIDC.ValidateIDTokenExplicit {
			oidcValidateIDTokenDefault = s.cfg.OIDC.ValidateIDToken
		}
	}
	loginAgreementDocumentsJSON, err := marshalLoginAgreementDocuments(defaultLoginAgreementDocuments())
	if err != nil {
		return err
	}
	forwardedClientIPHeaders := []string{}
	if s != nil && s.cfg != nil {
		forwardedClientIPHeaders = s.cfg.ForwardedClientIPSettings().Headers
	}
	forwardedClientIPHeadersJSON, err := json.Marshal(forwardedClientIPHeaders)
	if err != nil {
		return fmt.Errorf("marshal default forwarded client IP headers: %w", err)
	}

	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled:                       "true",
		SettingKeyEmailVerifyEnabled:                        "false",
		SettingKeyRegistrationEmailSuffixWhitelist:          "[]",
		SettingKeyRegistrationEmailDomainQuotaEnabled:       "false",
		SettingKeyPromoCodeEnabled:                          "true", // 默认启用优惠码功能
		SettingKeyLoginAgreementEnabled:                     "false",
		SettingKeyLoginAgreementMode:                        defaultLoginAgreementMode,
		SettingKeyLoginAgreementUpdatedAt:                   defaultLoginAgreementDate,
		SettingKeyLoginAgreementDocuments:                   loginAgreementDocumentsJSON,
		SettingKeyAPIKeyACLTrustForwardedIP:                 "true",
		SettingKeyForwardedClientIPHeaders:                  string(forwardedClientIPHeadersJSON),
		settingKeyForwardedClientIPModeV2:                   "true",
		SettingKeySiteName:                                  "Sub2API",
		SettingKeySiteLogo:                                  "",
		SettingKeyPurchaseSubscriptionEnabled:               "false",
		SettingKeyPurchaseSubscriptionURL:                   "",
		SettingKeyTableDefaultPageSize:                      "20",
		SettingKeyTablePageSizeOptions:                      "[10,20,50,100]",
		SettingKeyCustomMenuItems:                           "[]",
		SettingKeyCustomEndpoints:                           "[]",
		SettingKeyWeChatConnectEnabled:                      "false",
		SettingKeyWeChatConnectAppID:                        "",
		SettingKeyWeChatConnectAppSecret:                    "",
		SettingKeyWeChatConnectOpenAppID:                    "",
		SettingKeyWeChatConnectOpenAppSecret:                "",
		SettingKeyWeChatConnectMPAppID:                      "",
		SettingKeyWeChatConnectMPAppSecret:                  "",
		SettingKeyWeChatConnectMobileAppID:                  "",
		SettingKeyWeChatConnectMobileAppSecret:              "",
		SettingKeyWeChatConnectOpenEnabled:                  "false",
		SettingKeyWeChatConnectMPEnabled:                    "false",
		SettingKeyWeChatConnectMobileEnabled:                "false",
		SettingKeyWeChatConnectMode:                         "open",
		SettingKeyWeChatConnectScopes:                       "snsapi_login",
		SettingKeyWeChatConnectRedirectURL:                  "",
		SettingKeyWeChatConnectFrontendRedirectURL:          defaultWeChatConnectFrontend,
		SettingKeyGitHubOAuthEnabled:                        "false",
		SettingKeyGitHubOAuthClientID:                       "",
		SettingKeyGitHubOAuthClientSecret:                   "",
		SettingKeyGitHubOAuthRedirectURL:                    "",
		SettingKeyGitHubOAuthFrontendRedirectURL:            defaultGitHubOAuthFrontend,
		SettingKeyGoogleOAuthEnabled:                        "false",
		SettingKeyGoogleOAuthClientID:                       "",
		SettingKeyGoogleOAuthClientSecret:                   "",
		SettingKeyGoogleOAuthRedirectURL:                    "",
		SettingKeyGoogleOAuthFrontendRedirectURL:            defaultGoogleOAuthFrontend,
		SettingKeyOIDCConnectEnabled:                        "false",
		SettingKeyOIDCConnectProviderName:                   "OIDC",
		SettingKeyOIDCConnectClientID:                       "",
		SettingKeyOIDCConnectClientSecret:                   "",
		SettingKeyOIDCConnectIssuerURL:                      "",
		SettingKeyOIDCConnectDiscoveryURL:                   "",
		SettingKeyOIDCConnectAuthorizeURL:                   "",
		SettingKeyOIDCConnectTokenURL:                       "",
		SettingKeyOIDCConnectUserInfoURL:                    "",
		SettingKeyOIDCConnectJWKSURL:                        "",
		SettingKeyOIDCConnectScopes:                         "openid email profile",
		SettingKeyOIDCConnectRedirectURL:                    "",
		SettingKeyOIDCConnectFrontendRedirectURL:            "/auth/oidc/callback",
		SettingKeyOIDCConnectTokenAuthMethod:                "client_secret_post",
		SettingKeyOIDCConnectUsePKCE:                        strconv.FormatBool(oidcUsePKCEDefault),
		SettingKeyOIDCConnectValidateIDToken:                strconv.FormatBool(oidcValidateIDTokenDefault),
		SettingKeyOIDCConnectAllowedSigningAlgs:             "RS256,ES256,PS256",
		SettingKeyOIDCConnectClockSkewSeconds:               "120",
		SettingKeyOIDCConnectRequireEmailVerified:           "false",
		SettingKeyOIDCConnectUserInfoEmailPath:              "",
		SettingKeyOIDCConnectUserInfoIDPath:                 "",
		SettingKeyOIDCConnectUserInfoUsernamePath:           "",
		SettingKeyDefaultConcurrency:                        strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:                            strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyAffiliateRebateRate:                       strconv.FormatFloat(AffiliateRebateRateDefault, 'f', 8, 64),
		SettingKeyAffiliateRebateFreezeHours:                strconv.Itoa(AffiliateRebateFreezeHoursDefault),
		SettingKeyAffiliateRebateDurationDays:               strconv.Itoa(AffiliateRebateDurationDaysDefault),
		SettingKeyAffiliateRebatePerInviteeCap:              strconv.FormatFloat(AffiliateRebatePerInviteeCapDefault, 'f', 2, 64),
		SettingKeyDefaultUserRPMLimit:                       "0",
		SettingKeyDefaultSubscriptions:                      "[]",
		SettingKeyMonitorPageRefreshIntervalSeconds:         strconv.Itoa(MonitorPageRefreshIntervalSecondsDefault),
		SettingKeyAuthSourceDefaultEmailBalance:             "0",
		SettingKeyAuthSourceDefaultEmailConcurrency:         "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions:       "[]",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup:       "false",
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind:    "false",
		SettingKeyAuthSourceDefaultLinuxDoBalance:           "0",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:       "5",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind:  "false",
		SettingKeyAuthSourceDefaultOIDCBalance:              "0",
		SettingKeyAuthSourceDefaultOIDCConcurrency:          "5",
		SettingKeyAuthSourceDefaultOIDCSubscriptions:        "[]",
		SettingKeyAuthSourceDefaultOIDCGrantOnSignup:        "false",
		SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind:     "false",
		SettingKeyAuthSourceDefaultWeChatBalance:            "0",
		SettingKeyAuthSourceDefaultWeChatConcurrency:        "5",
		SettingKeyAuthSourceDefaultWeChatSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultWeChatGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGitHubBalance:            "0",
		SettingKeyAuthSourceDefaultGitHubConcurrency:        "5",
		SettingKeyAuthSourceDefaultGitHubSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGitHubGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGoogleBalance:            "0",
		SettingKeyAuthSourceDefaultGoogleConcurrency:        "5",
		SettingKeyAuthSourceDefaultGoogleSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGoogleGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultDingTalkBalance:          "0",
		SettingKeyAuthSourceDefaultDingTalkConcurrency:      "5",
		SettingKeyAuthSourceDefaultDingTalkSubscriptions:    "[]",
		SettingKeyAuthSourceDefaultDingTalkGrantOnSignup:    "false",
		SettingKeyAuthSourceDefaultDingTalkGrantOnFirstBind: "false",
		SettingKeyForceEmailOnThirdPartySignup:              "false",
		SettingKeySMTPPort:                                  "587",
		SettingKeySMTPUseTLS:                                "false",
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch: "true",
		SettingKeyIdentityPatchPrompt: "",

		// Ops monitoring defaults (vNext)
		SettingKeyOpsMonitoringEnabled:         "true",
		SettingKeyOpsRealtimeMonitoringEnabled: "true",
		SettingKeyOpsQueryModeDefault:          "auto",
		SettingKeyOpsMetricsIntervalSeconds:    "60",

		// Channel monitor defaults (enabled, 60s)
		SettingKeyChannelMonitorEnabled:                "true",
		SettingKeyChannelMonitorMode:                   ChannelMonitorModeV1,
		SettingKeyChannelMonitorDefaultIntervalSeconds: "60",
		SettingKeyChannelMonitorHideThroughput:         "true",
		SettingKeyChannelMonitorShowQuota:              "false",

		// Grok: safe defaults — no cross-vendor model rewrite unless operators enable it.
		SettingKeyGrokDefaultTextModel:           "grok-4.6",
		SettingKeyGrokCrossClientModelMapEnabled: "true",
		SettingKeyGrokDefaultBaseURLMode:         GrokDefaultBaseURLModeCLI,

		// Available channels feature (default disabled; opt-in)
		SettingKeyAvailableChannelsEnabled: "false",

		// Model plaza feature (default disabled; opt-in, public unless require_auth)
		SettingKeyModelPlazaEnabled:       "false",
		SettingKeyModelPlazaRequireAuth:   "false",
		SettingKeyModelPlazaDescription:   "",
		SettingKeyPluginManagementEnabled: "false",

		// Affiliate (邀请返利) feature (default disabled; opt-in)
		SettingKeyAffiliateEnabled:              "false",
		SettingKeyAffiliateAdminRechargeEnabled: strconv.FormatBool(AdminRechargeRebateEnabledDefault),

		// 风控中心功能（默认关闭，显式启用）
		SettingKeyRiskControlEnabled: "false",

		// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
		SettingKeyCyberSessionBlockEnabled:    "false",
		SettingKeyCyberSessionBlockTTLSeconds: "3600",

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
		SettingKeyMaxClaudeCodeVersion: "",

		// codex_cli_only 加固（默认：版本不检查、名单空、默认种子指纹信号）
		SettingKeyMinCodexVersion:                      "",
		SettingKeyMaxCodexVersion:                      "",
		SettingKeyCodexCLIOnlyBlacklist:                "",
		SettingKeyCodexCLIOnlyWhitelist:                "",
		SettingKeyCodexCLIOnlyAllowAppServerClients:    "false",
		SettingKeyCodexCLIOnlyEngineFingerprintSignals: openai.DefaultEngineFingerprintSignalsJSON(),

		// 分组隔离（默认不允许未分组 Key 调度）
		SettingKeyAllowUngroupedKeyScheduling:                        "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled:               "false",
		SettingKeyOpenAIOAuthSchedulingRateMultiplier:                "1",
		SettingKeyEnableAnthropicCacheTTL1hInjection:                 "false",
		SettingKeyRewriteMessageCacheControl:                         strconv.FormatBool(s.defaultRewriteMessageCacheControl()),
		SettingKeyEnableClientDatelineNormalization:                  "true",
		SettingKeyAntigravityUserAgentVersion:                        "",
		SettingKeyOpenAICodexUserAgent:                               "",
		SettingKeyOpenAICodexClientVersion:                           "",
		SettingKeyOpenAICodexClientVersionSynced:                     "",
		SettingKeyOpenAICodexVersionAutoSyncEnabled:                  "true",
		SettingPaymentVisibleMethodAlipaySource:                      "",
		SettingPaymentVisibleMethodWxpaySource:                       "",
		SettingPaymentVisibleMethodAlipayEnabled:                     "false",
		SettingPaymentVisibleMethodWxpayEnabled:                      "false",
		openAIAdvancedSchedulerSettingKey:                            "false",
		SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled:       "false",
		SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled: "false",
		SettingKeyOpenAIAdvancedSchedulerLBTopK:                      "",
		SettingKeyOpenAIAdvancedSchedulerWeightPriority:              "",
		SettingKeyOpenAIAdvancedSchedulerWeightLoad:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightQueue:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:             "",
		SettingKeyOpenAIAdvancedSchedulerWeightTTFT:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightReset:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:         "",
		SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost:          "",
		SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse:      "",
		SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:         "",
		SettingKeyOpenAIAdvancedSchedulerCandidatePoolMode:           OpenAISchedulerCandidatePoolModeHybrid,
		SettingKeyOpenAIAdvancedSchedulerExplorationRatio:            "20",
		SettingKeyOpenAIAdvancedSchedulerStarvationThresholdSeconds:  "21600",
		SettingKeyOpenAIAdvancedSchedulerFairnessWeight:              "2",
		SettingKeyOpenAIAdvancedSchedulerGroupOverrides:              "{}",

		SettingKeyAllowUserViewErrorRequests: "false",
	}

	return s.settingRepo.SetMultiple(ctx, defaults)
}

func parseForwardedClientIPHeadersSetting(value string) ([]string, error) {
	var headers []string
	if err := json.Unmarshal([]byte(value), &headers); err != nil {
		return nil, fmt.Errorf("parse forwarded_client_ip_headers: %w", err)
	}
	if headers == nil {
		return nil, fmt.Errorf("parse forwarded_client_ip_headers: value must be a JSON array")
	}
	normalized, err := config.NormalizeForwardedClientIPHeaders(headers)
	if err != nil {
		return nil, fmt.Errorf("parse forwarded_client_ip_headers: %w", err)
	}
	return normalized, nil
}

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	loginAgreementDocuments := parseLoginAgreementDocuments(settings[SettingKeyLoginAgreementDocuments])
	loginAgreementUpdatedAt := strings.TrimSpace(settings[SettingKeyLoginAgreementUpdatedAt])
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = defaultLoginAgreementDate
	}
	apiKeyACLTrustForwardedIP := false
	forwardedClientIPHeaders := []string{}
	if s != nil && s.cfg != nil {
		runtimeSettings := s.cfg.ForwardedClientIPSettings()
		apiKeyACLTrustForwardedIP = runtimeSettings.TrustForwardedIP
		forwardedClientIPHeaders = runtimeSettings.Headers
	}
	if value, ok := settings[SettingKeyAPIKeyACLTrustForwardedIP]; ok {
		apiKeyACLTrustForwardedIP = value == "true"
	}
	if value, ok := settings[SettingKeyForwardedClientIPHeaders]; ok {
		parsed, err := parseForwardedClientIPHeadersSetting(value)
		if err != nil {
			slog.Error("invalid persisted forwarded client IP headers; forwarded trust disabled", "error", err)
			apiKeyACLTrustForwardedIP = false
			forwardedClientIPHeaders = []string{}
		} else {
			forwardedClientIPHeaders = parsed
		}
	}
	result := &SystemSettings{
		RegistrationEnabled:                    settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:                     emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:       ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		RegistrationEmailDomainQuotaEnabled:    settings[SettingKeyRegistrationEmailDomainQuotaEnabled] == "true",
		PromoCodeEnabled:                       settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:                   emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		FrontendURL:                            settings[SettingKeyFrontendURL],
		InvitationCodeEnabled:                  settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                            settings[SettingKeyTotpEnabled] == "true",
		PasskeyEnabled:                         s.passkeySettingEnabled(settings),
		SessionBindingEnabled:                  settings[SettingKeySessionBindingEnabled] == "true", // 默认关闭
		StepUpEnabled:                          settings[SettingKeyStepUpEnabled] == "true",         // 默认关闭
		AuditLogRetentionDays:                  parseAuditLogRetentionDays(settings[SettingKeyAuditLogRetentionDays]),
		MonitorPageRefreshIntervalSeconds:      NormalizeMonitorPageRefreshIntervalSeconds(settings[SettingKeyMonitorPageRefreshIntervalSeconds]),
		LoginAgreementEnabled:                  settings[SettingKeyLoginAgreementEnabled] == "true",
		LoginAgreementMode:                     normalizeLoginAgreementMode(settings[SettingKeyLoginAgreementMode]),
		LoginAgreementUpdatedAt:                loginAgreementUpdatedAt,
		LoginAgreementDocuments:                loginAgreementDocuments,
		SMTPHost:                               settings[SettingKeySMTPHost],
		SMTPUsername:                           settings[SettingKeySMTPUsername],
		SMTPFrom:                               settings[SettingKeySMTPFrom],
		SMTPFromName:                           settings[SettingKeySMTPFromName],
		SMTPUseTLS:                             settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:                 settings[SettingKeySMTPPassword] != "",
		TurnstileEnabled:                       settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                       settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured:           settings[SettingKeyTurnstileSecretKey] != "",
		TencentCaptchaEnabled:                  settings[SettingKeyTencentCaptchaEnabled] == "true",
		TencentCaptchaAppID:                    settings[SettingKeyTencentCaptchaAppID],
		TencentCaptchaAppSecretKeyConfigured:   settings[SettingKeyTencentCaptchaAppSecretKey] != "",
		TencentCaptchaCloudSecretIDConfigured:  settings[SettingKeyTencentCaptchaCloudSecretID] != "",
		TencentCaptchaCloudSecretKeyConfigured: settings[SettingKeyTencentCaptchaCloudSecretKey] != "",
		TencentCaptchaRegion:                   normalizeTencentCaptchaRegion(settings[SettingKeyTencentCaptchaRegion]),
		AliyunCaptchaEnabled:                   settings[SettingKeyAliyunCaptchaEnabled] == "true",
		AliyunCaptchaAccessKeyID:               settings[SettingKeyAliyunCaptchaAccessKeyID],
		AliyunCaptchaAccessKeySecretConfigured: settings[SettingKeyAliyunCaptchaAccessKeySecret] != "",
		AliyunCaptchaSceneID:                   settings[SettingKeyAliyunCaptchaSceneID],
		AliyunCaptchaPrefix:                    settings[SettingKeyAliyunCaptchaPrefix],
		AliyunCaptchaRegion:                    normalizeAliyunCaptchaRegion(settings[SettingKeyAliyunCaptchaRegion]),
		APIKeyACLTrustForwardedIP:              apiKeyACLTrustForwardedIP,
		ForwardedClientIPHeaders:               forwardedClientIPHeaders,
		SiteName:                               s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                               settings[SettingKeySiteLogo],
		SiteSubtitle:                           s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                             settings[SettingKeyAPIBaseURL],
		ContactInfo:                            settings[SettingKeyContactInfo],
		DocURL:                                 settings[SettingKeyDocURL],
		HomeContent:                            settings[SettingKeyHomeContent],
		CompactHomeEnabled:                     settings[SettingKeyCompactHomeEnabled] == "true",
		HideCcsImportButton:                    settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:            settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:                strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		CustomMenuItems:                        settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                        settings[SettingKeyCustomEndpoints],
		BackendModeEnabled:                     settings[SettingKeyBackendModeEnabled] == "true",
	}
	result.TableDefaultPageSize, result.TablePageSizeOptions = parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
	} else {
		result.SMTPPort = 587
	}

	if concurrency, err := strconv.Atoi(settings[SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
	} else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
	}

	if rpm, err := strconv.Atoi(settings[SettingKeyDefaultUserRPMLimit]); err == nil && rpm >= 0 {
		result.DefaultUserRPMLimit = rpm
	}

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
	} else {
		result.DefaultBalance = s.cfg.Default.UserBalance
	}
	if rebateRate, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebateRate], 64); err == nil {
		result.AffiliateRebateRate = clampAffiliateRebateRate(rebateRate)
	} else {
		result.AffiliateRebateRate = AffiliateRebateRateDefault
	}
	if freezeHours, err := strconv.Atoi(settings[SettingKeyAffiliateRebateFreezeHours]); err == nil && freezeHours >= 0 {
		if freezeHours > AffiliateRebateFreezeHoursMax {
			freezeHours = AffiliateRebateFreezeHoursMax
		}
		result.AffiliateRebateFreezeHours = freezeHours
	}
	if durationDays, err := strconv.Atoi(settings[SettingKeyAffiliateRebateDurationDays]); err == nil && durationDays >= 0 {
		if durationDays > AffiliateRebateDurationDaysMax {
			durationDays = AffiliateRebateDurationDaysMax
		}
		result.AffiliateRebateDurationDays = durationDays
	}
	if perInviteeCap, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebatePerInviteeCap], 64); err == nil && perInviteeCap >= 0 {
		result.AffiliateRebatePerInviteeCap = perInviteeCap
	}
	result.AdminRechargeRebateEnabled = settings[SettingKeyAffiliateAdminRechargeEnabled] == "true"
	result.DefaultSubscriptions = parseDefaultSubscriptions(settings[SettingKeyDefaultSubscriptions])

	// 敏感信息直接返回，方便测试连接时使用
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	result.TurnstileSecretKey = settings[SettingKeyTurnstileSecretKey]
	result.TencentCaptchaAppSecretKey = settings[SettingKeyTencentCaptchaAppSecretKey]
	result.TencentCaptchaCloudSecretID = settings[SettingKeyTencentCaptchaCloudSecretID]
	result.TencentCaptchaCloudSecretKey = settings[SettingKeyTencentCaptchaCloudSecretKey]
	result.AliyunCaptchaAccessKeySecret = settings[SettingKeyAliyunCaptchaAccessKeySecret]

	// LinuxDo Connect 设置：
	// - 兼容 config.yaml/env（避免老部署因为未迁移到数据库设置而被意外关闭）
	// - 支持在后台“系统设置”中覆盖并持久化（存储于 DB）
	linuxDoBase := config.LinuxDoConnectConfig{}
	if s.cfg != nil {
		linuxDoBase = s.cfg.LinuxDo
	}

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		result.LinuxDoConnectEnabled = raw == "true"
	} else {
		result.LinuxDoConnectEnabled = linuxDoBase.Enabled
	}

	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectClientID = strings.TrimSpace(v)
	} else {
		result.LinuxDoConnectClientID = linuxDoBase.ClientID
	}

	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectRedirectURL = strings.TrimSpace(v)
	} else {
		result.LinuxDoConnectRedirectURL = linuxDoBase.RedirectURL
	}

	result.LinuxDoConnectClientSecret = strings.TrimSpace(settings[SettingKeyLinuxDoConnectClientSecret])
	if result.LinuxDoConnectClientSecret == "" {
		result.LinuxDoConnectClientSecret = strings.TrimSpace(linuxDoBase.ClientSecret)
	}
	result.LinuxDoConnectClientSecretConfigured = result.LinuxDoConnectClientSecret != ""

	// DingTalk Connect 设置：
	// - 兼容 config.yaml/env
	// - 支持后台系统设置覆盖并持久化（存储于 DB）
	dingTalkBase := config.DingTalkConnectConfig{}
	if s.cfg != nil {
		dingTalkBase = s.cfg.DingTalk
	}

	if raw, ok := settings[SettingKeyDingTalkConnectEnabled]; ok {
		result.DingTalkConnectEnabled = raw == "true"
	} else {
		result.DingTalkConnectEnabled = dingTalkBase.Enabled
	}

	if v, ok := settings[SettingKeyDingTalkConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectClientID = strings.TrimSpace(v)
	} else {
		result.DingTalkConnectClientID = dingTalkBase.ClientID
	}

	if v, ok := settings[SettingKeyDingTalkConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectRedirectURL = strings.TrimSpace(v)
	} else {
		result.DingTalkConnectRedirectURL = dingTalkBase.RedirectURL
	}

	result.DingTalkConnectClientSecret = strings.TrimSpace(settings[SettingKeyDingTalkConnectClientSecret])
	if result.DingTalkConnectClientSecret == "" {
		result.DingTalkConnectClientSecret = strings.TrimSpace(dingTalkBase.ClientSecret)
	}
	result.DingTalkConnectClientSecretConfigured = result.DingTalkConnectClientSecret != ""

	if v, ok := settings[SettingKeyDingTalkConnectCorpRestrictionPolicy]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectCorpRestrictionPolicy = strings.TrimSpace(v)
	} else {
		result.DingTalkConnectCorpRestrictionPolicy = dingTalkBase.CorpRestrictionPolicy
	}
	result.DingTalkConnectCorpRestrictionPolicy = coerceDeprecatedDingTalkCorpPolicy(result.DingTalkConnectCorpRestrictionPolicy)

	if v, ok := settings[SettingKeyDingTalkConnectInternalCorpID]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectInternalCorpID = strings.TrimSpace(v)
	} else {
		result.DingTalkConnectInternalCorpID = dingTalkBase.InternalCorpID
	}

	if v, ok := settings[SettingKeyDingTalkConnectBypassRegistration]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectBypassRegistration = strings.EqualFold(strings.TrimSpace(v), "true")
	} else {
		result.DingTalkConnectBypassRegistration = dingTalkBase.BypassRegistration
	}
	// bypass_registration 仅在 internal_only 模式下有意义；其它策略下强制 false，
	// 以保证加载出的 effective config 永远是一致状态。
	if result.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		result.DingTalkConnectBypassRegistration = false
	}

	if v, ok := settings[SettingKeyDingTalkConnectSyncCorpEmail]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncCorpEmail = strings.EqualFold(strings.TrimSpace(v), "true")
	} else {
		result.DingTalkConnectSyncCorpEmail = dingTalkBase.SyncCorpEmail
	}
	if v, ok := settings[SettingKeyDingTalkConnectSyncDisplayName]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncDisplayName = strings.EqualFold(strings.TrimSpace(v), "true")
	} else {
		result.DingTalkConnectSyncDisplayName = dingTalkBase.SyncDisplayName
	}
	if v, ok := settings[SettingKeyDingTalkConnectSyncDept]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncDept = strings.EqualFold(strings.TrimSpace(v), "true")
	} else {
		result.DingTalkConnectSyncDept = dingTalkBase.SyncDept
	}
	// 身份同步三开关仅在 internal_only 模式下有意义；其它策略强制 false。
	if result.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		result.DingTalkConnectSyncCorpEmail = false
		result.DingTalkConnectSyncDisplayName = false
		result.DingTalkConnectSyncDept = false
	}

	// 身份同步目标 attr key（DB 空 → fallback 默认值）
	result.DingTalkConnectSyncCorpEmailAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncCorpEmailAttrKey])
	if result.DingTalkConnectSyncCorpEmailAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncCorpEmailAttrKey); v != "" {
			result.DingTalkConnectSyncCorpEmailAttrKey = v
		} else {
			result.DingTalkConnectSyncCorpEmailAttrKey = "dingtalk_email"
		}
	}
	result.DingTalkConnectSyncDisplayNameAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDisplayNameAttrKey])
	if result.DingTalkConnectSyncDisplayNameAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDisplayNameAttrKey); v != "" {
			result.DingTalkConnectSyncDisplayNameAttrKey = v
		} else {
			result.DingTalkConnectSyncDisplayNameAttrKey = "dingtalk_name"
		}
	}
	result.DingTalkConnectSyncDeptAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDeptAttrKey])
	if result.DingTalkConnectSyncDeptAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDeptAttrKey); v != "" {
			result.DingTalkConnectSyncDeptAttrKey = v
		} else {
			result.DingTalkConnectSyncDeptAttrKey = "dingtalk_department"
		}
	}

	// 身份同步目标 attr 显示名称（DB 空 → fallback 默认中文）
	result.DingTalkConnectSyncCorpEmailAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncCorpEmailAttrName])
	if result.DingTalkConnectSyncCorpEmailAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncCorpEmailAttrName); v != "" {
			result.DingTalkConnectSyncCorpEmailAttrName = v
		} else {
			result.DingTalkConnectSyncCorpEmailAttrName = "钉钉企业邮箱"
		}
	}
	result.DingTalkConnectSyncDisplayNameAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDisplayNameAttrName])
	if result.DingTalkConnectSyncDisplayNameAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDisplayNameAttrName); v != "" {
			result.DingTalkConnectSyncDisplayNameAttrName = v
		} else {
			result.DingTalkConnectSyncDisplayNameAttrName = "钉钉姓名"
		}
	}
	result.DingTalkConnectSyncDeptAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDeptAttrName])
	if result.DingTalkConnectSyncDeptAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDeptAttrName); v != "" {
			result.DingTalkConnectSyncDeptAttrName = v
		} else {
			result.DingTalkConnectSyncDeptAttrName = "钉钉部门"
		}
	}

	// Generic OIDC 设置：
	// - 兼容 config.yaml/env
	// - 支持后台系统设置覆盖并持久化（存储于 DB）
	oidcBase := config.OIDCConnectConfig{}
	if s.cfg != nil {
		oidcBase = s.cfg.OIDC
	}

	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		result.OIDCConnectEnabled = raw == "true"
	} else {
		result.OIDCConnectEnabled = oidcBase.Enabled
	}

	if v, ok := settings[SettingKeyOIDCConnectProviderName]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectProviderName = strings.TrimSpace(v)
	} else {
		result.OIDCConnectProviderName = strings.TrimSpace(oidcBase.ProviderName)
	}
	if result.OIDCConnectProviderName == "" {
		result.OIDCConnectProviderName = "OIDC"
	}

	if v, ok := settings[SettingKeyOIDCConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectClientID = strings.TrimSpace(v)
	} else {
		result.OIDCConnectClientID = strings.TrimSpace(oidcBase.ClientID)
	}
	if v, ok := settings[SettingKeyOIDCConnectIssuerURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectIssuerURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectIssuerURL = strings.TrimSpace(oidcBase.IssuerURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectDiscoveryURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(oidcBase.DiscoveryURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectAuthorizeURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(oidcBase.AuthorizeURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectTokenURL = strings.TrimSpace(oidcBase.TokenURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(oidcBase.UserInfoURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectJWKSURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectJWKSURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectJWKSURL = strings.TrimSpace(oidcBase.JWKSURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectScopes]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectScopes = strings.TrimSpace(v)
	} else {
		result.OIDCConnectScopes = strings.TrimSpace(oidcBase.Scopes)
	}
	if v, ok := settings[SettingKeyOIDCConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectRedirectURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectRedirectURL = strings.TrimSpace(oidcBase.RedirectURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectFrontendRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(oidcBase.FrontendRedirectURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenAuthMethod]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(v))
	} else {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(oidcBase.TokenAuthMethod))
	}
	if raw, ok := settings[SettingKeyOIDCConnectUsePKCE]; ok {
		result.OIDCConnectUsePKCE = raw == "true"
	} else {
		result.OIDCConnectUsePKCE = oidcUsePKCECompatibilityDefault(oidcBase)
	}
	if raw, ok := settings[SettingKeyOIDCConnectValidateIDToken]; ok {
		result.OIDCConnectValidateIDToken = raw == "true"
	} else {
		result.OIDCConnectValidateIDToken = oidcValidateIDTokenCompatibilityDefault(oidcBase)
	}
	if v, ok := settings[SettingKeyOIDCConnectAllowedSigningAlgs]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(v)
	} else {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(oidcBase.AllowedSigningAlgs)
	}
	clockSkewSet := false
	if raw, ok := settings[SettingKeyOIDCConnectClockSkewSeconds]; ok && strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			result.OIDCConnectClockSkewSeconds = parsed
			clockSkewSet = true
		}
	}
	if !clockSkewSet {
		result.OIDCConnectClockSkewSeconds = oidcBase.ClockSkewSeconds
	}
	if !clockSkewSet && result.OIDCConnectClockSkewSeconds == 0 {
		result.OIDCConnectClockSkewSeconds = 120
	}
	if raw, ok := settings[SettingKeyOIDCConnectRequireEmailVerified]; ok {
		result.OIDCConnectRequireEmailVerified = raw == "true"
	} else {
		result.OIDCConnectRequireEmailVerified = oidcBase.RequireEmailVerified
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoEmailPath]; ok {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(oidcBase.UserInfoEmailPath)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoIDPath]; ok {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(oidcBase.UserInfoIDPath)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoUsernamePath]; ok {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(oidcBase.UserInfoUsernamePath)
	}
	result.OIDCConnectClientSecret = strings.TrimSpace(settings[SettingKeyOIDCConnectClientSecret])
	if result.OIDCConnectClientSecret == "" {
		result.OIDCConnectClientSecret = strings.TrimSpace(oidcBase.ClientSecret)
	}
	result.OIDCConnectClientSecretConfigured = result.OIDCConnectClientSecret != ""

	gitHubEffective := s.effectiveEmailOAuthConfig(settings, "github")
	result.GitHubOAuthEnabled = gitHubEffective.Enabled
	result.GitHubOAuthClientID = strings.TrimSpace(gitHubEffective.ClientID)
	result.GitHubOAuthClientSecret = strings.TrimSpace(gitHubEffective.ClientSecret)
	result.GitHubOAuthClientSecretConfigured = result.GitHubOAuthClientSecret != ""
	result.GitHubOAuthRedirectURL = strings.TrimSpace(gitHubEffective.RedirectURL)
	result.GitHubOAuthFrontendRedirectURL = strings.TrimSpace(gitHubEffective.FrontendRedirectURL)

	googleEffective := s.effectiveEmailOAuthConfig(settings, "google")
	result.GoogleOAuthEnabled = googleEffective.Enabled
	result.GoogleOAuthClientID = strings.TrimSpace(googleEffective.ClientID)
	result.GoogleOAuthClientSecret = strings.TrimSpace(googleEffective.ClientSecret)
	result.GoogleOAuthClientSecretConfigured = result.GoogleOAuthClientSecret != ""
	result.GoogleOAuthRedirectURL = strings.TrimSpace(googleEffective.RedirectURL)
	result.GoogleOAuthFrontendRedirectURL = strings.TrimSpace(googleEffective.FrontendRedirectURL)

	// WeChat Connect 设置：
	// - 优先读取 DB 系统设置
	// - 缺失时回退到 config/env，保持升级兼容
	weChatEffective := s.effectiveWeChatConnectOAuthConfig(settings)
	result.WeChatConnectEnabled = weChatEffective.Enabled
	result.WeChatConnectAppID = weChatEffective.LegacyAppID
	result.WeChatConnectAppSecret = weChatEffective.LegacyAppSecret
	result.WeChatConnectAppSecretConfigured = weChatEffective.LegacyAppSecret != ""
	result.WeChatConnectOpenAppID = weChatEffective.OpenAppID
	result.WeChatConnectOpenAppSecret = weChatEffective.OpenAppSecret
	result.WeChatConnectOpenAppSecretConfigured = weChatEffective.OpenAppSecret != ""
	result.WeChatConnectMPAppID = weChatEffective.MPAppID
	result.WeChatConnectMPAppSecret = weChatEffective.MPAppSecret
	result.WeChatConnectMPAppSecretConfigured = weChatEffective.MPAppSecret != ""
	result.WeChatConnectMobileAppID = weChatEffective.MobileAppID
	result.WeChatConnectMobileAppSecret = weChatEffective.MobileAppSecret
	result.WeChatConnectMobileAppSecretConfigured = weChatEffective.MobileAppSecret != ""
	result.WeChatConnectOpenEnabled = weChatEffective.OpenEnabled
	result.WeChatConnectMPEnabled = weChatEffective.MPEnabled
	result.WeChatConnectMobileEnabled = weChatEffective.MobileEnabled
	result.WeChatConnectMode = weChatEffective.Mode
	result.WeChatConnectScopes = weChatEffective.Scopes
	result.WeChatConnectRedirectURL = weChatEffective.RedirectURL
	result.WeChatConnectFrontendRedirectURL = weChatEffective.FrontendRedirectURL

	// Model fallback settings
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = s.getStringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = s.getStringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = s.getStringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = s.getStringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")

	// Identity patch settings (default: enabled, to preserve existing behavior)
	if v, ok := settings[SettingKeyEnableIdentityPatch]; ok && v != "" {
		result.EnableIdentityPatch = v == "true"
	} else {
		result.EnableIdentityPatch = true
	}
	result.IdentityPatchPrompt = settings[SettingKeyIdentityPatchPrompt]

	// Ops monitoring settings (default: enabled, fail-open)
	result.OpsMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsQueryModeDefault = string(ParseOpsQueryMode(settings[SettingKeyOpsQueryModeDefault]))
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
			}
			if v > 3600 {
				v = 3600
			}
			result.OpsMetricsIntervalSeconds = v
		}
	}

	// Channel monitor feature (default: enabled, 60s)
	result.ChannelMonitorEnabled = !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled])
	result.ChannelMonitorMode = normalizeChannelMonitorMode(settings[SettingKeyChannelMonitorMode])
	result.ChannelMonitorDefaultIntervalSeconds = parseChannelMonitorInterval(
		settings[SettingKeyChannelMonitorDefaultIntervalSeconds],
	)
	// 默认隐藏吞吐（迁移 206 的隐私默认）：未配置时必须与 setting_public.go 的
	// 公开读取路径给出同一个值，否则管理端看到“未隐藏”而用户端实际已隐藏。
	result.ChannelMonitorHideThroughput = !isFalseSettingValue(settings[SettingKeyChannelMonitorHideThroughput])
	// 配额展示默认关闭且 fail-closed：仅字面 "true" 视为开启
	// （与 setting_public.go 公开读取路径保持一致）。
	result.ChannelMonitorShowQuota = settings[SettingKeyChannelMonitorShowQuota] == "true"

	// Grok default mapping policy
	result.GrokDefaultTextModel = strings.TrimSpace(settings[SettingKeyGrokDefaultTextModel])
	if result.GrokDefaultTextModel == "" {
		result.GrokDefaultTextModel = "grok-4.6"
	}
	// Default true (missing/empty → enabled) so Claude/Codex→Grok mapping keeps working.
	// Operators can set false to disable silent cross-client rewrite.
	result.GrokCrossClientModelMapEnabled = !isFalseSettingValue(settings[SettingKeyGrokCrossClientModelMapEnabled])
	result.GrokDefaultBaseURLMode = normalizeGrokDefaultBaseURLMode(settings[SettingKeyGrokDefaultBaseURLMode])

	// Available channels feature (default: disabled; strict true)
	result.AvailableChannelsEnabled = settings[SettingKeyAvailableChannelsEnabled] == "true"

	// Model plaza feature (default: disabled; strict true)
	result.ModelPlazaEnabled = settings[SettingKeyModelPlazaEnabled] == "true"
	result.ModelPlazaRequireAuth = settings[SettingKeyModelPlazaRequireAuth] == "true"
	result.ModelPlazaDescription = settings[SettingKeyModelPlazaDescription]
	result.PluginManagementEnabled = settings[SettingKeyPluginManagementEnabled] == "true"

	// Affiliate (邀请返利) feature (default: disabled; strict true)
	result.AffiliateEnabled = settings[SettingKeyAffiliateEnabled] == "true"

	// 风控中心功能（默认关闭，严格 true 才启用）
	result.RiskControlEnabled = settings[SettingKeyRiskControlEnabled] == "true"

	// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
	result.CyberSessionBlockEnabled = settings[SettingKeyCyberSessionBlockEnabled] == "true"
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyCyberSessionBlockTTLSeconds])); err == nil && v > 0 {
		result.CyberSessionBlockTTLSeconds = v
	} else {
		result.CyberSessionBlockTTLSeconds = 3600
	}

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]
	result.MaxClaudeCodeVersion = settings[SettingKeyMaxClaudeCodeVersion]

	// 分组隔离
	result.AllowUngroupedKeyScheduling = settings[SettingKeyAllowUngroupedKeyScheduling] == "true"

	// Gateway forwarding behavior (defaults: fingerprint=true, metadata_passthrough=false,
	// cch_signing=false, claude_oauth_system_prompt_injection=true)
	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
	} else {
		result.EnableFingerprintUnification = true // default: enabled (current behavior)
	}
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	if v, ok := settings[SettingKeyEnableClaudeOAuthSystemPromptInjection]; ok && v != "" {
		result.EnableClaudeOAuthSystemPromptInjection = v == "true"
	} else {
		result.EnableClaudeOAuthSystemPromptInjection = true
	}
	result.ClaudeOAuthSystemPrompt = settings[SettingKeyClaudeOAuthSystemPrompt]
	result.ClaudeOAuthSystemPromptBlocks = settings[SettingKeyClaudeOAuthSystemPromptBlocks]
	result.EnableAnthropicCacheTTL1hInjection = settings[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
	if v, ok := settings[SettingKeyRewriteMessageCacheControl]; ok && v != "" {
		result.RewriteMessageCacheControl = v == "true"
	} else {
		result.RewriteMessageCacheControl = s.defaultRewriteMessageCacheControl()
	}
	if v, ok := settings[SettingKeyEnableClientDatelineNormalization]; ok && v != "" {
		result.EnableClientDatelineNormalization = v == "true"
	} else {
		result.EnableClientDatelineNormalization = true
	}
	result.AntigravityUserAgentVersion = antigravity.NormalizeUserAgentVersion(settings[SettingKeyAntigravityUserAgentVersion])
	result.OpenAICodexUserAgent = strings.TrimSpace(settings[SettingKeyOpenAICodexUserAgent])
	result.OpenAICodexClientVersion = NormalizeCodexClientVersion(settings[SettingKeyOpenAICodexClientVersion])
	result.OpenAICodexClientVersionSynced = NormalizeCodexClientVersion(settings[SettingKeyOpenAICodexClientVersionSynced])
	// 自动同步默认开启：缺失/空值一律视为开启，与 enable_client_dateline_normalization 同一惯例。
	if v, ok := settings[SettingKeyOpenAICodexVersionAutoSyncEnabled]; ok && v != "" {
		result.OpenAICodexVersionAutoSyncEnabled = v == "true"
	} else {
		result.OpenAICodexVersionAutoSyncEnabled = true
	}
	// codex_cli_only 加固
	result.MinCodexVersion = settings[SettingKeyMinCodexVersion]
	result.MaxCodexVersion = settings[SettingKeyMaxCodexVersion]
	result.CodexCLIOnlyBlacklist = settings[SettingKeyCodexCLIOnlyBlacklist]
	result.CodexCLIOnlyWhitelist = settings[SettingKeyCodexCLIOnlyWhitelist]
	result.CodexCLIOnlyAllowAppServerClients = settings[SettingKeyCodexCLIOnlyAllowAppServerClients] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyCodexCLIOnlyEngineFingerprintSignals]); raw != "" {
		result.CodexCLIOnlyEngineFingerprintSignals = raw
	} else {
		result.CodexCLIOnlyEngineFingerprintSignals = openai.DefaultEngineFingerprintSignalsJSON() // 缺失/空 → 展示默认种子
	}

	// Web search emulation: quick enabled check from the JSON config
	if raw := settings[SettingKeyWebSearchEmulationConfig]; raw != "" {
		var wsCfg WebSearchEmulationConfig
		if err := json.Unmarshal([]byte(raw), &wsCfg); err == nil {
			result.WebSearchEmulationEnabled = wsCfg.Enabled && len(wsCfg.Providers) > 0
		}
	}
	result.PaymentVisibleMethodAlipaySource = NormalizeVisibleMethodSource("alipay", settings[SettingPaymentVisibleMethodAlipaySource])
	result.PaymentVisibleMethodWxpaySource = NormalizeVisibleMethodSource("wxpay", settings[SettingPaymentVisibleMethodWxpaySource])
	result.PaymentVisibleMethodAlipayEnabled = settings[SettingPaymentVisibleMethodAlipayEnabled] == "true"
	result.PaymentVisibleMethodWxpayEnabled = settings[SettingPaymentVisibleMethodWxpayEnabled] == "true"
	result.OpenAILowUpstreamRatePriorityEnabled = settings[SettingKeyOpenAILowUpstreamRatePriorityEnabled] == "true"
	result.OpenAIOAuthSchedulingRateMultiplier = parseOpenAIOAuthSchedulingRateMultiplier(settings[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
	result.OpenAIAdvancedSchedulerEnabled = settings[openAIAdvancedSchedulerSettingKey] == "true"
	result.OpenAIAdvancedSchedulerStickyWeightedEnabled = settings[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] == "true"
	result.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled = settings[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] == "true"
	result.OpenAIAdvancedSchedulerLBTopK = normalizeOpenAISchedulerTopKForRead(settings[SettingKeyOpenAIAdvancedSchedulerLBTopK])
	result.OpenAIAdvancedSchedulerWeightPriority = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightPriority])
	result.OpenAIAdvancedSchedulerWeightLoad = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightLoad])
	result.OpenAIAdvancedSchedulerWeightQueue = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightQueue])
	result.OpenAIAdvancedSchedulerWeightErrorRate = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate])
	result.OpenAIAdvancedSchedulerWeightTTFT = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightTTFT])
	result.OpenAIAdvancedSchedulerWeightReset = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightReset])
	result.OpenAIAdvancedSchedulerWeightQuotaHeadroom = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom])
	result.OpenAIAdvancedSchedulerWeightUpstreamCost = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost])
	result.OpenAIAdvancedSchedulerWeightPreviousResponse = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse])
	result.OpenAIAdvancedSchedulerWeightSessionSticky = normalizeOpenAISchedulerWeightForRead(settings[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky])
	fairness := normalizeOpenAISchedulerFairnessSettingsForRead(OpenAISchedulerFairnessSettings{
		CandidatePoolMode:          settings[SettingKeyOpenAIAdvancedSchedulerCandidatePoolMode],
		ExplorationRatio:           parseIntSettingOrDefault(settings[SettingKeyOpenAIAdvancedSchedulerExplorationRatio], 20),
		StarvationThresholdSeconds: parseIntSettingOrDefault(settings[SettingKeyOpenAIAdvancedSchedulerStarvationThresholdSeconds], 21600),
		FairnessWeight:             parseFloatSettingOrDefault(settings[SettingKeyOpenAIAdvancedSchedulerFairnessWeight], 2),
	})
	result.OpenAIAdvancedSchedulerCandidatePoolMode = fairness.CandidatePoolMode
	result.OpenAIAdvancedSchedulerExplorationRatio = fairness.ExplorationRatio
	result.OpenAIAdvancedSchedulerStarvationThresholdSeconds = fairness.StarvationThresholdSeconds
	result.OpenAIAdvancedSchedulerFairnessWeight = fairness.FairnessWeight
	result.OpenAIAdvancedSchedulerGroupOverrides = normalizeOpenAISchedulerFairnessOverridesForRead(parseOpenAISchedulerFairnessOverrides(settings[SettingKeyOpenAIAdvancedSchedulerGroupOverrides]))
	result.OpenAIAdvancedSchedulerCustomPresets, _ = parseOpenAISchedulerCustomPresets(settings[SettingKeyOpenAIAdvancedSchedulerCustomPresets])
	result.OpenAIAdvancedSchedulerGroupPolicies, _ = parseOpenAISchedulerGroupPolicies(settings[SettingKeyOpenAIAdvancedSchedulerGroupOverrides])
	result.OpenAIAdvancedSchedulerGroupPolicies = normalizeOpenAISchedulerGroupPoliciesForRead(result.OpenAIAdvancedSchedulerGroupPolicies)
	global := openAISchedulerPolicyValuesFromSettings(result)
	if policies, err := normalizeOpenAISchedulerGroupPoliciesWithPresets(result.OpenAIAdvancedSchedulerGroupPolicies, global, nil, result.OpenAIAdvancedSchedulerCustomPresets); err == nil {
		result.OpenAIAdvancedSchedulerGroupPolicies = policies
	}
	result.OpenAIAdvancedSchedulerAvailablePresets = openAISchedulerAvailablePresets(result.OpenAIAdvancedSchedulerCustomPresets)
	result.OpenAIAdvancedSchedulerEffectiveLBTopK = s.openAIAdvancedSchedulerEffectiveLBTopK()
	effectiveWeights := s.openAIAdvancedSchedulerEffectiveWeights()
	result.OpenAIAdvancedSchedulerEffectiveWeightPriority = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Priority)
	result.OpenAIAdvancedSchedulerEffectiveWeightLoad = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Load)
	result.OpenAIAdvancedSchedulerEffectiveWeightQueue = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Queue)
	result.OpenAIAdvancedSchedulerEffectiveWeightErrorRate = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.ErrorRate)
	result.OpenAIAdvancedSchedulerEffectiveWeightTTFT = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.TTFT)
	result.OpenAIAdvancedSchedulerEffectiveWeightReset = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Reset)
	result.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.QuotaHeadroom)
	result.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.UpstreamCost)
	result.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.PreviousResponse)
	result.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.SessionSticky)

	// 余额、订阅到期与账号限额通知
	result.BalanceLowNotifyEnabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		result.BalanceLowNotifyThreshold = v
	}
	result.BalanceLowNotifyRechargeURL = settings[SettingKeyBalanceLowNotifyRechargeURL]
	result.SubscriptionExpiryNotifyEnabled = !isFalseSettingValue(settings[SettingKeySubscriptionExpiryNotifyEnabled])

	// 账号限额通知
	result.AccountQuotaNotifyEnabled = settings[SettingKeyAccountQuotaNotifyEnabled] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyAccountQuotaNotifyEmails]); raw != "" {
		result.AccountQuotaNotifyEmails = ParseNotifyEmails(raw)
	}
	if result.AccountQuotaNotifyEmails == nil {
		result.AccountQuotaNotifyEmails = []NotifyEmailEntry{}
	}

	// 系统层默认 platform quota（修复 Bug B：parseSettings 不填充导致回显恒为 nil）
	if raw := settings[SettingKeyDefaultPlatformQuotas]; raw != "" {
		parsed := map[string]*DefaultPlatformQuotaSetting{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("[Setting] parseSettings: unmarshal default_platform_quotas failed", "error", err)
		} else {
			result.DefaultPlatformQuotas = parsed
		}
	}
	result.AccountSchedulingThresholds = defaultAccountSchedulingThresholds()
	if raw := strings.TrimSpace(settings[SettingKeyAccountSchedulingThresholds]); raw != "" {
		if thresholds, err := parseAccountSchedulingThresholdsSetting(raw); err != nil {
			slog.Warn("[Setting] parseSettings: unmarshal account_scheduling_thresholds failed", "error", err)
		} else {
			result.AccountSchedulingThresholds = thresholds
		}
	}

	result.AllowUserViewErrorRequests = settings[SettingKeyAllowUserViewErrorRequests] == "true" // default false

	// Publish Grok default model_mapping options for accounts with empty mapping.
	xai.SetRuntimeModelMappingOptions(xai.ModelMappingOptions{
		DefaultText:          result.GrokDefaultTextModel,
		EnableCrossClientMap: result.GrokCrossClientModelMapEnabled,
	})

	return result
}

// NormalizeMonitorPageRefreshIntervalSeconds parses the stored administrator
// monitor page refresh policy. Missing and legacy invalid values safely fall
// back to the default 60-second refresh interval.
func NormalizeMonitorPageRefreshIntervalSeconds(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || !IsMonitorPageRefreshIntervalSeconds(value) {
		return MonitorPageRefreshIntervalSecondsDefault
	}
	return value
}

func clampAffiliateRebateRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return AffiliateRebateRateDefault
	}
	if value < AffiliateRebateRateMin {
		return AffiliateRebateRateMin
	}
	if value > AffiliateRebateRateMax {
		return AffiliateRebateRateMax
	}
	return value
}

func isFalseSettingValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return true
	default:
		return false
	}
}

func normalizeVisibleMethodSettingSource(method, source string, enabled bool) (string, error) {
	_ = enabled
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}

	normalized := NormalizeVisibleMethodSource(method, source)
	if normalized == "" {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	return normalized, nil
}

func (s *SettingService) openAIAdvancedSchedulerEffectiveLBTopK() string {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return strconv.Itoa(s.cfg.Gateway.OpenAIWS.LBTopK)
	}
	return "7"
}

func (s *SettingService) openAIAdvancedSchedulerEffectiveWeights() config.GatewayOpenAIWSSchedulerScoreWeights {
	defaults := config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         1.0,
		Load:             1.0,
		Queue:            0.7,
		ErrorRate:        0.8,
		TTFT:             0.5,
		Reset:            0.0,
		QuotaHeadroom:    0.0,
		UpstreamCost:     0.0,
		PreviousResponse: 5.0,
		SessionSticky:    3.0,
	}
	if s == nil || s.cfg == nil {
		return defaults
	}

	weights := s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights
	if !weights.IsValid() {
		return defaults
	}
	return weights
}

func formatOpenAIAdvancedSchedulerFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (s *SettingService) normalizeOpenAIAdvancedSchedulerOverrides(settings *SystemSettings) error {
	if rate := settings.OpenAIOAuthSchedulingRateMultiplier; rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return infraerrors.BadRequest("INVALID_OPENAI_OAUTH_SCHEDULING_RATE_MULTIPLIER", "OpenAI OAuth scheduling rate multiplier must be a finite non-negative number")
	}

	lbTopK, err := normalizeOptionalPositiveIntString(settings.OpenAIAdvancedSchedulerLBTopK)
	if err != nil {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_LB_TOP_K", "openai advanced scheduler TopK must be a positive integer or empty")
	}
	if lbTopK != "" {
		value, _ := strconv.Atoi(lbTopK)
		if value > 32 {
			return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_LB_TOP_K", "openai advanced scheduler TopK must be between 1 and 32")
		}
	}
	settings.OpenAIAdvancedSchedulerLBTopK = lbTopK

	weights := []*string{
		&settings.OpenAIAdvancedSchedulerWeightPriority,
		&settings.OpenAIAdvancedSchedulerWeightLoad,
		&settings.OpenAIAdvancedSchedulerWeightQueue,
		&settings.OpenAIAdvancedSchedulerWeightErrorRate,
		&settings.OpenAIAdvancedSchedulerWeightTTFT,
		&settings.OpenAIAdvancedSchedulerWeightReset,
		&settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		&settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		&settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		&settings.OpenAIAdvancedSchedulerWeightSessionSticky,
	}
	for _, target := range weights {
		normalized, err := normalizeOptionalNonNegativeFloatString(*target)
		if err != nil {
			return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must be non-negative numbers or empty")
		}
		if normalized != "" {
			value, _ := strconv.ParseFloat(normalized, 64)
			if value > 10 {
				return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must be between 0 and 10")
			}
		}
		*target = normalized
	}

	// 与 config.Validate 的 "scheduler_score_weights must not all be zero" 保持一致：
	// 覆盖值（空则回退到生效的配置值）叠加后的基础权重和不允许为 0，
	// 否则调度会静默退化为 TopK 内均匀随机。
	effective := s.openAIAdvancedSchedulerEffectiveWeights()
	resolved := config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightPriority, effective.Priority),
		Load:             resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightLoad, effective.Load),
		Queue:            resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQueue, effective.Queue),
		ErrorRate:        resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightErrorRate, effective.ErrorRate),
		TTFT:             resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightTTFT, effective.TTFT),
		Reset:            resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightReset, effective.Reset),
		QuotaHeadroom:    resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom, effective.QuotaHeadroom),
		UpstreamCost:     resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightUpstreamCost, effective.UpstreamCost),
		PreviousResponse: resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightPreviousResponse, effective.PreviousResponse),
		SessionSticky:    resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightSessionSticky, effective.SessionSticky),
	}
	if !resolved.IsValid() {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must have finite non-zero base and total sums")
	}
	fairness, err := normalizeOpenAISchedulerFairnessSettings(OpenAISchedulerFairnessSettings{
		CandidatePoolMode:          settings.OpenAIAdvancedSchedulerCandidatePoolMode,
		ExplorationRatio:           settings.OpenAIAdvancedSchedulerExplorationRatio,
		StarvationThresholdSeconds: settings.OpenAIAdvancedSchedulerStarvationThresholdSeconds,
		FairnessWeight:             settings.OpenAIAdvancedSchedulerFairnessWeight,
		GroupOverrides:             settings.OpenAIAdvancedSchedulerGroupOverrides,
	})
	if err != nil {
		return err
	}
	settings.OpenAIAdvancedSchedulerCandidatePoolMode = fairness.CandidatePoolMode
	settings.OpenAIAdvancedSchedulerExplorationRatio = fairness.ExplorationRatio
	settings.OpenAIAdvancedSchedulerStarvationThresholdSeconds = fairness.StarvationThresholdSeconds
	settings.OpenAIAdvancedSchedulerFairnessWeight = fairness.FairnessWeight
	settings.OpenAIAdvancedSchedulerGroupOverrides = fairness.GroupOverrides
	if settings.OpenAIAdvancedSchedulerGroupPolicies != nil {
		global := openAISchedulerPolicyValuesFromSettings(settings)
		customPresets, err := normalizeOpenAISchedulerCustomPresets(settings.OpenAIAdvancedSchedulerCustomPresets)
		if err != nil {
			return err
		}
		normalized, err := normalizeOpenAISchedulerGroupPoliciesWithPresets(settings.OpenAIAdvancedSchedulerGroupPolicies, global, nil, customPresets)
		if err != nil {
			return err
		}
		settings.OpenAIAdvancedSchedulerGroupPolicies = normalized
		settings.OpenAIAdvancedSchedulerCustomPresets = customPresets
		settings.OpenAIAdvancedSchedulerAvailablePresets = openAISchedulerAvailablePresets(customPresets)
	}
	return nil
}

func openAISchedulerPolicyValuesFromSettings(settings *SystemSettings) OpenAISchedulerPolicyValues {
	return OpenAISchedulerPolicyValues{TopK: parsePositiveIntOverride(settings.OpenAIAdvancedSchedulerLBTopK), Priority: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightPriority, 1), Load: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightLoad, 1), Queue: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightQueue, .7), ErrorRate: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightErrorRate, .8), TTFT: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightTTFT, .5), Reset: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightReset, 0), QuotaHeadroom: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom, 0), UpstreamCost: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightUpstreamCost, 0), PreviousResponse: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightPreviousResponse, 5), SessionSticky: parseFloatSettingOrDefault(settings.OpenAIAdvancedSchedulerWeightSessionSticky, 3), CandidatePoolMode: settings.OpenAIAdvancedSchedulerCandidatePoolMode, ExplorationRatio: settings.OpenAIAdvancedSchedulerExplorationRatio, StarvationThresholdSeconds: settings.OpenAIAdvancedSchedulerStarvationThresholdSeconds, FairnessWeight: settings.OpenAIAdvancedSchedulerFairnessWeight}
}

func normalizeOpenAISchedulerFairnessSettings(value OpenAISchedulerFairnessSettings) (OpenAISchedulerFairnessSettings, error) {
	defaults := defaultOpenAISchedulerFairnessSettings()
	if value.CandidatePoolMode == "" && value.ExplorationRatio == 0 && value.StarvationThresholdSeconds == 0 && value.FairnessWeight == 0 && len(value.GroupOverrides) == 0 {
		return defaults, nil
	}
	mode := strings.TrimSpace(strings.ToLower(value.CandidatePoolMode))
	if mode == "" {
		mode = defaults.CandidatePoolMode
	}
	switch mode {
	case OpenAISchedulerCandidatePoolModeTopK, OpenAISchedulerCandidatePoolModeAllEligible, OpenAISchedulerCandidatePoolModeHybrid:
	default:
		return OpenAISchedulerFairnessSettings{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_CANDIDATE_POOL_MODE", "candidate pool mode must be top_k, all_eligible, or hybrid")
	}
	ratio := value.ExplorationRatio
	if ratio == 0 && value.CandidatePoolMode == "" && value.StarvationThresholdSeconds == 0 && value.FairnessWeight == 0 {
		ratio = defaults.ExplorationRatio
	}
	if ratio < 0 || ratio > 100 {
		return OpenAISchedulerFairnessSettings{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_EXPLORATION_RATIO", "exploration ratio must be between 0 and 100")
	}
	threshold := value.StarvationThresholdSeconds
	if threshold == 0 && value.CandidatePoolMode == "" && value.ExplorationRatio == 0 && value.FairnessWeight == 0 {
		threshold = defaults.StarvationThresholdSeconds
	}
	if threshold != 0 && (threshold < 300 || threshold > 86400) {
		return OpenAISchedulerFairnessSettings{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_STARVATION_THRESHOLD", "starvation threshold must be 0 or between 300 and 86400 seconds")
	}
	weight := value.FairnessWeight
	if weight == 0 && value.CandidatePoolMode == "" && value.ExplorationRatio == 0 && value.StarvationThresholdSeconds == 0 {
		weight = defaults.FairnessWeight
	}
	if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight > 10 {
		return OpenAISchedulerFairnessSettings{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_FAIRNESS_WEIGHT", "fairness weight must be between 0 and 10")
	}
	overrides := make(map[int64]OpenAISchedulerFairnessOverride, len(value.GroupOverrides))
	for groupID, override := range value.GroupOverrides {
		if groupID <= 0 {
			return OpenAISchedulerFairnessSettings{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_OVERRIDE", "group override group id must be positive")
		}
		if err := validateOpenAISchedulerFairnessOverride(override); err != nil {
			return OpenAISchedulerFairnessSettings{}, err
		}
		overrides[groupID] = override
	}
	return OpenAISchedulerFairnessSettings{CandidatePoolMode: mode, ExplorationRatio: ratio, StarvationThresholdSeconds: threshold, FairnessWeight: weight, GroupOverrides: overrides}, nil
}

func parseIntSettingOrDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func normalizeOpenAISchedulerTopKForRead(raw string) string {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return ""
	}
	if value > 32 {
		value = 32
	}
	return strconv.Itoa(value)
}

func normalizeOpenAISchedulerWeightForRead(raw string) string {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	if value < 0 {
		value = 0
	} else if value > 10 {
		value = 10
	}
	return formatOpenAIAdvancedSchedulerFloat(value)
}

func parseFloatSettingOrDefault(raw string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fallback
	}
	return value
}

func normalizeOpenAISchedulerFairnessSettingsForRead(value OpenAISchedulerFairnessSettings) OpenAISchedulerFairnessSettings {
	defaults := defaultOpenAISchedulerFairnessSettings()
	mode := strings.TrimSpace(strings.ToLower(value.CandidatePoolMode))
	if mode != OpenAISchedulerCandidatePoolModeTopK && mode != OpenAISchedulerCandidatePoolModeAllEligible && mode != OpenAISchedulerCandidatePoolModeHybrid {
		mode = defaults.CandidatePoolMode
	}
	ratio := value.ExplorationRatio
	if ratio < 0 {
		ratio = 0
	} else if ratio > 100 {
		ratio = 100
	}
	threshold := value.StarvationThresholdSeconds
	if threshold != 0 {
		if threshold < 300 {
			threshold = 300
		} else if threshold > 86400 {
			threshold = 86400
		}
	}
	weight := value.FairnessWeight
	if math.IsNaN(weight) || math.IsInf(weight, 0) {
		weight = defaults.FairnessWeight
	} else if weight < 0 {
		weight = 0
	} else if weight > 10 {
		weight = 10
	}
	return OpenAISchedulerFairnessSettings{
		CandidatePoolMode:          mode,
		ExplorationRatio:           ratio,
		StarvationThresholdSeconds: threshold,
		FairnessWeight:             weight,
		GroupOverrides:             normalizeOpenAISchedulerFairnessOverridesForRead(value.GroupOverrides),
	}
}

func normalizeOpenAISchedulerFairnessOverridesForRead(values map[int64]OpenAISchedulerFairnessOverride) map[int64]OpenAISchedulerFairnessOverride {
	if len(values) == 0 {
		return map[int64]OpenAISchedulerFairnessOverride{}
	}
	result := make(map[int64]OpenAISchedulerFairnessOverride, len(values))
	for groupID, value := range values {
		if groupID <= 0 {
			continue
		}
		override := value
		if override.CandidatePoolMode != nil {
			mode := strings.ToLower(strings.TrimSpace(*override.CandidatePoolMode))
			if mode != OpenAISchedulerCandidatePoolModeTopK && mode != OpenAISchedulerCandidatePoolModeAllEligible && mode != OpenAISchedulerCandidatePoolModeHybrid {
				override.CandidatePoolMode = nil
			} else {
				*override.CandidatePoolMode = mode
			}
		}
		if override.ExplorationRatio != nil {
			value := *override.ExplorationRatio
			if value < 0 {
				value = 0
			} else if value > 100 {
				value = 100
			}
			*override.ExplorationRatio = value
		}
		if override.StarvationThresholdSeconds != nil {
			value := *override.StarvationThresholdSeconds
			if value != 0 {
				if value < 300 {
					value = 300
				} else if value > 86400 {
					value = 86400
				}
			}
			*override.StarvationThresholdSeconds = value
		}
		if override.FairnessWeight != nil {
			value := *override.FairnessWeight
			if math.IsNaN(value) || math.IsInf(value, 0) {
				override.FairnessWeight = nil
			} else {
				if value < 0 {
					value = 0
				} else if value > 10 {
					value = 10
				}
				*override.FairnessWeight = value
			}
		}
		result[groupID] = override
	}
	return result
}

func normalizeOpenAISchedulerGroupPoliciesForRead(policies map[int64]OpenAISchedulerGroupPolicy) map[int64]OpenAISchedulerGroupPolicy {
	result := make(map[int64]OpenAISchedulerGroupPolicy, len(policies))
	for id, policy := range policies {
		if policy.TopK != nil {
			value := *policy.TopK
			if value < 1 {
				value = 1
			} else if value > 32 {
				value = 32
			}
			policy.TopK = &value
		}
		if policy.WeightOverrides != nil {
			weights := make(map[string]float64, len(policy.WeightOverrides))
			for key, value := range policy.WeightOverrides {
				if !openAISchedulerPolicyWeightKeys[key] {
					continue
				}
				weights[key] = normalizeOpenAISchedulerWeightValueForRead(value, 0)
			}
			policy.WeightOverrides = weights
		}
		if policy.Fairness != nil {
			override := normalizeOpenAISchedulerFairnessOverridesForRead(map[int64]OpenAISchedulerFairnessOverride{id: *policy.Fairness})[id]
			policy.Fairness = &override
		}
		policy.QualityGate = normalizeOpenAISchedulerQualityGateForRead(policy.QualityGate)
		policy.SessionEscape = normalizeOpenAISchedulerSessionEscapeForRead(policy.SessionEscape)
		legacy := normalizeOpenAISchedulerFairnessOverridesForRead(map[int64]OpenAISchedulerFairnessOverride{id: policy.LegacyFairness})[id]
		policy.LegacyFairness = legacy
		if policy.Values.TopK != 0 || policy.Values.Priority != 0 || policy.Values.CandidatePoolMode != "" {
			policy.Values = normalizeOpenAISchedulerPresetValuesForRead(policy.Values)
		}
		result[id] = policy
	}
	return result
}

func parseOpenAISchedulerFairnessOverrides(raw string) map[int64]OpenAISchedulerFairnessOverride {
	result := map[int64]OpenAISchedulerFairnessOverride{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[int64]OpenAISchedulerFairnessOverride{}
	}
	return result
}

func parseOpenAISchedulerGroupPolicies(raw string) (map[int64]OpenAISchedulerGroupPolicy, error) {
	result := map[int64]OpenAISchedulerGroupPolicy{}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return result, nil
	}
	var objects map[int64]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &objects); err != nil {
		return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policies must be valid JSON")
	}
	for id, blob := range objects {
		if id <= 0 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy group id must be positive")
		}
		var legacy OpenAISchedulerFairnessOverride
		_ = json.Unmarshal(blob, &legacy)
		var rawFields map[string]json.RawMessage
		_ = json.Unmarshal(blob, &rawFields)
		var policy OpenAISchedulerGroupPolicy
		isLegacy := rawFields["candidate_pool_mode"] != nil || rawFields["exploration_ratio"] != nil || rawFields["starvation_threshold_seconds"] != nil || rawFields["fairness_weight"] != nil
		if isLegacy {
			policy.Mode = OpenAISchedulerGroupPolicyModeWeightedOverride
			policy.LegacyFairness = legacy
		} else {
			dec := json.NewDecoder(bytes.NewReader(blob))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&policy); err != nil {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy contains unknown or invalid fields")
			}
		}
		result[id] = policy
	}
	return result, nil
}

func parseOpenAISchedulerCustomPresets(raw string) (map[string]OpenAISchedulerCustomPreset, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return map[string]OpenAISchedulerCustomPreset{}, nil
	}
	var presets map[string]OpenAISchedulerCustomPreset
	if err := json.Unmarshal([]byte(raw), &presets); err != nil {
		return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESETS", "custom presets must be valid JSON")
	}
	return normalizeOpenAISchedulerCustomPresetsForRead(presets), nil
}

func normalizeOpenAISchedulerPresetValuesForRead(values OpenAISchedulerPolicyValues) OpenAISchedulerPolicyValues {
	defaults := openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	if values.TopK < 1 {
		values.TopK = 1
	} else if values.TopK > 32 {
		values.TopK = 32
	}
	values.Priority = normalizeOpenAISchedulerWeightValueForRead(values.Priority, defaults.Priority)
	values.Load = normalizeOpenAISchedulerWeightValueForRead(values.Load, defaults.Load)
	values.Queue = normalizeOpenAISchedulerWeightValueForRead(values.Queue, defaults.Queue)
	values.ErrorRate = normalizeOpenAISchedulerWeightValueForRead(values.ErrorRate, defaults.ErrorRate)
	values.TTFT = normalizeOpenAISchedulerWeightValueForRead(values.TTFT, defaults.TTFT)
	values.Reset = normalizeOpenAISchedulerWeightValueForRead(values.Reset, defaults.Reset)
	values.QuotaHeadroom = normalizeOpenAISchedulerWeightValueForRead(values.QuotaHeadroom, defaults.QuotaHeadroom)
	values.UpstreamCost = normalizeOpenAISchedulerWeightValueForRead(values.UpstreamCost, defaults.UpstreamCost)
	values.PreviousResponse = normalizeOpenAISchedulerWeightValueForRead(values.PreviousResponse, defaults.PreviousResponse)
	values.SessionSticky = normalizeOpenAISchedulerWeightValueForRead(values.SessionSticky, defaults.SessionSticky)
	mode := strings.ToLower(strings.TrimSpace(values.CandidatePoolMode))
	if mode != OpenAISchedulerCandidatePoolModeTopK && mode != OpenAISchedulerCandidatePoolModeAllEligible && mode != OpenAISchedulerCandidatePoolModeHybrid {
		mode = defaults.CandidatePoolMode
	}
	values.CandidatePoolMode = mode
	if values.ExplorationRatio < 0 {
		values.ExplorationRatio = 0
	} else if values.ExplorationRatio > 100 {
		values.ExplorationRatio = 100
	}
	if values.StarvationThresholdSeconds != 0 {
		if values.StarvationThresholdSeconds < 300 {
			values.StarvationThresholdSeconds = 300
		} else if values.StarvationThresholdSeconds > 86400 {
			values.StarvationThresholdSeconds = 86400
		}
	}
	values.FairnessWeight = normalizeOpenAISchedulerWeightValueForRead(values.FairnessWeight, defaults.FairnessWeight)
	return values
}

func normalizeOpenAISchedulerWeightValueForRead(value, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	if value < 0 {
		return 0
	}
	if value > 10 {
		return 10
	}
	return value
}

func normalizeOpenAISchedulerCustomPresetsForRead(presets map[string]OpenAISchedulerCustomPreset) map[string]OpenAISchedulerCustomPreset {
	result := make(map[string]OpenAISchedulerCustomPreset, len(presets))
	names := map[string]struct{}{}
	for key, preset := range presets {
		id := strings.TrimSpace(preset.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		parsed, err := uuid.Parse(strings.TrimPrefix(id, "custom:"))
		if !strings.HasPrefix(id, "custom:") || err != nil || parsed.String() != strings.TrimPrefix(id, "custom:") {
			continue
		}
		name := strings.TrimSpace(preset.Name)
		if len([]rune(name)) < 1 || len([]rune(name)) > 40 {
			continue
		}
		if _, exists := names[name]; exists {
			continue
		}
		names[name] = struct{}{}
		result[id] = OpenAISchedulerCustomPreset{ID: id, Name: name, Values: normalizeOpenAISchedulerPresetValuesForRead(preset.Values)}
	}
	return result
}

func normalizeOpenAISchedulerCustomPresets(presets map[string]OpenAISchedulerCustomPreset) (map[string]OpenAISchedulerCustomPreset, error) {
	result := make(map[string]OpenAISchedulerCustomPreset, len(presets))
	names := map[string]struct{}{}
	for key, preset := range presets {
		id := strings.TrimSpace(preset.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		if strings.HasPrefix(id, "custom:new:") {
			id = "custom:" + uuid.NewString()
		}
		parsed, err := uuid.Parse(strings.TrimPrefix(id, "custom:"))
		if !strings.HasPrefix(id, "custom:") || err != nil || parsed.String() != strings.TrimPrefix(id, "custom:") {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_ID", "custom preset id must be custom:<uuid>")
		}
		name := strings.TrimSpace(preset.Name)
		if len([]rune(name)) < 1 || len([]rune(name)) > 40 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_NAME", "custom preset name must be 1-40 characters")
		}
		if _, exists := names[name]; exists {
			return nil, infraerrors.BadRequest("DUPLICATE_OPENAI_SCHEDULER_PRESET_NAME", "custom preset names must be unique")
		}
		names[name] = struct{}{}
		values, err := normalizeOpenAISchedulerPresetValues(preset.Values)
		if err != nil {
			return nil, err
		}
		result[id] = OpenAISchedulerCustomPreset{ID: id, Name: name, Values: values}
	}
	return result, nil
}

func normalizeOpenAISchedulerPresetValues(values OpenAISchedulerPolicyValues) (OpenAISchedulerPolicyValues, error) {
	if values.TopK < 1 || values.TopK > 32 {
		return OpenAISchedulerPolicyValues{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_VALUES", "top_k must be between 1 and 32")
	}
	weights := []float64{values.Priority, values.Load, values.Queue, values.ErrorRate, values.TTFT, values.Reset, values.QuotaHeadroom, values.UpstreamCost, values.PreviousResponse, values.SessionSticky}
	sum := 0.0
	for _, value := range weights {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 10 {
			return OpenAISchedulerPolicyValues{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_VALUES", "weights must be finite values between 0 and 10")
		}
		sum += value
	}
	if sum == 0 {
		return OpenAISchedulerPolicyValues{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_VALUES", "base weights must have a positive total")
	}
	if values.CandidatePoolMode != OpenAISchedulerCandidatePoolModeTopK && values.CandidatePoolMode != OpenAISchedulerCandidatePoolModeAllEligible && values.CandidatePoolMode != OpenAISchedulerCandidatePoolModeHybrid {
		return OpenAISchedulerPolicyValues{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_VALUES", "candidate pool mode is invalid")
	}
	if values.ExplorationRatio < 0 || values.ExplorationRatio > 100 || (values.StarvationThresholdSeconds != 0 && (values.StarvationThresholdSeconds < 300 || values.StarvationThresholdSeconds > 86400)) || values.FairnessWeight < 0 || values.FairnessWeight > 10 || math.IsNaN(values.FairnessWeight) || math.IsInf(values.FairnessWeight, 0) {
		return OpenAISchedulerPolicyValues{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_PRESET_VALUES", "fairness values are invalid")
	}
	return values, nil
}

func openAISchedulerAvailablePresets(custom map[string]OpenAISchedulerCustomPreset) []OpenAISchedulerPresetDefinition {
	defs := []OpenAISchedulerPresetDefinition{{ID: "builtin:special_offer", Name: "体验优先", Kind: OpenAISchedulerPresetKindBuiltin, Values: openAISchedulerPresetValues(OpenAISchedulerPresetSpecialOffer)}, {ID: "builtin:balanced", Name: "体验均衡", Kind: OpenAISchedulerPresetKindBuiltin, Values: openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)}, {ID: "builtin:pro", Name: "利润优先", Kind: OpenAISchedulerPresetKindBuiltin, Values: openAISchedulerPresetValues(OpenAISchedulerPresetPro)}}
	keys := make([]string, 0, len(custom))
	for key := range custom {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p := custom[key]
		defs = append(defs, OpenAISchedulerPresetDefinition{ID: p.ID, Name: p.Name, Kind: OpenAISchedulerPresetKindCustom, Values: p.Values})
	}
	return defs
}

func normalizeOpenAISchedulerPresetState(presets map[string]OpenAISchedulerCustomPreset, policies map[int64]OpenAISchedulerGroupPolicy, previous map[string]OpenAISchedulerCustomPreset, global OpenAISchedulerPolicyValues, knownGroups map[int64]struct{}) (map[string]OpenAISchedulerCustomPreset, map[int64]OpenAISchedulerGroupPolicy, error) {
	normalized, err := normalizeOpenAISchedulerCustomPresets(presets)
	if err != nil {
		return nil, nil, err
	}
	for id := range previous {
		if _, ok := normalized[id]; !ok {
			for _, policy := range policies {
				if policy.PresetID == id {
					return nil, nil, infraerrors.BadRequest("OPENAI_SCHEDULER_PRESET_IN_USE", "cannot delete a preset referenced by a group policy")
				}
			}
		}
	}
	groups, err := normalizeOpenAISchedulerGroupPoliciesWithPresets(policies, global, knownGroups, normalized)
	if err != nil {
		return nil, nil, err
	}
	return normalized, groups, nil
}

// ValidateOpenAISchedulerPresetUpdate rejects deleting a custom preset while a
// group policy still points at it. Callers should run this before the combined
// settings transaction so neither settings key is written on failure.
func ValidateOpenAISchedulerPresetUpdate(previous, next map[string]OpenAISchedulerCustomPreset, policies map[int64]OpenAISchedulerGroupPolicy) error {
	for id := range previous {
		if _, ok := next[id]; ok {
			continue
		}
		for _, policy := range policies {
			if policy.PresetID == id {
				return infraerrors.BadRequest("OPENAI_SCHEDULER_PRESET_IN_USE", "cannot delete a preset referenced by a group policy")
			}
		}
	}
	return nil
}

func normalizeOpenAISchedulerGroupPoliciesWithPresets(policies map[int64]OpenAISchedulerGroupPolicy, global OpenAISchedulerPolicyValues, knownGroups map[int64]struct{}, custom map[string]OpenAISchedulerCustomPreset) (map[int64]OpenAISchedulerGroupPolicy, error) {
	result := make(map[int64]OpenAISchedulerGroupPolicy, len(policies))
	for id, policy := range policies {
		if id <= 0 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy group id must be positive")
		}
		if len(knownGroups) > 0 {
			if _, ok := knownGroups[id]; !ok {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy references unknown group")
			}
		}
		if policy.QualityGate != nil && !validateOpenAISchedulerQualityGatePolicy(*policy.QualityGate) {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy quality gate is invalid")
		}
		if policy.SessionEscape != nil && !validateOpenAISchedulerSessionEscapePolicy(*policy.SessionEscape) {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy session escape is invalid")
		}
		if policy.Priority != (OpenAISchedulerBusinessPriority{}) {
			business, err := parseOpenAISchedulerBusinessPolicy(policy)
			if err != nil {
				return nil, err
			}
			values, err := compileOpenAISchedulerBusinessPolicy(business, global)
			if err != nil {
				return nil, err
			}
			policy.Mode = OpenAISchedulerGroupPolicyModeCustom
			policy.Preset = ""
			policy.PresetID = ""
			policy.Values = values
			policy.CompiledSnapshot = values
			policy.Operations = business.Operations
			result[id] = policy
			continue
		}
		if policy.Mode == OpenAISchedulerGroupPolicyModeWeightedOverride {
			policy.Mode = OpenAISchedulerGroupPolicyModeCustom
		}
		if policy.Mode == OpenAISchedulerGroupPolicyModeFair {
			policy.Mode = OpenAISchedulerGroupPolicyModePreset
			policy.PresetID = "builtin:" + string(policy.Preset)
		}
		if policy.Mode == OpenAISchedulerGroupPolicyModePreset {
			if policy.PresetID == "" {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "preset mode requires preset_id")
			}
			values, ok := schedulerPresetValuesByID(policy.PresetID, custom)
			if !ok {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "unknown preset")
			}
			values = applyOpenAISchedulerGroupPolicySnapshot(values, policy)
			if _, err := normalizeOpenAISchedulerPresetValues(values); err != nil {
				return nil, err
			}
			policy.Values = values
			result[id] = policy
			continue
		}
		if policy.Mode == "" {
			policy.Mode = OpenAISchedulerGroupPolicyModeCustom
		}
		if policy.Mode != OpenAISchedulerGroupPolicyModeCustom {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy mode is invalid")
		}
		policy.Mode = OpenAISchedulerGroupPolicyModeWeightedOverride
		normalized, err := normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{id: policy}, global, knownGroups)
		if err != nil {
			return nil, err
		}
		policy = normalized[id]
		policy.Mode = OpenAISchedulerGroupPolicyModeCustom
		policy.Preset = ""
		policy.PresetID = ""
		result[id] = policy
	}
	return result, nil
}

func applyOpenAISchedulerGroupPolicySnapshot(values OpenAISchedulerPolicyValues, policy OpenAISchedulerGroupPolicy) OpenAISchedulerPolicyValues {
	if policy.TopK != nil {
		values.TopK = *policy.TopK
	}
	for key, value := range policy.WeightOverrides {
		switch key {
		case "priority":
			values.Priority = value
		case "load":
			values.Load = value
		case "queue":
			values.Queue = value
		case "error_rate":
			values.ErrorRate = value
		case "ttft":
			values.TTFT = value
		case "reset":
			values.Reset = value
		case "quota_headroom":
			values.QuotaHeadroom = value
		case "upstream_cost":
			values.UpstreamCost = value
		case "previous_response":
			values.PreviousResponse = value
		case "session_sticky":
			values.SessionSticky = value
		}
	}
	if fairness := policy.Fairness; fairness != nil {
		if fairness.CandidatePoolMode != nil {
			values.CandidatePoolMode = *fairness.CandidatePoolMode
		}
		if fairness.ExplorationRatio != nil {
			values.ExplorationRatio = *fairness.ExplorationRatio
		}
		if fairness.StarvationThresholdSeconds != nil {
			values.StarvationThresholdSeconds = *fairness.StarvationThresholdSeconds
		}
		if fairness.FairnessWeight != nil {
			values.FairnessWeight = *fairness.FairnessWeight
		}
	}
	return values
}

func schedulerPresetValuesByID(id string, custom map[string]OpenAISchedulerCustomPreset) (OpenAISchedulerPolicyValues, bool) {
	switch id {
	case "builtin:special_offer":
		return openAISchedulerPresetValues(OpenAISchedulerPresetSpecialOffer), true
	case "builtin:balanced":
		return openAISchedulerPresetValues(OpenAISchedulerPresetBalanced), true
	case "builtin:pro":
		return openAISchedulerPresetValues(OpenAISchedulerPresetPro), true
	}
	p, ok := custom[id]
	return p.Values, ok
}

func normalizeOpenAISchedulerGroupPolicies(policies map[int64]OpenAISchedulerGroupPolicy, global OpenAISchedulerPolicyValues, knownGroups map[int64]struct{}) (map[int64]OpenAISchedulerGroupPolicy, error) {
	result := make(map[int64]OpenAISchedulerGroupPolicy, len(policies))
	for id, policy := range policies {
		if id <= 0 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy group id must be positive")
		}
		if len(knownGroups) > 0 {
			if _, ok := knownGroups[id]; !ok {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy references unknown group")
			}
		}
		if policy.QualityGate != nil && !validateOpenAISchedulerQualityGatePolicy(*policy.QualityGate) {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy quality gate is invalid")
		}
		if policy.SessionEscape != nil && !validateOpenAISchedulerSessionEscapePolicy(*policy.SessionEscape) {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy session escape is invalid")
		}
		if policy.Mode == "" {
			policy.Mode = OpenAISchedulerGroupPolicyModeWeightedOverride
		}
		if policy.Mode != OpenAISchedulerGroupPolicyModeWeightedOverride && policy.Mode != OpenAISchedulerGroupPolicyModeFair {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy mode is invalid")
		}
		values := global
		if policy.Fairness != nil {
			if err := validateOpenAISchedulerFairnessOverride(*policy.Fairness); err != nil {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy fairness override is invalid")
			}
		}
		if policy.Mode == OpenAISchedulerGroupPolicyModeFair {
			if policy.Preset != OpenAISchedulerPresetSpecialOffer && policy.Preset != OpenAISchedulerPresetBalanced && policy.Preset != OpenAISchedulerPresetPro {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "fair group policy preset is invalid")
			}
			values = openAISchedulerPresetValues(policy.Preset)
			if policy.Fairness != nil {
				if policy.Fairness.CandidatePoolMode != nil {
					values.CandidatePoolMode = *policy.Fairness.CandidatePoolMode
				}
				if policy.Fairness.ExplorationRatio != nil {
					values.ExplorationRatio = *policy.Fairness.ExplorationRatio
				}
				if policy.Fairness.StarvationThresholdSeconds != nil {
					values.StarvationThresholdSeconds = *policy.Fairness.StarvationThresholdSeconds
				}
				if policy.Fairness.FairnessWeight != nil {
					values.FairnessWeight = *policy.Fairness.FairnessWeight
				}
			}
		} else if policy.Preset != "" {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "weighted group policy cannot set a preset")
		}
		if policy.TopK != nil {
			if *policy.TopK < 1 || *policy.TopK > 32 {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy TopK must be between 1 and 32")
			}
			values.TopK = *policy.TopK
		}
		for key, value := range policy.WeightOverrides {
			if !openAISchedulerPolicyWeightKeys[key] || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 10 {
				return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy weight is invalid")
			}
			switch key {
			case "priority":
				values.Priority = value
			case "load":
				values.Load = value
			case "queue":
				values.Queue = value
			case "error_rate":
				values.ErrorRate = value
			case "ttft":
				values.TTFT = value
			case "reset":
				values.Reset = value
			case "quota_headroom":
				values.QuotaHeadroom = value
			case "upstream_cost":
				values.UpstreamCost = value
			case "previous_response":
				values.PreviousResponse = value
			case "session_sticky":
				values.SessionSticky = value
			}
		}
		if values.TopK <= 0 || values.TopK > 32 || values.Priority+values.Load+values.Queue+values.ErrorRate+values.TTFT+values.Reset+values.QuotaHeadroom+values.UpstreamCost <= 0 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy weights must have a positive total")
		}
		if values.ExplorationRatio < 0 || values.ExplorationRatio > 100 || (values.StarvationThresholdSeconds != 0 && (values.StarvationThresholdSeconds < 300 || values.StarvationThresholdSeconds > 86400)) || values.FairnessWeight < 0 || values.FairnessWeight > 10 {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_POLICY", "group policy fairness values are invalid")
		}
		policy.Values = values
		result[id] = policy
	}
	return result, nil
}

var openAISchedulerPolicyWeightKeys = map[string]bool{"priority": true, "load": true, "queue": true, "error_rate": true, "ttft": true, "reset": true, "quota_headroom": true, "upstream_cost": true, "previous_response": true, "session_sticky": true}

func openAISchedulerPresetValues(p OpenAISchedulerPreset) OpenAISchedulerPolicyValues {
	v := OpenAISchedulerPolicyValues{TopK: 7, Priority: 1, Load: 1, Queue: .7, ErrorRate: .8, TTFT: .5, PreviousResponse: 5, SessionSticky: 3, CandidatePoolMode: OpenAISchedulerCandidatePoolModeHybrid, ExplorationRatio: 25, StarvationThresholdSeconds: 21600, FairnessWeight: 3}
	switch p {
	case OpenAISchedulerPresetSpecialOffer:
		v.Priority = .8
		v.Load = .8
		v.Queue = .5
		v.ErrorRate = .8
		v.TTFT = .2
		v.UpstreamCost = 2.5
		v.ExplorationRatio = 15
		v.FairnessWeight = 2
	case OpenAISchedulerPresetPro:
		v.TopK = 10
		v.Priority = 1.2
		v.Load = 1.4
		v.Queue = 1.2
		v.ErrorRate = 2.5
		v.TTFT = 2
		v.Reset = .5
		v.QuotaHeadroom = .2
		v.UpstreamCost = 1.5
		v.ExplorationRatio = 40
		v.StarvationThresholdSeconds = 10800
		v.FairnessWeight = 5
	}
	return v
}

func resolveOpenAISchedulerPolicyForGroup(policies map[int64]OpenAISchedulerGroupPolicy, global OpenAISchedulerPolicyValues, groupID int64) OpenAISchedulerGroupPolicy {
	if p, ok := policies[groupID]; ok {
		return p
	}
	return OpenAISchedulerGroupPolicy{Mode: OpenAISchedulerGroupPolicyModeWeightedOverride, Values: global}
}

func validateOpenAISchedulerFairnessOverride(value OpenAISchedulerFairnessOverride) error {
	if value.CandidatePoolMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*value.CandidatePoolMode))
		if mode != OpenAISchedulerCandidatePoolModeTopK && mode != OpenAISchedulerCandidatePoolModeAllEligible && mode != OpenAISchedulerCandidatePoolModeHybrid {
			return infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_OVERRIDE", "group override candidate pool mode is invalid")
		}
	}
	if value.ExplorationRatio != nil && (*value.ExplorationRatio < 0 || *value.ExplorationRatio > 100) {
		return infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_OVERRIDE", "group override exploration ratio must be between 0 and 100")
	}
	if value.StarvationThresholdSeconds != nil && *value.StarvationThresholdSeconds != 0 && (*value.StarvationThresholdSeconds < 300 || *value.StarvationThresholdSeconds > 86400) {
		return infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_OVERRIDE", "group override starvation threshold is invalid")
	}
	if value.FairnessWeight != nil && (math.IsNaN(*value.FairnessWeight) || math.IsInf(*value.FairnessWeight, 0) || *value.FairnessWeight < 0 || *value.FairnessWeight > 10) {
		return infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_GROUP_OVERRIDE", "group override fairness weight is invalid")
	}
	return nil
}

func resolveOpenAISchedulerFairnessForGroup(value OpenAISchedulerFairnessSettings, groupID int64) OpenAISchedulerFairnessSettings {
	resolved := value
	resolved.GroupOverrides = nil
	if override, ok := value.GroupOverrides[groupID]; ok {
		if override.CandidatePoolMode != nil {
			resolved.CandidatePoolMode = *override.CandidatePoolMode
		}
		if override.ExplorationRatio != nil {
			resolved.ExplorationRatio = *override.ExplorationRatio
		}
		if override.StarvationThresholdSeconds != nil {
			resolved.StarvationThresholdSeconds = *override.StarvationThresholdSeconds
		}
		if override.FairnessWeight != nil {
			resolved.FairnessWeight = *override.FairnessWeight
		}
	}
	return resolved
}

func parseOpenAIOAuthSchedulingRateMultiplier(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return defaultOpenAIOAuthSchedulingRateMultiplier
	}
	return value
}

// resolveOpenAIAdvancedSchedulerWeight 返回覆盖值（已归一化的非空字符串），空则回退默认值。
func resolveOpenAIAdvancedSchedulerWeight(normalized string, fallback float64) float64 {
	if normalized == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return fallback
	}
	return value
}

func normalizeOptionalPositiveIntString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return "", fmt.Errorf("invalid positive integer")
	}
	return strconv.Itoa(value), nil
}

func normalizeOptionalNonNegativeFloatString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("invalid non-negative float")
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func parseDefaultSubscriptions(raw string) []DefaultSubscriptionSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var items []DefaultSubscriptionSetting
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}

	normalized := make([]DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
		}
		if item.ValidityDays > MaxValidityDays {
			item.ValidityDays = MaxValidityDays
		}
		normalized = append(normalized, item)
	}

	return normalized
}

func parseProviderDefaultGrantSettings(settings map[string]string, keys authSourceDefaultKeySet) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          defaultAuthSourceBalance,
		Concurrency:      defaultAuthSourceConcurrency,
		Subscriptions:    []DefaultSubscriptionSetting{},
		GrantOnSignup:    false,
		GrantOnFirstBind: false,
	}

	if v, err := strconv.ParseFloat(strings.TrimSpace(settings[keys.balance]), 64); err == nil {
		result.Balance = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(settings[keys.concurrency])); err == nil {
		result.Concurrency = v
	}
	if items := parseDefaultSubscriptions(settings[keys.subscriptions]); items != nil {
		result.Subscriptions = items
	}
	if raw, ok := settings[keys.grantOnSignup]; ok {
		result.GrantOnSignup = raw == "true"
	}
	if raw, ok := settings[keys.grantOnFirstBind]; ok {
		result.GrantOnFirstBind = raw == "true"
	}

	if raw := settings[keys.platformQuotas]; raw != "" {
		parsed := map[string]*DefaultPlatformQuotaSetting{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("[Setting] parseProviderDefaultGrantSettings: unmarshal auth source platform quotas failed", "source", keys.source, "error", err)
		} else {
			result.PlatformQuotas = parsed
		}
	}

	return result
}

func writeProviderDefaultGrantUpdates(updates map[string]string, keys authSourceDefaultKeySet, settings ProviderDefaultGrantSettings) {
	updates[keys.balance] = strconv.FormatFloat(settings.Balance, 'f', 8, 64)
	updates[keys.concurrency] = strconv.Itoa(settings.Concurrency)

	subscriptions := settings.Subscriptions
	if subscriptions == nil {
		subscriptions = []DefaultSubscriptionSetting{}
	}
	raw, err := json.Marshal(subscriptions)
	if err != nil {
		raw = []byte("[]")
	}
	updates[keys.subscriptions] = string(raw)
	updates[keys.grantOnSignup] = strconv.FormatBool(settings.GrantOnSignup)
	updates[keys.grantOnFirstBind] = strconv.FormatBool(settings.GrantOnFirstBind)

	// auth source platform quota：整体替换语义。
	// nil = 请求未携带该字段，跳过写入以保留既有配置（与系统层 buildSystemSettingsUpdates 的
	// DefaultPlatformQuotas nil 守卫一致）；非 nil（含空 map）才整体替换。二者语义不可混同。
	if keys.platformQuotas != "" && settings.PlatformQuotas != nil {
		blob, err := json.Marshal(settings.PlatformQuotas)
		if err != nil {
			blob = []byte("{}")
		}
		updates[keys.platformQuotas] = string(blob)
	}
}

func mergeProviderDefaultGrantSettings(globalDefaults ProviderDefaultGrantSettings, providerDefaults ProviderDefaultGrantSettings) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          globalDefaults.Balance,
		Concurrency:      globalDefaults.Concurrency,
		Subscriptions:    append([]DefaultSubscriptionSetting(nil), globalDefaults.Subscriptions...),
		GrantOnSignup:    providerDefaults.GrantOnSignup,
		GrantOnFirstBind: providerDefaults.GrantOnFirstBind,
	}

	// 注意：不能把 parse 默认值 (defaultAuthSourceBalance / defaultAuthSourceConcurrency)
	// 当作"未配置"哨兵——admin 完全有权显式设成相同的值，那时仍应覆盖 globalDefaults。
	// 旧实现的 `!= defaultAuthSourceConcurrency` 会把 admin 设的 5 与 fallback 5 混淆，
	// 导致渠道发放退回到全局默认（如 1），表现为"管理员设 5、新用户实际拿 1"。
	if providerDefaults.Balance >= 0 {
		result.Balance = providerDefaults.Balance
	}
	if providerDefaults.Concurrency > 0 {
		result.Concurrency = providerDefaults.Concurrency
	}
	if len(providerDefaults.Subscriptions) > 0 {
		result.Subscriptions = append([]DefaultSubscriptionSetting(nil), providerDefaults.Subscriptions...)
	}

	return result
}

func parseTablePreferences(defaultPageSizeRaw, optionsRaw string) (int, []int) {
	defaultPageSize := 20
	if v, err := strconv.Atoi(strings.TrimSpace(defaultPageSizeRaw)); err == nil {
		defaultPageSize = v
	}

	var options []int
	if strings.TrimSpace(optionsRaw) != "" {
		_ = json.Unmarshal([]byte(optionsRaw), &options)
	}

	return normalizeTablePreferences(defaultPageSize, options)
}

func normalizeTablePreferences(defaultPageSize int, options []int) (int, []int) {
	const minPageSize = 5
	const maxPageSize = 1000
	const fallbackPageSize = 20

	seen := make(map[int]struct{}, len(options))
	normalizedOptions := make([]int, 0, len(options))
	for _, option := range options {
		if option < minPageSize || option > maxPageSize {
			continue
		}
		if _, ok := seen[option]; ok {
			continue
		}
		seen[option] = struct{}{}
		normalizedOptions = append(normalizedOptions, option)
	}
	sort.Ints(normalizedOptions)

	if defaultPageSize < minPageSize || defaultPageSize > maxPageSize {
		defaultPageSize = fallbackPageSize
	}

	if len(normalizedOptions) == 0 {
		normalizedOptions = []int{10, 20, 50}
	}

	return defaultPageSize, normalizedOptions
}
func normalizeOpenAISchedulerBusinessPriority(value OpenAISchedulerBusinessPriority) (OpenAISchedulerBusinessPriority, error) {
	if value.Profit < 1 || value.Profit > 3 || value.TTFT < 1 || value.TTFT > 3 || value.Latency < 1 || value.Latency > 3 {
		return OpenAISchedulerBusinessPriority{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_BUSINESS_PRIORITY", "business priorities must be integers between 1 and 3")
	}
	return value, nil
}

func normalizeOpenAISchedulerOperations(value OpenAISchedulerOperations) (OpenAISchedulerOperations, error) {
	if strings.TrimSpace(value.Balance) == "" {
		value.Balance = "standard"
	}
	if strings.TrimSpace(value.PeakProtection) == "" {
		value.PeakProtection = "strict"
	}
	if strings.TrimSpace(value.SessionContinuity) == "" {
		value.SessionContinuity = "standard"
	}
	if value.Balance != "low" && value.Balance != "standard" && value.Balance != "high" {
		return OpenAISchedulerOperations{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_OPERATIONS", "balance mode is invalid")
	}
	if value.PeakProtection != "strict" && value.PeakProtection != "standard" && value.PeakProtection != "open" {
		return OpenAISchedulerOperations{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_OPERATIONS", "peak protection mode is invalid")
	}
	if value.SessionContinuity != "keep" && value.SessionContinuity != "standard" && value.SessionContinuity != "switch" {
		return OpenAISchedulerOperations{}, infraerrors.BadRequest("INVALID_OPENAI_SCHEDULER_OPERATIONS", "session continuity mode is invalid")
	}
	return value, nil
}

func recommendedOpenAISchedulerBusinessPolicy(groupName string) OpenAISchedulerBusinessGroupPolicy {
	priority := OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1}
	switch strings.TrimSpace(groupName) {
	case "GPT-特惠":
		priority = OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 2, Latency: 3}
	case "GPT-Pro", "【专属】GPT-PRO":
		priority = OpenAISchedulerBusinessPriority{Profit: 3, TTFT: 1, Latency: 2}
	}
	return OpenAISchedulerBusinessGroupPolicy{Priority: priority, Operations: OpenAISchedulerOperations{Balance: "standard", PeakProtection: "strict", SessionContinuity: "standard"}}
}

func schedulerBusinessTierFactor(priority int) float64 {
	switch priority {
	case 1:
		return 3
	case 2:
		return 1.5
	default:
		return 0.5
	}
}

func compileOpenAISchedulerBusinessPolicy(policy OpenAISchedulerBusinessGroupPolicy, base OpenAISchedulerPolicyValues) (OpenAISchedulerPolicyValues, error) {
	priority, err := normalizeOpenAISchedulerBusinessPriority(policy.Priority)
	if err != nil {
		return OpenAISchedulerPolicyValues{}, err
	}
	operations, err := normalizeOpenAISchedulerOperations(policy.Operations)
	if err != nil {
		return OpenAISchedulerPolicyValues{}, err
	}
	if base.TopK <= 0 {
		base = openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	}
	if base.CandidatePoolMode == "" {
		base.CandidatePoolMode = OpenAISchedulerCandidatePoolModeHybrid
	}
	base.UpstreamCost = schedulerBusinessTierFactor(priority.Profit)
	base.TTFT = schedulerBusinessTierFactor(priority.TTFT)
	latencyFactor := schedulerBusinessTierFactor(priority.Latency)
	base.Load = latencyFactor
	base.Queue = latencyFactor
	switch operations.Balance {
	case "low":
		base.CandidatePoolMode = OpenAISchedulerCandidatePoolModeTopK
		base.ExplorationRatio = 0
	case "high":
		base.CandidatePoolMode = OpenAISchedulerCandidatePoolModeAllEligible
		if base.ExplorationRatio < 40 {
			base.ExplorationRatio = 40
		}
	}
	switch operations.PeakProtection {
	case "strict":
		base.ExplorationRatio = 0
	case "open":
		base.CandidatePoolMode = OpenAISchedulerCandidatePoolModeAllEligible
		if base.ExplorationRatio < 40 {
			base.ExplorationRatio = 40
		}
	}
	switch operations.SessionContinuity {
	case "keep":
		if base.PreviousResponse < 5 {
			base.PreviousResponse = 5
		}
		if base.SessionSticky < 4 {
			base.SessionSticky = 4
		}
	case "switch":
		if base.PreviousResponse > 1 {
			base.PreviousResponse = 1
		}
		if base.SessionSticky > 1 {
			base.SessionSticky = 1
		}
	}
	return base, nil
}

func parseOpenAISchedulerBusinessPolicy(legacy OpenAISchedulerGroupPolicy) (OpenAISchedulerBusinessGroupPolicy, error) {
	if legacy.Priority != (OpenAISchedulerBusinessPriority{}) {
		priority, err := normalizeOpenAISchedulerBusinessPriority(legacy.Priority)
		if err != nil {
			return OpenAISchedulerBusinessGroupPolicy{}, err
		}
		operations, err := normalizeOpenAISchedulerOperations(legacy.Operations)
		if err != nil {
			return OpenAISchedulerBusinessGroupPolicy{}, err
		}
		return OpenAISchedulerBusinessGroupPolicy{Priority: priority, Operations: operations, CompiledSnapshot: legacy.CompiledSnapshot}, nil
	}
	recommendation := recommendedOpenAISchedulerBusinessPolicy("")
	switch legacy.Preset {
	case OpenAISchedulerPresetSpecialOffer:
		recommendation = recommendedOpenAISchedulerBusinessPolicy("GPT-特惠")
	case OpenAISchedulerPresetPro:
		recommendation = recommendedOpenAISchedulerBusinessPolicy("GPT-Pro")
	}
	recommendation.CompiledSnapshot = legacy.Values
	return recommendation, nil
}
