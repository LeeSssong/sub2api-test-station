package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	openAIModelTransientFailureWindow = time.Minute
	openAIModelTransientShortCooldown = 10 * time.Second
	openAIModelTransientLongCooldown  = 45 * time.Second
	openAIModelTransientDefaultMax    = 4096
	openAIModelTransientMaxModelBytes = 512
)

type openAIAccountModelKey struct {
	AccountID int64
	Model     string
}

type openAIAccountModelTransientEntry struct {
	failureStreak    int
	lastFailure      time.Time
	blockUntil       time.Time
	lastTouched      time.Time
	halfOpenInFlight bool
	lastStatusCode   int
	lastErrorType    string
	outputStarted    bool
}

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

// OpenAIAccountModelFailureEvent describes a failed account/model attempt.
type OpenAIAccountModelFailureEvent struct {
	AccountID      int64
	CanonicalModel string
	StatusCode     int
	ErrorType      string
	OutputStarted  bool
	SafeToReplay   bool
	HasSideEffect  bool
	UsageKnown     bool
	Now            time.Time
}

// OpenAIAccountModelRuntimeDecision is the scheduling decision after a failure.
type OpenAIAccountModelRuntimeDecision struct {
	FailureStreak       int
	Cooldown            time.Duration
	BlockUntil          time.Time
	CurrentRequestRetry bool
	ExcludeFromRequest  bool
	HalfOpenProbe       bool
	RetryAfterSeconds   int
}

type OpenAIAccountModelRuntimeSnapshot struct {
	AccountID            int64
	CanonicalModel       string
	State                string
	FailureStreak        int
	LastFailureAt        time.Time
	BlockUntil           time.Time
	HalfOpenInFlight     bool
	LastStatusCode       int
	LastErrorType        string
	OutputStarted        bool
	StickyReferenceCount int
}

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
}

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
	}
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		maxEntries: maxEntries,
	}
}

func normalizeOpenAIAccountModelTransientModel(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > openAIModelTransientMaxModelBytes {
		return ""
	}
	return strings.ToLower(model)
}

func openAIAccountModelTransientKey(accountID int64, model string) (openAIAccountModelKey, bool) {
	model = normalizeOpenAIAccountModelTransientModel(model)
	if accountID <= 0 || model == "" {
		return openAIAccountModelKey{}, false
	}
	return openAIAccountModelKey{AccountID: accountID, Model: model}, true
}

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}

	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
	}
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
	}
	entry.failureStreak++
	entry.lastFailure = now
	entry.lastTouched = now

	cooldown := time.Duration(0)
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
	}
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
	} else {
		entry.blockUntil = time.Time{}
	}
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockUntil,
	}
}

func (s *openAIAccountModelTransientState) setFailureDetails(accountID int64, model string, statusCode int, errorType string, outputStarted bool) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return
	}
	entry.lastStatusCode = statusCode
	entry.lastErrorType = strings.TrimSpace(errorType)
	entry.outputStarted = outputStarted
	s.entries[key] = entry
}

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

func (s *openAIAccountModelTransientState) isBlocked(accountID int64, model string, now time.Time) bool {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)) {
		delete(s.entries, key)
		return false
	}
	entry.lastTouched = now
	s.entries[key] = entry
	// An expired cooldown remains scheduler-blocked until exactly one half-open
	// lease acquires it. Letting it fall through here would bypass the lease and
	// admit every concurrent request as an unrestricted normal candidate.
	return !entry.blockUntil.IsZero()
}

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if !found || entry.lastTouched.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastTouched
			found = true
		}
	}
	if found {
		delete(s.entries, oldestKey)
	}
}

func transientFailureStatus(status int) bool {
	switch status {
	case 500, 502, 503, 504, 520, 521, 522, 523, 524:
		return true
	default:
		return false
	}
}

