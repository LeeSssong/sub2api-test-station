package service

import (
	"context"
	"errors"
	"sync"
	"time"

	upstreamnotify "github.com/Wei-Shaw/sub2api/internal/notify"
)

const (
	upstreamBalanceLowRepeatInterval  = 30 * time.Minute
	upstreamBalanceZeroRepeatInterval = 5 * time.Minute
	upstreamBalanceDeliveryLease      = time.Minute
	upstreamBalanceDueInterval        = time.Minute
	upstreamBalanceActiveBatchSize    = 500
)

type UpstreamBalanceEvaluationReader interface {
	ReadUpstreamBalanceEvaluations(context.Context) ([]UpstreamBalanceEvaluation, error)
}

type UpstreamBalanceScopeRefresher interface {
	RefreshUpstreamBalanceScopes(context.Context, map[string]struct{}) error
}

type UpstreamBalanceNotificationSender interface {
	Send(context.Context, upstreamnotify.UpstreamBalanceCardInput) (string, error)
}

type UpstreamBalanceLoginLookup interface {
	Lookup(string) (string, string, bool)
}

type UpstreamBalanceNotificationService struct {
	repo       UpstreamBalanceEventRepository
	reader     UpstreamBalanceEvaluationReader
	sender     UpstreamBalanceNotificationSender
	registry   UpstreamBalanceLoginLookup
	recipients []string
	now        func() time.Time

	mu          sync.Mutex
	started     bool
	stopped     bool
	ctx         context.Context
	cancel      context.CancelFunc
	trigger     chan struct{}
	dueInterval time.Duration
	wg          sync.WaitGroup
}

func (s *AccountMonitorService) ReadUpstreamBalanceEvaluations(ctx context.Context) ([]UpstreamBalanceEvaluation, error) {
	if s == nil {
		return nil, errors.New("account monitor service unavailable")
	}
	page, err := s.ListWindow(ctx, string(AccountMonitorRange24Hours))
	if err != nil {
		return nil, err
	}
	accounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return buildUpstreamBalanceEvaluations(accounts, page)
}

func buildUpstreamBalanceEvaluations(accounts []Account, page AccountMonitorPage) ([]UpstreamBalanceEvaluation, error) {
	balances := make(map[int64]*AccountMonitorBalance, len(page.Accounts))
	for i := range page.Accounts {
		balances[page.Accounts[i].AccountID] = page.Accounts[i].Balance
	}
	ranks := make(map[int64][]UpstreamBalanceAccountRank)
	for i := range page.Groups {
		group := page.Groups[i]
		for j := range group.Accounts {
			row := group.Accounts[j]
			ranks[row.AccountID] = append(ranks[row.AccountID], UpstreamBalanceAccountRank{
				GroupName: group.Name,
				Rank:      row.SchedulerRank,
			})
		}
	}
	projected := make([]UpstreamBalanceAccount, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		baseURL, _ := account.Credentials["base_url"].(string)
		projected = append(projected, UpstreamBalanceAccount{
			AccountID:             account.ID,
			Name:                  account.Name,
			Platform:              account.Platform,
			Type:                  account.Type,
			Status:                account.Status,
			BaseURL:               baseURL,
			Snapshot:              balances[account.ID],
			CredentialFingerprint: accountMonitorBalanceCredentialFingerprint(account.GetOpenAIApiKey()),
			Ranks:                 append([]UpstreamBalanceAccountRank(nil), ranks[account.ID]...),
		})
	}
	return EvaluateUpstreamBaseURLBalance(projected, page.ObservedAt)
}

func NewUpstreamBalanceNotificationService(
	repo UpstreamBalanceEventRepository,
	reader UpstreamBalanceEvaluationReader,
	sender UpstreamBalanceNotificationSender,
	registry UpstreamBalanceLoginLookup,
	recipients []string,
) *UpstreamBalanceNotificationService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamBalanceNotificationService{
		repo: repo, reader: reader, sender: sender, registry: registry,
		recipients: append([]string(nil), recipients...), now: time.Now,
		ctx: ctx, cancel: cancel, trigger: make(chan struct{}, 1), dueInterval: upstreamBalanceDueInterval,
	}
}

func (s *UpstreamBalanceNotificationService) Start() {
	if s == nil || s.repo == nil || s.reader == nil || s.sender == nil || s.registry == nil {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.loop()
}

func (s *UpstreamBalanceNotificationService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *UpstreamBalanceNotificationService) TriggerEvaluate() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *UpstreamBalanceNotificationService) loop() {
	defer s.wg.Done()
	interval := s.dueInterval
	if interval <= 0 {
		interval = upstreamBalanceDueInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.trigger:
			_ = s.Evaluate(s.ctx)
		case <-ticker.C:
			_ = s.RunDue(s.ctx)
		}
	}
}

func (s *UpstreamBalanceNotificationService) Evaluate(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	evaluations, err := s.reader.ReadUpstreamBalanceEvaluations(ctx)
	if err != nil {
		return errors.New("read upstream balance projection")
	}
	ruleID, err := s.repo.GetRuleID(ctx)
	if err != nil {
		return errors.New("read upstream balance notification rule")
	}
	return s.processEvaluations(ctx, ruleID, evaluations, nil)
}

