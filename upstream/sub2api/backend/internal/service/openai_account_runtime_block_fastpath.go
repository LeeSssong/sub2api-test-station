package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	openAIAccountStateUpdateTimeout       = 5 * time.Second
	openAIOAuth429FallbackCooldown        = 5 * time.Second
	openAIOAuth429RetryWindow             = 2 * time.Minute
	openAIOAuth429RetryDelay              = 500 * time.Millisecond
	openAIOAuth429MaxRetryDelay           = 8 * time.Second
	openAIOAuth429MaxAccountAttempts      = 3
	openAIStopSchedulingBridgeCooldown    = 2 * time.Minute
	openAIOAuth429StormWindow             = 10 * time.Second
	openAIOAuth429StormMaxAccountSwitches = 1
)

// OpenAIOAuth429FailoverState tracks the request-local follow-up budget after
// the first Grok OAuth 429. Once that 429 occurs, exactly one different account
// may be attempted; any failure from that follow-up account ends failover.
type OpenAIOAuth429FailoverState struct {
	grokOAuth429FollowupPending bool
}

type openAIOAuth429Disposition uint8

const (
	openAIOAuth429Transient openAIOAuth429Disposition = iota
	openAIOAuth429Quota5h
	openAIOAuth429Quota7d
	openAIOAuth429QuotaReset
)

// classifyOpenAIOAuth429 区分账号配额耗尽信号与普通瞬时 429。明确窗口达到
// 100% 时以该窗口为准；没有 100% 标记但包含重置头时，沿用 v179 的兼容语义，
// 仍视为配额限流信号。
func classifyOpenAIOAuth429(headers http.Header, responseBody []byte) (openAIOAuth429Disposition, *time.Time) {
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		if normalized := snapshot.Normalize(); normalized != nil {
			if normalized.Used7dPercent != nil && *normalized.Used7dPercent >= 100 {
				if normalized.Reset7dSeconds != nil {
					now := time.Now()
					resetAt := now.Add(time.Duration(*normalized.Reset7dSeconds) * time.Second)
					return openAIOAuth429Quota7d, &resetAt
				}
				return openAIOAuth429Quota7d, nil
			}
			if normalized.Used5hPercent != nil && *normalized.Used5hPercent >= 100 {
				if normalized.Reset5hSeconds != nil {
					now := time.Now()
					resetAt := now.Add(time.Duration(*normalized.Reset5hSeconds) * time.Second)
					return openAIOAuth429Quota5h, &resetAt
				}
				return openAIOAuth429Quota5h, nil
			}
		}
	}
	if resetAt := calculateOpenAI429ResetTime(headers); resetAt != nil {
		return openAIOAuth429QuotaReset, resetAt
	}
	if resetUnix := parseOpenAIRateLimitResetTime(responseBody); resetUnix != nil {
		resetAt := time.Unix(*resetUnix, 0)
		return openAIOAuth429QuotaReset, &resetAt
	}
	return openAIOAuth429Transient, nil
}

func openAIAccountStateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, openAIAccountStateUpdateTimeout)
}

func isOpenAIOAuthAccount(account *Account) bool {
	return account != nil && account.IsOpenAIOAuthLike()
}

func isGrokOAuthAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
}

func isOpenAIAccount(account *Account) bool {
	return account != nil && (account.Platform == PlatformOpenAI || account.Platform == PlatformGrok)
}

