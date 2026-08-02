package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	accountMonitorMultiplierRefreshTimeout = 2 * time.Minute
	accountMonitorProbeTimeout             = 60 * time.Second
)

type accountMonitorProbeConnection func(
	context.Context,
	int64,
	string,
	string,
	string,
) (AccountMonitorProbeResult, error)

type accountMonitorRunKind uint8

const (
	accountMonitorFullRun accountMonitorRunKind = iota
	accountMonitorSingleRun
)

type accountMonitorRun struct {
	kind      accountMonitorRunKind
	done      chan struct{}
	completed int
	err       error
}

type accountMonitorMultiplierResolver interface {
	Resolve(*Account, time.Time) AccountMonitorMultiplier
	Refresh(context.Context, *Account, bool) error
}

type AccountMonitorService struct {
	repo        AccountMonitorRepository
	accountRepo AccountMonitorAccountRepository
	testService *AccountTestService
	usage       *AccountUsageService
	multiplier  accountMonitorMultiplierResolver

	probeConnection accountMonitorProbeConnection
	probeTimeout    time.Duration

	runStateMu sync.Mutex
	activeRun  *accountMonitorRun
}

type AccountMonitorAccountRepository interface {
	ListAllWithFilters(
		ctx context.Context,
		platform string,
		accountType string,
		status string,
		search string,
		groupID int64,
		privacyMode string,
	) ([]Account, error)
}

func NewAccountMonitorService(
	repo AccountMonitorRepository,
	accountRepo AccountMonitorAccountRepository,
	testService *AccountTestService,
	usage *AccountUsageService,
	multiplier accountMonitorMultiplierResolver,
) *AccountMonitorService {
	return &AccountMonitorService{
		repo:         repo,
		accountRepo:  accountRepo,
		testService:  testService,
		usage:        usage,
		multiplier:   multiplier,
		probeTimeout: accountMonitorProbeTimeout,
	}
}

func (s *AccountMonitorService) List(ctx context.Context) (AccountMonitorPage, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	accounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	aggregates, err := s.repo.ListAggregates(ctx, ids, time.Now().Add(-AccountMonitorHistoryDays*24*time.Hour))
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor aggregates: %w", err)
	}
	latest, err := s.repo.ListLatest(ctx, ids)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor latest: %w", err)
	}
	today, err := s.loadTodayStats(ctx, ids)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor groups: %w", err)
	}
	for i := range groups {
		if groups[i].ScoreWeights == (AccountMonitorScoreWeights{}) {
			groups[i].ScoreWeights = DefaultAccountMonitorScoreWeights
		}
	}

	observedAt := time.Now().UTC()
	rows := make([]AccountMonitorAccount, 0, len(accounts))
	schedulableIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		aggregate := aggregates[account.ID]
		row := AccountMonitorAccount{
			AccountID:    account.ID,
			Name:         account.Name,
			Platform:     account.Platform,
			AccountType:  account.Type,
			Status:       account.Status,
			Schedulable:  account.Schedulable,
			Priority:     account.Priority,
			HomepageURL:  accountMonitorHomepageURL(account),
			GroupIDs:     append([]int64{}, account.GroupIDs...),
			GroupNames:   accountGroupNames(account),
			ModelID:      monitorModelForAccount(&account),
			LatestStatus: "unavailable",
			SuccessRate:  aggregate.SuccessRate,
			SampleCount:  aggregate.SampleCount,
			TTFTP50MS:    aggregate.TTFTP50MS,
			TTFTP95MS:    aggregate.TTFTP95MS,
			LatencyP95MS: aggregate.LatencyP95MS,
			Multiplier:   s.resolveMultiplier(&account, observedAt),
			ErrorCount:   int64(aggregate.ErrorCount),
		}
		if stats := today[account.ID]; stats != nil {
			row.RequestCount = stats.Requests
			row.TodayStats = stats
		}
		if current, ok := latest[account.ID]; ok {
			row.LatestStatus = current.Status
			row.ErrorCode = current.ErrorCode
			row.Latest = &current
			checkedAt := current.CheckedAt.UTC()
			row.CheckedAt = &checkedAt
			row.Stale = observedAt.Sub(checkedAt) > time.Duration(settings.IntervalSeconds*2)*time.Second
		} else {
			row.Stale = true
		}
		row.UsageWindows = s.loadUsageWindows(ctx, account.ID)
		row.MonitorBucket = accountMonitorGlobalBucket(account, row, observedAt)
		rows = append(rows, row)
		if !accountMonitorAccountPaused(account, observedAt) {
			schedulableIDs = append(schedulableIDs, account.ID)
		}
	}
	groups = s.projectGroupQualityEvidence(ctx, groups, accounts, rows, aggregates, latest, settings, observedAt)
	health := summarizeAccountMonitorHealth(rows)
	if provider, ok := s.repo.(AccountMonitorCombinedAggregateRepository); ok {
		if aggregate, err := provider.LoadAggregate(ctx, schedulableIDs, observedAt.Add(-AccountMonitorHistoryDays*24*time.Hour)); err == nil {
			health = applyAccountMonitorAggregate(health, aggregate)
		}
	}

	return AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		SchemaVersion: AccountMonitorSchemaVersion,
		ObservedAt:    observedAt,
		Stale:         len(rows) == 0 || anyMonitorRowStale(rows),
		Settings:      settings,
		Health:        health,
		Groups:        groups,
		Accounts:      rows,
	}}, nil
}