func (s *UpstreamBalanceNotificationService) RunDue(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	active, err := s.repo.ListActive(ctx, upstreamBalanceActiveBatchSize)
	if err != nil {
		return errors.New("list active upstream balance events")
	}
	if len(active) == 0 {
		return nil
	}
	zeroScopes := make(map[string]struct{})
	for _, event := range active {
		if event.NotificationState == UpstreamBalanceNotificationStateZero {
			zeroScopes[event.ScopeKey] = struct{}{}
		}
	}
	if refresher, ok := s.reader.(UpstreamBalanceScopeRefresher); ok && len(zeroScopes) > 0 {
		if err := refresher.RefreshUpstreamBalanceScopes(ctx, zeroScopes); err != nil {
			return errors.New("refresh active upstream balance scopes")
		}
	}
	scopes := make(map[string]struct{}, len(active))
	for _, event := range active {
		if event.Status == OpsAlertStatusFiring && event.ScopeType == UpstreamBalanceScopeTypeBaseURL {
			scopes[event.ScopeKey] = struct{}{}
		}
	}
	evaluations, err := s.reader.ReadUpstreamBalanceEvaluations(ctx)
	if err != nil {
		return errors.New("read upstream balance projection")
	}
	ruleID, err := s.repo.GetRuleID(ctx)
	if err != nil {
		return errors.New("read upstream balance notification rule")
	}
	return s.processEvaluations(ctx, ruleID, evaluations, scopes)
}

