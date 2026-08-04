package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
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
	accountMonitorMultiplierRefreshTimeout     = 2 * time.Minute
	accountMonitorProbeTimeout                 = 60 * time.Second
	accountMonitorManagementEnabled            = "enabled"
	accountMonitorManagementPaused             = "paused"
	accountMonitorServiceAvailable             = "available"
	accountMonitorServiceUnavailable           = "unavailable"
	accountMonitorServicePending               = "pending"
	accountMonitorServiceNotMonitored          = "not_monitored"
	accountMonitorEligibilityEligible          = "eligible"
	accountMonitorEligibilityCostIneligible    = "cost_ineligible"
	accountMonitorEligibilityMultiplierPending = "multiplier_pending"
	accountMonitorEligibilityNotApplicable     = "not_applicable"
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
	observedAt := time.Now().UTC()
	aggregates, err := s.repo.ListAggregates(ctx, ids, observedAt.Add(-AccountMonitorHistoryDays*24*time.Hour), observedAt)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor aggregates: %w", err)
	}
	latest, err := s.repo.ListLatest(ctx, ids)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor latest: %w", err)
	}
	timelines, err := s.repo.ListTimelines(ctx, ids, AccountMonitorTimelineLimit)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor timelines: %w", err)
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
		groups[i].ScoreWeights = normalizeAccountMonitorScoreWeights(groups[i].ScoreWeights)
	}

	rows := make([]AccountMonitorAccount, 0, len(accounts))
	schedulableIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		aggregate := aggregates[account.ID]
		row := AccountMonitorAccount{
			AccountID:                  account.ID,
			Name:                       account.Name,
			Platform:                   account.Platform,
			AccountType:                account.Type,
			Status:                     account.Status,
			Schedulable:                account.Schedulable,
			Priority:                   account.Priority,
			HomepageURL:                accountMonitorHomepageURL(account),
			GroupIDs:                   append([]int64{}, account.GroupIDs...),
			GroupNames:                 accountGroupNames(account),
			ModelID:                    monitorModelForAccount(&account),
			LatestStatus:               "unavailable",
			SuccessRate:                aggregate.SuccessRate,
			SampleCount:                aggregate.SampleCount,
			SuccessSampleCount:         aggregate.SuccessSampleCount,
			TTFTSampleCount:            aggregate.TTFTSampleCount,
			LatencySampleCount:         aggregate.LatencySampleCount,
			TTFTP50MS:                  aggregate.TTFTP50MS,
			TTFTP95MS:                  aggregate.TTFTP95MS,
			LatencyP95MS:               aggregate.LatencyP95MS,
			Multiplier:                 s.resolveMultiplier(&account, observedAt),
			ProcurementCostCNY:         account.ProcurementCostCNY,
			ProcurementCostEffectiveAt: account.ProcurementCostEffectiveAt,
			ExpiresAt:                  account.ExpiresAt,
			ErrorCount:                 int64(aggregate.ErrorCount),
			Timeline:                   append([]AccountMonitorTimelinePoint{}, timelines[account.ID]...),
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
		row.ManagementState = accountMonitorManagementState(account, observedAt)
		row.ServiceState = accountMonitorServiceState(row, row.ManagementState)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		rows = append(rows, row)
		if row.ManagementState == accountMonitorManagementEnabled {
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

func ParseAccountMonitorRange(raw string) (AccountMonitorRange, time.Duration, error) {
	switch AccountMonitorRange(raw) {
	case "", AccountMonitorRange24Hours:
		return AccountMonitorRange24Hours, 24 * time.Hour, nil
	case AccountMonitorRange7Days:
		return AccountMonitorRange7Days, 7 * 24 * time.Hour, nil
	case AccountMonitorRange30Days:
		return AccountMonitorRange30Days, 30 * 24 * time.Hour, nil
	default:
		return "", 0, fmt.Errorf("invalid account monitor range %q", raw)
	}
}

// ListWindow produces the V3 account-monitor projection. Its request metrics
// come exclusively from usage_logs; monitor probes are fallback quality evidence.
func (s *AccountMonitorService) ListWindow(ctx context.Context, rawRange string) (AccountMonitorPage, error) {
	rangeValue, duration, err := ParseAccountMonitorRange(rawRange)
	if err != nil {
		return AccountMonitorPage{}, err
	}
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
	observedAt := time.Now().UTC()
	since := observedAt.Add(-duration)
	windowAggregates, err := s.repo.ListWindowAggregates(ctx, ids, since, observedAt)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor window aggregates: %w", err)
	}
	probeAggregates, err := s.repo.ListAggregates(ctx, ids, since, observedAt)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor probe aggregates: %w", err)
	}
	latest, err := s.repo.ListLatest(ctx, ids)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor latest: %w", err)
	}
	timelines, err := s.repo.ListTimelines(ctx, ids, AccountMonitorTimelineLimit)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor timelines: %w", err)
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor groups: %w", err)
	}
	for i := range groups {
		groups[i].ScoreWeights = normalizeAccountMonitorScoreWeights(groups[i].ScoreWeights)
	}

	rows := make([]AccountMonitorAccount, 0, len(accounts))
	for _, account := range accounts {
		window := windowAggregates[account.ID]
		row := AccountMonitorAccount{
			AccountID: account.ID, Name: account.Name, Platform: account.Platform, AccountType: account.Type,
			Status: account.Status, Schedulable: account.Schedulable, Priority: account.Priority,
			HomepageURL: accountMonitorHomepageURL(account), GroupIDs: append([]int64{}, account.GroupIDs...),
			GroupNames: accountGroupNames(account), ModelID: monitorModelForAccount(&account),
			LatestStatus: "unavailable", Multiplier: s.resolveMultiplier(&account, observedAt),
			ProcurementCostCNY:         account.ProcurementCostCNY,
			ProcurementCostEffectiveAt: account.ProcurementCostEffectiveAt,
			ExpiresAt:                  account.ExpiresAt,
			Range:                      rangeValue, RequestCount: window.RequestCount, ErrorCount: window.ErrorCount, BaseCost: window.BaseCost,
			Timeline: append([]AccountMonitorTimelinePoint{}, timelines[account.ID]...),
		}
		cost := accountMonitorWindowCost(account, since, observedAt, window.BaseCost)
		row.CostMode = cost.Mode
		row.EffectiveMultiplier = cost.EffectiveMultiplier
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
		row.ManagementState = accountMonitorManagementState(account, observedAt)
		row.ServiceState = accountMonitorServiceState(row, row.ManagementState)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		rows = append(rows, row)
	}

	rows = s.projectGlobalWindowQuality(accounts, rows, windowAggregates, probeAggregates, latest, settings, observedAt)
	groups = s.projectGroupWindowQuality(groups, accounts, rows, windowAggregates, probeAggregates, latest, settings, since, observedAt)
	return AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		SchemaVersion: AccountMonitorSchemaVersion, Range: rangeValue, ObservedAt: observedAt,
		Stale: len(rows) == 0 || anyMonitorRowStale(rows), Settings: settings,
		Health: summarizeAccountMonitorHealth(rows), Groups: groups, Accounts: rows,
	}}, nil
}

