package collection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/store"
)

const (
	RoleProduction = "production"
	RoleCandidate  = "candidate"
)

type Source struct {
	ID             domain.UpstreamID
	Name           string
	Role           string
	BaseURL        string
	PricingURL     string
	UsageURL       string
	PerformanceURL string
	ProbeSecretRef string
	Enabled        bool
}

type Repository interface {
	LatestPricingSnapshot(context.Context, domain.UpstreamID) (store.PricingSnapshot, bool, error)
	AppendPricingSnapshot(context.Context, store.PricingSnapshot) (int64, error)
	AppendProbeRun(context.Context, domain.UpstreamID, probes.ProbeRun, time.Time) error
}

type ProbeRunner interface {
	Watch(context.Context, candidates.Candidate) (probes.ProbeRun, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type IncidentMachine interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type Collector struct {
	Repository Repository
	Fetcher    pricing.Fetcher
	Extractor  pricing.Extractor
	Incidents  IncidentMachine
	Agent      AnalysisRunner
	Notifier   MessageSender
	Probes     ProbeRunner
	Now        func() time.Time
}

func (c Collector) Run(ctx context.Context, source Source, allowPaidProbe bool) error {
	if c.Repository == nil {
		return fmt.Errorf("pricing repository is required")
	}
	if !source.Enabled {
		return nil
	}
	if strings.TrimSpace(source.PricingURL) == "" {
		return fmt.Errorf("pricing URL is required for %s", source.Name)
	}
	previous, found, err := c.Repository.LatestPricingSnapshot(ctx, source.ID)
	if err != nil {
		return err
	}
	previousHash := ""
	var before pricing.Evidence
	if found {
		if err := json.Unmarshal(previous.NormalizedJSON, &before); err == nil && before.SchemaVersion == pricing.EvidenceSchemaVersion {
			previousHash = previous.ContentHash
		}
	}
	fetched, changed, err := c.Fetcher.Fetch(ctx, source.PricingURL, previousHash)
	if err != nil {
		return err
	}
	if changed {
		evidence, err := c.Extractor.Extract(fetched)
		if pricing.IsUnparseable(err) {
			evidence = pricing.NewUnparseableEvidence(fetched.URL)
		} else if err != nil {
			return err
		}
		diff := pricing.Diff(before, evidence)
		normalized, err := json.Marshal(evidence)
		if err != nil {
			return fmt.Errorf("encode pricing evidence: %w", err)
		}
		diffJSON, err := json.Marshal(diff)
		if err != nil {
			return fmt.Errorf("encode pricing diff: %w", err)
		}
		now := time.Now().UTC()
		if c.Now != nil {
			now = c.Now().UTC()
		}
		snapshotID, err := c.Repository.AppendPricingSnapshot(ctx, store.PricingSnapshot{
			UpstreamID: source.ID, SourceURL: fetched.URL, SourceType: "public_page", FetchedAt: fetched.FetchedAt,
			ContentHash: fetched.ContentHash, NormalizedJSON: normalized, DiffSummary: diffJSON, EvidenceLevel: evidence.Confidence,
		})
		if err != nil {
			return err
		}
		if found && diff.SemanticChange() {
			evidenceHash := sha256.Sum256(normalized)
			if err := c.notifyChange(ctx, source, snapshotID, fmt.Sprintf("%x", evidenceHash), before, evidence, diff, now); err != nil {
				return err
			}
		}
	}
	if allowPaidProbe && source.Role == RoleCandidate && c.Probes != nil {
		run, err := c.Probes.Watch(ctx, candidates.Candidate{
			ID: source.ID, Name: source.Name, BaseURL: source.BaseURL, PricingURL: source.PricingURL,
			UsageURL: source.UsageURL, PerformanceURL: source.PerformanceURL, ProbeSecretRef: source.ProbeSecretRef, Enabled: source.Enabled,
		})
		if err != nil {
			return err
		}
		if err := c.Repository.AppendProbeRun(ctx, source.ID, run, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (c Collector) notifyChange(ctx context.Context, source Source, snapshotID int64, evidenceHash string, before, after pricing.Evidence, diff pricing.SemanticDiff, now time.Time) error {
	if c.Incidents == nil {
		return nil
	}
	key := "upstream:" + strconv.FormatInt(int64(source.ID), 10) + ":pricing"
	current := summarizeDiff(diff)
	transition, err := c.Incidents.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: true, EvidenceHash: evidenceHash, CurrentValue: current, ConfirmationWindows: 1,
	})
	if err != nil || !transition.Notify {
		return err
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1", IncidentID: key, Severity: "P1", Upstream: source.Name,
		MetricName: "public_pricing_change", BaselineValue: summarizeEvidence(before), CurrentValue: summarizeEvidence(after), Samples: 1,
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
		Title:       source.Name + " 上游公开定价变化",
		Severity:    "P1",
		Current:     summarizeEvidence(after),
		Baseline:    summarizeEvidence(before),
		Analysis:    analysis.Summary,
		WhatWasDone: []string{"读取公开定价页 1 次", "比较前后归一化模型价格和倍率"},
		Results:     []string{current},
		Change:      analysis.Change,
		Focus:       analysis.Focus,
		Links:       []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
	return c.Notifier.SendIncident(ctx, key, evidenceHash, message)
}

func summarizeDiff(diff pricing.SemanticDiff) string {
	parts := make([]string, 0, 4)
	if diff.Multiplier != nil {
		parts = append(parts, "倍率 "+optionalMultiplierText(diff.Multiplier.Before, diff.Multiplier.BeforePresent)+" -> "+optionalMultiplierText(diff.Multiplier.After, diff.Multiplier.AfterPresent))
	}
	if len(diff.PriceChanges) > 0 {
		parts = append(parts, fmt.Sprintf("%d 个模型价格变化", len(diff.PriceChanges)))
	}
	if len(diff.AddedModels) > 0 {
		parts = append(parts, fmt.Sprintf("新增模型 %d 个", len(diff.AddedModels)))
	}
	if len(diff.RemovedModels) > 0 {
		parts = append(parts, fmt.Sprintf("移除模型 %d 个", len(diff.RemovedModels)))
	}
	if len(diff.UnparseableFields) > 0 {
		parts = append(parts, "公开页暂时无法提取可靠定价证据")
	}
	if len(parts) == 0 {
		return "页面内容变化，但没有解析到语义差异"
	}
	return strings.Join(parts, "；")
}

func optionalMultiplierText(value domain.MultiplierBPS, present bool) string {
	if !present {
		return "未解析"
	}
	return multiplierText(value)
}

func summarizeEvidence(evidence pricing.Evidence) string {
	if evidence.AdvertisedMultiplier == nil {
		return fmt.Sprintf("models=%d", len(evidence.Models))
	}
	return multiplierText(*evidence.AdvertisedMultiplier) + ", models=" + strconv.Itoa(len(evidence.Models))
}

func multiplierText(value domain.MultiplierBPS) string {
	return strconv.FormatFloat(float64(value)/10_000, 'f', -1, 64) + "x"
}