func (s *AccountMonitorService) projectGroupQualityEvidence(
	ctx context.Context,
	groups []AccountMonitorGroup,
	accounts []Account,
	rows []AccountMonitorAccount,
	globalAggregates map[int64]AccountMonitorAggregate,
	latest map[int64]AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
) []AccountMonitorGroup {
	rowsByID := make(map[int64]AccountMonitorAccount, len(rows))
	accountsByID := make(map[int64]Account, len(accounts))
	for i := range rows {
		rowsByID[rows[i].AccountID] = rows[i]
	}
	for i := range accounts {
		accountsByID[accounts[i].ID] = accounts[i]
	}
	for i := range groups {
		group := &groups[i]
		members := make([]int64, 0)
		for _, account := range accounts {
			if accountMonitorAccountInGroup(account, group.ID) {
				members = append(members, account.ID)
			}
		}
		groupAggregates := map[int64]AccountMonitorAggregate(nil)
		groupEvidenceAvailable := false
		if provider, ok := s.repo.(AccountMonitorGroupAggregateRepository); ok && len(members) > 0 {
			loaded, err := provider.ListGroupAggregates(ctx, group.ID, members, now.Add(-AccountMonitorGroupEvidenceWindow))
			if err == nil {
				groupAggregates = loaded
				groupEvidenceAvailable = true
			}
		}
		combinedAggregate := AccountMonitorAggregate{}
		combinedAggregateAvailable := false
		if provider, ok := s.repo.(AccountMonitorCombinedAggregateRepository); ok {
			loaded, err := provider.LoadGroupAggregate(ctx, group.ID, now.Add(-AccountMonitorGroupEvidenceWindow))
			if err == nil {
				combinedAggregate = loaded
				combinedAggregateAvailable = true
			}
		}
		projected := make([]AccountMonitorGroupAccount, 0, len(members))
		for _, accountID := range members {
			base, ok := rowsByID[accountID]
			account, accountOK := accountsByID[accountID]
			if !ok || !accountOK {
				continue
			}
			groupAggregate := groupAggregates[accountID]
			globalAggregate := globalAggregates[accountID]
			evidence, _ := accountMonitorEvidence(groupAggregate, globalAggregate, groupEvidenceAvailable, latest[accountID], settings, now)
			row := AccountMonitorGroupAccount{AccountMonitorAccount: base, Evidence: evidence}
			row.SampleCount = evidence.SampleCount
			row.SuccessRate = evidence.SuccessRate
			row.TTFTP50MS = evidence.TTFTP50MS
			row.LatencyP95MS = evidence.LatencyP95MS
			if checkedAt := evidence.ObservedAt; !checkedAt.IsZero() {
				checkedAt = checkedAt.UTC()
				row.CheckedAt = &checkedAt
			}
			costEligible := account.BillingRateMultiplier() <= group.RateMultiplier
			serviceEligible := !accountMonitorAccountPaused(account, now)
			row.MonitorBucket = accountMonitorGroupBucket(account, row, evidence, costEligible, now)
			row.Eligible = evidence.Source != "stale" && costEligible && serviceEligible
			if row.Eligible {
				row.QualityScore = CalculateAccountMonitorQualityScore(group.RateMultiplier, account.BillingRateMultiplier(), group.ScoreWeights, evidence)
			}
			projected = append(projected, row)
		}
		sort.SliceStable(projected, func(left, right int) bool {
			if projected[left].Eligible != projected[right].Eligible {
				return projected[left].Eligible
			}
			if projected[left].Eligible && projected[right].Eligible {
				leftScore, rightScore := 0.0, 0.0
				if projected[left].QualityScore != nil {
					leftScore = *projected[left].QualityScore
				}
				if projected[right].QualityScore != nil {
					rightScore = *projected[right].QualityScore
				}
				if leftScore != rightScore {
					return leftScore > rightScore
				}
			}
			return projected[left].AccountID < projected[right].AccountID
		})
		rank := 0
		for j := range projected {
			if projected[j].Eligible {
				rank++
				value := rank
				projected[j].GroupRank = &value
			}
		}
		group.Accounts = projected
		group.Health = summarizeGroupHealth(projected)
		if combinedAggregateAvailable {
			group.Health = applyAccountMonitorAggregate(group.Health, combinedAggregate)
		}
		if !group.CustomerVisible {
			group.OperationalState = "closed"
		} else if group.Health.AvailableAccounts > 0 {
			group.OperationalState = "operational"
		} else {
			group.OperationalState = "unavailable"
		}
	}
	return groups
}

