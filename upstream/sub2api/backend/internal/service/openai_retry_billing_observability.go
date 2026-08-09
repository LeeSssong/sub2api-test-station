package service

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	openAIRetryBillingPendingTTL = 30 * time.Minute
	openAIRetryBillingPendingMax = 4096
)

type openAIRetryBillingPendingKey struct {
	platform         string
	apiKeyID         int64
	logicalRequestID string
}

type openAIRetryBillingPendingEntry struct {
	requestID string
	expiresAt time.Time
}

var openAIRetryBillingPending struct {
	sync.Mutex
	entries map[openAIRetryBillingPendingKey]openAIRetryBillingPendingEntry
}

// recordOpenAIRetryBillingResult only emits a reconciliation event when a
// later complete observation reaches the same physical billing boundary and
// the repository confirms it was already applied. A queued partial usage row
// alone never emits the completion event.
func recordOpenAIRetryBillingResult(p *postUsageBillingParams, cmd *UsageBillingCommand, result *UsageBillingApplyResult) {
	if p == nil || p.APIKey == nil || cmd == nil || result == nil {
		return
	}
	key := openAIRetryBillingPendingKey{
		platform:         strings.TrimSpace(p.Platform),
		apiKeyID:         p.APIKey.ID,
		logicalRequestID: strings.TrimSpace(cmd.LogicalRequestID),
	}
	if key.platform == "" || key.apiKeyID <= 0 || key.logicalRequestID == "" {
		return
	}
	now := time.Now().UTC()
	openAIRetryBillingPending.Lock()
	if openAIRetryBillingPending.entries == nil {
		openAIRetryBillingPending.entries = make(map[openAIRetryBillingPendingKey]openAIRetryBillingPendingEntry)
	}
	for pendingKey, pending := range openAIRetryBillingPending.entries {
		if !now.Before(pending.expiresAt) {
			delete(openAIRetryBillingPending.entries, pendingKey)
		}
	}
	if cmd.ReconciliationRequired || cmd.UsageCompleteness == UsageCompletenessPartial {
		if len(openAIRetryBillingPending.entries) >= openAIRetryBillingPendingMax {
			var oldestKey openAIRetryBillingPendingKey
			var oldest time.Time
			for pendingKey, pending := range openAIRetryBillingPending.entries {
				if oldest.IsZero() || pending.expiresAt.Before(oldest) {
					oldestKey, oldest = pendingKey, pending.expiresAt
				}
			}
			delete(openAIRetryBillingPending.entries, oldestKey)
		}
		openAIRetryBillingPending.entries[key] = openAIRetryBillingPendingEntry{requestID: cmd.RequestID, expiresAt: now.Add(openAIRetryBillingPendingTTL)}
		openAIRetryBillingPending.Unlock()
		return
	}
	pending, ok := openAIRetryBillingPending.entries[key]
	if ok && pending.requestID == cmd.RequestID && cmd.UsageCompleteness == UsageCompletenessComplete && !result.Applied {
		delete(openAIRetryBillingPending.entries, key)
	} else {
		ok = false
	}
	openAIRetryBillingPending.Unlock()
	if !ok {
		return
	}

	reconciliationCtx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{
		LogicalRequestID: key.logicalRequestID, AttemptID: p.AttemptID, AttemptNumber: p.AttemptNumber,
		AccountID: p.Account.ID, CanonicalModel: p.CanonicalModel, CachePreservationMode: p.CacheMode,
		OutputStarted: p.OutputStarted, UsageProduced: p.UsageProduced,
	})
	RecordOpenAIRetryBillingReconciled(reconciliationCtx, key.platform, p.APIKey.GroupID, 0, p.OutputStarted, p.UsageProduced, 0)
}