// handleOpenAIAccountUpstreamError expects canonicalModel to be the model used
// for scheduling after applying account mapping exactly once.
func (s *OpenAIGatewayService) handleOpenAIAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte, canonicalModel ...string) bool {
	if account != nil && account.Platform == PlatformGrok && isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
	}
	// Any non-2xx upstream HTTP response means the model request was actually sent.
	if s != nil {
		scheduleOllamaCloudUsageActivity(s.deferredService, account)
	}
	// Capacity shedding describes this request, not account health. Keep the
	// account schedulable while the request-local retry budget handles recovery.
	if account != nil && account.Platform == PlatformOpenAI && isOpenAIRequestScopedCapacityShed("", responseBody) {
		return false
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if account != nil && account.Platform == PlatformOpenAI && isOpenAIHTTPUpstreamAccessStateError(statusCode, "", responseBody) {
		message := "OpenAI upstream account or workspace is unavailable"
		if upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(responseBody)); upstreamMsg != "" {
			message = upstreamMsg
		}
		if s != nil && s.rateLimitService != nil {
			s.rateLimitService.handleAuthError(stateCtx, account, message)
		}
		if s != nil {
			s.BlockAccountScheduling(account, time.Time{}, "openai_access_state")
		}
		return true
	}

	if account != nil && account.Platform == PlatformOpenAI && isOpenAIContextWindowError("", responseBody) {
		return false
	}

	if isOpenAIImageRateLimitError(statusCode, responseBody) {
		if s != nil && s.rateLimitService != nil {
			_ = s.rateLimitService.HandleOpenAIImageRateLimit(stateCtx, account, statusCode, headers, responseBody)
		}
		return false
	}

	if s == nil || account == nil {
		return false
	}
	// Team 联动熔断必须先于 model-not-found 与账户级临时不可调度规则的早退。
	if s.rateLimitService != nil {
		s.rateLimitService.maybeHandleOpenAITeamLinkedError(stateCtx, account, statusCode, responseBody)
	}
	stateCtx = withTempUnschedulableModel(stateCtx, canonicalModel)
	if s.rateLimitService != nil && len(canonicalModel) > 0 && s.rateLimitService.HandleUpstreamModelNotFound(stateCtx, account, canonicalModel[0], statusCode, responseBody) {
		return true
	}
	// A non-model OpenAI 404 is a request-scoped route/channel failure. Let the
	// handler make one bounded account switch, but do not write account or
	// account-model persistent failure state.
	notFound := ClassifyOpenAINotFound(statusCode, responseBody)
	if account.Platform == PlatformOpenAI && notFound.Kind != OpenAINotFoundNone && notFound.Kind != OpenAINotFoundModel {
		return true
	}
	// Isolate a custom temporary-unschedulable match to the known upstream
	// model before entering the generic account error path. This keeps the
	// account available to other models and avoids the account runtime blocker.
	if s.rateLimitService != nil && statusCode != http.StatusUnauthorized && len(canonicalModel) > 0 && strings.TrimSpace(canonicalModel[0]) != "" &&
		s.rateLimitService.HandleTempUnschedulable(stateCtx, account, statusCode, responseBody, canonicalModel[0]) {
		return true
	}
	if statusCode == http.StatusTooManyRequests {
		s.markOpenAIOAuth429RateLimited(stateCtx, account, headers, responseBody)
	}
	if s.rateLimitService == nil {
		return false
	}
	shouldDisable := s.rateLimitService.HandleUpstreamError(stateCtx, account, statusCode, headers, responseBody)
	modelTempMatched := statusCode != http.StatusUnauthorized && tempUnschedulableModel(stateCtx, nil) != "" &&
		len(matchTempUnschedulableRules(account, statusCode, responseBody)) > 0
	if shouldDisable && !modelTempMatched {
		s.BlockAccountScheduling(account, time.Time{}, "upstream_disable")
	}
	// Pool-mode retryable upstream errors are already bounded by the request-local
	// same-account retry budget. Recording the generic account+model transient
	// cooldown here would block the next approved retry before that budget is used.
	poolModeRetryable := account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
	_, handlerOwnsAttemptFailure := OpenAIRequestAttemptMetadataFromContext(stateCtx)
	if !shouldDisable && !handlerOwnsAttemptFailure && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		shouldCooldownOpenAITransientUpstreamError(statusCode, responseBody) && !poolModeRetryable {
		model := ""
		if len(canonicalModel) > 0 {
			model = canonicalModel[0]
		}
		decision := s.RecordOpenAIAccountModelFailure(stateCtx, OpenAIAccountModelFailureEvent{
			AccountID: account.ID, CanonicalModel: model, StatusCode: statusCode,
			ErrorType: "transient_upstream", Now: time.Now(),
		})
		if decision.FailureStreak > 0 {
			slog.Warn("openai_model_transient_state",
				"account_id", account.ID,
				"model", openAIAccountModelTransientModel(model),
				"failure_streak", decision.FailureStreak,
				"cooldown_ms", decision.Cooldown.Milliseconds(),
				"block_scope", "account_model",
			)
		}
	}
	return shouldDisable
}

