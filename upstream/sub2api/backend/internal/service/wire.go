package service

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	upstreamnotify "github.com/Wei-Shaw/sub2api/internal/notify"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauthobs"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualityqueue"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func ProvideGrokOAuthService(proxyRepo ProxyRepository, oauthClient GrokOAuthClient, cfg *config.Config, redisClient *redis.Client) *GrokOAuthService {
	svc := NewGrokOAuthService(proxyRepo, oauthClient, cfg)
	// wire.go is depguard-exempt for redis; construct the Redis session store here.
	if redisClient != nil {
		svc = svc.WithSessionStore(xai.NewRedisSessionStore(redisClient))
	}
	return svc
}

// BuildInfo contains build information
type BuildInfo struct {
	Version   string
	BuildType string
}

// ShouldStartRequestLocal reports whether work owned by requests accepted by
// this process should run for the configured process role.
func ShouldStartRequestLocal(role config.ProcessRole) bool {
	return role == config.ProcessRoleAll || role == config.ProcessRoleAPI
}

// ShouldStartSingleton reports whether process-wide schedulers and shared
// queue consumers should run for the configured process role.
func ShouldStartSingleton(role config.ProcessRole) bool {
	return role == config.ProcessRoleAll || role == config.ProcessRoleWorker
}

func configuredProcessRole(cfg *config.Config) config.ProcessRole {
	if cfg == nil || cfg.Server.ProcessRole == "" {
		return config.ProcessRoleAll
	}
	return cfg.Server.ProcessRole
}

func shouldStartRequestLocal(cfg *config.Config) bool {
	return ShouldStartRequestLocal(configuredProcessRole(cfg))
}

func shouldStartSingleton(cfg *config.Config) bool {
	return ShouldStartSingleton(configuredProcessRole(cfg))
}

// ProvidePricingService creates and initializes PricingService
func ProvidePricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if !shouldStartSingleton(cfg) {
		if err := svc.InitializeReadOnly(); err != nil {
			println("[Service] Warning: read-only pricing initialization failed:", err.Error())
		}
		return svc, nil
	}
	if err := svc.Initialize(); err != nil {
		// Pricing service initialization failure should not block startup, use fallback prices
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
	}
	return svc, nil
}

// ProvideUpdateService creates UpdateService with BuildInfo
func ProvideUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, buildInfo BuildInfo) *UpdateService {
	return NewUpdateService(cache, githubClient, buildInfo.Version, buildInfo.BuildType)
}

// ProvideEmailQueueService creates EmailQueueService with default worker count

func ProvideEmailQueueService(emailService *EmailService, cfg *config.Config) *EmailQueueService {
	if !shouldStartRequestLocal(cfg) {
		return &EmailQueueService{
			emailService: emailService,
			taskChan:     make(chan EmailTask, 100),
			stopChan:     make(chan struct{}),
			workers:      3,
		}
	}
	return NewEmailQueueService(emailService, 3)
}

// ProvideAuthService wires the optional captcha providers into AuthService while
// keeping NewAuthService's public constructor compatible with existing tests.
func ProvideAuthService(
	entClient *dbent.Client,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	refreshTokenCache RefreshTokenCache,
	cfg *config.Config,
	settingService *SettingService,
	emailService *EmailService,
	turnstileService *TurnstileService,
	tencentCaptchaService *TencentCaptchaService,
	aliyunCaptchaService *AliyunCaptchaService,
	emailQueueService *EmailQueueService,
	promoService *PromoService,
	defaultSubAssigner DefaultSubscriptionAssigner,
	affiliateService *AffiliateService,
	userPlatformQuotaRepo UserPlatformQuotaRepository, wallets ...QuotaWalletService,
) *AuthService {
	svc := NewAuthService(
		entClient,
		userRepo,
		redeemRepo,
		refreshTokenCache,
		cfg,
		settingService,
		emailService,
		turnstileService,
		emailQueueService,
		promoService,
		defaultSubAssigner,
		affiliateService,
		userPlatformQuotaRepo,
		wallets...,
	)
	svc.SetTencentCaptchaService(tencentCaptchaService)
	svc.SetAliyunCaptchaService(aliyunCaptchaService)
	return svc
}

// ProvideOAuthRefreshAPI creates OAuthRefreshAPI with the default lock TTL.
func ProvideOAuthRefreshAPI(accountRepo AccountRepository, tokenCache GeminiTokenCache) *OAuthRefreshAPI {
	return NewOAuthRefreshAPI(accountRepo, tokenCache)
}

func ProvideBatchImageModelPricingResolver(resolver *ModelPricingResolver) *BatchImageModelPricingResolver {
	return &BatchImageModelPricingResolver{Resolver: resolver}
}

func ProvideBatchImageCleanupService(repo BatchImageRepository, accountRepo AccountRepository, cfg *config.Config) *BatchImageCleanupService {
	svc := NewBatchImageCleanupService(repo, accountRepo, cfg)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpenAIOAuthService creates OpenAIOAuthService with privacy/account enrichment support.
func ProvideOpenAIOAuthService(
	proxyRepo ProxyRepository,
	oauthClient OpenAIOAuthClient,
	privacyClientFactory PrivacyClientFactory,
) *OpenAIOAuthService {
	svc := NewOpenAIOAuthService(proxyRepo, oauthClient)
	svc.SetPrivacyClientFactory(privacyClientFactory)
	return svc
}

// ProvideOpenAIOAuthReauthService wires the durable mailbox/task flow while
// keeping the pure HTTP protocol runner in a separately run worker process.
func ProvideOpenAIOAuthReauthService(
	repo OpenAIOAuthReauthRepository,
	adminService AdminService,
	accountRepo AccountRepository,
	openaiOAuthService *OpenAIOAuthService,
	secretEncryptor OpenAICredentialEncryptor,
	cfg *config.Config,
	tokenCacheInvalidator TokenCacheInvalidator,
	runtimeBlocker AccountRuntimeBlocker,
	buildInfo BuildInfo,
	settings SettingRepository,
) *OpenAIOAuthReauthService {
	credentialUpdater, _ := accountRepo.(OpenAIOAuthReauthCredentialUpdater)
	svc := NewOpenAIOAuthReauthService(repo, adminService, credentialUpdater, openaiOAuthService, secretEncryptor, cfg != nil && cfg.Totp.EncryptionKeyConfigured, tokenCacheInvalidator, runtimeBlocker)
	svc.settings = settings
	if admin, ok := adminService.(*adminServiceImpl); ok {
		admin.excelOAuthReauth = svc
	}
	svc.configureWorker(cfg, buildInfo)
	if gateway, ok := runtimeBlocker.(*OpenAIGatewayService); ok {
		gateway.excelOAuthReauth = svc
	}
	return svc
}

// ProvideTokenRefreshService creates and starts TokenRefreshService
func ProvideTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	grokOAuthService *GrokOAuthService,
	cacheInvalidator TokenCacheInvalidator,
	schedulerCache SchedulerCache,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
	privacyClientFactory PrivacyClientFactory,
	proxyRepo ProxyRepository,
	refreshAPI *OAuthRefreshAPI,
	runtimeBlocker AccountRuntimeBlocker,
) *TokenRefreshService {
	svc := NewTokenRefreshService(accountRepo, oauthService, openaiOAuthService, geminiOAuthService, antigravityOAuthService, cacheInvalidator, schedulerCache, cfg, tempUnschedCache, grokOAuthService)
	// 注入 OpenAI privacy opt-out 依赖
	svc.SetPrivacyDeps(privacyClientFactory, proxyRepo)
	// 注入统一 OAuth 刷新 API（消除 TokenRefreshService 与 TokenProvider 之间的竞争条件）
	svc.SetRefreshAPI(refreshAPI)
	// 调用侧显式注入后台刷新策略，避免策略漂移
	svc.SetRefreshPolicy(DefaultBackgroundRefreshPolicy())
	svc.SetAccountRuntimeBlocker(runtimeBlocker)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideClaudeTokenProvider creates ClaudeTokenProvider with OAuthRefreshAPI injection
func ProvideClaudeTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	oauthService *OAuthService,
	refreshAPI *OAuthRefreshAPI,
) *ClaudeTokenProvider {
	p := NewClaudeTokenProvider(accountRepo, tokenCache, oauthService)
	executor := NewClaudeTokenRefresher(oauthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(ClaudeProviderRefreshPolicy())
	return p
}

// ProvideOpenAITokenProvider creates OpenAITokenProvider with OAuthRefreshAPI injection
func ProvideOpenAITokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	openaiOAuthService *OpenAIOAuthService,
	refreshAPI *OAuthRefreshAPI,
) *OpenAITokenProvider {
	p := NewOpenAITokenProvider(accountRepo, tokenCache, openaiOAuthService)
	executor := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(OpenAIProviderRefreshPolicy())
	return p
}

