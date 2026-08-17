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
	// openAIModelTransientStreakTTL bounds how long a failure streak survives
	// without a new failure. It exists only so the map does not keep state for
	// account+model pairs that stopped being used; a streak is otherwise reset
	// by recordSuccess alone.
	//
	// It must stay well above the cooldowns. Resetting the streak on a short
	// wall-clock window makes the breaker's sensitivity depend on request rate:
	// a gateway called less often than the window never reaches streak 2, so a
	// broken upstream is never cooled down and every request pays a failed
	// attempt plus a failover before reaching a healthy account. Low-traffic
	// deployments were hit hardest, which is the opposite of what a breaker
	// should do.
	openAIModelTransientStreakTTL     = 30 * time.Minute
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
	EventID        string
	AccountID      int64
	CanonicalModel string
	Domains        []OpenAIFailureDomain
	StatusCode     int
	ErrorType      string
	TTFT           time.Duration
	OutputStarted  bool
	SafeToReplay   bool
	HasSideEffect  bool
	UsageKnown     bool
	Platform       string
	GroupID        *int64
	CacheMode      string
	Now            time.Time
}

// OpenAIAccountModelSuccessEvent describes a successful account/model attempt
// that should reset both the local breaker and the rebuildable shared health
// projection.
type OpenAIAccountModelSuccessEvent struct {
	EventID        string
	AccountID      int64
	CanonicalModel string
	Domains        []OpenAIFailureDomain
	TTFT           time.Duration
	Platform       string
	Now            time.Time
}

// recordOpenAIIncompleteStreamFailure routes an SSE that ended before a
// successful terminal event through the existing account-model transient
// state. It deliberately preserves the no-replay boundary after downstream
// output or side effects have started.
func (s *OpenAIGatewayService) recordOpenAIIncompleteStreamFailure(ctx context.Context, accountID int64, canonicalModel string, outputStarted, safeToReplay, hasSideEffect, usageKnown bool) OpenAIAccountModelRuntimeDecision {
	return s.RecordOpenAIAccountModelFailure(ctx, OpenAIAccountModelFailureEvent{
		AccountID: accountID, CanonicalModel: canonicalModel, StatusCode: 0,
		ErrorType: "transient_stream_disconnected_before_completion", OutputStarted: outputStarted,
		SafeToReplay: safeToReplay, HasSideEffect: hasSideEffect, UsageKnown: usageKnown,
		Platform: PlatformOpenAI, Now: time.Now(),
	})
}