func hardOpenAIAccountModelFailureType(errorType string) bool {
	errorType = strings.ToLower(strings.TrimSpace(errorType))
	for _, token := range []string{
		"model_not_found", "model not found", "insufficient_balance", "insufficient balance",
		"balance", "permission", "forbidden", "unauthorized",
	} {
		if strings.Contains(errorType, token) {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) RecordOpenAIAccountModelFailure(_ context.Context, event OpenAIAccountModelFailureEvent) OpenAIAccountModelRuntimeDecision {
	decision := OpenAIAccountModelRuntimeDecision{ExcludeFromRequest: true}
	if s == nil || event.AccountID <= 0 || normalizeOpenAIAccountModelTransientModel(event.CanonicalModel) == "" {
		return decision
	}
	errorType := strings.ToLower(event.ErrorType)
	if hardOpenAIAccountModelFailureType(errorType) {
		return decision
	}
	if event.StatusCode == 0 || event.StatusCode == 400 {
		if !strings.Contains(errorType, "transient") {
			return decision
		}
	} else if !transientFailureStatus(event.StatusCode) {
		return decision
	}
	now := event.Now
	if now.IsZero() {
		now = time.Now()
	}
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return decision
	}
	raw := state.recordFailure(event.AccountID, event.CanonicalModel, now)
	state.setFailureDetails(event.AccountID, event.CanonicalModel, event.StatusCode, event.ErrorType, event.OutputStarted)
	decision.FailureStreak, decision.Cooldown, decision.BlockUntil = raw.FailureStreak, raw.Cooldown, raw.BlockUntil
	decision.CurrentRequestRetry = !event.OutputStarted && event.SafeToReplay && !event.HasSideEffect && raw.FailureStreak == 1
	if !decision.BlockUntil.IsZero() && decision.BlockUntil.After(now) {
		decision.RetryAfterSeconds = int((decision.BlockUntil.Sub(now) + time.Second - 1) / time.Second)
	}
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelSoftFailure, raw.FailureStreak, "")
	slog.Info(OpenAIEventAccountModelSoftFailure,
		"account_id", event.AccountID, "canonical_scheduling_model", normalizeOpenAIAccountModelTransientModel(event.CanonicalModel),
		"attempt", raw.FailureStreak, "status_code", event.StatusCode, "output_started", event.OutputStarted,
		"usage_produced", event.UsageKnown, "cache_preservation_mode", "", "cooldown_seconds", int(decision.Cooldown.Seconds()), "retry_after_seconds", decision.RetryAfterSeconds)
	if decision.Cooldown > 0 {
		RecordOpenAIResilienceEvent(OpenAIEventAccountModelCooldownStarted, raw.FailureStreak, "")
		slog.Warn(OpenAIEventAccountModelCooldownStarted,
			"account_id", event.AccountID, "canonical_scheduling_model", normalizeOpenAIAccountModelTransientModel(event.CanonicalModel),
			"attempt", raw.FailureStreak, "status_code", event.StatusCode, "output_started", event.OutputStarted,
			"usage_produced", event.UsageKnown, "cache_preservation_mode", "", "cooldown_seconds", int(decision.Cooldown.Seconds()), "retry_after_seconds", decision.RetryAfterSeconds)
	}
	return decision
}

// ImmediatelyCooldownAccountModel records a transient-only operator cooldown.
// It never mutates persistent account status, so a hard-disabled account remains disabled.
func (s *OpenAIGatewayService) ImmediatelyCooldownAccountModel(_ context.Context, accountID int64, canonicalModel string, cooldown time.Duration, now time.Time) (OpenAIAccountModelRuntimeSnapshot, error) {
	state := s.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	if state == nil || !ok {
		return OpenAIAccountModelRuntimeSnapshot{}, fmt.Errorf("account_id and canonical_scheduling_model are required")
	}
	if cooldown <= 0 {
		return OpenAIAccountModelRuntimeSnapshot{}, fmt.Errorf("cooldown must be positive")
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	entry := state.entries[key]
	if entry.failureStreak < 2 {
		entry.failureStreak = 2
	}
	entry.lastFailure = now
	entry.lastTouched = now
	entry.blockUntil = now.Add(cooldown)
	entry.halfOpenInFlight = false
	entry.lastStatusCode = 503
	entry.lastErrorType = "admin_immediate_cooldown"
	state.entries[key] = entry
	state.mu.Unlock()
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelCooldownStarted, entry.failureStreak, "")
	slog.Warn(OpenAIEventAccountModelCooldownStarted, "account_id", accountID, "canonical_scheduling_model", key.Model, "attempt", entry.failureStreak, "status_code", entry.lastStatusCode, "output_started", false, "usage_produced", false, "cache_preservation_mode", "", "cooldown_seconds", int(cooldown.Seconds()), "retry_after_seconds", int(cooldown.Seconds()), "source", "admin")
	return s.snapshotOpenAIAccountModelRuntime(key, now), nil
}