// ProvidePluginManager preserves account-directory wiring when regenerating Wire.
func ProvidePluginManager(repo PluginRepository, encryptor SecretEncryptor, cfg *config.Config, hostInfo PluginHostInfo, kvStore PluginKVStore, gateway *OpenAIGatewayService) *PluginManager {
	manager := NewPluginManager(repo, encryptor, cfg, hostInfo, kvStore)
	manager.SetAccountDirectory(gateway)
	return manager
}

// ProvideOpenAIQuotaService wires the OpenAI quota query/reset service.
// It depends on the OpenAI token provider for refreshed access tokens and the
// privacy client factory for the impersonated upstream HTTP client.
func ProvideOpenAIQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	privacyClientFactory PrivacyClientFactory,
	referralClient OpenAIReferralClient,
	openAIGatewayService *OpenAIGatewayService,
) *OpenAIQuotaService {
	service := NewOpenAIQuotaService(accountRepo, proxyRepo, tokenProvider, privacyClientFactory, referralClient)
	service.agentIdentityWS = openAIGatewayService
	return service
}

// ProvideOpenAIQuotaAutoResetService 启动账号级自动用卡队列与补偿扫描。
func ProvideOpenAIQuotaAutoResetService(
	accountRepo AccountRepository,
	quotaService *OpenAIQuotaService,
	rateLimitService *RateLimitService,
	idempotency *IdempotencyCoordinator,
	audit *AuditLogService,
	settingService *SettingService,
	leaderLock LeaderLockCache,
	cfg *config.Config,
) *OpenAIQuotaAutoResetService {
	service := NewOpenAIQuotaAutoResetService(
		accountRepo,
		quotaService,
		rateLimitService,
		idempotency,
		audit,
		settingService,
		leaderLock,
	)
	startBackgroundService(cfg, service)
	return service
}

func ProvideAccountUsageService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageFetcher ClaudeUsageFetcher,
	geminiQuotaService *GeminiQuotaService,
	antigravityQuotaFetcher *AntigravityQuotaFetcher,
	grokQuotaFetcher *GrokQuotaFetcher,
	grokQuotaService *GrokQuotaService,
	openAIQuotaService *OpenAIQuotaService,
	cache *UsageCache,
	identityCache IdentityCache,
	tlsFPProfileService *TLSFingerprintProfileService,
	openAIGatewayService *OpenAIGatewayService,
) *AccountUsageService {
	service := NewAccountUsageService(
		accountRepo,
		usageLogRepo,
		usageFetcher,
		geminiQuotaService,
		antigravityQuotaFetcher,
		grokQuotaFetcher,
		grokQuotaService,
		openAIQuotaService,
		cache,
		identityCache,
		tlsFPProfileService,
	)
	service.agentIdentityWS = openAIGatewayService
	return service
}

func ProvideAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	claudeTokenProvider *ClaudeTokenProvider,
	grokTokenProvider *GrokTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
	openAIGatewayService *OpenAIGatewayService,
	settingService *SettingService,
	probeCostRecorder *AccountProbeCostService,
	pluginManager *PluginManager,
) *AccountTestService {
	service := NewAccountTestService(
		accountRepo,
		geminiTokenProvider,
		claudeTokenProvider,
		grokTokenProvider,
		antigravityGatewayService,
		httpUpstream,
		cfg,
		tlsFPProfileService,
	)
	service.agentIdentityWS = openAIGatewayService
	service.SetOpenAIGatewayService(openAIGatewayService)
	service.SetSettingService(settingService)
	service.SetProbeCostRecorder(probeCostRecorder)
	service.SetPluginManager(pluginManager)
	if p, ok := httpUpstream.(AstraGatewayRuntimeProvider); ok {
		p.SetAstraGatewayPreparer(service.prepareAstraGatewaySource)
	}
	if recorder, ok := httpUpstream.(AstraGatewayHistoryRecorder); ok {
		if history, ok := settingService.settingRepo.(AstraGatewayHistoryRepository); ok {
			recorder.SetAstraGatewayHistoryRecorder(func(row AstraGatewayHistoryRecord, passed bool) {
				if row.Gateway == "" {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if err := history.RecordAstraGateway(ctx, row, passed); err != nil {
					logger.L().Warn("astra gateway history persistence failed")
				}
			})
		}
	}
	// The singleton owns preparation, validation and scheduling from one runtime.
	// APIs still keep their request-local source preparer above. Database polling
	// discovers saves made by any API process without using its readiness state.
	stopScheduling := service.startAstraAccountScheduling()
	openAIGatewayService.stopAstraSetup = func() {
		stopScheduling()
		service.StopAstraAutomaticSetup()
	}
	return service
}

func ProvideAccountProbeCostService(billingService *BillingService, repository AccountProbeCostRepository) *AccountProbeCostService {
	return NewAccountProbeCostService(billingService, repository)
}

func ProvideGrokQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	usageLogRepo UsageLogRepository,
	settingService *SettingService,
) *GrokQuotaService {
	service := NewGrokQuotaService(accountRepo, proxyRepo, tokenProvider, httpUpstream, cfg, usageLogRepo)
	service.SetSettingService(settingService)
	return service
}

// ProvideCNProviderQuotaService 构造国产供应商 Coding Plan 额度探测服务。
func ProvideCNProviderQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderQuotaService {
	return NewCNProviderQuotaService(accountRepo, proxyRepo, httpUpstream, cfg)
}

// ProvideCNProviderBalanceService 构造国产供应商余额探测服务。
func ProvideCNProviderBalanceService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderBalanceService {
	return NewCNProviderBalanceService(accountRepo, proxyRepo, httpUpstream, cfg)
}

