package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"example.invalid/relay-ops-service/internal/acceptance"
	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/adapter"
	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/collection"
	"example.invalid/relay-ops-service/internal/compare"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/controlplane"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/events"
	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/groupimpact"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/nativealerts"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/pricingevents"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/projection"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"example.invalid/relay-ops-service/internal/scheduler"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreampricing"
	"example.invalid/relay-ops-service/internal/upstreams"
)

const feishuOpenAPIBaseURL = "https://open.feishu.cn"

type App struct {
	Store      *store.Store
	Scheduler  *scheduler.Scheduler
	Handler    http.Handler
	Readiness  *Readiness
	Agent      *agent.Service
	Accounting *accounting.Service
	Consumer   *events.Consumer
	CoreOutbox *sub2api.CoreOutbox
}

type incidentMessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type fastCandidateRepository interface {
	ListCandidates(context.Context) ([]candidates.Candidate, error)
}

type fastCandidateRunner interface {
	Fast(context.Context, candidates.Candidate, string) (probes.FastResult, error)
}

type qualityReportSink interface {
	PutQualityReport(context.Context, qualityreports.Report) error
}

func executeFastCandidate(
	ctx context.Context,
	upstreamID domain.UpstreamID,
	jobKind string,
	repository fastCandidateRepository,
	runner fastCandidateRunner,
	sink qualityReportSink,
) error {
	items, err := repository.ListCandidates(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range items {
		if candidate.ID != upstreamID {
			continue
		}
		if !candidate.Enabled {
			return candidates.ErrNotFound
		}
		result, err := runner.Fast(ctx, candidate, jobKind)
		if err != nil {
			return err
		}
		report, err := qualityreports.Build(candidate.ID, candidate.Name, result)
		if err != nil {
			return err
		}
		if err := sink.PutQualityReport(ctx, report); err != nil {
			return err
		}
		return nil
	}
	return candidates.ErrNotFound
}

type qualityReportStoreAdapter struct{ Store *store.Store }

func (a qualityReportStoreAdapter) Get(ctx context.Context, reportID string) (qualityreports.Report, bool, error) {
	return a.Store.GetQualityReport(ctx, reportID)
}

func (a qualityReportStoreAdapter) Put(ctx context.Context, report qualityreports.Report) error {
	return a.Store.PutQualityReport(ctx, report)
}

type qualityReviewAdapter struct{ Service qualityreports.Service }

func (a qualityReviewAdapter) Preview(ctx context.Context, actor domain.AdminActor, input httpserver.QualityPreviewInput) (httpserver.QualitySwitchPreview, error) {
	preview, err := a.Service.Preview(ctx, actor, input.ReportID, input.ReportHash)
	if err != nil {
		if errors.Is(err, qualityreports.ErrStale) {
			return httpserver.QualitySwitchPreview{}, httpserver.ErrQualityReportStale
		}
		return httpserver.QualitySwitchPreview{}, err
	}
	return httpserver.QualitySwitchPreview{
		ReportID: preview.ReportID, ReportHash: preview.ReportHash, Status: preview.Status,
		Writes: preview.Writes, Summary: preview.Summary,
	}, nil
}

func acceptanceAnalysisRunner(service *agent.Service) acceptance.AnalysisRunner {
	if service == nil {
		return nil
	}
	return service
}

func configuredUpstreamPricingResolver(configPath string) *upstreampricing.Resolver {
	if configPath == "" {
		return nil
	}
	return &upstreampricing.Resolver{
		ConfigPath: configPath, RequireHTTPS: true, TTL: 10 * time.Minute,
	}
}

// upstreamPricingFallback keeps the current daily-report fallback contract.
// Pricing events receive the Resolver itself so an explicit mapping can
// suppress a duplicate account-multiplier event.
func upstreamPricingFallback(configPath string) func(context.Context, string) *float64 {
	return upstreamPricingFallbackFromResolver(configuredUpstreamPricingResolver(configPath))
}

func upstreamPricingFallbackFromResolver(
	resolver *upstreampricing.Resolver,
) func(context.Context, string) *float64 {
	if resolver == nil {
		return nil
	}
	return func(ctx context.Context, accountName string) *float64 {
		value, ok := resolver.Lookup(ctx, accountName)
		if !ok {
			return nil
		}
		return value
	}
}

func notificationClient(cfg config.Config, appSender notify.MessageSender) (notify.MessageClient, error) {
	if cfg.FeishuAlertChatIDFile != "" {
		if appSender == nil {
			return nil, fmt.Errorf("Feishu App alert sender is unavailable")
		}
		chatID, err := readFeishuSecret(cfg.FeishuAlertChatIDFile)
		if err != nil {
			return nil, fmt.Errorf("Feishu alert chat ID is unavailable")
		}
		recipients, err := notify.LoadRecipientOpenIDs(cfg.FeishuAlertRecipientsFile)
		if err != nil {
			return nil, fmt.Errorf("Feishu alert recipients are unavailable")
		}
		return notify.AppClient{
			Sender: appSender, ChatID: chatID, BaseURL: cfg.PublicBaseURL,
			RecipientOpenIDs: recipients,
		}, nil
	}
	if cfg.FeishuWebhookFile != "" {
		return notify.Client{WebhookFile: cfg.FeishuWebhookFile, BaseURL: cfg.PublicBaseURL}, nil
	}
	return nil, nil
}

func readFeishuSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 64<<10 {
		return "", errors.New("secret is unavailable")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return "", errors.New("secret is empty")
	}
	return string(data), nil
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	trustedProxy, err := configuredTrustedProxy(cfg, nil)
	if err != nil {
		return nil, err
	}
	database, err := store.Open(ctx, cfg.DatabaseURLFile)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			database.Close()
		}
	}()
	if err := database.Migrate(ctx); err != nil {
		return nil, err
	}
	accountingService := configuredAccountingService(cfg, database)
	if _, err := database.SupersedeLegacyNotificationIncidents(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	reader, err := sub2api.NewHTTPReader(cfg.Sub2APIBaseURL, cfg.Sub2APIAdminKeyFile)
	if err != nil {
		return nil, err
	}
	var coreOutbox *sub2api.CoreOutbox
	if cfg.ExternalizationEnabled {
		if cfg.CoreDatabaseURLFile == "" {
			return nil, fmt.Errorf("RELAY_OPS_CORE_DATABASE_URL_FILE is required for persistent externalization consumption")
		}
		coreURL, readErr := os.ReadFile(cfg.CoreDatabaseURLFile)
		if readErr != nil {
			return nil, fmt.Errorf("read core database URL: %w", readErr)
		}
		coreOutbox, err = sub2api.NewCoreOutbox(ctx, string(bytes.TrimSpace(coreURL)))
		if err != nil {
			return nil, err
		}
		defer func() {
			if failed {
				coreOutbox.Close()
			}
		}()
	}
	readiness := &Readiness{Database: database}
	syncer := sub2api.Synchronizer{
		Reader: reader,
		Sink:   database,
		Observer: nativealerts.Service{
			Signals: database,
			Policy:  cfg.NotificationPolicy,
		},
	}
	if cfg.Mode != config.ModeClosed {
		bootstrapCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = BootstrapNativeReadiness(bootstrapCtx, syncer.Sync, readiness)
		cancel()
	}
	candidateService := configuredCandidateService(cfg, database)
	productionService := upstreams.Service{Repository: database}
	probeRunner := &probes.V2Executor{RubyPath: cfg.RubyPath, ScriptPath: cfg.V2ScriptPath, ProfilePath: cfg.CandidateProfilePath, FastProfilePath: cfg.FastProfilePath, QualificationProfilePath: cfg.QualificationProfilePath, MaxOutputBytes: 2 << 20, MaxRequestCost: domain.MicroUSD(1_000)}
	var analysisService *agent.Service
	if cfg.AgentBaseURL != "" && cfg.AgentAPIKeyFile != "" && cfg.AgentModel != "" {
		client := &agent.Client{BaseURL: cfg.AgentBaseURL, APIKeyFile: cfg.AgentAPIKeyFile, Model: cfg.AgentModel}
		analysisService = &agent.Service{Analyzer: client, Repository: database}
	}
	acceptanceAnalysis := acceptanceAnalysisRunner(analysisService)
	var appAlertSender notify.MessageSender
	if cfg.FeishuAlertChatIDFile != "" {
		appClient, clientErr := feishuapi.NewClient(feishuOpenAPIBaseURL, cfg.FeishuAppIDFile, cfg.FeishuAppSecretFile)
		if clientErr != nil {
			return nil, fmt.Errorf("Feishu App alert client is unavailable")
		}
		appAlertSender = appClient
	}
	notificationTransport, err := notificationClient(cfg, appAlertSender)
	if err != nil {
		return nil, err
	}
	var notifier incidentMessageSender
	var oneShotNotifier pricingevents.EventSender
	if notificationTransport != nil {
		notifier = notify.DeliverySender{
			Client: notificationTransport, Repository: database,
		}
		oneShotNotifier = notify.OneShotSender{
			Client: notificationTransport, Repository: database,
		}
	}
	incidentMachine := &incidents.Machine{Repository: database, Policy: incidents.DefaultPolicy()}
	pricingResolver := configuredUpstreamPricingResolver(cfg.UpstreamGroupMappingFile)
	pricingFallback := upstreamPricingFallbackFromResolver(pricingResolver)
	dailyReportService := dailyreport.Service{
		Reader: reader, Summary: database, Reconciliation: database, Notifier: oneShotNotifier,
		Decisions: database, Policy: cfg.NotificationPolicy,
		Timezone: cfg.Timezone, Fallback: pricingFallback,
	}
	collector := &collection.Collector{
		Repository: database, Fetcher: pricing.Fetcher{}, Extractor: pricing.CompositeExtractor{}, Probes: probeRunner,
		Notifier: oneShotNotifier, Decisions: database, Policy: cfg.NotificationPolicy,
	}
	pricingEventService := pricingevents.Service{
		Accounts: reader, Multipliers: reader, Baselines: database,
		Resolver: pricingResolver, Notifier: oneShotNotifier, Decisions: database,
		Policy: cfg.NotificationPolicy,
	}
	groupImpactService := groupimpact.Service{
		Reader: reader, Signals: database, Incidents: incidentMachine,
		Notifier: notifier, Policy: cfg.NotificationPolicy, Decisions: database,
	}
	escalationService := alerting.Service{Repository: database, Sender: notifier}
	retryService := notify.DeliveryRetryService{Repository: database, Client: notificationTransport}
	usageReader := billing.SessionReader{Reporter: database}
	costCollector := reconciliation.Collector{
		Sources:    database,
		Reconciler: reconciliation.Service{Repository: database},
		Snapshots:  database,
	}
	reconciliationRuntime := reconciliation.RuntimeService{
		Repository:          database,
		CostGuardRepository: database,
		Accounts:            reader,
		Monitors:            reader,
		NativeResolve: func(resolveCtx context.Context, accountName string) (float64, string, bool) {
			if pricingResolver == nil {
				return 0, "unknown", false
			}
			resolution, ok := pricingResolver.Resolve(resolveCtx, accountName)
			if !ok {
				return 0, "unknown", false
			}
			return resolution.Multiplier, "upstream_pricing", true
		},
		Importer: reconciliation.UsageImporter{
			Sources:  database,
			Reader:   reader,
			Attempts: database,
		},
		Collector: costCollector,
		Grace:     10 * time.Minute,
	}
	accountsProjection := projection.NewAccountsWithRepository(database)
	profitabilityProjection := projection.NewProfitabilityWithRepository(database)
	accountingProjection := projection.NewAccountingWithRepository(database)
	reconciliationProjection := projection.NewReconciliationWithRepository(database)
	consumer, err := events.NewPersistentConsumer(database, accountsProjection, profitabilityProjection, accountingProjection, reconciliationProjection)
	if err != nil {
		return nil, err
	}
	var accountingDaily func(context.Context) error
	if accountingService != nil {
		accountingDaily = func(runCtx context.Context) error {
			location := cfg.Timezone
			if location == nil {
				location = time.FixedZone("Asia/Shanghai", 8*60*60)
			}
			local := time.Now().In(location)
			dayEnd := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
			collection, err := reconciliationRuntime.DailyClose(runCtx, dayEnd.AddDate(0, 0, -3).UTC(), dayEnd.UTC())
			if err != nil {
				return err
			}
			if collection.AccountsTotal == 0 {
				return fmt.Errorf("daily accounting requires at least one configured billing source")
			}
			if collection.TransactionsObserved > 0 && collection.Scanned == 0 {
				return fmt.Errorf("daily accounting observed %d upstream billing records without local cost attempts", collection.TransactionsObserved)
			}
			summary, err := reconciliationRuntime.ReadReconciliationSummary(runCtx, 0, dayEnd.AddDate(0, 0, -1).UTC(), dayEnd.UTC(), "USD")
			if err != nil {
				return err
			}
			if summary.PendingAttempts > 0 || summary.ConflictAttempts > 0 {
				return fmt.Errorf("daily accounting blocked by %d pending and %d conflicting upstream costs", summary.PendingAttempts, summary.ConflictAttempts)
			}
			_, err = accountingService.RecomputeRecent(runCtx)
			if err != nil {
				return err
			}
			return database.UpdateDailyReconciliation(runCtx, dayEnd.AddDate(0, 0, -1), summary)
		}
	}
	scheduled := &scheduler.Scheduler{
		Mode: cfg.Mode, Store: database, Timezone: cfg.Timezone,
		Production: func(runCtx context.Context) error {
			if err := syncer.Sync(runCtx); err != nil {
				return err
			}
			readiness.MarkNativeSuccess()
			sources, err := database.ListProduction(runCtx)
			if err != nil {
				return err
			}
			var failures []error
			for _, source := range sources {
				if !source.Enabled {
					continue
				}
				if err := collector.Run(runCtx, collection.Source{
					ID: source.ID, Name: source.Name, Role: source.Role, BaseURL: source.BaseURL,
					PricingURL: source.PricingURL, UsageURL: source.UsageURL, PerformanceURL: source.PerformanceURL,
					Enabled: source.Enabled,
				}, false); err != nil {
					failures = append(failures, err)
				}
			}
			observation := time.Now().UTC()
			billingSources, err := database.ListBillingSources(runCtx)
			if err != nil {
				return err
			}
			for _, source := range billingSources {
				reader, readerErr := billing.NewBalanceReader(source, nil)
				if readerErr != nil {
					failures = append(failures, readerErr)
					continue
				}
				collectCtx, cancel := context.WithTimeout(runCtx, 20*time.Second)
				_, collectErr := (billing.BalanceCollector{Reader: reader, Writer: database, FreshFor: 10 * time.Minute, Source: source.AdapterType}).CollectAt(collectCtx, source.AccountID, observation)
				cancel()
				if collectErr != nil {
					failures = append(failures, collectErr)
				}
			}
			return errors.Join(failures...)
		},
		Candidates: func(runCtx context.Context) ([]domain.UpstreamID, error) {
			items, err := database.ListCandidates(runCtx)
			if err != nil {
				return nil, err
			}
			ids := make([]domain.UpstreamID, 0, len(items))
			for _, item := range items {
				if item.Enabled {
					ids = append(ids, item.ID)
				}
			}
			return ids, nil
		},
		Candidate: func(runCtx context.Context, upstreamID domain.UpstreamID, paidProbe bool) error {
			items, err := database.ListCandidates(runCtx)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.ID != upstreamID {
					continue
				}
				return collector.Run(runCtx, collection.Source{
					ID: item.ID, Name: item.Name, Role: collection.RoleCandidate, BaseURL: item.BaseURL,
					PricingURL: item.PricingURL, UsageURL: item.UsageURL, PerformanceURL: item.PerformanceURL,
					ProbeSecretRef: item.ProbeSecretRef, Enabled: item.Enabled,
				}, paidProbe)
			}
			return candidates.ErrNotFound
		},
		FastCandidate: func(runCtx context.Context, upstreamID domain.UpstreamID, jobKind string, _ bool) error {
			return executeFastCandidate(
				runCtx, upstreamID, jobKind, database, probeRunner, database,
			)
		},
		UsageSessions: database.ListUsageSessions,
		Usage: func(runCtx context.Context, session billing.SessionConfig) error {
			evidence, err := usageReader.ReadUsage(runCtx, session)
			if err != nil {
				var expired *billing.SessionExpiredError
				if errors.As(err, &expired) {
					return nil
				}
				return err
			}
			if err := database.AppendCostObservation(runCtx, evidence); err != nil {
				return err
			}
			return nil
		},
		DailyReport: func(runCtx context.Context) error {
			_, err := dailyReportService.Run(runCtx)
			return err
		},
		AccountingDaily: accountingDaily,
		SiteMonitor: func(runCtx context.Context) error {
			return errors.Join(
				groupImpactService.Run(runCtx),
				pricingEventService.Run(runCtx),
			)
		},
		IncidentEscalation: func(runCtx context.Context) error {
			if notifier == nil {
				return nil
			}
			return escalationService.Run(runCtx)
		},
		NotificationRetry: func(runCtx context.Context) error {
			if notificationTransport == nil {
				return nil
			}
			return retryService.Run(runCtx)
		},
		ReconciliationSweep: func(runCtx context.Context) error {
			now := time.Now().UTC()
			_, err := reconciliationRuntime.Sweep(runCtx, now.Add(-24*time.Hour), now)
			return err
		},
		GroupAvailability: func(runCtx context.Context) error {
			return runGroupAvailability(
				runCtx, reader, database, cfg.NotificationPolicy, time.Now().UTC(),
			)
		},
	}
	qualityRepository := qualityReportStoreAdapter{Store: database}
	qualityReview := qualityReviewAdapter{Service: qualityreports.Service{Repository: qualityRepository}}
	operations, err := newOperationsServer(httpserver.Dependencies{
		BaseOrigin: cfg.PublicBaseURL, Auth: reader, TrustedProxy: trustedProxy, Pricing: httpserver.NativePricingSource{Reader: reader},
		Candidates: candidateService, Upstreams: productionService,
		Billing:        billing.SessionRegistrationService{Repository: database},
		Acceptance:     acceptance.Service{Incidents: incidentMachine, Agent: acceptanceAnalysis},
		DailyReport:    dailyReportService,
		Reconciliation: reconciliationRuntime,
		CostGuard:      reconciliationRuntime,
		QualityReview:  qualityReview,
		ControlPlane: controlplane.NewServerWithRuntimeCutover(
			controlplane.StoreReader{Store: database},
			controlplane.CommandRefresher{Sender: adapter.Sub2API{Client: reader}, Audit: database},
			adapter.Sub2API{Client: reader}, database,
			compare.NewJSONLCutoverAuthority(
				cfg.CutoverStateFile,
				compare.NewJSONLReportSetRepository(cfg.ComparisonReportSetFile),
				nil,
				10*time.Minute,
			),
		),
		ControlPlaneAuth: reader,
	}, accountingService)
	if err != nil {
		return nil, err
	}
	root := http.NewServeMux()
	root.Handle("/healthz", HealthHandler(readiness))
	root.Handle("/readyz", HealthHandler(readiness))
	root.Handle("/", operations)
	failed = false
	return &App{Store: database, Scheduler: scheduled, Handler: root, Readiness: readiness, Agent: analysisService, Accounting: accountingService, Consumer: consumer, CoreOutbox: coreOutbox}, nil
}