func (s *AccountMonitorService) projectGlobalWindowQuality(
	accounts []Account,
	rows []AccountMonitorAccount,
	windows map[int64]AccountMonitorWindowAggregate,
	probes map[int64]AccountMonitorAggregate,
	latest map[int64]AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
) []AccountMonitorAccount {
	accountsByID := make(map[int64]Account, len(accounts))
	for _, account := range accounts {
		accountsByID[account.ID] = account
	}
	for i := range rows {
		row := &rows[i]
		account, ok := accountsByID[row.AccountID]
		if !ok {
			continue
		}
		evidence := accountMonitorWindowEvidence(windows[row.AccountID], probes[row.AccountID], latest[row.AccountID], settings, now)
		row.SampleCount = evidence.SampleCount
		row.SuccessSampleCount = evidence.SuccessSampleCount
		row.TTFTSampleCount = evidence.TTFTSampleCount
		row.LatencySampleCount = evidence.LatencySampleCount
		row.SuccessRate = evidence.SuccessRate
		row.TTFTP50MS = evidence.TTFTP50MS
		row.LatencyP95MS = evidence.LatencyP95MS
		row.CheckedAt = accountMonitorWindowCheckedAt(latest[row.AccountID], evidence)
		row.ManagementState = accountMonitorManagementState(account, now)
		row.ServiceState = accountMonitorWindowServiceState(*row, evidence, row.ManagementState)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		row.Eligible = row.ManagementState == accountMonitorManagementEnabled && row.ServiceState == accountMonitorServiceAvailable && evidence.Source != "stale"
		if row.Eligible {
			row.QualityScore = CalculateAccountMonitorWindowQualityScore(1, row.EffectiveMultiplier, DefaultAccountMonitorScoreWeights, evidence)
		}
	}
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].Eligible != rows[right].Eligible {
			return rows[left].Eligible
		}
		if rows[left].Eligible {
			leftScore, rightScore := 0.0, 0.0
			if rows[left].QualityScore != nil {
				leftScore = *rows[left].QualityScore
			}
			if rows[right].QualityScore != nil {
				rightScore = *rows[right].QualityScore
			}
			if leftScore != rightScore {
				return leftScore > rightScore
			}
		}
		return rows[left].AccountID < rows[right].AccountID
	})
	rank := 0
	for i := range rows {
		if rows[i].Eligible {
			rank++
			value := rank
			rows[i].GroupRank = &value
		}
	}
	return rows
}

