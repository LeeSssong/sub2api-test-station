package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
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

type Collector struct {
	Repository Repository
	Fetcher    pricing.Fetcher
	Extractor  pricing.Extractor
	Probes     ProbeRunner
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
		_, err = c.Repository.AppendPricingSnapshot(ctx, store.PricingSnapshot{
			UpstreamID: source.ID, SourceURL: fetched.URL, SourceType: "public_page", FetchedAt: fetched.FetchedAt,
			ContentHash: fetched.ContentHash, NormalizedJSON: normalized, DiffSummary: diffJSON, EvidenceLevel: evidence.Confidence,
		})
		if err != nil {
			return err
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
