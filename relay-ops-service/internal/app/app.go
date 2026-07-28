package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/acceptance"
	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/collection"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/groupimpact"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/nativealerts"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/opsmonitor"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/scheduler"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreampricing"
	"example.invalid/relay-ops-service/internal/upstreams"
)

type App struct {
	Store     *store.Store
	Scheduler *scheduler.Scheduler
	Handler   http.Handler
	Readiness *Readiness
	Agent     *agent.Service
}

type dailyReportIncidents struct {
	*store.Store
	state *incidents.Machine
}

type incidentAcknowledgements struct {
	store *store.Store
}

func (service incidentAcknowledgements) Acknowledge(ctx context.Context, acknowledgement incidents.Acknowledgement) error {
	return service.store.AcknowledgeIncident(ctx, acknowledgement)
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

type qualityReportNotifier interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type qualityReportIncidentStore interface {
	Put(context.Context, incidents.Record) error
}

func qualityNotificationEvidence(report qualityreports.Report) string {
	payload, _ := json.Marshal(struct {
		UpstreamID   domain.UpstreamID `json:"upstream_id"`
		JobKind      string            `json:"job_kind"`
		Status       string            `json:"status"`
		QualityScore int               `json:"quality_score"`
		TotalScore   int               `json:"total_score"`
		Direct       string            `json:"direct"`
		Gateway      string            `json:"gateway"`
		Models       string            `json:"models"`
		Pricing      string            `json:"pricing"`
		Capacity     string            `json:"capacity"`
	}{
		UpstreamID: report.UpstreamID, JobKind: report.JobKind, Status: report.Status,
		QualityScore: report.QualityScore, TotalScore: report.TotalScore,
		Direct: report.Direct, Gateway: report.Gateway, Models: report.Models,
		Pricing: report.Pricing, Capacity: report.Capacity,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func executeFastCandidate(ctx context.Context, upstreamID domain.UpstreamID, jobKind string, repository fastCandidateRepository, runner fastCandidateRunner, sink qualityReportSink, incidentStore qualityReportIncidentStore, notifier qualityReportNotifier) error {
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
		if notifier == nil {
			return nil
		}
		if incidentStore == nil {
			return fmt.Errorf("quality report incident store is unavailable")
		}
		unknowns := make([]string, 0, 3)
		if report.Gateway == "unknown" {
			unknowns = append(unknowns, "网关测量")
		}
		if report.Pricing == "unknown" {
			unknowns = append(unknowns, "价格与账单")
		}
		if report.Capacity == "unknown" {
			unknowns = append(unknowns, "容量下界")
		}
		message := notify.RenderUpstreamReport(notify.UpstreamReportView{
			Title: report.UpstreamName + " 质量评测", Status: report.Status,
			QualityScore: report.QualityScore, TotalScore: report.TotalScore,
			Direct: report.Direct, Gateway: report.Gateway, Models: report.Models,
			Pricing: report.Pricing, Capacity: report.Capacity, Unknowns: unknowns,
			ReportID: report.ReportID, ReportHash: report.ReportHash,
			Links: []notify.Link{{Label: "运维后台", URL: "/ops"}},
		})
		key := "quality-report:" + strconv.FormatInt(int64(upstreamID), 10) + ":" + jobKind
		evidence := qualityNotificationEvidence(report)
		if err := incidentStore.Put(ctx, incidents.Record{
			Key: key, Severity: "P2", State: "confirmed", SampleCount: 1,
			EvidenceHash: evidence, CurrentValue: report.Status,
		}); err != nil {
			return err
		}
		message = notify.WithDeliveryIdentity(message, 1, "confirmed")
		return notifier.SendIncident(ctx, key, evidence, message)
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

func (source dailyReportIncidents) Observe(ctx context.Context, observation incidents.Observation) (incidents.Transition, error) {
	return source.state.Observe(ctx, observation)
}

func analysisRunners(service *agent.Service) (collection.AnalysisRunner, acceptance.AnalysisRunner) {
	if service == nil {
		return nil, nil
	}
	return service, service
}

func operationalReportAnalysisRunner(service *agent.Service) dailyreport.AnalysisRunner {
	if service == nil {
		return nil
	}
	return service
}

func configuredMultiplierWatcher(
	reader opsmetrics.Reader,
	multipliers opsmonitor.MultiplierSource,
	state *incidents.Machine,
	notifier opsmonitor.MessageSender,
	policy notificationpolicy.Policy,
) opsmonitor.Service {
	return opsmonitor.Service{
		Reader: reader, Multipliers: multipliers, Incidents: state,
		Notifier: notifier, Policy: policy,
	}
}

// upstreamPricingFallback adapts upstreampricing.Resolver.Lookup to the
// fallback signature shared by the daily report and the multiplier alert:
// every failure path already returns (nil, false), so nil means "cannot
// account", never a guessed value. A nil config path disables the fallback
// entirely and both consumers behave exactly as before.
func upstreamPricingFallback(configPath string) func(context.Context, string) *float64 {
	if configPath == "" {
		return nil
	}
	resolver := &upstreampricing.Resolver{ConfigPath: configPath, RequireHTTPS: true, TTL: 10 * time.Minute}
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
		chatID, err := readFeishuCommandSecret(cfg.FeishuAlertChatIDFile)
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

func New(ctx context.Context, cfg config.Config) (*App, error) {
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
	reader, err := sub2api.NewHTTPReader(cfg.Sub2APIBaseURL, cfg.Sub2APIAdminKeyFile)
	if err != nil {
		return nil, err
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
	collectorAnalysis, acceptanceAnalysis := analysisRunners(analysisService)
	reportAnalysis := operationalReportAnalysisRunner(analysisService)
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
	var notifier collection.MessageSender
	if notificationTransport != nil {
		notifier = notify.DeliverySender{Client: notificationTransport, Repository: database}
	}
	incidentMachine := &incidents.Machine{Repository: database, Policy: incidents.DefaultPolicy()}
	var accountQualitySource httpserver.AccountQualitySource
	if cfg.AccountQualityResultFile != "" {
		source := accountquality.FileSource{Path: cfg.AccountQualityResultFile}
		accountQualitySource = source
	}
	pricingFallback := upstreamPricingFallback(cfg.UpstreamGroupMappingFile)
	dailyReportService := dailyreport.Service{
		Reader: reader, Candidates: database, Incidents: dailyReportIncidents{Store: database, state: incidentMachine},
		Agent: reportAnalysis, Notifier: notifier, Timezone: cfg.Timezone,
		Fallback: pricingFallback,
	}
	collector := &collection.Collector{
		Repository: database, Fetcher: pricing.Fetcher{}, Extractor: pricing.CompositeExtractor{}, Probes: probeRunner,
		Incidents: incidentMachine, Agent: collectorAnalysis, Notifier: notifier,
	}
	multiplierWatcher := configuredMultiplierWatcher(
		reader, reader, incidentMachine, notifier, cfg.NotificationPolicy,
	)
	multiplierWatcher.Fallback = pricingFallback
	groupImpactService := groupimpact.Service{
		Reader: reader, Signals: database, Incidents: incidentMachine,
		Notifier: notifier, Policy: cfg.NotificationPolicy, Decisions: database,
	}
	escalationService := alerting.Service{Repository: database, Sender: notifier}
	retryService := notify.DeliveryRetryService{Repository: database, Client: notificationTransport}
	usageReader := billing.SessionReader{Reporter: database}
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
			return executeFastCandidate(runCtx, upstreamID, jobKind, database, probeRunner, database, database, notifier)
		},
		UsageSessions: database.ListUsageSessions,
		Usage: func(runCtx context.Context, session billing.SessionConfig) error {
			evidence, err := usageReader.ReadUsage(runCtx, session)
			if err != nil {
				var expired *billing.SessionExpiredError
				if errors.As(err, &expired) {
					if expired.Notify {
						key := "upstream:" + fmt.Sprint(session.UpstreamID) + ":usage_session"
						hash := sha256.Sum256([]byte(session.LoginURL))
						transition, observeErr := incidentMachine.Observe(runCtx, incidents.Observation{Key: key, Severity: "P2", Failing: true, EvidenceHash: hex.EncodeToString(hash[:]), CurrentValue: "登录会话失效", ConfirmationWindows: 1})
						if observeErr == nil && transition.Notify && notifier != nil {
							message := notify.WithDeliveryIdentity(
								notify.RenderSessionExpired(session.UpstreamName, session.LoginURL),
								transition.OccurrenceNo,
								transition.Kind,
							)
							_ = notifier.SendIncident(runCtx, key, hex.EncodeToString(hash[:]), message)
						}
					}
					return nil
				}
				return err
			}
			if err := database.AppendCostObservation(runCtx, evidence); err != nil {
				return err
			}
			key := "upstream:" + fmt.Sprint(session.UpstreamID) + ":usage_session"
			hash := sha256.Sum256([]byte(session.LoginURL))
			transition, observeErr := incidentMachine.Observe(runCtx, incidents.Observation{Key: key, Severity: "P2", Failing: false, EvidenceHash: hex.EncodeToString(hash[:]), CurrentValue: "登录会话正常"})
			if observeErr == nil && transition.Notify && notifier != nil {
				message := notify.WithDeliveryIdentity(
					renderUsageSessionRecovery(session.UpstreamName, evidence.ObservedAt),
					transition.OccurrenceNo,
					transition.Kind,
				)
				_ = notifier.SendIncident(runCtx, key, hex.EncodeToString(hash[:])+":recovered", message)
			}
			return nil
		},
		DailyReport: func(runCtx context.Context) error {
			_, err := dailyReportService.Run(runCtx)
			return err
		},
		SiteMonitor: func(runCtx context.Context) error {
			return errors.Join(
				groupImpactService.Run(runCtx),
				multiplierWatcher.Run(runCtx),
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
		GroupAvailability: func(runCtx context.Context) error {
			return runGroupAvailability(
				runCtx, reader, database, cfg.NotificationPolicy, time.Now().UTC(),
			)
		},
	}
	qualityRepository := qualityReportStoreAdapter{Store: database}
	qualityReview := qualityReviewAdapter{Service: qualityreports.Service{Repository: qualityRepository}}
	operations, err := httpserver.NewServer(httpserver.Dependencies{
		BaseOrigin: cfg.PublicBaseURL, Auth: reader, Pricing: httpserver.NativePricingSource{Reader: reader},
		Ops:        httpserver.DatabaseOpsSource{Repository: database, Production: database, Pricing: database, Evidence: database, Quality: database, Native: reader, AccountQuality: accountQualitySource},
		Candidates: candidateService, Upstreams: productionService,
		Billing:                  billing.SessionRegistrationService{Repository: database},
		Acceptance:               acceptance.Service{Incidents: incidentMachine, Agent: acceptanceAnalysis, Notifier: notifier},
		DailyReport:              dailyReportService,
		QualityReview:            qualityReview,
		IncidentAcknowledgements: incidentAcknowledgements{store: database},
	})
	if err != nil {
		return nil, err
	}
	root := http.NewServeMux()
	root.Handle("/healthz", HealthHandler(readiness))
	root.Handle("/readyz", HealthHandler(readiness))
	root.Handle("/", operations)
	failed = false
	return &App{Store: database, Scheduler: scheduled, Handler: root, Readiness: readiness, Agent: analysisService}, nil
}

func renderUsageSessionRecovery(upstream string, observedAt time.Time) notify.FeishuMessage {
	metrics := []notify.RecoveryMetric{
		{Label: "会话状态", Value: "正常"},
		{Label: "消费核对", Value: "已恢复"},
	}
	if !observedAt.IsZero() {
		metrics = append(metrics, notify.RecoveryMetric{Label: "证据时间", Value: observedAt.UTC().Format("15:04 UTC")})
	}
	return notify.RenderRecoveryCard(notify.RecoveryCardView{
		Title:   "上游用量读取会话已恢复：" + upstream,
		Summary: "用量读取会话已恢复",
		Detail:  "会话已回到正常状态，真实消费核对继续",
		Metrics: metrics,
		Basis:   []string{"上游用量页面已可正常读取"},
		Source:  "上游用量页面只读读取结果",
		Focus:   "继续关注倍率与真实费用辅助证据",
		Links:   []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
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
}