func accountMonitorAccountInGroup(account Account, groupID int64) bool {
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	for _, group := range account.Groups {
		if group != nil && group.ID == groupID {
			return true
		}
	}
	return false
}

func accountMonitorAccountPaused(account Account, now time.Time) bool {
	if account.Status != StatusActive || !account.Schedulable {
		return true
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !account.ExpiresAt.After(now) {
		return true
	}
	for _, until := range []*time.Time{account.TempUnschedulableUntil, account.RateLimitResetAt, account.OverloadUntil} {
		if until != nil && until.After(now) {
			return true
		}
	}
	return false
}

func (s *AccountMonitorService) listMonitorAccounts(ctx context.Context) ([]Account, error) {
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return nil, fmt.Errorf("list active monitor accounts: %w", err)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	return accounts, nil
}

func summarizeAccountMonitorHealth(rows []AccountMonitorAccount) AccountMonitorHealthSummary {
	samples := make([]accountMonitorHealthSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, accountMonitorHealthSample{
			bucket:      row.MonitorBucket,
			serviceable: row.MonitorBucket != "paused",
			sampleCount: row.SampleCount,
			successRate: row.SuccessRate,
			ttftP50MS:   row.TTFTP50MS,
			latencyP95:  row.LatencyP95MS,
		})
	}
	return summarizeHealthSamples(samples)
}

func summarizeGroupHealth(rows []AccountMonitorGroupAccount) AccountMonitorHealthSummary {
	samples := make([]accountMonitorHealthSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, accountMonitorHealthSample{
			bucket:      row.MonitorBucket,
			serviceable: row.MonitorBucket != "paused",
			sampleCount: row.SampleCount,
			successRate: row.SuccessRate,
			ttftP50MS:   row.TTFTP50MS,
			latencyP95:  row.LatencyP95MS,
		})
	}
	return summarizeHealthSamples(samples)
}

func applyAccountMonitorAggregate(summary AccountMonitorHealthSummary, aggregate AccountMonitorAggregate) AccountMonitorHealthSummary {
	if aggregate.SampleCount == 0 {
		return summary
	}
	summary.SuccessRate = aggregate.SuccessRate
	summary.TTFTP50MS = aggregate.TTFTP50MS
	summary.LatencyP95MS = aggregate.LatencyP95MS
	return summary
}

type accountMonitorHealthSample struct {
	bucket      string
	serviceable bool
	sampleCount int
	successRate float64
	ttftP50MS   *float64
	latencyP95  *float64
}