// ProvideCNProviderBalanceCheckService 构造并启动周期余额/额度检测任务。
// payg 账号探余额（低余额停调）；coding plan 账号探 5h/weekly 滚动窗口
// （落 extra 快照供调度阈值评估自动停调）。
// 间隔取自 gateway.cn_providers.balance_check_interval_minutes；<=0 或关闭时不启动。
func ProvideCNProviderBalanceCheckService(
	accountRepo AccountRepository,
	balanceService *CNProviderBalanceService,
	quotaService *CNProviderQuotaService,
	cfg *config.Config,
) *CNProviderBalanceCheckService {
	minutes := 10
	if cfg != nil && cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes > 0 {
		minutes = cfg.Gateway.CNProviders.BalanceCheckIntervalMinutes
	}
	svc := NewCNProviderBalanceCheckService(accountRepo, balanceService, quotaService, cfg, time.Duration(minutes)*time.Minute)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideGeminiTokenProvider creates GeminiTokenProvider with OAuthRefreshAPI injection
func ProvideGeminiTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	geminiOAuthService *GeminiOAuthService,
	refreshAPI *OAuthRefreshAPI,
) *GeminiTokenProvider {
	p := NewGeminiTokenProvider(accountRepo, tokenCache, geminiOAuthService)
	executor := NewGeminiTokenRefresher(geminiOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(GeminiProviderRefreshPolicy())
	return p
}

// ProvideAntigravityTokenProvider creates AntigravityTokenProvider with OAuthRefreshAPI injection
func ProvideAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
	refreshAPI *OAuthRefreshAPI,
	tempUnschedCache TempUnschedCache,
) *AntigravityTokenProvider {
	p := NewAntigravityTokenProvider(accountRepo, tokenCache, antigravityOAuthService)
	executor := NewAntigravityTokenRefresher(antigravityOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(AntigravityProviderRefreshPolicy())
	p.SetTempUnschedCache(tempUnschedCache)
	return p
}

// ProvideGrokTokenProvider creates GrokTokenProvider with OAuthRefreshAPI injection.
func ProvideGrokTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	grokOAuthService *GrokOAuthService,
	refreshAPI *OAuthRefreshAPI,
	tempUnschedCache TempUnschedCache,
) *GrokTokenProvider {
	p := NewGrokTokenProvider(accountRepo, tokenCache)
	executor := NewGrokTokenRefresher(grokOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(GrokProviderRefreshPolicy())
	p.SetTempUnschedCache(tempUnschedCache)
	return p
}

// ProvideDashboardAggregationService 创建并启动仪表盘聚合服务
func ProvideDashboardAggregationService(repo DashboardAggregationRepository, timingWheel *TimingWheelService, lockCache LeaderLockCache, db *sql.DB, cfg *config.Config, settingRepo SettingRepository) *DashboardAggregationService {
	svc := NewDashboardAggregationService(repo, timingWheel, cfg)
	svc.settingRepo = settingRepo
	svc.SetLeaderLock(lockCache, db)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideUsageCleanupService 创建并启动使用记录清理任务服务
func ProvideUsageCleanupService(repo UsageCleanupRepository, timingWheel *TimingWheelService, dashboardAgg *DashboardAggregationService, cfg *config.Config) *UsageCleanupService {
	svc := NewUsageCleanupService(repo, timingWheel, dashboardAgg, cfg)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideAccountExpiryService creates and starts AccountExpiryService.
func ProvideAccountExpiryService(accountRepo AccountRepository, cfg *config.Config) *AccountExpiryService {
	svc := NewAccountExpiryService(accountRepo, time.Minute)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpenAICodexVersionSyncService creates and starts OpenAICodexVersionSyncService.
// 出站 Codex 身份的版本号靠它跟随官方发布，无需为了跟版本而发新版本；面板可关闭。
func ProvideOpenAICodexVersionSyncService(
	settingRepo SettingRepository,
	settingService *SettingService,
	githubClient GitHubReleaseClient,
	cfg *config.Config,
) *OpenAICodexVersionSyncService {
	svc := NewOpenAICodexVersionSyncService(settingRepo, settingService, githubClient, openAICodexVersionSyncInterval)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideClaudeCodeVersionSyncService creates and starts ClaudeCodeVersionSyncService.
// 出站 Claude Code 身份的版本号靠它跟随官方发布，无需为了跟版本而发新版本；面板可关闭。
func ProvideClaudeCodeVersionSyncService(
	settingRepo SettingRepository,
	settingService *SettingService,
	githubClient GitHubReleaseClient,
	cfg *config.Config,
) *ClaudeCodeVersionSyncService {
	svc := NewClaudeCodeVersionSyncService(settingRepo, settingService, githubClient, claudeCodeVersionSyncInterval)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideProxyExpiryService creates and starts ProxyExpiryService.
func ProvideProxyExpiryService(proxyRepo ProxyRepository, cfg *config.Config) *ProxyExpiryService {
	svc := NewProxyExpiryService(proxyRepo, time.Minute)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideSubscriptionExpiryService creates and starts SubscriptionExpiryService.
func ProvideSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, settingRepo SettingRepository, notificationEmailService *NotificationEmailService, lockCache LeaderLockCache, db *sql.DB, cfg *config.Config) *SubscriptionExpiryService {
	svc := NewSubscriptionExpiryService(userSubRepo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(notificationEmailService)
	svc.SetLeaderLock(lockCache, db)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideTimingWheelService creates and starts TimingWheelService
func ProvideTimingWheelService(cfg *config.Config) (*TimingWheelService, error) {
	svc, err := NewTimingWheelService()
	if err != nil {
		return nil, err
	}
	if shouldStartRequestLocal(cfg) || shouldStartSingleton(cfg) {
		svc.Start()
	}
	return svc, nil
}

// ProvideDeferredService creates and starts DeferredService
func ProvideDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService, cfg *config.Config) *DeferredService {
	svc := NewDeferredService(accountRepo, timingWheel, 10*time.Second)
	if shouldStartRequestLocal(cfg) {
		svc.Start()
	}
	return svc
}

// ProvideConcurrencyService creates ConcurrencyService and starts slot cleanup worker.
func ProvideConcurrencyService(cache ConcurrencyCache, accountRepo AccountRepository, cfg *config.Config) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	if store, ok := accountRepo.(oauthobs.Store); ok {
		svc.observations = oauthobs.New(store, "", 4096)
		if cfg != nil {
			svc.observationSlotTTL = time.Duration(cfg.Gateway.ConcurrencySlotTTLMinutes) * time.Minute
		}
		svc.observations.Start()
	}
	if runtime, ok := cache.(interface{ RunRegionalRuntime(context.Context) }); ok {
		go runtime.RunRegionalRuntime(svc.apiKeyQueueStopContext())
	}
	if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: startup cleanup stale process slots failed: %v", err)
	}
	if cfg != nil {
		svc.SetAPIKeyQueuePolicy(APIKeyQueuePolicy{
			MaxWaiting: cfg.Gateway.APIKeyQueue.MaxWaiting,
			Timeout:    cfg.Gateway.APIKeyQueue.Timeout(),
		})
	}
	if shouldStartRequestLocal(cfg) {
		if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {
			logger.LegacyPrintf("service.concurrency", "Warning: startup cleanup stale process slots failed: %v", err)
		}
	}
	if cfg != nil && shouldStartRequestLocal(cfg) {
		svc.SetAccountLoadBatchCacheTTL(time.Duration(cfg.Gateway.Scheduling.LoadBatchCacheTTLMS) * time.Millisecond)
		svc.StartSlotCleanupWorker(accountRepo, cfg.Gateway.Scheduling.SlotCleanupInterval)
	}
	return svc
}

// ProvideUserMessageQueueService 创建用户消息串行队列服务并启动清理 worker
func ProvideUserMessageQueueService(cache UserMsgQueueCache, rpmCache RPMCache, cfg *config.Config) *UserMessageQueueService {
	svc := NewUserMessageQueueService(cache, rpmCache, &cfg.Gateway.UserMessageQueue)
	if shouldStartRequestLocal(cfg) && cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds > 0 {
		svc.StartCleanupWorker(time.Duration(cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds) * time.Second)
	}
	return svc
}

// ProvideSchedulerSnapshotService creates and starts SchedulerSnapshotService.
func ProvideSchedulerSnapshotService(
	cache SchedulerCache,
	outboxRepo SchedulerOutboxRepository,
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cfg *config.Config,
	authority RegionalSchedulerAuthority,
) (*SchedulerSnapshotService, error) {
	svc := NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, groupRepo, cfg)
	if cfg != nil && cfg.Runtime.RegionalSchedulerFile != "" {
		runtime, err := startRegionalSchedulerRuntime(cfg, authority)
		if err != nil {
			return nil, err
		}
		svc.regionalRuntime = runtime
		if runtime.replica != nil {
			reader, err := NewRegionalSchedulerReader(runtime.replica)
			if err != nil {
				runtime.stop()
				return nil, err
			}
			svc.SetLocalSnapshotReader(reader)
		}
	}
	if shouldStartSingleton(cfg) { svc.Start() }
	return svc, nil
}

// ProvideRateLimitService creates RateLimitService with optional dependencies.
func ProvideRateLimitService(
	accountRepo AccountRepository,
	usageRepo UsageLogRepository,
	cfg *config.Config,
	geminiQuotaService *GeminiQuotaService,
	tempUnschedCache TempUnschedCache,
	timeoutCounterCache TimeoutCounterCache,
	openAI403CounterCache OpenAI403CounterCache,
	settingService *SettingService,
	tokenCacheInvalidator TokenCacheInvalidator,
	ollamaCloudUsage *OllamaCloudUsageService,
	accountOps *AccountOpsService,
	rdb *redis.Client,
) *RateLimitService {
	svc := NewRateLimitService(accountRepo, usageRepo, cfg, geminiQuotaService, tempUnschedCache)
	svc.accountOps = accountOps
	if rdb != nil {
		svc.qualityTrigger = &quality5xxTrigger{queue: qualityqueue.NewRedis(rdb)}
	}
	if svc.qualityTrigger != nil {
		svc.qualityTrigger.immediate, _ = accountRepo.(quality5xxImmediateRepository)
	}
	if healthCache, ok := tempUnschedCache.(OpenAIAPIKeyHealthCache); ok {
		svc.SetOpenAIAPIKeyHealthCache(healthCache)
	}
	svc.SetTimeoutCounterCache(timeoutCounterCache)
	svc.SetOpenAI403CounterCache(openAI403CounterCache)
	svc.SetSettingService(settingService)
	svc.SetTokenCacheInvalidator(tokenCacheInvalidator)
	svc.SetOllamaCloudUsageProbeScheduler(ollamaCloudUsage)
	return svc
}

// ProvideOpsMetricsCollector creates and starts OpsMetricsCollector.
func ProvideOpsMetricsCollector(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	accountRepo AccountRepository,
	concurrencyService *ConcurrencyService,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsMetricsCollector {
	collector := NewOpsMetricsCollector(opsRepo, settingRepo, accountRepo, concurrencyService, db, redisClient, cfg)
	if shouldStartSingleton(cfg) {
		collector.Start()
	}
	return collector
}

// ProvideOpsAggregationService creates and starts OpsAggregationService (hourly/daily pre-aggregation).
func ProvideOpsAggregationService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsAggregationService {
	svc := NewOpsAggregationService(opsRepo, settingRepo, db, redisClient, cfg)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpsAlertEvaluatorService creates and starts OpsAlertEvaluatorService.
func ProvideOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
) *OpsAlertEvaluatorService {
	svc := NewOpsAlertEvaluatorService(opsService, opsRepo, emailService, redisClient, cfg, proxyRepo)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpsCleanupService creates and starts OpsCleanupService (cron scheduled).
// channelMonitorSvc 让维护任务（聚合 + 历史/聚合软删）跟随 ops 清理 cron 一起跑，
// 共享 leader lock + heartbeat。
// settingRepo 让 cleanup service 自己读 ops_advanced_settings.data_retention 覆盖 cfg；
// opsService 用来反向注入 cleanup hook，以便 UI 改清理设置时能 Reload cron。
func ProvideOpsCleanupService(
	opsRepo OpsRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
	channelMonitorSvc *ChannelMonitorService,
	settingRepo SettingRepository,
	opsService *OpsService,
) *OpsCleanupService {
	svc := NewOpsCleanupService(opsRepo, db, redisClient, cfg, channelMonitorSvc, settingRepo)
	startBackgroundService(cfg, svc)
	if opsService != nil && cfg.RunsBackgroundJobs() {
		opsService.SetCleanupReloader(svc)
	}
	return svc
}

func ProvideOpsSystemLogSink(opsRepo OpsRepository, cfg *config.Config) *OpsSystemLogSink {
	sink := NewOpsSystemLogSink(opsRepo)
	if shouldStartRequestLocal(cfg) {
		sink.Start()
		logger.SetSink(sink)
	}
	return sink
}

// ProvideAuditLogService 创建操作审计日志服务并启动异步写入与保留期清理协程。
// 停止逻辑挂在 cmd/server 的 provideCleanup。
func ProvideAuditLogService(repo AuditLogRepository, settingService *SettingService, cfg *config.Config) *AuditLogService {
	svc := NewAuditLogService(repo, settingService)
	if shouldStartRequestLocal(cfg) {
		svc.Start()
	}
	return svc
}

func ProvideAccountFinancialService(repo AccountFinancialRepository, usageReader AccountFinancialUsageReader, audit *AuditLogService) *AccountFinancialService {
	return NewAccountFinancialServiceWithAudit(repo, usageReader, time.Now, NewAccountFinancialAudit(audit))
}

func ProvideBusinessOverviewService(db *sql.DB) BusinessOverviewService {
	return NewBusinessOverviewService(db)
}

func buildIdempotencyConfig(cfg *config.Config) IdempotencyConfig {
	idempotencyCfg := DefaultIdempotencyConfig()
	if cfg != nil {
		if cfg.Idempotency.DefaultTTLSeconds > 0 {
			idempotencyCfg.DefaultTTL = time.Duration(cfg.Idempotency.DefaultTTLSeconds) * time.Second
		}
		if cfg.Idempotency.SystemOperationTTLSeconds > 0 {
			idempotencyCfg.SystemOperationTTL = time.Duration(cfg.Idempotency.SystemOperationTTLSeconds) * time.Second
		}
		if cfg.Idempotency.ProcessingTimeoutSeconds > 0 {
			idempotencyCfg.ProcessingTimeout = time.Duration(cfg.Idempotency.ProcessingTimeoutSeconds) * time.Second
		}
		if cfg.Idempotency.FailedRetryBackoffSeconds > 0 {
			idempotencyCfg.FailedRetryBackoff = time.Duration(cfg.Idempotency.FailedRetryBackoffSeconds) * time.Second
		}
		if cfg.Idempotency.MaxStoredResponseLen > 0 {
			idempotencyCfg.MaxStoredResponseLen = cfg.Idempotency.MaxStoredResponseLen
		}
		idempotencyCfg.ObserveOnly = cfg.Idempotency.ObserveOnly
	}
	return idempotencyCfg
}

func ProvideIdempotencyCoordinator(repo IdempotencyRepository, cfg *config.Config) *IdempotencyCoordinator {
	coordinator := NewIdempotencyCoordinator(repo, buildIdempotencyConfig(cfg))
	SetDefaultIdempotencyCoordinator(coordinator)
	return coordinator
}

func ProvideSystemOperationLockService(repo IdempotencyRepository, cfg *config.Config) *SystemOperationLockService {
	return NewSystemOperationLockService(repo, buildIdempotencyConfig(cfg))
}

func ProvideIdempotencyCleanupService(repo IdempotencyRepository, cfg *config.Config) *IdempotencyCleanupService {
	svc := NewIdempotencyCleanupService(repo, cfg)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideGeminiCachedContentService creates GeminiCachedContentService and starts its mutation recovery loop.
func ProvideGeminiCachedContentService(repo GeminiCachedContentRepository, compat *GeminiMessagesCompatService, cfg *config.Config) *GeminiCachedContentService {
	svc := NewGeminiCachedContentService(repo, compat, cfg)
	svc.Start()
	return svc
}

// ProvideScheduledTestService creates ScheduledTestService.
func ProvideScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
	templateRepo QualityRuleTemplateRepository,
	accountTests *AccountTestService,
) *ScheduledTestService {
	svc := NewScheduledTestService(planRepo, resultRepo)
	svc.templateRepo = templateRepo
	svc.accountTests = accountTests
	if accountTests != nil {
		svc.qualityModels = svc.accountQualityModels
	}
	return svc
}

// ProvideScheduledTestRunnerService creates and starts ScheduledTestRunnerService.
func ProvideScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
	judge *QualityJudgeService,
	rdb *redis.Client,
	groupTests *PelicanGroupTestService,
	monitor *ChannelMonitorV2Service,
) *ScheduledTestRunnerService {
	svc := NewScheduledTestRunnerService(planRepo, scheduledSvc, accountTestSvc, rateLimitSvc, cfg)
	if rdb != nil {
		svc.qualityTrigger = &quality5xxTrigger{queue: qualityqueue.NewRedis(rdb)}
	}
	svc.judgeQuality = judge.Judge
	svc.groupTests = groupTests
	if groupTests != nil {
		groupTests.judgeQuality = judge.Judge
		groupTests.judgeDrawing = judge.JudgeDrawing
		groupTests.renderDrawing = renderIntelligenceDrawing
	}
	svc.candyMonitor = monitor.candy
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpsScheduledReportService creates and starts OpsScheduledReportService.
func ProvideOpsScheduledReportService(
	opsService *OpsService,
	userService *UserService,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsScheduledReportService {
	svc := NewOpsScheduledReportService(opsService, userService, emailService, redisClient, cfg)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideAPIKeyAuthCacheInvalidator 提供 API Key 认证缓存失效能力
func ProvideAPIKeyAuthCacheInvalidator(apiKeyService *APIKeyService, cfg *config.Config) APIKeyAuthCacheInvalidator {
	// Start Pub/Sub subscriber for L1 cache invalidation across instances
	if shouldStartRequestLocal(cfg) {
		apiKeyService.StartAuthCacheInvalidationSubscriber(context.Background())
	}
	return apiKeyService
}

// ProvideImageStorageSettingService 构造异步生图对象存储的后台设置服务。
//
// config.yaml 里的 image_storage 作为回落：后台从未保存过设置时沿用它，
// 使升级前已通过配置文件开启该功能的部署不被打断。
func ProvideImageStorageSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	backup *BackupService,
	factory ImageStorageFactory,
	cfg *config.Config,
) *ImageStorageSettingService {
	if cfg.ImageStorage.Enabled && !cfg.ImageStorage.Active() {
		// 列出具体缺失的键。若这些键其实已在环境变量里设过，说明它们没被读进来，
		// 请确认 setDefaults 中已为其注册默认值（见 config.setEnvReachableDefaults）。
		logger.L().Warn("image_storage.enabled is true in config but object storage is not fully configured; configure it in the admin UI or complete the config file",
			zap.Strings("missing_keys", cfg.ImageStorage.MissingCredentialKeys()))
	}
	return NewImageStorageSettingService(settingRepo, encryptor, backup, factory, cfg.ImageStorage)
}

// ProvideImageTaskService 构造异步图片任务服务。
//
// 对象存储是异步图片任务的启用前提：仅当开关打开且凭证齐全时功能才可用，否则整体禁用
// （handler 返回 404，不创建任务、不写 Redis），从而避免大 base64 结果撑爆 Redis。
// 启用状态由 settings 服务在运行时解析，因此后台改开关后无需重启即可生效。
func ProvideImageTaskService(store ImageTaskStore, settings *ImageStorageSettingService) *ImageTaskService {
	return NewImageTaskServiceWithResolver(store, settings.Resolver(), defaultImageTaskTTL, defaultImageTaskExecutionTimeout)
}

// ProvideBackupService creates and starts BackupService
func ProvideBackupService(
	settingRepo SettingRepository,
	cfg *config.Config,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
	dumper DBDumper,
	lockCache LeaderLockCache,
	db *sql.DB,
) *BackupService {
	svc := NewBackupService(settingRepo, cfg, encryptor, storeFactory, dumper)
	svc.SetLeaderLock(lockCache, db)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideOpsService constructs OpsService and wires the SettingService-backed quota
// auto-pause cache sink. Mirrors the SetCleanupReloader pattern: OpsService doesn't
// hold a *SettingService reference, but wire injects a tiny callback so writes to
// ops_advanced_settings immediately propagate into the scheduler hot-path cache.
func ProvideOpsService(
	accountOps *AccountOpsService,
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	cfg *config.Config,
	accountRepo AccountRepository,
	userRepo UserRepository,
	concurrencyService *ConcurrencyService,
	gatewayService *GatewayService,
	openAIGatewayService *OpenAIGatewayService,
	geminiCompatService *GeminiMessagesCompatService,
	antigravityGatewayService *AntigravityGatewayService,
	systemLogSink *OpsSystemLogSink,
	settingService *SettingService,
	authCacheInvalidationWorker *AuthCacheInvalidationWorker,
	apiKeyService *APIKeyService,
) *OpsService {
	svc := NewOpsService(
		opsRepo,
		settingRepo,
		cfg,
		accountRepo,
		userRepo,
		concurrencyService,
		gatewayService,
		openAIGatewayService,
		geminiCompatService,
		antigravityGatewayService,
		systemLogSink,
	)
	if settingService != nil {
		svc.SetOpenAIQuotaAutoPauseSettingsSink(settingService.SetOpenAIQuotaAutoPauseSettings)
		// Optional warm-up so the first scheduled request after process start observes
		// a populated cache rather than zero defaults. Best-effort, sync-bounded.
		if shouldStartSingleton(cfg) {
			settingService.WarmOpenAIQuotaAutoPauseSettings(context.Background())
		}
	}
	svc.SetAutoConfigObserver(accountOps.ObserveConcurrencyResult)
	svc.authCacheInvalidationWorker = authCacheInvalidationWorker
	svc.apiKeyService = apiKeyService
	if shouldStartSingleton(cfg) {
		svc.StartRuntimeSettingsRefresh(context.Background())
	}
	return svc
}

// ProvideOpsIngressRejectAggregator starts the bounded security aggregation
// runtime and attaches it to OpsService, which is the middleware recorder.
func ProvideOpsIngressRejectAggregator(opsRepo OpsRepository, opsService *OpsService, cfg *config.Config) *OpsIngressRejectAggregator {
	if !shouldStartRequestLocal(cfg) {
		return nil
	}
	repo, ok := opsRepo.(OpsIngressRejectRepository)
	if !ok {
		return nil
	}
	aggregator := NewOpsIngressRejectAggregator(repo)
	aggregator.Start()
	opsService.SetIngressRejectAggregator(aggregator)
	return aggregator
}

// ProvideSettingService wires SettingService with group reader and proxy repo.
func ProvideSettingService(settingRepo SettingRepository, groupRepo GroupRepository, proxyRepo ProxyRepository, cfg *config.Config) *SettingService {
	svc := NewSettingService(settingRepo, cfg)
	svc.SetDefaultSubscriptionGroupReader(groupRepo)
	svc.SetProxyRepository(proxyRepo)
	if err := svc.LoadForwardedClientIPSettings(context.Background()); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: load forwarded client IP settings failed: %v", err)
	}
	if configuredProcessRole(cfg).RunsMigrations() {
		if err := svc.MigrateOpenAIAllowClaudeCodeCodexPluginSetting(context.Background()); err != nil {
			logger.LegacyPrintf("service.setting", "Warning: migrate openai allow Claude Code Codex plugin setting failed: %v", err)
		}
		if err := svc.MigrateCodexBodyFingerprintToSignals(context.Background()); err != nil {
			logger.LegacyPrintf("service.setting", "Warning: migrate codex body fingerprint to signals failed: %v", err)
		}
	}
	if err := svc.MigrateGrokDefaultTextModel(context.Background()); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: migrate Grok default text model failed: %v", err)
	}
	antigravity.SetUserAgentVersionResolver(svc.GetAntigravityUserAgentVersion)
	// enforceCodexIdentityHeaders 是所有 Codex 出站路径共用的纯函数收口点，拿不到 ctx，
	// 故注入无参解析器；解析器内部自带 60s TTL 缓存，热路径不触库。
	SetCodexCanonicalUserAgentResolver(func() string {
		return svc.GetOpenAICodexCanonicalUserAgent(context.Background())
	})
	// Claude CLI 伪装版本号同理：运行期解析（面板手动值 → 后台同步值 → 内置基线），
	// 解析器内部自带 60s TTL 缓存，热路径不触库。
	claude.SetCLIVersionResolver(func() string {
		return svc.GetClaudeCodeClientVersion(context.Background())
	})
	return svc
}

// ProvideBillingCacheService wires BillingCacheService with its RPM dependencies.
func ProvideBillingCacheService(
	cache BillingCache,
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	apiKeyRepo APIKeyRepository,
	rpmCache UserRPMCache,
	rateRepo UserGroupRateRepository,
	cfg *config.Config,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
) *BillingCacheService {
	return NewBillingCacheService(cache, userRepo, subRepo, apiKeyRepo, rpmCache, rateRepo, cfg, userPlatformQuotaRepo)
}

// ProvideUsageRecordWorkerPool starts the request-owned usage writer only on
// roles that serve API traffic.
func ProvideUsageRecordWorkerPool(cfg *config.Config) *UsageRecordWorkerPool {
	if !shouldStartRequestLocal(cfg) {
		return &UsageRecordWorkerPool{}
	}
	return NewUsageRecordWorkerPool(cfg)
}

// ProvideContentModerationService starts only the in-memory workers fed by
// requests accepted by this process.
func ProvideContentModerationService(
	settingRepo SettingRepository,
	repo ContentModerationRepository,
	hashCache ContentModerationHashCache,
	groupRepo GroupRepository,
	userRepo UserRepository,
	proxyRepo ProxyRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	emailService *EmailService,
	cfg *config.Config,
) *ContentModerationService {
	return newContentModerationService(
		settingRepo, repo, hashCache, groupRepo, userRepo, proxyRepo, authCacheInvalidator, emailService,
		shouldStartRequestLocal(cfg), shouldStartSingleton(cfg),
	)
}

// ProvideAPIKeyService wires APIKeyService and connects rate-limit cache invalidation.
func ProvideAPIKeyService(
	apiKeyRepo APIKeyRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache APIKeyCache,
	cfg *config.Config,
	billingCacheService *BillingCacheService,
	concurrencyService *ConcurrencyService,
	authority RegionalAuthAuthority,
) (*APIKeyService, error) {
	svc := NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, userGroupRateRepo, cache, cfg)
	svc.SetRateLimitCacheInvalidator(billingCacheService)
	svc.SetConcurrencyService(concurrencyService)
	if cfg != nil && cfg.Runtime.RegionalAuthFile != "" {
		runtime, err := startRegionalAuthRuntime(cfg, authority)
		if err != nil {
			return nil, err
		}
		svc.regionalAuthRuntime = runtime
		if runtime.replica != nil {
			reader, err := NewRegionalAuthReader(runtime.replica, runtime.cfg.AuthLookupKey)
			if err != nil {
				runtime.stop()
				return nil, err
			}
			svc.localAuth = reader
		}
	}
	return svc, nil
}

func ProvideUsageCostEvidenceRegistrar(
	usageRepo UsageLogRepository,
	evidenceRepo UsageCostEvidenceRepository,
	activation AccountFinancialActivationReader,
	accountRepo AccountRepository,
) *UsageCostEvidenceRegistrar {
	registrar := NewUsageCostEvidenceRegistrar(usageRepo, evidenceRepo, activation)
	if rateRepo, ok := accountRepo.(NewAPIRateRefreshRepository); ok {
		registrar.SetNewAPIRateRefreshRepository(rateRepo)
	}
	return registrar
}

// ProviderSet is the Wire provider set for all services
var ProviderSet = wire.NewSet(
	ProvideCodexRadarInsightsService,
	ProvideCodexRadarCommunityService,
	ProvideRequestCaptureManager,
	// Core services
	ProvideAuthService,
	NewPasskeyService,
	NewUserService,
	ProvideAPIKeyService,
	ProvideAPIKeyAuthCacheInvalidator,
	ProvideAuthCacheInvalidationWorker,
	NewGroupService,
	NewCompositeRouteResolver,
	NewAccountService,
	NewProxyService,
	NewRedeemService,
	NewPromoService,
	ProvideQuotaWalletServices,
	NewUsageService,
	NewSubUpstreamCostService,
	NewDashboardService,
	NewAccountProfitabilityService,
	ProvideAccountFinancialService,
	ProvideBusinessOverviewService,
	ProvidePricingService,
	NewBillingService,
	ProvideBillingCacheService,
	NewAnnouncementService,
	NewAdminService,
	NewGatewayService,
	ProvideOpenAIGatewayService,
	ProvideUsageCostEvidenceRegistrar,
	wire.Struct(new(OpenAIGatewayDependencies), "*"),
	ProvideCodexHarvestService,
	ProvideImageStorageSettingService,
	ProvideImageTaskService,
	ProvideBatchImageModelPricingResolver,
	NewBatchImagePublicService,
	NewBatchImageDownloadService,
	ProvideBatchImageCleanupService,
	ProvideBatchImageWorkerRuntime,
	wire.Bind(new(AccountRuntimeBlocker), new(*OpenAIGatewayService)),
	NewOAuthService,
	ProvideOpenAIOAuthService,
	ProvideOpenAIOAuthReauthService,
	ProvideGrokOAuthService,
	wire.Bind(new(GrokOAuthTokenService), new(*GrokOAuthService)),
	NewGeminiOAuthService,
	ProvideGeminiCachedContentService,
	NewGeminiQuotaService,
	NewCompositeTokenCacheInvalidator,
	wire.Bind(new(TokenCacheInvalidator), new(*CompositeTokenCacheInvalidator)),
	NewAntigravityOAuthService,
	ProvideOAuthRefreshAPI,
	ProvideGeminiTokenProvider,
	NewGeminiMessagesCompatService,
	ProvideAntigravityTokenProvider,
	ProvideGrokTokenProvider,
	ProvideOpenAITokenProvider,
	ProvideOpenAIQuotaService,
	ProvideOpenAIQuotaAutoResetService,
	ProvideGrokQuotaService,
	ProvideCNProviderQuotaService,
	ProvideCNProviderBalanceService,
	ProvideCNProviderBalanceCheckService,
	ProvideClaudeTokenProvider,
	NewAntigravityGatewayService,
	ProvideRateLimitService,
	ProvideAccountUsageService,
	ProvideAccountProbeCostService,
	ProvideAccountTestService,
	ProvideUpstreamBillingProbeService,
	ProvideOllamaCloudUsageService,
	ProvideOpenCodeGoUsageService,
	ProvideSettingService,
	NewDataManagementService,
	ProvideBackupService,
	ProvideOpsSystemLogSink,
	ProvideOpsService,
	ProvideOpsIngressRejectAggregator,
	ProvideAuditLogService,
	ProvideOpsMetricsCollector,
	ProvideOpsAggregationService,
	ProvideOpsAlertEvaluatorService,
	ProvideOpsCleanupService,
	ProvideOpsScheduledReportService,
	ProvideAccountOpsService,
	ProvideAccountTokenGuardService,
	ProvideAccountTokenGuardV2Service,
	NewEmailService,
	NewNotificationEmailService,
	ProvideEmailQueueService,
	NewTurnstileService,
	NewTencentCaptchaService,
	NewAliyunCaptchaService,
	NewSubscriptionService,
	wire.Bind(new(DefaultSubscriptionAssigner), new(*SubscriptionService)),
	ProvideConcurrencyService,
	ProvideUserMessageQueueService,
	ProvideUsageRecordWorkerPool,
	ProvideSchedulerSnapshotService,
	NewIdentityService,
	ProvideCRSSyncService,
	ProvideUpdateService,
	ProvideTokenRefreshService,
	wire.Bind(new(GrokOAuthReconciler), new(*TokenRefreshService)),
	ProvideAccountExpiryService,
	ProvideOpenAICodexVersionSyncService,
	ProvideClaudeCodeVersionSyncService,
	ProvideProxyExpiryService,
	ProvideSubscriptionExpiryService,
	ProvideTimingWheelService,
	ProvideDashboardAggregationService,
	ProvideUsageCleanupService,
	ProvideDeferredService,
	NewAntigravityQuotaFetcher,
	NewGrokQuotaFetcher,
	NewUserAttributeService,
	NewUsageCache,
	NewTotpService,
	NewErrorPassthroughService,
	NewTLSFingerprintProfileService,
	ProvidePluginManager,
	NewDigestSessionStore,
	ProvideIdempotencyCoordinator,
	ProvideSystemOperationLockService,
	ProvideIdempotencyCleanupService,
	NewPelicanShowcaseService,
	NewPelicanGroupTestService,
	NewControlledExperimentGateway,
	NewControlledExperimentService,
	ProvideScheduledTestService,
	ProvideScheduledTestRunnerService,
	ProvideAccountAdmissionService,
	NewQualityJudgeService,
	NewGroupCapacityService,
	NewChannelService,
	NewModelPlazaService,
	wire.Bind(new(ChannelCacheInvalidator), new(*ChannelService)),
	NewModelPricingResolver,
	ProvideContentModerationService,
	NewAffiliateService,
	ProvidePaymentConfigService,
	ProvidePaymentService,
	ProvidePaymentOrderExpiryService,
	ProvideBalanceNotifyService,
	ProvideChannelMonitorService,
	NewChannelMonitorQuotaFetcher,
	ProvideMonitorV2Service,
	ProvideMonitorV4Service,
	ProvideChannelMonitorRunner,
	ProvideAccountMonitorAccountRepository,
	ProvideAccountMonitorService,
	ProvideUpstreamBalanceNotificationService,
	ProvideAccountMonitorRunner,
	ProvideAccountModelDetectionSidecar,
	ProvideAccountModelDetectionAccountReader,
	ProvideAccountModelDetectionService,
	ProvideChannelMonitorV2Service,
	ProvideChannelMonitorV2Aggregator,
	ProvideChannelMonitorV3Service,
	ProvideChannelMonitorV3Prober,
	NewSupportTicketService,
	NewChannelMonitorRequestTemplateService,
	ProvideUserPlatformQuotaUsageFlusher,
)

func ProvideQuotaWalletServices(repo QuotaWalletRepository) []QuotaWalletService {
	return []QuotaWalletService{NewQuotaWalletService(repo)}
}

// ProvideUserPlatformQuotaUsageFlusher 创建并启动 UserPlatformQuotaUsageFlusher。
func ProvideUserPlatformQuotaUsageFlusher(cfg *config.Config, cache BillingCache, quotaRepo UserPlatformQuotaRepository, tw *TimingWheelService) *UserPlatformQuotaUsageFlusher {
	svc := NewUserPlatformQuotaUsageFlusher(cfg, cache, quotaRepo, tw)
	if shouldStartSingleton(cfg) {
		svc.Start()
	}
	return svc
}

// ProvidePaymentConfigService wraps NewPaymentConfigService to accept the named
// payment.EncryptionKey type instead of raw []byte, avoiding Wire ambiguity.
func ProvidePaymentConfigService(entClient *dbent.Client, settingRepo SettingRepository, key payment.EncryptionKey) *PaymentConfigService {
	return NewPaymentConfigService(entClient, settingRepo, []byte(key))
}

// ProvideBalanceNotifyService creates BalanceNotifyService
func ProvideBalanceNotifyService(emailService *EmailService, settingRepo SettingRepository, accountRepo AccountRepository, notificationEmailService *NotificationEmailService) *BalanceNotifyService {
	svc := NewBalanceNotifyService(emailService, settingRepo, accountRepo)
	svc.SetNotificationEmailService(notificationEmailService)
	return svc
}

// ProvidePaymentService creates PaymentService and attaches notification email delivery.
func ProvidePaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository, affiliateService *AffiliateService, notificationEmailService *NotificationEmailService, wallets ...QuotaWalletService) *PaymentService {
	svc := NewPaymentService(entClient, registry, loadBalancer, redeemService, subscriptionSvc, configService, userRepo, groupRepo, affiliateService, wallets...)
	svc.SetNotificationEmailService(notificationEmailService)
	return svc
}

// ProvidePaymentOrderExpiryService creates and starts PaymentOrderExpiryService.
func ProvidePaymentOrderExpiryService(paymentSvc *PaymentService, lockCache LeaderLockCache, db *sql.DB, cfg *config.Config) *PaymentOrderExpiryService {
	svc := NewPaymentOrderExpiryService(paymentSvc, 60*time.Second)
	svc.SetLeaderLock(lockCache, db)
	startBackgroundService(cfg, svc)
	return svc
}

// ProvideChannelMonitorService 创建渠道监控服务（CRUD + RunCheck + 用户视图聚合）。
// 加密器复用 wire 中已注入的 SecretEncryptor（AES-256-GCM）。
// settingService gates RunCheck via channel_monitor_enabled + channel_monitor_mode.
func ProvideChannelMonitorService(
	repo ChannelMonitorRepository,
	encryptor SecretEncryptor,
	groupRepo GroupRepository,
	settingService *SettingService,
) *ChannelMonitorService {
	svc := NewChannelMonitorService(repo, encryptor, groupRepo)
	svc.SetRuntimeReader(settingService)
	return svc
}

func ProvideMonitorV2Service(
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	channelMonitorV2Repository ChannelMonitorV2Repository,
	native *AccountMonitorService,
	settingService *SettingService,
) *MonitorV2Service {
	return NewMonitorV2Service(groupRepo, apiKeyService, native, settingService, channelMonitorV2Repository)
}

func ProvideMonitorV4Service(
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	channelMonitorV2Repository ChannelMonitorV2Repository,
	native *AccountMonitorService,
	settingService *SettingService,
) *MonitorV4Service {
	return NewMonitorV4Service(groupRepo, apiKeyService, native, settingService, channelMonitorV2Repository)
}

// ProvideChannelMonitorRunner 创建并启动渠道监控调度器。
// 通过 SetScheduler 注入回 service 后再 Start，确保启动时加载所有 enabled monitor，
// 后续 CRUD 也能即时同步任务表。Runner.Stop 由 cleanup function 调用。
// settingService 用于 runner 每次 fire 读取功能开关。
// quotaFetcher（账号侧用量聚合）也在此注入：accountUsage/CN 服务在 wire 图中
// 晚于 channelMonitorService 构造，走 setter 注入避免调整既有构造顺序。
func ProvideChannelMonitorRunner(
	svc *ChannelMonitorService,
	settingService *SettingService,
	quotaFetcher *ChannelMonitorQuotaFetcher,
	usageService *AccountUsageService,
	cfg *config.Config,
) *ChannelMonitorRunner {
	r := NewChannelMonitorRunner(svc, settingService)
	if svc != nil {
		// Ensure runtime reader is set even if ProvideChannelMonitorService
		// was constructed without settings (tests / alternate providers).
		svc.SetRuntimeReader(settingService)
		svc.SetActiveProbeUsageReader(usageService)
		svc.SetQuotaFetcher(quotaFetcher)
		svc.SetScheduler(r)
	}
	startBackgroundService(cfg, r)
	return r
}

func ProvideAccountMonitorService(
	repo AccountMonitorRepository,
	accountRepo AccountMonitorAccountRepository,
	fullAccountRepo AccountRepository,
	accountTestService *AccountTestService,
	accountUsageService *AccountUsageService,
	billingService *BillingService,
	upstreamBillingProbeService *UpstreamBillingProbeService,
	detectionService *AccountModelDetectionService,
	openAIGatewayService *OpenAIGatewayService,
	concurrencyService *ConcurrencyService,
) *AccountMonitorService {
	multiplierService := NewAccountMultiplierService(fullAccountRepo, accountTestService, billingService)
	multiplierService.SetDeclarationProbe(upstreamBillingProbeService)
	service := NewAccountMonitorService(repo, accountRepo, accountTestService, accountUsageService, multiplierService)
	if upstreamBillingProbeService != nil {
		upstreamBillingProbeService.SetAccountRuntimeBlocker(openAIGatewayService)
	}
	service.SetAccountRuntimeBlocker(openAIGatewayService)
	service.costPricing = billingService
	service.SetModelDetectionService(detectionService)
	service.SetAccountMonitorConcurrencyService(concurrencyService)
	return service
}

func ProvideAccountMonitorAccountRepository(repo AccountRepository) AccountMonitorAccountRepository {
	return repo
}

func ProvideUpstreamBalanceNotificationService(
	repo UpstreamBalanceEventRepository,
	reader *AccountMonitorService,
	cfg *config.Config,
) *UpstreamBalanceNotificationService {
	disabled := NewUpstreamBalanceNotificationService(nil, nil, nil, nil, nil)
	enabledValue := strings.TrimSpace(os.Getenv("SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED"))
	if (enabledValue != "1" && !strings.EqualFold(enabledValue, "true")) || !shouldStartSingleton(cfg) {
		return disabled
	}
	secrets, err := upstreamnotify.LoadUpstreamBalanceSecrets(upstreamnotify.UpstreamBalanceSecretPaths{
		AppID:         os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_APP_ID_FILE"),
		AppSecret:     os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_APP_SECRET_FILE"),
		ChatID:        os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_CHAT_ID_FILE"),
		Recipients:    os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_RECIPIENTS_FILE"),
		Registry:      os.Getenv("SUB2API_UPSTREAM_BALANCE_LOGIN_REGISTRY_FILE"),
		CallbackToken: os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_CALLBACK_TOKEN_FILE"),
	})
	if err != nil {
		code := upstreamnotify.SecretErrorCode(err)
		if code == "" {
			code = "secret_unavailable"
		}
		logger.LegacyPrintf("service.upstream_balance_notification", "notification subsystem disabled: code=%s", code)
		return disabled
	}
	baseURL := strings.TrimSpace(os.Getenv("SUB2API_UPSTREAM_BALANCE_FEISHU_API_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	sender := upstreamnotify.NewFeishuSender(baseURL, secrets)
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, secrets.Registry, secrets.RecipientOpenIDs)
	if secrets.CallbackToken != "" {
		if silenceRepo, ok := repo.(UpstreamBalanceNotificationSilenceRepository); ok {
			svc.ConfigureFeishuSilence(silenceRepo, secrets.CallbackToken)
		} else {
			logger.LegacyPrintf("service.upstream_balance_notification", "feishu_silence_actions=disabled code=silence_repository_unavailable")
		}
	} else {
		logger.LegacyPrintf("service.upstream_balance_notification", "feishu_silence_actions=disabled code=callback_token_unconfigured")
	}
	svc.Start()
	return svc
}

func ProvideAccountMonitorRunner(
	svc *AccountMonitorService,
	detector *AccountModelDetectionService,
	balanceNotification *UpstreamBalanceNotificationService,
	monitorV4 *MonitorV4Service,
	leaderLockCache LeaderLockCache,
	db *sql.DB,
	cfg *config.Config,
) *AccountMonitorRunner {
	runner := NewAccountMonitorRunner(svc)
	runner.SetModelDetectionService(detector)
	runner.SetUpstreamBalanceNotificationTrigger(balanceNotification)
	runner.SetMonitorV4SnapshotRefresher(monitorV4)
	runner.SetMonitorV4SnapshotCoordination(leaderLockCache, db)
	if shouldStartSingleton(cfg) {
		runner.Start()
	}
	return runner
}

func ProvideAccountModelDetectionAccountReader(repo AccountRepository) AccountModelDetectionAccountReader {
	return repo
}

func ProvideAccountModelDetectionSidecar() AccountModelDetectionSidecar {
	return NewHTTPAccountModelDetectionSidecar(os.Getenv("SUB2API_MODEL_DETECTOR_URL"), os.Getenv("SUB2API_MODEL_DETECTOR_TOKEN"), &http.Client{Timeout: 15 * time.Minute})
}

func ProvideAccountModelDetectionService(repo AccountModelDetectionRepository, accounts AccountModelDetectionAccountReader, sidecar AccountModelDetectionSidecar, usageService *AccountUsageService) *AccountModelDetectionService {
	service := NewAccountModelDetectionService(repo, accounts, sidecar)
	service.SetActiveProbeUsageReader(usageService)
	return service
}

// ProvideChannelMonitorV2Service wires settings for user-facing privacy flags
// (e.g. hide RPM/TPM throughput).
func ProvideChannelMonitorV2Service(repo ChannelMonitorV2Repository, settingService *SettingService, groups *PelicanGroupTestService) *ChannelMonitorV2Service {
	svc := NewChannelMonitorV2Service(repo)
	svc.SetRuntimeReader(settingService)
	svc.candy = newChannelMonitorV2CandyService(repo, groups, settingService)
	return svc
}

// ProvideChannelMonitorV3Service wires the component status page. It reads
// the V2 passive aggregates, which the V2 aggregator keeps in v2 and v3 mode.
func ProvideChannelMonitorV3Service(repo ChannelMonitorV3Repository, groupRepo GroupRepository) *ChannelMonitorV3Service {
	return NewChannelMonitorV3Service(repo, groupRepo)
}

// ProvideChannelMonitorV3Prober starts the opt-in idle probes of V3, which send
// requests only for checked groups while the site shows V3.
func ProvideChannelMonitorV3Prober(repo ChannelMonitorV3Repository, groups *PelicanGroupTestService, settingService *SettingService, cfg *config.Config) *ChannelMonitorV3Prober {
	prober := NewChannelMonitorV3Prober(repo, groups, settingService)
	startBackgroundService(cfg, prober)
	return prober
}

// ProvideChannelMonitorV2Aggregator starts the passive minute-rollup worker.
// Aggregation only runs when channel_monitor_enabled=true and mode=v2 (and V2 config enabled).
// Set CHANNEL_MONITOR_V2_DISABLE_AGGREGATOR=1 to skip Start (local demo with seeded facts).
func ProvideChannelMonitorV2Aggregator(repo ChannelMonitorV2Repository, db *sql.DB, settingService *SettingService, cfg *config.Config) *ChannelMonitorV2Aggregator {
	aggregator := NewChannelMonitorV2Aggregator(repo, db, settingService)
	if os.Getenv("CHANNEL_MONITOR_V2_DISABLE_AGGREGATOR") == "1" || !shouldStartSingleton(cfg) {
		return aggregator
	}
	startBackgroundService(cfg, aggregator)
	return aggregator
}

func ProvideAccountOpsService(settings SettingRepository, repo AccountOpsRepository, email *EmailService, accounts AccountRepository, groups GroupRepository, cfg *config.Config, encryptor SecretEncryptor, usageCache *UsageCache, usageLogs UsageLogRepository, geminiQuota *GeminiQuotaService, newAPI NewAPIAuthorizationRepository) *AccountOpsService {
	svc := NewAccountOpsService(settings, repo, email)
	svc.SetNotificationDependencies(accounts, encryptor, cfg.Totp.EncryptionKeyConfigured, cfg.Timezone)
	svc.SetNewAPIGroupReader(newAPI)
	svc.SetNotificationUsageCache(usageCache)
	svc.SetNotificationQuotaReaders(usageLogs, geminiQuota)
	svc.autoAccounts, _ = accounts.(AccountConcurrencyRepository)
	svc.autoGroups = groups
	svc.start(cfg.RunsBackgroundJobs())
	return svc
}

// ProvideAccountTokenGuardService 创建并启动「凭证守护」后台巡检（智能运维子页面）。
func ProvideAccountTokenGuardService(settings SettingRepository, repo AccountTokenGuardRepository,
	accounts AccountRepository, admin AdminService, invalidator TokenCacheInvalidator,
	cfg *config.Config,
) *AccountTokenGuardService {
	svc := NewAccountTokenGuardService(settings, repo, accounts, admin, invalidator)
	startBackgroundService(cfg, svc)
	return svc
}

func ProvideAccountTokenGuardV2Service(repo AccountTokenGuardV2Repository, settings SettingRepository, admin AdminService,
	openAIGateway *OpenAIGatewayService, reauth *OpenAIOAuthReauthService,
	cfg *config.Config,
) *AccountTokenGuardV2Service {
	svc := NewAccountTokenGuardV2Service(repo, settings, admin, openAIGateway, reauth)
	startBackgroundService(cfg, svc)
	return svc
}

func ProvideCRSSyncService(accounts AccountRepository, proxies ProxyRepository, oauth *OAuthService, openai *OpenAIOAuthService, gemini *GeminiOAuthService, cfg *config.Config, settings *SettingService, groups GroupRepository) *CRSSyncService {
	svc := NewCRSSyncService(accounts, proxies, oauth, openai, gemini, cfg)
	defaults := &adminServiceImpl{settingService: settings, groupRepo: groups}
	svc.autoConfigure = defaults.ApplyOAuthAutoConfig
	return svc
}

func ProvideAccountAdmissionService(repo AccountAdmissionRepository, test *AccountTestService, cfg *config.Config) *AccountAdmissionService {
	svc := NewAccountAdmissionService(repo, test)
	if shouldStartSingleton(cfg) {
		svc.Start()
	}
	return svc
}