func shouldCooldownOpenAITransientUpstreamError(statusCode int, responseBody []byte) bool {
	switch statusCode {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 520, 521, 522, 523, 524:
		return true
	case http.StatusBadRequest:
		return isOpenAITransientProcessingError(statusCode, "", responseBody)
	default:
		return false
	}
}

func (s *OpenAIGatewayService) markOpenAIOAuth429RateLimited(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	if s == nil || !isOpenAIOAuthAccount(account) {
		return
	}
	// Spark 影子：不按 /responses 429 的 global x-codex-* 信号做内存运行时熔断(同 handle429,外审第8轮 P1)。
	// 同时避免把 spark 的 429 计入全局 429 storm 计数(recordOpenAIOAuth429),否则会误伤母账号 failover 决策。
	if account.IsShadow() {
		return
	}
	s.recordOpenAIOAuth429()
	disposition, resetAt := classifyOpenAIOAuth429(headers, responseBody)
	if disposition == openAIOAuth429Transient && s.openAIOAuth429RetryWindowActive(account) {
		return
	}

	cooldownUntil := time.Now().Add(openAIOAuth429FallbackCooldown)
	if resetAt != nil && resetAt.After(time.Now()) {
		cooldownUntil = *resetAt
	} else if retryAfter := parseRetryAfterResetTime(headers, time.Now()); retryAfter != nil && retryAfter.After(time.Now()) {
		cooldownUntil = *retryAfter
	}
	s.BlockAccountScheduling(account, cooldownUntil, "429")
	s.openaiOAuth429RetryStartedAt.Delete(account.ID)
}

// PersistOpenAIOAuth429Cooldown parks an OAuth account after the request-local
// same-account retry budget is exhausted or a failover transition is about to
// happen. The first transient 429 remains retryable and is intentionally not
// persisted by this method's callers until that transition point.
func (s *OpenAIGatewayService) PersistOpenAIOAuth429Cooldown(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) {
	if s == nil || account == nil || statusCode != http.StatusTooManyRequests || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return
	}
	fallback := true
	if classify, resetAt := classifyOpenAIOAuth429(headers, responseBody); classify != openAIOAuth429Transient {
		return
	} else if resetAt != nil && resetAt.After(time.Now()) {
		fallback = false
		s.persistOpenAIOAuth429Cooldown(ctx, account, *resetAt, fallback)
		return
	} else if retryAfter := parseRetryAfterResetTime(headers, time.Now()); retryAfter != nil && retryAfter.After(time.Now()) {
		fallback = false
		s.persistOpenAIOAuth429Cooldown(ctx, account, *retryAfter, fallback)
		return
	}
	s.persistOpenAIOAuth429Cooldown(ctx, account, time.Now().Add(openAIOAuth429FallbackCooldown), fallback)
}

type openAIAccountRateLimitExtender interface {
	SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error
}

type openAIOAuth429FallbackObservation struct {
	resetAt time.Time
}

func (s *OpenAIGatewayService) persistOpenAIOAuth429Cooldown(ctx context.Context, account *Account, resetAt time.Time, fallback bool) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	s.BlockAccountScheduling(account, resetAt, "429")
	if s.accountRepo == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	var err error
	if extender, ok := s.accountRepo.(openAIAccountRateLimitExtender); ok {
		err = extender.SetRateLimitedIfLater(stateCtx, account.ID, resetAt)
	} else {
		err = s.accountRepo.SetRateLimited(stateCtx, account.ID, resetAt)
	}
	if err != nil {
		slog.Warn("openai_oauth_429_rate_limit_persist_failed", "account_id", account.ID, "reset_at", resetAt.UTC(), "error", err)
		return
	}
	s.openaiOAuth429FallbackObservations.Delete(account.ID)
	if fallback {
		s.openaiOAuth429FallbackObservations.Store(account.ID, openAIOAuth429FallbackObservation{resetAt: resetAt})
	}
	s.openaiOAuth429RetryStartedAt.Delete(account.ID)
}

// PersistOpenAIOAuth429CooldownFromError adapts service-owned upstream errors
// such as the Codex models manifest error to the shared OAuth 429 policy.
func (s *OpenAIGatewayService) PersistOpenAIOAuth429CooldownFromError(ctx context.Context, account *Account, err error) {
	if s == nil || err == nil {
		return
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		s.PersistOpenAIOAuth429Cooldown(ctx, account, failoverErr.StatusCode, failoverErr.ResponseHeaders, failoverErr.ResponseBody)
		return
	}
	var manifestErr *codexModelsManifestUpstreamError
	if errors.As(err, &manifestErr) {
		s.PersistOpenAIOAuth429Cooldown(ctx, account, manifestErr.statusCode, manifestErr.headers, manifestErr.body)
	}
}

