package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/scheduler"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type App struct {
	Store     *store.Store
	Scheduler *scheduler.Scheduler
	Handler   http.Handler
	Readiness *Readiness
	Agent     *agent.Service
	Feishu    *notify.Client
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
	candidateService := candidates.Service{Repository: database}
	probeRunner := &probes.V2Executor{RubyPath: cfg.RubyPath, ScriptPath: cfg.V2ScriptPath, ProfilePath: cfg.CandidateProfilePath, QualificationProfilePath: cfg.QualificationProfilePath, MaxOutputBytes: 2 << 20, MaxRequestCost: domain.MicroUSD(1_000)}
	var analysisService *agent.Service
	if cfg.AgentBaseURL != "" && cfg.AgentAPIKeyFile != "" && cfg.AgentModel != "" {
		client := &agent.Client{BaseURL: cfg.AgentBaseURL, APIKeyFile: cfg.AgentAPIKeyFile, Model: cfg.AgentModel}
		analysisService = &agent.Service{Analyzer: client, Repository: database}
	}
	var feishu *notify.Client
	if cfg.FeishuWebhookFile != "" {
		feishu = &notify.Client{WebhookFile: cfg.FeishuWebhookFile}
	}
	collector := &candidateCollector{
		Store: database, Fetcher: pricing.Fetcher{}, Extractor: pricing.CompositeExtractor{}, Probes: probeRunner,
		Incidents: &incidents.Machine{Repository: database, Policy: incidents.DefaultPolicy()}, Agent: analysisService, Notifier: feishu,
	}
	scheduled := &scheduler.Scheduler{
		Mode: cfg.Mode, Store: database, Timezone: cfg.Timezone,
		Production: func(runCtx context.Context) error {
			if err := syncer.Sync(runCtx); err != nil {
				return err
			}
			readiness.MarkNativeSuccess()
			return nil
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
		Candidate: collector.Run,
	}
	operations, err := httpserver.NewServer(httpserver.Dependencies{BaseOrigin: cfg.PublicBaseURL, Auth: reader, Pricing: httpserver.NativePricingSource{Reader: reader}, Ops: httpserver.DatabaseOpsSource{Repository: database}, Candidates: candidateService})
	if err != nil {
		return nil, err
	}
	root := http.NewServeMux()
	root.Handle("/healthz", HealthHandler(readiness))
	root.Handle("/readyz", HealthHandler(readiness))
	root.Handle("/", operations)
	failed = false
	return &App{Store: database, Scheduler: scheduled, Handler: root, Readiness: readiness, Agent: analysisService, Feishu: feishu}, nil
}

func (a *App) Close() {
	if a != nil && a.Store != nil {
		a.Store.Close()
	}
}

type candidateCollector struct {
	Store     *store.Store
	Fetcher   pricing.Fetcher
	Extractor pricing.Extractor
	Probes    ProbeRunner
	Incidents *incidents.Machine
	Agent     AnalysisRunner
	Notifier  MessageSender
}

type ProbeRunner interface {
	Watch(context.Context, candidates.Candidate) (probes.ProbeRun, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type MessageSender interface {
	Send(context.Context, notify.FeishuMessage) error
}

func (c *candidateCollector) Run(ctx context.Context, upstreamID domain.UpstreamID, paidProbe bool) error {
	items, err := c.Store.ListCandidates(ctx)
	if err != nil {
		return err
	}
	var candidate candidates.Candidate
	for _, item := range items {
		if item.ID == upstreamID {
			candidate = item
			break
		}
	}
	if candidate.ID == 0 || !candidate.Enabled {
		return candidates.ErrNotFound
	}
	previous, found, err := c.Store.LatestPricingSnapshot(ctx, upstreamID)
	if err != nil {
		return err
	}
	previousHash := ""
	if found {
		previousHash = previous.ContentHash
	}
	fetched, changed, err := c.Fetcher.Fetch(ctx, candidate.PricingURL, previousHash)
	if err != nil {
		return err
	}
	if changed {
		evidence, err := c.Extractor.Extract(fetched)
		if err != nil {
			return err
		}
		normalized, _ := json.Marshal(evidence)
		var semantic pricing.SemanticDiff
		if found {
			var before pricing.Evidence
			if json.Unmarshal(previous.NormalizedJSON, &before) == nil {
				semantic = pricing.Diff(before, evidence)
			}
		}
		diff, _ := json.Marshal(semantic)
		snapshotID, err := c.Store.AppendPricingSnapshot(ctx, store.PricingSnapshot{UpstreamID: upstreamID, SourceURL: fetched.URL, SourceType: "public_page", FetchedAt: fetched.FetchedAt, ContentHash: fetched.ContentHash, NormalizedJSON: normalized, DiffSummary: diff, EvidenceLevel: evidence.Confidence})
		if err != nil {
			return err
		}
		if semantic.Multiplier != nil {
			if err := c.notifyMultiplierChange(ctx, candidate, snapshotID, fetched.ContentHash, *semantic.Multiplier); err != nil {
				return err
			}
		}
	}
	if !paidProbe {
		return nil
	}
	run, err := c.Probes.Watch(ctx, candidate)
	if err != nil {
		return err
	}
	return c.Store.AppendProbeRun(ctx, upstreamID, run, time.Now().UTC())
}

func (c *candidateCollector) notifyMultiplierChange(ctx context.Context, candidate candidates.Candidate, snapshotID int64, evidenceHash string, change pricing.MultiplierChange) error {
	if c.Incidents == nil {
		return nil
	}
	key := "upstream:" + strconv.FormatInt(int64(candidate.ID), 10) + ":advertised_multiplier"
	transition, err := c.Incidents.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: evidenceHash,
		CurrentValue: strconv.FormatInt(int64(change.After), 10), ConfirmationWindows: 1,
	})
	if err != nil || !transition.Notify {
		return err
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1", IncidentID: key, Severity: "P1", Upstream: candidate.Name,
		MetricName: "advertised_multiplier_bps", BaselineValue: strconv.FormatInt(int64(change.Before), 10), CurrentValue: strconv.FormatInt(int64(change.After), 10), Samples: 1,
		EvidenceRefs: []string{"pricing_snapshot:" + strconv.FormatInt(snapshotID, 10)}, AllowedActions: []string{"observe", "request_human_review"},
	}
	analysis := agent.Fallback(contract)
	if c.Agent != nil {
		if generated, analyzeErr := c.Agent.AnalyzeOnce(ctx, contract); analyzeErr == nil {
			analysis = generated
		}
	}
	if c.Notifier == nil {
		return nil
	}
	message := notify.RenderFeishu(notify.IncidentView{
		Title:       candidate.Name + " 上游倍率变化",
		WhatWasDone: []string{"读取公开价格页 1 次", "比较前后归一化价格快照"},
		Results:     []string{"倍率由 " + bpsText(change.Before) + " 调整为 " + bpsText(change.After)},
		Change:      analysis.Change, Focus: analysis.Focus, Links: []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
	return c.Notifier.Send(ctx, message)
}

func bpsText(value domain.MultiplierBPS) string {
	return strconv.FormatFloat(float64(value)/10_000, 'f', -1, 64) + "x"
}