func usageHasAnyTokens(usage *OpenAIUsage) bool {
	return usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0 || usage.ImageInputTokens > 0 || usage.ImageOutputTokens > 0)
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
	// The streak is cleared by recordSuccess. Only drop it here when the entry
	// is stale beyond the TTL, or when the clock moved backwards.
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL || now.Before(entry.lastFailure) {
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
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)) {
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

func (s *OpenAIGatewayService) RecordOpenAIAccountModelFailure(ctx context.Context, event OpenAIAccountModelFailureEvent) OpenAIAccountModelRuntimeDecision {
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
	if shared, ok := s.recordOpenAISharedHealthFailure(ctx, event, now, raw); ok {
		if shared.FailureStreak > decision.FailureStreak {
			decision.FailureStreak = shared.FailureStreak
		}
		if shared.CooldownUntil.After(decision.BlockUntil) {
			decision.BlockUntil = shared.CooldownUntil
			decision.Cooldown = shared.CooldownUntil.Sub(now)
		}
	}
	decision.CurrentRequestRetry = !event.OutputStarted && event.SafeToReplay && !event.HasSideEffect && decision.FailureStreak == 1
	if !decision.BlockUntil.IsZero() && decision.BlockUntil.After(now) {
		decision.RetryAfterSeconds = int((decision.BlockUntil.Sub(now) + time.Second - 1) / time.Second)
	}
	resilienceEvent := OpenAIResilienceEvent{
		At: now, Platform: event.Platform, GroupID: event.GroupID, Name: OpenAIEventAccountModelSoftFailure,
		AccountID: event.AccountID, CanonicalModel: event.CanonicalModel, StatusCode: event.StatusCode,
		OutputStarted: event.OutputStarted, UsageProduced: event.UsageKnown, FailureStreak: decision.FailureStreak,
		CacheMode: event.CacheMode, CooldownSeconds: int(decision.Cooldown.Seconds()), RetryAfterSeconds: decision.RetryAfterSeconds,
		Outcome: "failure",
	}
	RecordOpenAIResilienceOutcomeWithContext(ctx, resilienceEvent)
	slog.Info(OpenAIEventAccountModelSoftFailure,
		"account_id", event.AccountID, "canonical_scheduling_model", normalizeOpenAIAccountModelTransientModel(event.CanonicalModel),
		"attempt", decision.FailureStreak, "status_code", event.StatusCode, "output_started", event.OutputStarted,
		"usage_produced", event.UsageKnown, "cache_preservation_mode", event.CacheMode, "cooldown_seconds", int(decision.Cooldown.Seconds()), "retry_after_seconds", decision.RetryAfterSeconds)
	if decision.Cooldown > 0 {
		resilienceEvent.Name = OpenAIEventAccountModelCooldownStarted
		RecordOpenAIResilienceOutcomeWithContext(ctx, resilienceEvent)
		slog.Warn(OpenAIEventAccountModelCooldownStarted,
			"account_id", event.AccountID, "canonical_scheduling_model", normalizeOpenAIAccountModelTransientModel(event.CanonicalModel),
			"attempt", decision.FailureStreak, "status_code", event.StatusCode, "output_started", event.OutputStarted,
			"usage_produced", event.UsageKnown, "cache_preservation_mode", event.CacheMode, "cooldown_seconds", int(decision.Cooldown.Seconds()), "retry_after_seconds", decision.RetryAfterSeconds)
	}
	return decision
}

func (s *OpenAIGatewayService) RecordOpenAIAccountModelSuccess(ctx context.Context, event OpenAIAccountModelSuccessEvent) {
	if s == nil || event.AccountID <= 0 || normalizeOpenAIAccountModelTransientModel(event.CanonicalModel) == "" {
		return
	}
	now := event.Now
	if now.IsZero() {
		now = time.Now()
	}
	s.clearOpenAIAccountModelTransientState(event.AccountID, event.CanonicalModel)
	s.recordOpenAISharedHealthSuccess(ctx, event, now)
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
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: now, Platform: PlatformOpenAI, Name: OpenAIEventAccountModelCooldownStarted,
		AccountID: accountID, CanonicalModel: key.Model, StatusCode: entry.lastStatusCode,
		FailureStreak: entry.failureStreak, CooldownSeconds: int(cooldown.Seconds()), RetryAfterSeconds: int(cooldown.Seconds()), Outcome: "manual",
	})
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
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: now, Platform: PlatformOpenAI, Name: OpenAIEventAccountModelHalfOpenProbe,
		AccountID: accountID, CanonicalModel: canonicalModel, AttemptNumber: 1, CacheMode: "half_open_probe", Outcome: "selected",
	})
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
	sharedKey, err := NewOpenAISharedHealthKey(accountID, canonicalModel)
	if err != nil || !s.hasOpenAISharedHealthStore() {
		return acquireLocalOpenAIAccountModelHalfOpenProbe(state, key, now)
	}
	shared, sharedKnown, readErr := s.readOpenAISharedHealthSnapshot(context.Background(), sharedKey, now, true)
	if readErr != nil {
		return false
	}
	sharedEligible := sharedKnown && shared.State == OpenAISharedHealthStateCooldown && !shared.CooldownUntil.IsZero() && !now.Before(shared.CooldownUntil)
	if sharedKnown && (shared.State == OpenAISharedHealthStateHalfOpen || (shared.State == OpenAISharedHealthStateCooldown && now.Before(shared.CooldownUntil))) {
		return false
	}

	state.mu.Lock()
	entry, exists := state.entries[key]
	if exists && !entry.lastFailure.IsZero() && (now.Before(entry.lastFailure) || (now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)))) {
		delete(state.entries, key)
		state.mu.Unlock()
		return false
	}
	localEligible := exists && !entry.blockUntil.IsZero() && !now.Before(entry.blockUntil)
	if (exists && (now.Before(entry.blockUntil) || entry.halfOpenInFlight)) || (!localEligible && !sharedEligible) {
		state.mu.Unlock()
		return false
	}
	if !exists {
		entry.failureStreak = shared.FailureStreak
		entry.lastFailure = shared.ObservedAt
		entry.blockUntil = shared.CooldownUntil
	}
	entry.halfOpenInFlight = true
	entry.lastTouched = now
	state.entries[key] = entry
	state.mu.Unlock()

	lease, acquired, acquireErr := s.acquireOpenAISharedHalfOpenLease(context.Background(), sharedKey)
	if acquireErr != nil || !acquired {
		state.mu.Lock()
		entry := state.entries[key]
		entry.halfOpenInFlight = false
		entry.lastTouched = now
		state.entries[key] = entry
		state.mu.Unlock()
		return false
	}
	s.storeOpenAISharedHalfOpenLease(lease)
	return true
}

func acquireLocalOpenAIAccountModelHalfOpenProbe(state *openAIAccountModelTransientState, key openAIAccountModelKey, now time.Time) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, exists := state.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && (now.Before(entry.lastFailure) || (now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)))) {
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
	entry, exists := state.entries[key]
	if exists && success {
		delete(state.entries, key)
	} else if exists {
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
	state.mu.Unlock()

	sharedKey, err := NewOpenAISharedHealthKey(accountID, canonicalModel)
	if err != nil {
		return
	}
	lease, held := s.takeOpenAISharedHalfOpenLease(sharedKey)
	if !held {
		return
	}
	s.completeOpenAISharedHalfOpenLease(context.Background(), lease, success, now)
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
	entries := make(map[openAIAccountModelKey]openAIAccountModelTransientEntry, len(state.entries))
	for key, entry := range state.entries {
		if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL && (entry.blockUntil.IsZero() || !now.Before(entry.blockUntil)) {
			continue
		}
		entries[key] = entry
	}
	state.mu.Unlock()

	result := make([]OpenAIAccountModelRuntimeSnapshot, 0, len(entries))
	for key, entry := range entries {
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