// RestoreAccountModelScheduling removes only the in-memory transient record.
// Persistent hard-disable state is intentionally owned by the account restore workflow.
func (s *OpenAIGatewayService) RestoreAccountModelScheduling(_ context.Context, accountID int64, canonicalModel string) error {
	state := s.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	if state == nil || !ok {
		return fmt.Errorf("account_id and canonical_scheduling_model are required")
	}
	state.recordSuccess(key.AccountID, key.Model)
	return nil
}

// ProbeAccountModelOnce authorizes exactly one real scheduler half-open attempt.
// It deliberately does not claim an upstream result: the request path that owns
// the actual probe must call ReleaseOpenAIAccountModelHalfOpenProbe.
func (s *OpenAIGatewayService) ProbeAccountModelOnce(_ context.Context, accountID int64, canonicalModel string, now time.Time) (bool, error) {
	if !s.AcquireOpenAIAccountModelHalfOpenProbe(accountID, canonicalModel, now) {
		return false, nil
	}
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelHalfOpenProbe, 0, "")
	slog.Info(OpenAIEventAccountModelHalfOpenProbe, "account_id", accountID, "canonical_scheduling_model", normalizeOpenAIAccountModelTransientModel(canonicalModel), "attempt", 1, "status_code", 0, "output_started", false, "usage_produced", false, "cache_preservation_mode", "half_open_probe", "cooldown_seconds", 0, "retry_after_seconds", 0, "state", "pending")
	return true, nil
}

func (s *OpenAIGatewayService) snapshotOpenAIAccountModelRuntime(key openAIAccountModelKey, now time.Time) OpenAIAccountModelRuntimeSnapshot {
	for _, snapshot := range s.SnapshotOpenAIAccountModelRuntime(now) {
		if snapshot.AccountID == key.AccountID && snapshot.CanonicalModel == key.Model {
			return snapshot
		}
	}
	return OpenAIAccountModelRuntimeSnapshot{}
}

func (s *OpenAIGatewayService) AcquireOpenAIAccountModelHalfOpenProbe(accountID int64, canonicalModel string, now time.Time) bool {
	state := s.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	if state == nil || !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, exists := state.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && (now.Before(entry.lastFailure) || (now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)))) {
		delete(state.entries, key)
		return false
	}
	if entry.blockUntil.IsZero() || now.Before(entry.blockUntil) || entry.halfOpenInFlight {
		return false
	}
	entry.halfOpenInFlight = true
	entry.lastTouched = now
	state.entries[key] = entry
	return true
}

func (s *OpenAIGatewayService) ReleaseOpenAIAccountModelHalfOpenProbe(accountID int64, canonicalModel string, success bool, now time.Time) {
	state := s.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(accountID, canonicalModel)
	if state == nil || !ok {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, exists := state.entries[key]
	if !exists {
		return
	}
	if success {
		delete(state.entries, key)
		return
	}
	entry.halfOpenInFlight = false
	entry.lastTouched = now
	entry.lastFailure = now
	cooldown := openAIModelTransientShortCooldown
	if entry.failureStreak >= 3 {
		cooldown = openAIModelTransientLongCooldown
	}
	entry.blockUntil = now.Add(cooldown)
	state.entries[key] = entry
}

func (s *OpenAIGatewayService) SnapshotOpenAIAccountModelRuntime(now time.Time) []OpenAIAccountModelRuntimeSnapshot {
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	result := make([]OpenAIAccountModelRuntimeSnapshot, 0, len(state.entries))
	for key, entry := range state.entries {
		if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientFailureWindow && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)) {
			continue
		}
		stateName := "soft_failure"
		if entry.halfOpenInFlight {
			stateName = "half_open"
		} else if !entry.blockUntil.IsZero() && now.Before(entry.blockUntil) {
			stateName = "cooldown"
		}
		result = append(result, OpenAIAccountModelRuntimeSnapshot{AccountID: key.AccountID, CanonicalModel: key.Model, State: stateName, FailureStreak: entry.failureStreak, LastFailureAt: entry.lastFailure, BlockUntil: entry.blockUntil, HalfOpenInFlight: entry.halfOpenInFlight, LastStatusCode: entry.lastStatusCode, LastErrorType: entry.lastErrorType, OutputStarted: entry.outputStarted, StickyReferenceCount: s.OpenAIRecoveryStickyReferenceCount(key.AccountID, key.Model, now)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AccountID != result[j].AccountID {
			return result[i].AccountID < result[j].AccountID
		}
		return result[i].CanonicalModel < result[j].CanonicalModel
	})
	return result
}