// RefreshOpenAIOAuth429Group clears one request's stale OAuth 429 exclusions
// so a group can make one fresh scheduling pass after its candidate pool is
// exhausted. Only short T105 cooldowns are cleared; native 7d quota pauses,
// disabled accounts, credential failures, and other durable state remain.
func (s *OpenAIGatewayService) RefreshOpenAIOAuth429Group(ctx context.Context, groupID int64, excludedIDs map[int64]struct{}) (map[int64]struct{}, error) {
	if s == nil || s.accountRepo == nil || groupID <= 0 || len(excludedIDs) == 0 {
		return nil, nil
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	accounts, err := s.accountRepo.ListByGroup(stateCtx, groupID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	cleared := make(map[int64]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account == nil || !isOpenAIOAuthAccount(account) {
			continue
		}
		if _, excluded := excludedIDs[account.ID]; !excluded {
			continue
		}
		if !account.IsActive() || !account.Schedulable || account.ErrorMessage != "" {
			continue
		}
		if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
			continue
		}
		if openAINative7dQuotaBlocked(stateCtx, account, now) {
			continue
		}
		changed := false
		observation, observed := s.openaiOAuth429FallbackObservations.Load(account.ID)
		fallback, ok := observation.(openAIOAuth429FallbackObservation)
		if observed && ok {
			clearer, supported := s.accountRepo.(openAIOAuth429FallbackClearer)
			if !supported {
				continue
			}
			clearedCurrent, err := clearer.ClearOpenAIOAuth429FallbackIfObserved(stateCtx, account.ID, fallback.resetAt)
			if err != nil {
				return cleared, err
			}
			if !clearedCurrent {
				continue
			}
			s.openaiOAuth429FallbackObservations.Delete(account.ID)
			if s.isOpenAIAccountRuntimeBlocked(account) {
				s.clearOpenAIAccountRuntimeBlockIfObserved(account.ID, fallback.resetAt)
			}
			changed = true
		}
		if changed {
			cleared[account.ID] = struct{}{}
		}
	}
	return cleared, nil
}

type openAIOAuth429FallbackClearer interface {
	ClearOpenAIOAuth429FallbackIfObserved(ctx context.Context, id int64, resetAt time.Time) (bool, error)
}

func openAINative7dQuotaBlocked(ctx context.Context, account *Account, now time.Time) bool {
	if account == nil {
		return false
	}
	if paused, decision := shouldAutoPauseOpenAIAccountByQuotaAt(ctx, account, now); paused && decision.window == "7d" {
		return true
	}
	utilization, ok := resolveOpenAIQuotaUtilization(account.Extra, "7d", now)
	return ok && utilization >= 1
}

func (s *OpenAIGatewayService) shouldRetryOpenAIOAuth429OnSameAccount(account *Account, statusCode int, shouldDisable bool) bool {
	return s.shouldRetryOpenAIOAuth429OnSameAccountWithResponse(account, statusCode, shouldDisable, nil, nil)
}

func (s *OpenAIGatewayService) shouldRetryOpenAIOAuth429OnSameAccountWithResponse(account *Account, statusCode int, shouldDisable bool, headers http.Header, responseBody []byte) bool {
	if shouldDisable || statusCode != http.StatusTooManyRequests || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return false
	}
	disposition, _ := classifyOpenAIOAuth429(headers, responseBody)
	if disposition != openAIOAuth429Transient {
		return false
	}
	// markOpenAIOAuth429RateLimited parks the account once the window expires.
	// Do not accidentally create a fresh window after that transition.
	if s.isOpenAIAccountRuntimeBlocked(account) {
		return false
	}
	return s.openAIOAuth429RetryWindowActive(account)
}

// ShouldRetryOpenAIOAuth429 lets RateLimitService defer persistent account
// cooldown until the gateway's same-account retry window is exhausted.
func (s *OpenAIGatewayService) ShouldRetryOpenAIOAuth429(account *Account, headers http.Header, responseBody []byte) bool {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() || s.isOpenAIAccountRuntimeBlocked(account) {
		return false
	}
	disposition, _ := classifyOpenAIOAuth429(headers, responseBody)
	if disposition != openAIOAuth429Transient {
		return false
	}
	return s.openAIOAuth429RetryWindowActive(account)
}