func configuredTrustedProxy(cfg config.Config, lookup func(string) ([]net.IP, error)) (adminauth.TrustedProxy, error) {
	policy, err := adminauth.NewTrustedProxyPolicy(cfg.TrustedProxyHost, lookup)
	if err != nil {
		return nil, fmt.Errorf("resolve trusted proxy host: %w", err)
	}
	return policy, nil
}

func (a *App) RunExternalization(ctx context.Context) error {
	if a == nil || a.CoreOutbox == nil || a.Consumer == nil {
		return fmt.Errorf("externalization consumer is not configured")
	}
	return a.CoreOutbox.Run(ctx, "relay-ops", a.Consumer)
}

func newOperationsServer(dependencies httpserver.Dependencies, accountingService *accounting.Service) (http.Handler, error) {
	if accountingService != nil {
		dependencies.Accounting = accountingService
	}
	return httpserver.NewServer(dependencies)
}

func configuredAccountingService(cfg config.Config, repository accounting.Repository) *accounting.Service {
	if !cfg.AccountingEnabled {
		return nil
	}
	return &accounting.Service{
		Repository: repository,
		Timezone:   cfg.Timezone,
		StartDate:  cfg.AccountingLedgerStartDate,
		Exclusions: accounting.ExclusionPolicy{
			InternalUserIDs:   cfg.AccountingInternalUserIDs,
			InternalAPIKeyIDs: cfg.AccountingInternalAPIKeyIDs,
		},
	}
}

func configuredCandidateService(cfg config.Config, repository candidates.Repository) candidates.Service {
	return candidates.Service{
		Repository:  repository,
		SecretStore: candidates.FileSecretStore{Directory: cfg.CandidateSecretDir},
	}
}

func (a *App) Close() {
	if a != nil && a.Store != nil {
		a.Store.Close()
	}
	if a != nil && a.CoreOutbox != nil {
		a.CoreOutbox.Close()
	}
}