func summarizeHealthSamples(rows []accountMonitorHealthSample) AccountMonitorHealthSummary {
	var summary AccountMonitorHealthSummary
	summary.TotalAccounts = len(rows)
	var samples float64
	var successes float64
	var ttftWeighted float64
	var ttftSamples float64
	var latencyWeighted float64
	var latencySamples float64
	for _, row := range rows {
		switch row.bucket {
		case "paused":
			summary.PausedAccounts++
		case "pending":
			summary.PendingAccounts++
		case "available":
			summary.AvailableAccounts++
		default:
			summary.UnavailableAccounts++
		}
		if !row.serviceable {
			continue
		}
		weight := float64(row.sampleCount)
		if weight <= 0 {
			continue
		}
		samples += weight
		successes += row.successRate * weight
		if row.ttftP50MS != nil {
			ttftWeighted += *row.ttftP50MS * weight
			ttftSamples += weight
		}
		if row.latencyP95 != nil {
			latencyWeighted += *row.latencyP95 * weight
			latencySamples += weight
		}
	}
	if samples > 0 {
		summary.SuccessRate = successes / samples
	}
	if ttftSamples > 0 {
		value := ttftWeighted / ttftSamples
		summary.TTFTP50MS = &value
	}
	if latencySamples > 0 {
		value := latencyWeighted / latencySamples
		summary.LatencyP95MS = &value
	}
	return summary
}

func accountMonitorGlobalBucket(account Account, row AccountMonitorAccount, now time.Time) string {
	if accountMonitorAccountPaused(account, now) {
		return "paused"
	}
	if row.Stale || row.Latest == nil {
		return "pending"
	}
	if row.LatestStatus == "success" {
		return "available"
	}
	return "unavailable"
}

func accountMonitorGroupBucket(
	account Account,
	row AccountMonitorGroupAccount,
	evidence AccountMonitorQualityEvidence,
	costEligible bool,
	now time.Time,
) string {
	if accountMonitorAccountPaused(account, now) {
		return "paused"
	}
	if evidence.Source == "stale" || row.CheckedAt == nil {
		return "pending"
	}
	if !costEligible {
		return "cost_ineligible"
	}
	if row.LatestStatus == "success" && evidence.SuccessRate > 0 {
		return "available"
	}
	return "unavailable"
}

func accountMonitorEvidence(
	groupAggregate AccountMonitorAggregate,
	globalAggregate AccountMonitorAggregate,
	groupEvidenceAvailable bool,
	latest AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
) (AccountMonitorQualityEvidence, AccountMonitorAggregate) {
	aggregate := groupAggregate
	source := "group"
	if !groupEvidenceAvailable || aggregate.SampleCount < AccountMonitorGroupEvidenceMinSamples {
		aggregate = globalAggregate
		source = "global_fallback"
	}
	if aggregate.SampleCount < AccountMonitorGroupEvidenceMinSamples {
		source = "stale"
	}
	observedAt := time.Time{}
	if aggregate.LastCheckedAt != nil {
		observedAt = *aggregate.LastCheckedAt
	}
	if observedAt.IsZero() {
		observedAt = latest.CheckedAt
	}
	if observedAt.IsZero() || now.Sub(observedAt) > time.Duration(settings.IntervalSeconds*2)*time.Second {
		source = "stale"
	}
	evidence := AccountMonitorQualityEvidence{
		Source: source, SampleCount: aggregate.SampleCount, SuccessRate: aggregate.SuccessRate,
		TTFTP50MS: aggregate.TTFTP50MS, LatencyP95MS: aggregate.LatencyP95MS, ObservedAt: observedAt.UTC(),
	}
	if source == "stale" {
		return evidence, aggregate
	}
	return evidence, aggregate
}

// CalculateAccountMonitorQualityScore is deliberately pure: it only uses the
// group/account billing multipliers, score weights, and supplied evidence.
func CalculateAccountMonitorQualityScore(
	groupMultiplier float64,
	accountMultiplier float64,
	weights AccountMonitorScoreWeights,
	evidence AccountMonitorQualityEvidence,
) *float64 {
	if groupMultiplier <= 0 || evidence.Source == "stale" {
		return nil
	}
	clamp01 := func(value float64) float64 {
		if value < 0 {
			return 0
		}
		if value > 1 {
			return 1
		}
		return value
	}
	latencyScore := func(value *float64) float64 {
		if value == nil {
			return 0
		}
		return clamp01(1 - (*value / 1000))
	}
	costAdvantage := float64(weights.Cost) * (groupMultiplier - accountMultiplier) / groupMultiplier
	if costAdvantage < 0 {
		costAdvantage = 0
	}
	if costAdvantage > float64(weights.Cost) {
		costAdvantage = float64(weights.Cost)
	}
	score := costAdvantage + float64(weights.Success)*clamp01(evidence.SuccessRate) +
		float64(weights.TTFT)*latencyScore(evidence.TTFTP50MS) +
		float64(weights.Latency)*latencyScore(evidence.LatencyP95MS)
	return &score
}

