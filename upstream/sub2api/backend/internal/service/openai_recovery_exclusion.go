package service

import (
	"strings"
	"sync"
	"time"
)

const openAIRecoveryExclusionTTL = 10 * time.Minute

type openAIRecoveryExclusionState struct {
	mu      sync.Mutex
	entries map[string]map[openAIAccountModelKey]time.Time
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
func (s *OpenAIGatewayService) RecordOpenAIRecoveryFailedAccount(sessionKey string, accountID int64, canonicalModel string, now time.Time) {
	state := s.getOpenAIRecoveryExclusionState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	sessionKey = strings.TrimSpace(sessionKey)
	if state == nil || !ok || sessionKey == "" {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.entries[sessionKey] == nil {
		state.entries[sessionKey] = make(map[openAIAccountModelKey]time.Time)
	}
	state.entries[sessionKey][key] = now.Add(openAIRecoveryExclusionTTL)
}

// OpenAIRecoveryExcludedAccountIDs returns active exclusions for the session.
// Scheduling accepts account IDs, so each failed account-model excludes that
// account for this continuation request without affecting unrelated sessions.
func (s *OpenAIGatewayService) OpenAIRecoveryExcludedAccountIDs(sessionKey string, now time.Time) map[int64]struct{} {
	state := s.getOpenAIRecoveryExclusionState()
	sessionKey = strings.TrimSpace(sessionKey)
	if state == nil || sessionKey == "" {
		return map[int64]struct{}{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	result := make(map[int64]struct{})
	for key, until := range state.entries[sessionKey] {
		if !now.Before(until) {
			delete(state.entries[sessionKey], key)
			continue
		}
		result[key.AccountID] = struct{}{}
	}
	if len(state.entries[sessionKey]) == 0 {
		delete(state.entries, sessionKey)
	}
	return result
}

// OpenAIRecoveryStickyReferenceCount reports live continuation/session bindings
// for one account-model. It is derived from the same recovery session records
// used to hydrate a new logical request rather than a DTO placeholder.
func (s *OpenAIGatewayService) OpenAIRecoveryStickyReferenceCount(accountID int64, canonicalModel string, now time.Time) int {
	state := s.getOpenAIRecoveryExclusionState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	if state == nil || !ok {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	count := 0
	for session, entries := range state.entries {
		if until, exists := entries[key]; exists {
			if now.Before(until) {
				count++
				continue
			}
			delete(entries, key)
		}
		if len(entries) == 0 {
			delete(state.entries, session)
		}
	}
	return count
}