func (s *OpenAIGatewayService) openAIOAuth429RetryWindowActive(account *Account) bool {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return false
	}
	now := time.Now()
	value, _ := s.openaiOAuth429RetryStartedAt.LoadOrStore(account.ID, now)
	startedAt, ok := value.(time.Time)
	if !ok {
		s.openaiOAuth429RetryStartedAt.Store(account.ID, now)
		startedAt = now
	}
	return now.Before(startedAt.Add(openAIOAuth429RetryWindow))
}

func (s *OpenAIGatewayService) openAIOAuth429RetryDeadline(account *Account) time.Time {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return time.Time{}
	}
	value, ok := s.openaiOAuth429RetryStartedAt.Load(account.ID)
	if !ok {
		return time.Time{}
	}
	startedAt, ok := value.(time.Time)
	if !ok {
		return time.Time{}
	}
	return startedAt.Add(openAIOAuth429RetryWindow)
}

func openAIOAuth429SameAccountRetryDelay(headers http.Header, deadline time.Time) time.Duration {
	delay := openAIOAuth429RetryDelay
	now := time.Now()
	if resetAt := parseRetryAfterResetTime(headers, now); resetAt != nil && resetAt.After(now) {
		delay = resetAt.Sub(now)
	}
	if delay > openAIOAuth429MaxRetryDelay {
		delay = openAIOAuth429MaxRetryDelay
	}
	if remaining := time.Until(deadline); !deadline.IsZero() && delay > remaining {
		delay = remaining
	}
	if delay < 0 {
		return 0
	}
	return delay
}

func (s *OpenAIGatewayService) BlockAccountScheduling(account *Account, until time.Time, reason string) {
	if s == nil || !isOpenAIAccount(account) {
		return
	}
	mu := s.openAIAccountRuntimeBlockLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	_, _ = s.blockAccountSchedulingLocked(account, until, reason)
}

func (s *OpenAIGatewayService) openAIAccountRuntimeBlockLock(accountID int64) *sync.Mutex {
	actual, _ := s.openaiAccountRuntimeBlockLocks.LoadOrStore(accountID, &sync.Mutex{})
	mu, ok := actual.(*sync.Mutex)
	if !ok {
		mu = &sync.Mutex{}
		s.openaiAccountRuntimeBlockLocks.Store(accountID, mu)
	}
	return mu
}

func (s *OpenAIGatewayService) blockAccountSchedulingLocked(account *Account, until time.Time, _ string) (uint64, bool) {
	generation := s.openaiAccountRuntimeBlockSequence.Add(1)
	s.openaiAccountRuntimeBlockGeneration.Store(account.ID, generation)
	now := time.Now()
	blockUntil := until
	if blockUntil.IsZero() || !blockUntil.After(now) {
		blockUntil = now.Add(openAIStopSchedulingBridgeCooldown)
	}

	for {
		current, loaded := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
		if !loaded {
			actual, stored := s.openaiAccountRuntimeBlockUntil.LoadOrStore(account.ID, blockUntil)
			if !stored {
				return generation, true
			}
			current = actual
		}

		currentUntil, ok := current.(time.Time)
		if !ok || currentUntil.IsZero() {
			if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, blockUntil) {
				return generation, true
			}
			continue
		}
		if !blockUntil.After(currentUntil) {
			return generation, false
		}
		if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, blockUntil) {
			return generation, true
		}
	}
}

func (s *OpenAIGatewayService) ClearAccountSchedulingBlock(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	mu := s.openAIAccountRuntimeBlockLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	s.openaiAccountRuntimeBlockUntil.Delete(accountID)
	s.openaiOAuth429RetryStartedAt.Delete(accountID)
	s.openaiOAuth429FallbackObservations.Delete(accountID)
	s.openaiAccountRuntimeBlockGeneration.Store(accountID, s.openaiAccountRuntimeBlockSequence.Add(1))
}

