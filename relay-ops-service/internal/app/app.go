package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
	httpserver "example.invalid/relay-ops-service/internal/http"
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
	candidateService := candidates.Service{Repository: database}
	probeRunner := &probes.V2Executor{RubyPath: cfg.RubyPath, ScriptPath: cfg.V2ScriptPath, ProfilePath: cfg.CandidateProfilePath, QualificationProfilePath: cfg.QualificationProfilePath, MaxOutputBytes: 2 << 20, MaxRequestCost: domain.MicroUSD(1_000)}
	collector := &candidateCollector{Store: database, Fetcher: pricing.Fetcher{}, Extractor: pricing.CompositeExtractor{}, Probes: probeRunner}
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
	var analysisService *agent.Service
	if cfg.AgentBaseURL != "" && cfg.AgentAPIKeyFile != "" && cfg.AgentModel != "" {
		client := &agent.Client{BaseURL: cfg.AgentBaseURL, APIKeyFile: cfg.AgentAPIKeyFile, Model: cfg.AgentModel}
		analysisService = &agent.Service{Analyzer: client, Repository: database}
	}
	var feishu *notify.Client
	if cfg.FeishuWebhookFile != "" {
		feishu = &notify.Client{WebhookFile: cfg.FeishuWebhookFile}
	}
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
	Probes    *probes.V2Executor
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
		if _, err := c.Store.AppendPricingSnapshot(ctx, store.PricingSnapshot{UpstreamID: upstreamID, SourceURL: fetched.URL, SourceType: "public_page", FetchedAt: fetched.FetchedAt, ContentHash: fetched.ContentHash, NormalizedJSON: normalized, DiffSummary: diff, EvidenceLevel: evidence.Confidence}); err != nil {
			return err
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