func (s *AccountMonitorService) projectGroupWindowQuality(
	groups []AccountMonitorGroup,
	accounts []Account,
	rows []AccountMonitorAccount,
	windows map[int64]AccountMonitorWindowAggregate,
	probes map[int64]AccountMonitorAggregate,
	latest map[int64]AccountMonitorLatest,
	settings AccountMonitorSettings,
	windowStart, now time.Time,
) []AccountMonitorGroup {
	rowsByID := make(map[int64]AccountMonitorAccount, len(rows))
	for _, row := range rows {
		rowsByID[row.AccountID] = row
	}
	for i := range groups {
		group := &groups[i]
		projected := make([]AccountMonitorGroupAccount, 0)
		for _, account := range accounts {
			if !accountMonitorAccountInGroup(account, group.ID) {
				continue
			}
			base, ok := rowsByID[account.ID]
			if !ok {
				continue
			}
			window := windows[account.ID]
			evidence := accountMonitorWindowEvidence(window, probes[account.ID], latest[account.ID], settings, now)
			row := AccountMonitorGroupAccount{AccountMonitorAccount: base, Evidence: evidence}
			row.SampleCount = evidence.SampleCount
			row.SuccessSampleCount = evidence.SuccessSampleCount
			row.TTFTSampleCount = evidence.TTFTSampleCount
			row.LatencySampleCount = evidence.LatencySampleCount
			row.SuccessRate = evidence.SuccessRate
			row.TTFTP50MS = evidence.TTFTP50MS
			row.LatencyP95MS = evidence.LatencyP95MS
			row.CheckedAt = accountMonitorWindowCheckedAt(latest[account.ID], evidence)
			row.ManagementState = accountMonitorManagementState(account, now)
			row.ServiceState = accountMonitorWindowServiceState(row.AccountMonitorAccount, evidence, row.ManagementState)
			row.GroupEligibility = accountMonitorEligibilityEligible
			row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
			cost := accountMonitorWindowCost(account, windowStart, now, window.BaseCost)
			row.CostMode = cost.Mode
			row.EffectiveMultiplier = cost.EffectiveMultiplier
			row.CostScore = accountMonitorCostScore(group.RateMultiplier, cost.EffectiveMultiplier, group.ScoreWeights)
			row.Eligible = row.ManagementState == accountMonitorManagementEnabled && row.ServiceState == accountMonitorServiceAvailable && evidence.Source != "stale"
			if row.Eligible {
				row.QualityScore = CalculateAccountMonitorWindowQualityScore(group.RateMultiplier, cost.EffectiveMultiplier, group.ScoreWeights, evidence)
			}
			projected = append(projected, row)
		}
		sort.SliceStable(projected, func(left, right int) bool {
			if projected[left].Eligible != projected[right].Eligible {
				return projected[left].Eligible
			}
			if projected[left].Eligible {
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
		for j := range projected {
			if projected[j].Eligible {
				rank := j + 1
				projected[j].GroupRank = &rank
			}
		}
		group.Accounts = projected
		group.Health = summarizeGroupHealth(projected)
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
		monitoredMembers := make([]int64, 0, len(members))
		for _, accountID := range members {
			account, ok := accountsByID[accountID]
			if ok && accountMonitorManagementState(account, now) == accountMonitorManagementEnabled {
				monitoredMembers = append(monitoredMembers, accountID)
			}
		}
		groupAggregates := map[int64]AccountMonitorAggregate(nil)
		groupEvidenceAvailable := false
		if provider, ok := s.repo.(AccountMonitorGroupAggregateRepository); ok && len(monitoredMembers) > 0 {
			loaded, err := provider.ListGroupAggregates(ctx, group.ID, monitoredMembers, now.Add(-AccountMonitorGroupEvidenceWindow))
			if err == nil {
				groupAggregates = loaded
				groupEvidenceAvailable = true
			}
		}
		combinedAggregate := AccountMonitorAggregate{}
		combinedAggregateAvailable := false
		if provider, ok := s.repo.(AccountMonitorCombinedAggregateRepository); ok && len(monitoredMembers) > 0 {
			loaded, err := provider.LoadGroupAggregate(ctx, group.ID, monitoredMembers, now.Add(-AccountMonitorGroupEvidenceWindow))
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
			managementState := accountMonitorManagementState(account, now)
			groupAggregate := groupAggregates[accountID]
			globalAggregate := globalAggregates[accountID]
			if managementState == accountMonitorManagementPaused {
				groupAggregate = AccountMonitorAggregate{}
				globalAggregate = AccountMonitorAggregate{}
			}
			evidence, _ := accountMonitorEvidence(groupAggregate, globalAggregate, groupEvidenceAvailable, latest[accountID], settings, now)
			row := AccountMonitorGroupAccount{AccountMonitorAccount: base, Evidence: evidence}
			row.SampleCount = evidence.SampleCount
			row.SuccessSampleCount = evidence.SuccessSampleCount
			row.TTFTSampleCount = evidence.TTFTSampleCount
			row.LatencySampleCount = evidence.LatencySampleCount
			row.SuccessRate = evidence.SuccessRate
			row.TTFTP50MS = evidence.TTFTP50MS
			row.LatencyP95MS = evidence.LatencyP95MS
			if checkedAt := evidence.ObservedAt; !checkedAt.IsZero() {
				checkedAt = checkedAt.UTC()
				row.CheckedAt = &checkedAt
			}
			row.ManagementState = managementState
			row.ServiceState = accountMonitorGroupServiceState(row, evidence, row.ManagementState)
			row.GroupEligibility = accountMonitorGroupEligibility(row, group.RateMultiplier)
			row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
			row.Eligible = row.ManagementState == accountMonitorManagementEnabled &&
				row.ServiceState == accountMonitorServiceAvailable &&
				row.GroupEligibility == accountMonitorEligibilityEligible
			if row.Eligible {
				row.QualityScore = CalculateAccountMonitorQualityScore(group.RateMultiplier, *row.Multiplier.Value, group.ScoreWeights, evidence)
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
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	unique := accounts[:0]
	seen := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if _, ok := seen[account.ID]; ok {
			continue
		}
		seen[account.ID] = struct{}{}
		unique = append(unique, account)
	}
	return unique, nil
}

func summarizeAccountMonitorHealth(rows []AccountMonitorAccount) AccountMonitorHealthSummary {
	samples := make([]accountMonitorHealthSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, accountMonitorHealthSample{
			managementState:    row.ManagementState,
			serviceState:       row.ServiceState,
			sampleCount:        row.SampleCount,
			successSampleCount: row.SuccessSampleCount,
			ttftSampleCount:    row.TTFTSampleCount,
			latencySampleCount: row.LatencySampleCount,
			successRate:        row.SuccessRate,
			ttftP50MS:          row.TTFTP50MS,
			latencyP95:         row.LatencyP95MS,
		})
	}
	return summarizeHealthSamples(samples)
}

func summarizeGroupHealth(rows []AccountMonitorGroupAccount) AccountMonitorHealthSummary {
	samples := make([]accountMonitorHealthSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, accountMonitorHealthSample{
			managementState:    row.ManagementState,
			serviceState:       row.ServiceState,
			sampleCount:        row.SampleCount,
			successSampleCount: row.SuccessSampleCount,
			ttftSampleCount:    row.TTFTSampleCount,
			latencySampleCount: row.LatencySampleCount,
			successRate:        row.SuccessRate,
			ttftP50MS:          row.TTFTP50MS,
			latencyP95:         row.LatencyP95MS,
		})
	}
	return summarizeHealthSamples(samples)
}

func applyAccountMonitorAggregate(summary AccountMonitorHealthSummary, aggregate AccountMonitorAggregate) AccountMonitorHealthSummary {
	if aggregate.SampleCount == 0 {
		return summary
	}
	summary.SuccessRate = aggregate.SuccessRate
	summary.SuccessSampleCount = aggregate.SuccessSampleCount
	summary.TTFTSampleCount = aggregate.TTFTSampleCount
	summary.LatencySampleCount = aggregate.LatencySampleCount
	if summary.SuccessSampleCount == 0 {
		summary.SuccessSampleCount = aggregate.SampleCount
	}
	summary.TTFTP50MS = aggregate.TTFTP50MS
	summary.LatencyP95MS = aggregate.LatencyP95MS
	return summary
}

type accountMonitorHealthSample struct {
	managementState    string
	serviceState       string
	sampleCount        int
	successSampleCount int
	ttftSampleCount    int
	latencySampleCount int
	successRate        float64
	ttftP50MS          *float64
	latencyP95         *float64
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
		if row.managementState == accountMonitorManagementPaused {
			summary.PausedAccounts++
			continue
		}
		summary.MonitoringAccounts++
		switch row.serviceState {
		case accountMonitorServicePending:
			summary.PendingAccounts++
		case accountMonitorServiceAvailable:
			summary.AvailableAccounts++
		default:
			summary.UnavailableAccounts++
		}
		weight := float64(row.sampleCount)
		if weight <= 0 {
			continue
		}
		samples += weight
		summary.SuccessSampleCount += row.successSampleCount
		successes += row.successRate * weight
		if row.ttftP50MS != nil {
			count := row.ttftSampleCount
			if count == 0 {
				count = row.sampleCount
			}
			ttftWeighted += *row.ttftP50MS * float64(count)
			ttftSamples += float64(count)
			summary.TTFTSampleCount += count
		}
		if row.latencyP95 != nil {
			count := row.latencySampleCount
			if count == 0 {
				count = row.sampleCount
			}
			latencyWeighted += *row.latencyP95 * float64(count)
			latencySamples += float64(count)
			summary.LatencySampleCount += count
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

func accountMonitorManagementState(account Account, now time.Time) string {
	if accountMonitorAccountPaused(account, now) {
		return accountMonitorManagementPaused
	}
	return accountMonitorManagementEnabled
}

func accountMonitorServiceState(row AccountMonitorAccount, managementState string) string {
	if managementState == accountMonitorManagementPaused {
		return accountMonitorServiceNotMonitored
	}
	if row.Stale || row.Latest == nil {
		return accountMonitorServicePending
	}
	if row.LatestStatus == "success" {
		return accountMonitorServiceAvailable
	}
	return accountMonitorServiceUnavailable
}

func accountMonitorWindowServiceState(
	row AccountMonitorAccount,
	evidence AccountMonitorQualityEvidence,
	managementState string,
) string {
	if managementState == accountMonitorManagementPaused {
		return accountMonitorServiceNotMonitored
	}
	if row.Stale || row.Latest == nil {
		return accountMonitorServicePending
	}
	if evidence.Source == "stale" {
		return accountMonitorServicePending
	}
	if evidence.Source == "real_requests" {
		if evidence.SuccessSampleCount > 0 {
			return accountMonitorServiceAvailable
		}
		return accountMonitorServiceUnavailable
	}
	if row.LatestStatus == "success" {
		return accountMonitorServiceAvailable
	}
	return accountMonitorServiceUnavailable
}

func accountMonitorWindowCheckedAt(latest AccountMonitorLatest, evidence AccountMonitorQualityEvidence) *time.Time {
	if !latest.CheckedAt.IsZero() {
		checkedAt := latest.CheckedAt.UTC()
		return &checkedAt
	}
	if !evidence.ObservedAt.IsZero() {
		checkedAt := evidence.ObservedAt.UTC()
		return &checkedAt
	}
	return nil
}

func accountMonitorGroupServiceState(
	row AccountMonitorGroupAccount,
	evidence AccountMonitorQualityEvidence,
	managementState string,
) string {
	if managementState == accountMonitorManagementPaused {
		return accountMonitorServiceNotMonitored
	}
	if evidence.Source == "stale" || row.CheckedAt == nil {
		return accountMonitorServicePending
	}
	if row.LatestStatus == "success" && evidence.SuccessRate > 0 {
		return accountMonitorServiceAvailable
	}
	return accountMonitorServiceUnavailable
}

func accountMonitorGroupEligibility(row AccountMonitorGroupAccount, groupMultiplier float64) string {
	if row.ManagementState == accountMonitorManagementPaused {
		return accountMonitorEligibilityNotApplicable
	}
	if row.Multiplier.Status != AccountMonitorMultiplierStatusOK || row.Multiplier.Value == nil {
		return accountMonitorEligibilityMultiplierPending
	}
	if *row.Multiplier.Value > groupMultiplier {
		return accountMonitorEligibilityCostIneligible
	}
	return accountMonitorEligibilityEligible
}

func accountMonitorBucket(managementState, serviceState, groupEligibility string) string {
	if managementState == accountMonitorManagementPaused {
		return accountMonitorManagementPaused
	}
	if serviceState == accountMonitorServicePending || groupEligibility == accountMonitorEligibilityMultiplierPending {
		return accountMonitorServicePending
	}
	if groupEligibility == accountMonitorEligibilityCostIneligible {
		return accountMonitorEligibilityCostIneligible
	}
	if serviceState == accountMonitorServiceAvailable {
		return accountMonitorServiceAvailable
	}
	return accountMonitorServiceUnavailable
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
		Source: source, SampleCount: aggregate.SampleCount,
		SuccessSampleCount: aggregate.SuccessSampleCount, TTFTSampleCount: aggregate.TTFTSampleCount,
		LatencySampleCount: aggregate.LatencySampleCount, SuccessRate: aggregate.SuccessRate,
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
	latencyScore := func(value *float64, targetMS, limitMS int) float64 {
		if value == nil {
			return 0
		}
		if limitMS <= targetMS || targetMS < 0 {
			return 0
		}
		if *value <= float64(targetMS) {
			return 1
		}
		if *value >= float64(limitMS) {
			return 0
		}
		return (float64(limitMS) - *value) / float64(limitMS-targetMS)
	}
	costAdvantage := float64(weights.Cost) * (groupMultiplier - accountMultiplier) / groupMultiplier
	if costAdvantage < 0 {
		costAdvantage = 0
	}
	if costAdvantage > float64(weights.Cost) {
		costAdvantage = float64(weights.Cost)
	}
	score := costAdvantage + float64(weights.Success)*clamp01(evidence.SuccessRate) +
		float64(weights.TTFT)*latencyScore(evidence.TTFTP50MS, weights.TTFTTargetMS, weights.TTFTLimitMS) +
		float64(weights.Latency)*latencyScore(evidence.LatencyP95MS, weights.LatencyTargetMS, weights.LatencyLimitMS)
	score = math.Round(score)
	return &score
}

type accountMonitorWindowCostResult struct {
	Mode                string
	WindowCost          float64
	EffectiveMultiplier *float64
	CostScoreEligible   bool
}

func accountMonitorWindowCost(account Account, windowStart, windowEnd time.Time, baseCost float64) accountMonitorWindowCostResult {
	if account.ProcurementCostCNY != nil {
		result := accountMonitorWindowCostResult{Mode: "procurement"}
		if account.ProcurementCostEffectiveAt == nil || account.ExpiresAt == nil || baseCost <= 0 ||
			!account.ExpiresAt.After(*account.ProcurementCostEffectiveAt) || !windowEnd.After(windowStart) {
			return result
		}
		overlapStart := windowStart
		if account.ProcurementCostEffectiveAt.After(overlapStart) {
			overlapStart = *account.ProcurementCostEffectiveAt
		}
		overlapEnd := windowEnd
		if account.ExpiresAt.Before(overlapEnd) {
			overlapEnd = *account.ExpiresAt
		}
		if !overlapEnd.After(overlapStart) {
			return result
		}
		term := account.ExpiresAt.Sub(*account.ProcurementCostEffectiveAt)
		result.WindowCost = *account.ProcurementCostCNY * overlapEnd.Sub(overlapStart).Hours() / term.Hours()
		effectiveMultiplier := result.WindowCost / baseCost
		result.EffectiveMultiplier = &effectiveMultiplier
		result.CostScoreEligible = true
		return result
	}

	result := accountMonitorWindowCostResult{Mode: "multiplier"}
	if account.RateMultiplier == nil || *account.RateMultiplier < 0 {
		return result
	}
	effectiveMultiplier := *account.RateMultiplier
	result.EffectiveMultiplier = &effectiveMultiplier
	result.CostScoreEligible = true
	return result
}

func accountMonitorCostScore(groupMultiplier float64, effectiveMultiplier *float64, weights AccountMonitorScoreWeights) float64 {
	if groupMultiplier <= 0 || effectiveMultiplier == nil {
		return 0
	}
	score := float64(weights.Cost) * (groupMultiplier - *effectiveMultiplier) / groupMultiplier
	if score < 0 {
		return 0
	}
	if score > float64(weights.Cost) {
		return float64(weights.Cost)
	}
	return score
}

func CalculateAccountMonitorWindowQualityScore(
	groupMultiplier float64,
	effectiveMultiplier *float64,
	weights AccountMonitorScoreWeights,
	evidence AccountMonitorQualityEvidence,
) *float64 {
	if groupMultiplier <= 0 || evidence.Source == "stale" {
		return nil
	}
	withoutCost := weights
	withoutCost.Cost = 0
	quality := CalculateAccountMonitorQualityScore(groupMultiplier, groupMultiplier, withoutCost, evidence)
	if quality == nil {
		return nil
	}
	score := math.Round(*quality + accountMonitorCostScore(groupMultiplier, effectiveMultiplier, weights))
	return &score
}

func accountMonitorWindowEvidence(
	window AccountMonitorWindowAggregate,
	probe AccountMonitorAggregate,
	latest AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
) AccountMonitorQualityEvidence {
	if window.RequestCount >= AccountMonitorGroupEvidenceMinSamples {
		return AccountMonitorQualityEvidence{
			Source: "real_requests", SampleCount: int(window.RequestCount), SuccessSampleCount: accountMonitorWindowSuccessSamples(window),
			TTFTSampleCount: window.TTFTSampleCount, LatencySampleCount: window.LatencySampleCount,
			SuccessRate: window.SuccessRate, TTFTP50MS: window.TTFTP50MS, LatencyP95MS: window.LatencyP95MS,
			ObservedAt: accountMonitorWindowObservedAt(window, latest),
		}
	}
	if probe.SampleCount > 0 {
		observedAt := accountMonitorProbeObservedAt(probe, latest)
		if observedAt.IsZero() || now.Sub(observedAt) > time.Duration(settings.IntervalSeconds*2)*time.Second {
			return AccountMonitorQualityEvidence{Source: "stale", ObservedAt: observedAt}
		}
		return AccountMonitorQualityEvidence{
			Source: "monitor_probe", SampleCount: probe.SampleCount, SuccessSampleCount: probe.SuccessSampleCount,
			TTFTSampleCount: probe.TTFTSampleCount, LatencySampleCount: probe.LatencySampleCount,
			SuccessRate: probe.SuccessRate, TTFTP50MS: probe.TTFTP50MS, LatencyP95MS: probe.LatencyP95MS,
			ObservedAt: observedAt,
		}
	}
	if window.RequestCount > 0 {
		return AccountMonitorQualityEvidence{
			Source: "real_requests", SampleCount: int(window.RequestCount), SuccessSampleCount: accountMonitorWindowSuccessSamples(window),
			TTFTSampleCount: window.TTFTSampleCount, LatencySampleCount: window.LatencySampleCount,
			SuccessRate: window.SuccessRate, TTFTP50MS: window.TTFTP50MS, LatencyP95MS: window.LatencyP95MS,
			ObservedAt: accountMonitorWindowObservedAt(window, latest),
		}
	}
	return AccountMonitorQualityEvidence{Source: "stale", ObservedAt: accountMonitorProbeObservedAt(probe, latest)}
}

func accountMonitorWindowSuccessSamples(window AccountMonitorWindowAggregate) int {
	successCount := window.SuccessCount
	// Keep compatibility with repository adapters that only populate the
	// aggregate rate while preserving the explicit zero-success contract.
	if successCount == 0 && window.RequestCount > 0 && window.SuccessRate > 0 {
		successCount = int64(math.Round(float64(window.RequestCount) * window.SuccessRate))
	}
	if successCount < 0 {
		return 0
	}
	if successCount > window.RequestCount {
		return int(window.RequestCount)
	}
	return int(successCount)
}

func accountMonitorWindowObservedAt(window AccountMonitorWindowAggregate, latest AccountMonitorLatest) time.Time {
	if window.LastObservedAt != nil {
		return window.LastObservedAt.UTC()
	}
	return latest.CheckedAt.UTC()
}

func accountMonitorProbeObservedAt(probe AccountMonitorAggregate, latest AccountMonitorLatest) time.Time {
	if probe.LastCheckedAt != nil {
		return probe.LastCheckedAt.UTC()
	}
	return latest.CheckedAt.UTC()
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
	weights = normalizeAccountMonitorScoreWeights(weights)
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
	weights = normalizeAccountMonitorScoreWeights(weights)
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
	if weights.TTFTTargetMS < 0 || weights.TTFTLimitMS <= weights.TTFTTargetMS {
		return errors.New("ttft target must be non-negative and less than limit")
	}
	if weights.LatencyTargetMS < 0 || weights.LatencyLimitMS <= weights.LatencyTargetMS {
		return errors.New("latency target must be non-negative and less than limit")
	}
	return nil
}

func normalizeAccountMonitorScoreWeights(weights AccountMonitorScoreWeights) AccountMonitorScoreWeights {
	if weights == (AccountMonitorScoreWeights{}) {
		return DefaultAccountMonitorScoreWeights
	}
	if weights.TTFTTargetMS == 0 && weights.TTFTLimitMS == 0 {
		weights.TTFTTargetMS = AccountMonitorDefaultTTFTTargetMS
		weights.TTFTLimitMS = AccountMonitorDefaultTTFTLimitMS
	}
	if weights.LatencyTargetMS == 0 && weights.LatencyLimitMS == 0 {
		weights.LatencyTargetMS = AccountMonitorDefaultLatencyTargetMS
		weights.LatencyLimitMS = AccountMonitorDefaultLatencyLimitMS
	}
	return weights
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
		if row.ManagementState == accountMonitorManagementEnabled && row.Stale {
			return true
		}
	}
	return false
}
