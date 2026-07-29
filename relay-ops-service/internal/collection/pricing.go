package collection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
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

type EventSender interface {
	SendOneShot(context.Context, notify.OneShotIdentity, notify.FeishuMessage) error
}

type DecisionRecorder interface {
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type Collector struct {
	Repository Repository
	Fetcher    pricing.Fetcher
	Extractor  pricing.Extractor
	Notifier   EventSender
	Decisions  DecisionRecorder
	Policy     notificationpolicy.Policy
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
	hasReliablePrevious := false
	if found {
		if err := json.Unmarshal(previous.NormalizedJSON, &before); err == nil && before.SchemaVersion == pricing.EvidenceSchemaVersion {
			previousHash = previous.ContentHash
			hasReliablePrevious = reliablePricingEvidence(before)
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
		_, err = c.Repository.AppendPricingSnapshot(ctx, store.PricingSnapshot{
			UpstreamID: source.ID, SourceURL: fetched.URL, SourceType: "public_page", FetchedAt: fetched.FetchedAt,
			ContentHash: fetched.ContentHash, NormalizedJSON: normalized, DiffSummary: diffJSON, EvidenceLevel: evidence.Confidence,
		})
		if err != nil {
			return err
		}
		if source.Role == RoleProduction &&
			hasReliablePrevious &&
			reliablePricingEvidence(evidence) &&
			diff.SemanticChange() {
			if err := c.notifyChange(ctx, source, diff, diffJSON, now); err != nil {
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

func (c Collector) notifyChange(
	ctx context.Context,
	source Source,
	diff pricing.SemanticDiff,
	diffJSON []byte,
	now time.Time,
) error {
	if c.Policy.Version <= 0 {
		return nil
	}
	sum := sha256.Sum256(diffJSON)
	key := "pricing:" + strconv.FormatInt(int64(source.ID), 10) + ":" + fmt.Sprintf("%x", sum)
	if !c.Policy.Enabled(notificationpolicy.FamilyPricingNotice) {
		return c.recordDecision(ctx, key, "suppressed", "policy_disabled", diffJSON, now)
	}
	switch c.Policy.Mode {
	case notificationpolicy.ModeShadow:
		return c.recordDecision(ctx, key, "shadow_would_deliver", key, diffJSON, now)
	case notificationpolicy.ModeEnabled:
		if c.Notifier == nil {
			return fmt.Errorf("pricing event notifier is required")
		}
	default:
		return c.recordDecision(ctx, key, "suppressed", "delivery_mode_disabled", diffJSON, now)
	}

	message := notify.RenderPricingNotice(notify.PricingNoticeView{
		Upstream:   pricingSourceName(source.Name),
		Change:     summarizeDiff(diff),
		Review:     "核对该上游公开价格与当前售价、毛利是否仍符合预期。",
		ObservedAt: now,
	})
	if err := c.Notifier.SendOneShot(ctx, notify.OneShotIdentity{
		Key: key, Family: string(notificationpolicy.FamilyPricingNotice),
		PolicyVersion: c.Policy.Version, SourceKind: "public_pricing",
	}, message); err != nil {
		return err
	}
	return c.recordDecision(ctx, key, "delivered", key, diffJSON, now)
}

func (c Collector) recordDecision(
	ctx context.Context,
	key string,
	decision string,
	reason string,
	details []byte,
	observedAt time.Time,
) error {
	if c.Decisions == nil {
		return fmt.Errorf("pricing notification decision recorder is required")
	}
	return c.Decisions.RecordNotificationDecision(ctx, store.DecisionRecord{
		DecisionKey: key, Family: string(notificationpolicy.FamilyPricingNotice),
		PolicyVersion: c.Policy.Version, SourceKind: "public_pricing",
		Decision: decision, Reason: reason, Details: details, ObservedAt: observedAt,
	})
}

func summarizeDiff(diff pricing.SemanticDiff) string {
	parts := make([]string, 0, 4)
	if diff.Multiplier != nil {
		switch {
		case diff.Multiplier.BeforePresent && diff.Multiplier.AfterPresent:
			parts = append(parts,
				"公开倍率由 "+multiplierText(diff.Multiplier.Before)+
					" 调整为 "+multiplierText(diff.Multiplier.After)+"。")
		case diff.Multiplier.AfterPresent:
			parts = append(parts,
				"公开页新增倍率 "+multiplierText(diff.Multiplier.After)+"。")
		default:
			parts = append(parts,
				"公开页不再列出此前的倍率 "+multiplierText(diff.Multiplier.Before)+"。")
		}
	}
	if len(diff.PriceChanges) > 0 {
		parts = append(parts, fmt.Sprintf("另有 %d 个模型价格发生变化。", len(diff.PriceChanges)))
	}
	if len(diff.AddedModels) > 0 {
		parts = append(parts, fmt.Sprintf("公开价目新增 %d 个模型。", len(diff.AddedModels)))
	}
	if len(diff.RemovedModels) > 0 {
		parts = append(parts, fmt.Sprintf("公开价目移除 %d 个模型。", len(diff.RemovedModels)))
	}
	if len(parts) == 0 {
		return "公开定价出现可靠变化。"
	}
	return strings.Join(parts, "\n")
}

func reliablePricingEvidence(evidence pricing.Evidence) bool {
	if evidence.SchemaVersion != pricing.EvidenceSchemaVersion ||
		evidence.Confidence == "" ||
		evidence.Confidence == "unparseable" {
		return false
	}
	return evidence.AdvertisedMultiplier != nil || len(evidence.Models) > 0
}

func multiplierText(value domain.MultiplierBPS) string {
	return strconv.FormatFloat(float64(value)/10_000, 'f', -1, 64) + "x"
}

func pricingSourceName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") {
		return "生产上游"
	}
	return value
}