func (s *OpenAIGatewayService) clearOpenAIAccountRuntimeBlockIfObserved(accountID int64, resetAt time.Time) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	mu := s.openAIAccountRuntimeBlockLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	value, ok := s.openaiAccountRuntimeBlockUntil.Load(accountID)
	if !ok {
		return false
	}
	current, ok := value.(time.Time)
	if !ok || !current.Equal(resetAt) {
		return false
	}
	s.openaiAccountRuntimeBlockUntil.Delete(accountID)
	s.openaiAccountRuntimeBlockGeneration.Store(accountID, s.openaiAccountRuntimeBlockSequence.Add(1))
	return true
}

func (s *OpenAIGatewayService) isOpenAIAccountRuntimeBlocked(account *Account) bool {
	return s.isOpenAIAccountRuntimeBlockedAt(account, time.Now())
}

func (s *OpenAIGatewayService) isOpenAIAccountRuntimeBlockedAt(account *Account, now time.Time) bool {
	if s == nil || !isOpenAIAccount(account) {
		return false
	}
	mu := s.openAIAccountRuntimeBlockLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	value, ok := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
	if !ok {
		return false
	}
	cooldownUntil, ok := value.(time.Time)
	if !ok || cooldownUntil.IsZero() {
		s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
		s.openaiAccountRuntimeBlockGeneration.Store(account.ID, s.openaiAccountRuntimeBlockSequence.Add(1))
		return false
	}
	if now.Before(cooldownUntil) {
		return true
	}
	s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
	s.openaiAccountRuntimeBlockGeneration.Store(account.ID, s.openaiAccountRuntimeBlockSequence.Add(1))
	return false
}

func (s *OpenAIGatewayService) isOpenAIAccountRuntimeBlockedAtReadOnly(account *Account, now time.Time) bool {
	if s == nil || !isOpenAIAccount(account) {
		return false
	}
	value, ok := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
	if !ok {
		return false
	}
	cooldownUntil, ok := value.(time.Time)
	return ok && !cooldownUntil.IsZero() && now.Before(cooldownUntil)
}

func (s *OpenAIGatewayService) getOpenAIAccountModelTransientState() *openAIAccountModelTransientState {
	if s == nil {
		return nil
	}
	s.openaiModelTransientOnce.Do(func() {
		if s.openaiModelTransient == nil {
			s.openaiModelTransient = newOpenAIAccountModelTransientState(openAIModelTransientDefaultMax)
		}
	})
	return s.openaiModelTransient
}

func canonicalOpenAIAccountSchedulingModel(account *Account, requestedModel string) string {
	model := strings.TrimSpace(requestedModel)
	if account == nil || model == "" {
		return model
	}
	if account.IsOpenAI() {
		return resolveOpenAIAccountUpstreamModelForRequest(account, model, false)
	}
	if mapped := strings.TrimSpace(account.GetMappedModel(model)); mapped != "" {
		return mapped
	}
	return model
}

func openAIAccountModelTransientModel(canonicalModel string) string {
	return normalizeOpenAIAccountModelTransientModel(canonicalModel)
}

func (s *OpenAIGatewayService) recordOpenAIAccountModelTransientFailure(account *Account, canonicalModel string, now time.Time) openAIAccountModelTransientDecision {
	if s == nil || account == nil {
		return openAIAccountModelTransientDecision{}
	}
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return openAIAccountModelTransientDecision{}
	}
	return state.recordFailure(account.ID, openAIAccountModelTransientModel(canonicalModel), now)
}

func (s *OpenAIGatewayService) clearOpenAIAccountModelTransientState(accountID int64, model string) {
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return
	}
	state.recordSuccess(accountID, model)
}

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlocked(account *Account, requestedModel string) bool {
	return s.isOpenAIAccountModelRuntimeBlockedAt(account, requestedModel, time.Now())
}

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlockedAt(account *Account, requestedModel string, now time.Time) bool {
	return s.isOpenAIAccountModelRuntimeBlockedAtContext(context.Background(), account, requestedModel, now, true)
}

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlockedAtContext(ctx context.Context, account *Account, requestedModel string, now time.Time, allowSharedRead bool) bool {
	if s == nil || account == nil {
		return false
	}
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return false
	}
	canonicalModel := canonicalOpenAIAccountSchedulingModel(account, requestedModel)
	if state.isBlocked(account.ID, openAIAccountModelTransientModel(canonicalModel), now) {
		return true
	}
	key, err := NewOpenAISharedHealthKey(account.ID, canonicalModel)
	if err != nil {
		return false
	}
	snapshot, known, _ := s.readOpenAISharedHealthSnapshot(ctx, key, now, allowSharedRead)
	return known && openAISharedHealthSnapshotBlocks(snapshot)
}