func (s *UpstreamBalanceNotificationService) processEvaluations(
	ctx context.Context,
	ruleID int64,
	evaluations []UpstreamBalanceEvaluation,
	allowedScopes map[string]struct{},
) error {
	var errs []error
	for i := range evaluations {
		evaluation := evaluations[i]
		if allowedScopes != nil {
			if _, ok := allowedScopes[evaluation.NormalizedBaseURL]; !ok {
				continue
			}
		}
		if evaluation.ValueUSD == nil || evaluation.ObservedAt.IsZero() {
			continue
		}
		if evaluation.State == UpstreamBalanceStateHealthy {
			if _, err := s.repo.Resolve(ctx, ruleID, evaluation.NormalizedBaseURL, evaluation.ObservedAt); err != nil {
				errs = append(errs, errors.New("resolve upstream balance event"))
			}
			continue
		}
		interval, state, ok := upstreamBalanceNotificationTiming(evaluation.State)
		if !ok {
			continue
		}
		lease, claimed, err := s.repo.Claim(ctx, UpstreamBalanceClaimInput{
			RuleID: ruleID, ScopeKey: evaluation.NormalizedBaseURL, NotificationState: state,
			ValueUSD: *evaluation.ValueUSD, ObservedAt: evaluation.ObservedAt, Now: s.currentTime(),
			RepeatInterval: interval, LeaseDuration: upstreamBalanceDeliveryLease,
		})
		if err != nil {
			errs = append(errs, errors.New("claim upstream balance event"))
			continue
		}
		if !claimed {
			continue
		}
		if err := s.deliver(ctx, lease); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *UpstreamBalanceNotificationService) deliver(ctx context.Context, lease UpstreamBalanceDeliveryLease) error {
	locked, err := s.repo.WithScopeLock(ctx, lease.RuleID, lease.ScopeKey, func(lockCtx context.Context) error {
		current, err := s.repo.GetCurrent(lockCtx, lease.EventID)
		if err != nil {
			return errors.New("read current upstream balance event")
		}
		if !upstreamBalanceLeaseMatches(current, lease, s.currentTime()) {
			return nil
		}
		evaluations, err := s.reader.ReadUpstreamBalanceEvaluations(lockCtx)
		if err != nil {
			return errors.New("read upstream balance projection")
		}
		evaluation, ok := findUpstreamBalanceEvaluation(evaluations, lease.ScopeKey)
		if !ok || !upstreamBalanceEvaluationMatchesLease(evaluation, lease) {
			return nil
		}
		loginAccount, loginPassword, _ := s.registry.Lookup(lease.ScopeKey)
		input := upstreamBalanceCardInput(evaluation, loginAccount, loginPassword, s.recipients)
		if _, err := s.sender.Send(lockCtx, input); err != nil {
			next := s.currentTime().Add(upstreamBalanceFailureDelay(current.DeliveryAttemptCount, lease.NotificationState))
			code := upstreamBalanceDeliveryErrorCode(err)
			_, _ = s.repo.RecordFailure(lockCtx, UpstreamBalanceDeliveryFailure{
				EventID: lease.EventID, Generation: lease.Generation, LeaseToken: lease.Token,
				NextAttemptAt: next, ErrorCode: code,
			})
			return errors.New("deliver upstream balance notification")
		}
		confirmed, err := s.repo.ConfirmDelivery(lockCtx, UpstreamBalanceDeliveryResult{
			EventID: lease.EventID, Generation: lease.Generation, LeaseToken: lease.Token, At: s.currentTime(),
		})
		if err != nil || !confirmed {
			return errors.New("confirm upstream balance delivery")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	return nil
}

func (s *UpstreamBalanceNotificationService) validate() error {
	if s == nil || s.repo == nil || s.reader == nil || s.sender == nil || s.registry == nil {
		return errors.New("upstream balance notification service unavailable")
	}
	return nil
}

func (s *UpstreamBalanceNotificationService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func upstreamBalanceNotificationTiming(state string) (time.Duration, string, bool) {
	switch state {
	case UpstreamBalanceStateLow:
		return upstreamBalanceLowRepeatInterval, UpstreamBalanceNotificationStateLow, true
	case UpstreamBalanceStateZero:
		return upstreamBalanceZeroRepeatInterval, UpstreamBalanceNotificationStateZero, true
	default:
		return 0, "", false
	}
}

func upstreamBalanceLeaseMatches(current *UpstreamBalanceEvent, lease UpstreamBalanceDeliveryLease, now time.Time) bool {
	return current != nil && current.ID == lease.EventID && current.RuleID == lease.RuleID &&
		current.Status == OpsAlertStatusFiring && current.ScopeType == UpstreamBalanceScopeTypeBaseURL &&
		current.ScopeKey == lease.ScopeKey && current.NotificationState == lease.NotificationState &&
		current.DeliveryGeneration == lease.Generation && current.DeliveryLeaseToken == lease.Token &&
		current.DeliveryLeaseUntil != nil && current.DeliveryLeaseUntil.After(now) &&
		upstreamBalanceObservationTimeEqual(current.LastObservedAt, lease.ObservedAt) && current.ValueUSD == lease.ValueUSD
}

func findUpstreamBalanceEvaluation(evaluations []UpstreamBalanceEvaluation, scopeKey string) (UpstreamBalanceEvaluation, bool) {
	for _, evaluation := range evaluations {
		if evaluation.NormalizedBaseURL == scopeKey {
			return evaluation, true
		}
	}
	return UpstreamBalanceEvaluation{}, false
}

func upstreamBalanceEvaluationMatchesLease(evaluation UpstreamBalanceEvaluation, lease UpstreamBalanceDeliveryLease) bool {
	return evaluation.ValueUSD != nil && upstreamBalanceObservationTimeEqual(evaluation.ObservedAt, lease.ObservedAt) &&
		*evaluation.ValueUSD == lease.ValueUSD && evaluation.State == lease.NotificationState
}

// PostgreSQL TIMESTAMPTZ stores microseconds, while account_monitor_balance
// JSON timestamps may retain nanoseconds. Normalize both sides before comparing
// the delivery fingerprint so a round-trip through the event ledger does not
// silently discard an otherwise current notification.
func upstreamBalanceObservationTimeEqual(a, b time.Time) bool {
	return a.Truncate(time.Microsecond).Equal(b.Truncate(time.Microsecond))
}

func upstreamBalanceCardInput(
	evaluation UpstreamBalanceEvaluation,
	loginAccount string,
	loginPassword string,
	recipients []string,
) upstreamnotify.UpstreamBalanceCardInput {
	accounts := make([]upstreamnotify.UpstreamBalanceCardAccount, 0, len(evaluation.Accounts))
	for _, account := range evaluation.Accounts {
		ranks := make([]upstreamnotify.UpstreamBalanceCardRank, 0, len(account.Ranks))
		for _, rank := range account.Ranks {
			ranks = append(ranks, upstreamnotify.UpstreamBalanceCardRank{GroupName: rank.GroupName, Rank: rank.Rank})
		}
		accounts = append(accounts, upstreamnotify.UpstreamBalanceCardAccount{ID: account.AccountID, Name: account.Name, BalanceUSD: balanceValue(account.Snapshot), Ranks: ranks})
	}
	return upstreamnotify.UpstreamBalanceCardInput{
		State: evaluation.State, ValueUSD: *evaluation.ValueUSD, BaseURL: evaluation.NormalizedBaseURL,
		LoginAccount: loginAccount, LoginPassword: loginPassword,
		RecipientOpenIDs: append([]string(nil), recipients...), Accounts: accounts,
	}
}

func balanceValue(snapshot *AccountMonitorBalance) *float64 {
	if snapshot == nil || snapshot.ValueUSD == nil || snapshot.Status != AccountMonitorBalanceStatusOK {
		return nil
	}
	value := *snapshot.ValueUSD
	return &value
}

func upstreamBalanceFailureDelay(attemptCount int, state string) time.Duration {
	delays := []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute}
	if attemptCount >= 0 && attemptCount < len(delays) {
		return delays[attemptCount]
	}
	if state == UpstreamBalanceNotificationStateZero {
		return upstreamBalanceZeroRepeatInterval
	}
	return upstreamBalanceLowRepeatInterval
}

func upstreamBalanceDeliveryErrorCode(err error) string {
	if code := upstreamnotify.CardErrorCode(err); code != "" {
		return code
	}
	return "feishu_send_failed"
}