func accountMonitorHomepageURL(account Account) string {
	if account.Type != AccountTypeAPIKey || account.Credentials == nil {
		return ""
	}
	raw, ok := account.Credentials["base_url"].(string)
	if !ok || raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (s *AccountMonitorService) resolveMultiplier(account *Account, now time.Time) AccountMonitorMultiplier {
	if s == nil || s.multiplier == nil {
		return unavailableAccountMultiplier()
	}
	return s.multiplier.Resolve(account, now)
}

func (s *AccountMonitorService) RunAll(ctx context.Context, actorID int64) (int, error) {
	run, leader, err := s.beginRun(ctx, accountMonitorFullRun)
	if err != nil {
		return 0, err
	}
	if !leader {
		return run.completed, run.err
	}

	completed, runErr := s.runAll(ctx, actorID)
	s.finishRun(run, completed, runErr)
	return completed, runErr
}

func (s *AccountMonitorService) runAll(ctx context.Context, actorID int64) (int, error) {
	accounts, err := s.listPool(ctx)
	if err != nil {
		return 0, err
	}
	runID := uuid.NewString()
	var mu sync.Mutex
	var completed int
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, account := range accounts {
		account := account
		g.Go(func() error {
			result := s.probeAccount(gctx, account)
			if err := s.repo.InsertResult(gctx, result, runID); err != nil {
				return fmt.Errorf("persist account %d monitor result: %w", account.ID, err)
			}
			s.refreshMultiplier(gctx, &account, false)
			mu.Lock()
			completed++
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return completed, err
	}
	if err := s.repo.DeleteBefore(ctx, time.Now().Add(-AccountMonitorHistoryDays*24*time.Hour)); err != nil {
		return completed, fmt.Errorf("delete account monitor history: %w", err)
	}
	_ = actorID
	return completed, nil
}

func (s *AccountMonitorService) RunOne(
	ctx context.Context,
	actorID int64,
	accountID int64,
) (AccountMonitorProbeResult, error) {
	run, _, err := s.beginRun(ctx, accountMonitorSingleRun)
	if err != nil {
		return AccountMonitorProbeResult{}, err
	}
	var completed int
	var runErr error
	defer func() {
		s.finishRun(run, completed, runErr)
	}()

	accounts, err := s.listPool(ctx)
	if err != nil {
		runErr = err
		return AccountMonitorProbeResult{}, err
	}
	var target *Account
	for i := range accounts {
		if accounts[i].ID == accountID {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		runErr = fmt.Errorf("account %d is not active and schedulable", accountID)
		return AccountMonitorProbeResult{}, runErr
	}
	result := s.probeAccount(ctx, *target)
	if err := s.repo.InsertResult(ctx, result, uuid.NewString()); err != nil {
		runErr = err
		return AccountMonitorProbeResult{}, err
	}
	completed = 1
	s.refreshMultiplier(ctx, target, true)
	_ = s.repo.DeleteBefore(ctx, time.Now().Add(-AccountMonitorHistoryDays*24*time.Hour))
	_ = actorID
	return result, nil
}

func (s *AccountMonitorService) beginRun(
	ctx context.Context,
	kind accountMonitorRunKind,
) (*accountMonitorRun, bool, error) {
	for {
		s.runStateMu.Lock()
		active := s.activeRun
		if active == nil {
			run := &accountMonitorRun{kind: kind, done: make(chan struct{})}
			s.activeRun = run
			s.runStateMu.Unlock()
			return run, true, nil
		}
		s.runStateMu.Unlock()

		if kind == accountMonitorFullRun && active.kind == accountMonitorFullRun {
			select {
			case <-active.done:
				return active, false, nil
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}

		select {
		case <-active.done:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (s *AccountMonitorService) finishRun(run *accountMonitorRun, completed int, err error) {
	if run == nil {
		return
	}
	s.runStateMu.Lock()
	run.completed = completed
	run.err = err
	if s.activeRun == run {
		s.activeRun = nil
	}
	close(run.done)
	s.runStateMu.Unlock()
}

func (s *AccountMonitorService) refreshMultiplier(ctx context.Context, account *Account, force bool) {
	if s == nil || s.multiplier == nil || account == nil {
		return
	}
	refreshCtx, cancel := context.WithTimeout(ctx, accountMonitorMultiplierRefreshTimeout)
	defer cancel()
	if err := s.multiplier.Refresh(refreshCtx, account, force); err != nil {
		slog.Warn("account_monitor: multiplier refresh failed",
			"account_id", account.ID,
			"error", err,
		)
	}
}

func (s *AccountMonitorService) UpdateSettings(
	ctx context.Context,
	actorID int64,
	intervalSeconds int,
) (AccountMonitorSettings, error) {
	if intervalSeconds < AccountMonitorMinIntervalSeconds ||
		intervalSeconds > AccountMonitorMaxIntervalSeconds {
		return AccountMonitorSettings{}, fmt.Errorf("interval_seconds must be between %d and %d",
			AccountMonitorMinIntervalSeconds, AccountMonitorMaxIntervalSeconds)
	}
	settings := AccountMonitorSettings{
		IntervalSeconds: intervalSeconds,
		UpdatedBy:       actorID,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := s.repo.SaveSettings(ctx, settings); err != nil {
		return AccountMonitorSettings{}, err
	}
	return s.loadSettings(ctx)
}

func (s *AccountMonitorService) GetGroupScoreWeights(
	ctx context.Context,
	groupID int64,
) (AccountMonitorScoreWeights, error) {
	if groupID <= 0 {
		return AccountMonitorScoreWeights{}, errors.New("invalid group id")
	}
	weights, err := s.repo.LoadGroupScoreWeights(ctx, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultAccountMonitorScoreWeights, nil
	}
	if err != nil {
		return AccountMonitorScoreWeights{}, fmt.Errorf("load group score weights: %w", err)
	}
	if err := validateAccountMonitorScoreWeights(weights); err != nil {
		return AccountMonitorScoreWeights{}, fmt.Errorf("stored group score weights: %w", err)
	}
	return weights, nil
}

func (s *AccountMonitorService) UpdateGroupScoreWeights(
	ctx context.Context,
	groupID int64,
	actorID int64,
	weights AccountMonitorScoreWeights,
) (AccountMonitorScoreWeights, error) {
	if groupID <= 0 {
		return AccountMonitorScoreWeights{}, errors.New("invalid group id")
	}
	if actorID <= 0 {
		return AccountMonitorScoreWeights{}, errors.New("invalid actor id")
	}
	if err := validateAccountMonitorScoreWeights(weights); err != nil {
		return AccountMonitorScoreWeights{}, err
	}
	if err := s.repo.SaveGroupScoreWeights(ctx, groupID, actorID, weights); err != nil {
		return AccountMonitorScoreWeights{}, fmt.Errorf("save group score weights: %w", err)
	}
	return s.GetGroupScoreWeights(ctx, groupID)
}

func (s *AccountMonitorService) ResetGroupScoreWeights(
	ctx context.Context,
	groupID int64,
	actorID int64,
) (AccountMonitorScoreWeights, error) {
	if groupID <= 0 {
		return AccountMonitorScoreWeights{}, errors.New("invalid group id")
	}
	if actorID <= 0 {
		return AccountMonitorScoreWeights{}, errors.New("invalid actor id")
	}
	if err := s.repo.ResetGroupScoreWeights(ctx, groupID); err != nil {
		return AccountMonitorScoreWeights{}, fmt.Errorf("reset group score weights: %w", err)
	}
	return DefaultAccountMonitorScoreWeights, nil
}

func validateAccountMonitorScoreWeights(weights AccountMonitorScoreWeights) error {
	if weights.Cost < 0 || weights.Success < 0 || weights.TTFT < 0 || weights.Latency < 0 {
		return errors.New("score weights must be non-negative")
	}
	if weights.Cost+weights.Success+weights.TTFT+weights.Latency != 100 {
		return errors.New("score weights must sum to 100")
	}
	return nil
}

func (s *AccountMonitorService) History(
	ctx context.Context,
	accountID int64,
	limit int,
) ([]AccountMonitorProbeResult, error) {
	return s.repo.ListHistory(ctx, accountID, limit)
}

func (s *AccountMonitorService) loadSettings(ctx context.Context) (AccountMonitorSettings, error) {
	settings, err := s.repo.LoadSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		settings = AccountMonitorSettings{
			IntervalSeconds: AccountMonitorDefaultIntervalSeconds,
			UpdatedAt:       time.Now().UTC(),
		}
		if saveErr := s.repo.SaveSettings(ctx, settings); saveErr != nil {
			return AccountMonitorSettings{}, saveErr
		}
		return settings, nil
	}
	if err != nil {
		return AccountMonitorSettings{}, err
	}
	if settings.IntervalSeconds < AccountMonitorMinIntervalSeconds {
		settings.IntervalSeconds = AccountMonitorMinIntervalSeconds
	}
	if settings.IntervalSeconds > AccountMonitorMaxIntervalSeconds {
		settings.IntervalSeconds = AccountMonitorMaxIntervalSeconds
	}
	return settings, nil
}

func (s *AccountMonitorService) listPool(ctx context.Context) ([]Account, error) {
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return nil, fmt.Errorf("list active schedulable accounts: %w", err)
	}
	filtered := accounts[:0]
	for _, account := range accounts {
		if account.Status == StatusActive && account.Schedulable {
			filtered = append(filtered, account)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered, nil
}

func (s *AccountMonitorService) probeAccount(ctx context.Context, account Account) AccountMonitorProbeResult {
	modelID := monitorModelForAccount(&account)
	probeConnection := s.probeConnection
	if probeConnection == nil && s.testService != nil {
		probeConnection = s.testService.ProbeAccountConnection
	}
	if probeConnection == nil {
		return AccountMonitorProbeResult{
			AccountID: account.ID,
			ModelID:   modelID,
			Status:    "failed",
			ErrorCode: "account_test_error",
			CheckedAt: time.Now().UTC(),
		}
	}
	timeout := s.probeTimeout
	if timeout <= 0 {
		timeout = accountMonitorProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := probeConnection(probeCtx, account.ID, modelID, "hi", AccountTestModeDefault)
	if err != nil {
		result.Status = "failed"
		result.ErrorCode = classifyAccountMonitorProbeError(err)
	}
	result.AccountID = account.ID
	result.ModelID = modelID
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	return result
}

func (s *AccountMonitorService) loadTodayStats(
	ctx context.Context,
	ids []int64,
) (map[int64]*WindowStats, error) {
	if s.usage == nil || len(ids) == 0 {
		return map[int64]*WindowStats{}, nil
	}
	stats, err := s.usage.GetTodayStatsBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load account invocation stats: %w", err)
	}
	return stats, nil
}

func (s *AccountMonitorService) loadUsageWindows(ctx context.Context, accountID int64) []AccountMonitorUsageWindow {
	if s.usage == nil {
		return nil
	}
	usage, err := s.usage.GetPassiveUsage(ctx, accountID)
	if err != nil || usage == nil {
		return nil
	}
	windows := make([]AccountMonitorUsageWindow, 0, 4)
	appendWindow := func(name string, progress *UsageProgress) {
		if window, ok := accountMonitorUsageWindow(name, progress); ok {
			windows = append(windows, window)
		}
	}
	appendWindow("5h", usage.FiveHour)
	appendWindow("7d", usage.SevenDay)
	appendWindow("7d-sonnet", usage.SevenDaySonnet)
	appendWindow("7d-fable", usage.SevenDayFable)
	return windows
}

func accountMonitorUsageWindow(name string, progress *UsageProgress) (AccountMonitorUsageWindow, bool) {
	if progress == nil {
		return AccountMonitorUsageWindow{}, false
	}
	utilization := progress.Utilization / 100
	if utilization < 0 {
		utilization = 0
	}
	window := AccountMonitorUsageWindow{
		Name:        name,
		Utilization: utilization,
		ResetsAt:    progress.ResetsAt,
	}
	if progress.WindowStats != nil {
		window.Requests = progress.WindowStats.Requests
		window.Tokens = progress.WindowStats.Tokens
	}
	return window, true
}

func accountGroupNames(account Account) []string {
	names := make([]string, 0, len(account.Groups))
	for _, group := range account.Groups {
		if group != nil && strings.TrimSpace(group.Name) != "" {
			names = append(names, group.Name)
		}
	}
	return names
}

func monitorModelForAccount(account *Account) string {
	if account == nil {
		return claude.DefaultTestModel
	}
	mapping := account.GetModelMapping()
	candidates := make([]string, 0, len(mapping))
	for modelID := range mapping {
		modelID = strings.TrimSpace(modelID)
		if isAccountMonitorTextModel(modelID) {
			candidates = append(candidates, modelID)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return naturalModelCompare(candidates[i], candidates[j]) > 0
	})

	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	switch platform {
	case PlatformOpenAI:
		for _, modelID := range candidates {
			if strings.HasPrefix(strings.ToLower(modelID), "gpt-") {
				return modelID
			}
		}
	case PlatformGrok:
		if modelID := firstCuratedMonitorModel(candidates, xai.DefaultModelIDs()); modelID != "" {
			return modelID
		}
	case PlatformAntigravity:
		defaults := antigravity.DefaultModels()
		defaultIDs := make([]string, 0, len(defaults))
		for _, model := range defaults {
			if isAccountMonitorTextModel(model.ID) {
				defaultIDs = append(defaultIDs, model.ID)
			}
		}
		if modelID := firstCuratedMonitorModel(candidates, defaultIDs); modelID != "" {
			return modelID
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}

	switch platform {
	case PlatformOpenAI:
		return openai.DefaultTestModel
	case PlatformGemini:
		return geminicli.DefaultTestModel
	case PlatformGrok:
		models := xai.DefaultModels()
		if len(models) > 0 {
			return models[0].ID
		}
	case PlatformAntigravity:
		for _, model := range antigravity.DefaultModels() {
			if isAccountMonitorTextModel(model.ID) {
				return model.ID
			}
		}
	default:
		return claude.DefaultTestModel
	}
	return claude.DefaultTestModel
}

func isAccountMonitorTextModel(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if modelID == "" || strings.Contains(modelID, "*") {
		return false
	}
	for _, marker := range []string{
		"audio",
		"embedding",
		"image",
		"moderation",
		"realtime",
		"transcri",
		"tts",
		"video",
		"whisper",
	} {
		if strings.Contains(modelID, marker) {
			return false
		}
	}
	return true
}

func firstCuratedMonitorModel(candidates []string, curated []string) string {
	available := make(map[string]string, len(candidates))
	for _, modelID := range candidates {
		available[strings.ToLower(modelID)] = modelID
	}
	for _, modelID := range curated {
		if candidate, ok := available[strings.ToLower(modelID)]; ok {
			return candidate
		}
	}
	return ""
}

func naturalModelCompare(left string, right string) int {
	left = strings.ToLower(left)
	right = strings.ToLower(right)
	for left != "" && right != "" {
		leftDigit := left[0] >= '0' && left[0] <= '9'
		rightDigit := right[0] >= '0' && right[0] <= '9'
		if leftDigit && rightDigit {
			leftEnd := leadingDigits(left)
			rightEnd := leadingDigits(right)
			leftNumber := strings.TrimLeft(left[:leftEnd], "0")
			rightNumber := strings.TrimLeft(right[:rightEnd], "0")
			if leftNumber == "" {
				leftNumber = "0"
			}
			if rightNumber == "" {
				rightNumber = "0"
			}
			if len(leftNumber) != len(rightNumber) {
				if len(leftNumber) > len(rightNumber) {
					return 1
				}
				return -1
			}
			if leftNumber != rightNumber {
				if leftNumber > rightNumber {
					return 1
				}
				return -1
			}
			left = left[leftEnd:]
			right = right[rightEnd:]
			continue
		}
		if left[0] != right[0] {
			if left[0] > right[0] {
				return 1
			}
			return -1
		}
		left = left[1:]
		right = right[1:]
	}
	switch {
	case left != "":
		return 1
	case right != "":
		return -1
	default:
		return 0
	}
}

func leadingDigits(value string) int {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return i
		}
	}
	return len(value)
}

func anyMonitorRowStale(rows []AccountMonitorAccount) bool {
	for _, row := range rows {
		if row.Stale {
			return true
		}
	}
	return false
}