func (s *OpenAIGatewayService) isOpenAIAccountRequestRuntimeBlocked(account *Account, requestedModel string) bool {
	return s != nil && (s.isOpenAIAccountRuntimeBlocked(account) || s.isOpenAIAccountModelRuntimeBlocked(account, requestedModel))
}

func (s *OpenAIGatewayService) isOpenAIAccountRequestRuntimeBlockedWithLease(account *Account, requestedModel string, lease *openAIAccountModelHalfOpenLease) bool {
	if s == nil {
		return false
	}
	if s.isOpenAIAccountRuntimeBlocked(account) {
		return true
	}
	return !lease.matches(account, requestedModel) && s.isOpenAIAccountModelRuntimeBlocked(account, requestedModel)
}

func (s *OpenAIGatewayService) isOpenAIAccountRequestRuntimeBlockedAtReadOnly(account *Account, requestedModel string, now time.Time) bool {
	if s == nil {
		return false
	}
	return s.isOpenAIAccountRuntimeBlockedAtReadOnly(account, now) || s.isOpenAIAccountModelRuntimeBlockedAtReadOnly(account, requestedModel, now)
}

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlockedAtReadOnly(account *Account, requestedModel string, now time.Time) bool {
	return s.isOpenAIAccountModelRuntimeBlockedAtReadOnlyContext(context.Background(), account, requestedModel, now, false)
}

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlockedAtReadOnlyContext(ctx context.Context, account *Account, requestedModel string, now time.Time, allowSharedRead bool) bool {
	if s == nil || account == nil {
		return false
	}
	canonicalModel := canonicalOpenAIAccountSchedulingModel(account, requestedModel)
	if state := s.getOpenAIAccountModelTransientState(); state != nil && state.isBlockedReadOnly(account.ID, openAIAccountModelTransientModel(canonicalModel), now) {
		return true
	}
	key, err := NewOpenAISharedHealthKey(account.ID, canonicalModel)
	if err != nil {
		return false
	}
	snapshot, known, _ := s.readOpenAISharedHealthSnapshot(ctx, key, now, allowSharedRead)
	return known && openAISharedHealthSnapshotBlocks(snapshot)
}

func (s *OpenAIGatewayService) recordOpenAIOAuth429() {
	if s == nil {
		return
	}
	now := time.Now()
	windowStart := s.openaiOAuth429WindowStartUnixNano.Load()
	if windowStart == 0 || now.Sub(time.Unix(0, windowStart)) >= openAIOAuth429StormWindow {
		if s.openaiOAuth429WindowStartUnixNano.CompareAndSwap(windowStart, now.UnixNano()) {
			s.openaiOAuth429WindowCount.Store(1)
			return
		}
	}
	s.openaiOAuth429WindowCount.Add(1)
}

func (s *OpenAIGatewayService) ShouldStopOpenAIOAuth429Failover(account *Account, statusCode int, failedSwitches int, state *OpenAIOAuth429FailoverState) bool {
	if failedSwitches < openAIOAuth429StormMaxAccountSwitches {
		return false
	}
	if state != nil && state.grokOAuth429FollowupPending {
		// The follow-up budget was armed by a Grok OAuth 429. Consume it on
		// any failing follow-up account, even if a mixed pool selected an API-key
		// account next.
		return true
	}
	if isGrokOAuthAccount(account) {
		if state == nil {
			// Preserve the old threshold for callers that have not adopted the
			// request-local state contract yet.
			return statusCode == http.StatusTooManyRequests && failedSwitches >= 2
		}
		if statusCode == http.StatusTooManyRequests {
			state.grokOAuth429FollowupPending = true
		}
		return false
	}
	if statusCode != http.StatusTooManyRequests || !isOpenAIOAuthAccount(account) {
		return false
	}
	// Each OpenAI OAuth candidate has already consumed its full same-account
	// retry window before reaching this switch point. A global storm is useful
	// telemetry, but must not prevent trying the bounded next-account budget.
	return failedSwitches >= openAIOAuth429MaxAccountAttempts
}
