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
	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/collection"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/d04readiness"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/feishuapi"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/nativealerts"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/opsmonitor"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/scheduler"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
)

type App struct {
	Store     *store.Store
	Scheduler *scheduler.Scheduler
	Handler   http.Handler
	Readiness *Readiness
	Agent     *agent.Service
	Feishu    *notify.Client
}

type dailyReportIncidents struct {
	*store.Store
	state *incidents.Machine
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

func operationalAnalysisRunners(service *agent.Service) (nativealerts.AnalysisRunner, dailyreport.AnalysisRunner) {
	if service == nil {
		return nil, nil
	}
	return service, service
}

func configuredSiteMonitor(reader opsmetrics.Reader, quality opsmonitor.QualitySource, state *incidents.Machine, notifier opsmonitor.MessageSender) opsmonitor.Service {
	return opsmonitor.Service{Reader: reader, Quality: quality, Incidents: state, Notifier: notifier}
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
		return notify.AppClient{Sender: appSender, ChatID: chatID, BaseURL: cfg.PublicBaseURL}, nil
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
	syncer := sub2api.Synchronizer{Reader: reader, Sink: database}
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
	nativeAnalysis, reportAnalysis := operationalAnalysisRunners(analysisService)
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
	nativeAlertService := nativealerts.Service{Incidents: incidentMachine, Agent: nativeAnalysis, Notifier: notifier}
	syncer.Observer = nativeAlertService
	var dailyAccountQualitySource dailyreport.AccountQualitySource
	var accountQualitySource httpserver.AccountQualitySource
	var siteAccountQualitySource opsmonitor.QualitySource
	if cfg.AccountQualityResultFile != "" {
		source := accountquality.FileSource{Path: cfg.AccountQualityResultFile}
		dailyAccountQualitySource = source
		accountQualitySource = source
		siteAccountQualitySource = source
	}
	dailyReportService := dailyreport.Service{
		Reader: reader, Candidates: database, Incidents: dailyReportIncidents{Store: database, state: incidentMachine},
		AccountQuality: dailyAccountQualitySource, Agent: reportAnalysis, Notifier: notifier, Timezone: cfg.Timezone,
	}
	collector := &collection.Collector{
		Repository: database, Fetcher: pricing.Fetcher{}, Extractor: pricing.CompositeExtractor{}, Probes: probeRunner,
		Incidents: incidentMachine, Agent: collectorAnalysis, Notifier: notifier,
	}
	siteMonitor := configuredSiteMonitor(reader, siteAccountQualitySource, incidentMachine, notifier)
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
							_ = notifier.SendIncident(runCtx, key, hex.EncodeToString(hash[:]), notify.RenderSessionExpired(session.UpstreamName, session.LoginURL))
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
				_ = notifier.SendIncident(runCtx, key, hex.EncodeToString(hash[:])+":recovered", notify.RenderFeishu(notify.IncidentView{Title: "上游用量读取会话已恢复：" + session.UpstreamName, Recovery: true, WhatWasDone: []string{"重新读取上游用量页面"}, Results: []string{"会话恢复，真实消费核对继续"}, Change: "会话从失效恢复为正常", Focus: "继续关注倍率与真实费用辅助证据", Links: []notify.Link{{Label: "运维后台", URL: "/ops"}}}))
			}
			return nil
		},
		DailyReport: func(runCtx context.Context) error {
			_, err := dailyReportService.Run(runCtx)
			return err
		},
		SiteMonitor: siteMonitor.Run,
	}
	qualityRepository := qualityReportStoreAdapter{Store: database}
	qualityReview := qualityReviewAdapter{Service: qualityreports.Service{Repository: qualityRepository}}
	operations, err := httpserver.NewServer(httpserver.Dependencies{
		BaseOrigin: cfg.PublicBaseURL, Auth: reader, Pricing: httpserver.NativePricingSource{Reader: reader},
		Ops:        httpserver.DatabaseOpsSource{Repository: database, Production: database, Pricing: database, Evidence: database, Quality: database, Native: reader, Readiness: d04readiness.FileSource{Path: cfg.D04ReadinessResultFile}, AccountQuality: accountQualitySource},
		Candidates: candidateService, Upstreams: productionService,
		Billing:       billing.SessionRegistrationService{Repository: database},
		Acceptance:    acceptance.Service{Incidents: incidentMachine, Agent: acceptanceAnalysis, Notifier: notifier},
		DailyReport:   dailyReportService,
		QualityReview: qualityReview,
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
