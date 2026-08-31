package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const openAIRecoveryExclusionTTL = 10 * time.Minute

type openAIRecoveryExclusionState struct {
	mu      sync.Mutex
	entries map[string]map[openAIAccountModelKey]time.Time
}

// OpenAIRecoveryScope identifies one customer's continuation context. Session
// keys may be content-derived, so they are never sufficient by themselves to
// carry recovery state across requests.
type OpenAIRecoveryScope struct {
	TenantID   string
	GroupID    *int64
	SessionKey string
}

func (s OpenAIRecoveryScope) key() string {
	tenantID := strings.TrimSpace(s.TenantID)
	sessionKey := strings.TrimSpace(s.SessionKey)
	if tenantID == "" || sessionKey == "" {
		return ""
	}
	groupID := "none"
	if s.GroupID != nil {
		groupID = strconv.FormatInt(*s.GroupID, 10)
	}
	return tenantID + "\x00" + groupID + "\x00" + sessionKey
}

func (s *OpenAIGatewayService) getOpenAIRecoveryExclusionState() *openAIRecoveryExclusionState {
	if s == nil {
		return nil
	}
	s.openaiRecoveryExclusionOnce.Do(func() {
		if s.openaiRecoveryExclusions == nil {
			s.openaiRecoveryExclusions = &openAIRecoveryExclusionState{entries: make(map[string]map[openAIAccountModelKey]time.Time)}
		}
	})
	return s.openaiRecoveryExclusions
}

// RecordOpenAIRecoveryFailedAccount persists a transient failed account-model
// exclusion for a new logical request that continues the same session.
func (s *OpenAIGatewayService) RecordOpenAIRecoveryFailedAccount(scope OpenAIRecoveryScope, accountID int64, canonicalModel string, now time.Time) {
	state := s.getOpenAIRecoveryExclusionState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	scopeKey := scope.key()
	if state == nil || !ok || scopeKey == "" {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.entries[scopeKey] == nil {
		state.entries[scopeKey] = make(map[openAIAccountModelKey]time.Time)
	}
	state.entries[scopeKey][key] = now.Add(openAIRecoveryExclusionTTL)
}

// OpenAIRecoveryExcludedAccountIDs returns active exclusions for the session.
// Scheduling accepts account IDs, so each failed account-model excludes that
// account for this continuation request without affecting unrelated sessions.
func (s *OpenAIGatewayService) OpenAIRecoveryExcludedAccountIDs(scope OpenAIRecoveryScope, now time.Time) map[int64]struct{} {
	state := s.getOpenAIRecoveryExclusionState()
	scopeKey := scope.key()
	if state == nil || scopeKey == "" {
		return map[int64]struct{}{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	result := make(map[int64]struct{})
	for key, until := range state.entries[scopeKey] {
		if !now.Before(until) {
			delete(state.entries[scopeKey], key)
			continue
		}
		result[key.AccountID] = struct{}{}
	}
	if len(state.entries[scopeKey]) == 0 {
		delete(state.entries, scopeKey)
	}
	return result
}

// ConsumeOpenAIRecoveryExcludedAccounts clears a continuation's inherited
// exclusions after that continuation completed successfully. A later failed
// terminal recovery will create a fresh, scoped exclusion set.
func (s *OpenAIGatewayService) ConsumeOpenAIRecoveryExcludedAccounts(scope OpenAIRecoveryScope) {
	state := s.getOpenAIRecoveryExclusionState()
	scopeKey := scope.key()
	if state == nil || scopeKey == "" {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.entries, scopeKey)
}

// ClearOpenAIRecoveryExcludedAccountIDs removes only the provided account IDs
// from a continuation scope, preserving unrelated failure exclusions.
func (s *OpenAIGatewayService) ClearOpenAIRecoveryExcludedAccountIDs(scope OpenAIRecoveryScope, accountIDs map[int64]struct{}) {
	state := s.getOpenAIRecoveryExclusionState()
	scopeKey := scope.key()
	if state == nil || scopeKey == "" || len(accountIDs) == 0 {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	entries := state.entries[scopeKey]
	for key := range entries {
		if _, ok := accountIDs[key.AccountID]; ok {
			delete(entries, key)
		}
	}
	if len(entries) == 0 {
		delete(state.entries, scopeKey)
	}
}

// OpenAIRecoveryStickyReferenceCount reports live Redis sticky session and
// response bindings for an account-model runtime row. The existing Redis
// bindings are the source of truth; recovery exclusions are never counted.
func (s *OpenAIGatewayService) OpenAIRecoveryStickyReferenceCount(accountID int64, canonicalModel string, now time.Time) int {
	if s == nil || accountID <= 0 || normalizeOpenAIAccountModelTransientModel(canonicalModel) == "" {
		return 0
	}
	if counter, ok := s.cache.(interface {
		CountStickyAccountReferences(context.Context, int64) (int, error)
	}); ok {
		count, err := counter.CountStickyAccountReferences(context.Background(), accountID)
		if err == nil && count > 0 {
			return count
		}
	}
	return 0
}
