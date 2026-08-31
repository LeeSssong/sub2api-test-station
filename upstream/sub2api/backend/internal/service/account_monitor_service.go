package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
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
	accountMonitorAvailabilityNormal           = "normal"
	accountMonitorAvailabilityAbnormal         = "abnormal"
	accountMonitorAvailabilityUnavailable      = "unavailable"
	accountMonitorAvailabilityDisabled         = "disabled"
	accountMonitorAvailabilityStale            = "stale"
	accountMonitorScoreEligible                = "eligible"
	accountMonitorScoreCapped                  = "capped"
	accountMonitorScoreIneligible              = "ineligible"
	accountMonitorAbnormalScoreCap             = 70.0
)

var ErrAccountMonitorInvalidScoreWeights = errors.New("invalid account monitor score weights")

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
	Refresh(context.Context, *Account, AccountMonitorRefreshOptions) error
}

type accountMonitorModelPricingReader interface {
	GetModelPricing(string) (*ModelPricing, error)
}

type accountMonitorRecommendationEvaluator func(
	Account,
	[]string,
	[]AccountMonitorGroup,
	AccountMonitorQualityEvidence,
	AccountMonitorLatest,
	time.Time,
) *AccountMonitorGroupRecommendation

type AccountMonitorService struct {
	repo                AccountMonitorRepository
	accountRepo         AccountMonitorAccountRepository
	testService         *AccountTestService
	usage               *AccountUsageService
	activeProbeUsage    ActiveProbeUsageWindowReader
	multiplier          accountMonitorMultiplierResolver
	costPricing         accountMonitorModelPricingReader
	recommend           accountMonitorRecommendationEvaluator
	modelDetection      *AccountModelDetectionService
	schedulerProjection OpenAIAccountSchedulerProjectionProvider
	concurrencyService  *ConcurrencyService
	runtimeBlocker      AccountRuntimeBlocker

	probeConnection accountMonitorProbeConnection
	probeTimeout    time.Duration

	runStateMu          sync.Mutex
	activeRun           *accountMonitorRun
	probeRecoveryMu     sync.Mutex
	apiKeyProbeFailures map[int64]int
	apiKeyProbeNextAt   map[int64]time.Time
}

// SetActiveProbeUsageReader attaches the bounded usage reader used by
// scheduled probes. A separate narrow interface keeps automatic admission
// independent from the broader usage service contract.
func (s *AccountMonitorService) SetActiveProbeUsageReader(reader ActiveProbeUsageWindowReader) {
	if s != nil {
		s.activeProbeUsage = reader
	}
}

// SetModelDetectionService attaches the optional detector projection and
// per-account connection-model resolver without changing legacy constructors.
func (s *AccountMonitorService) SetModelDetectionService(detector *AccountModelDetectionService) {
	if s != nil {
		s.modelDetection = detector
	}
}

// SetOpenAIAccountSchedulerProjectionProvider attaches the scheduler-owned
// read-only projection without changing legacy constructors.
func (s *AccountMonitorService) SetOpenAIAccountSchedulerProjectionProvider(provider OpenAIAccountSchedulerProjectionProvider) {
	if s != nil {
		s.schedulerProjection = provider
	}
}

// SetAccountMonitorConcurrencyService attaches the read-only load snapshot
// source used to build scheduler projection requests.
func (s *AccountMonitorService) SetAccountMonitorConcurrencyService(concurrency *ConcurrencyService) {
	if s != nil {
		s.concurrencyService = concurrency
	}
}

// SetAccountRuntimeBlocker lets a successful API-key recovery probe clear the
// request-path fast-path block together with the persisted temporary isolation.
func (s *AccountMonitorService) SetAccountRuntimeBlocker(blocker AccountRuntimeBlocker) {
	if s != nil {
		s.runtimeBlocker = blocker
	}
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
		repo:                repo,
		accountRepo:         accountRepo,
		testService:         testService,
		usage:               usage,
		multiplier:          multiplier,
		recommend:           EvaluateAccountMonitorGroupRecommendation,
		probeTimeout:        accountMonitorProbeTimeout,
		apiKeyProbeFailures: make(map[int64]int),
		apiKeyProbeNextAt:   make(map[int64]time.Time),
	}
}

func (s *AccountMonitorService) List(ctx context.Context) (AccountMonitorPage, error) {
	if err := ctx.Err(); err != nil {
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
	for _, account := range accounts {
		aggregate := aggregates[account.ID]
		modelID := monitorModelForAccount(&account)
		if current, ok := latest[account.ID]; ok {
			modelID = current.ModelID
		}
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
			ModelID:                    modelID,
			ConnectionProbeModel:       s.connectionProbeModel(ctx, &account),
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
			Balance:                    s.resolveBalance(&account, observedAt),
			ProcurementCostCNY:         account.ProcurementCostCNY,
			EstimatedUsableQuotaUSD:    account.EstimatedUsableQuotaUSD,
			ProcurementCostEffectiveAt: account.ProcurementCostEffectiveAt,
			ExpiresAt:                  account.ExpiresAt,
			ErrorCount:                 int64(aggregate.ErrorCount),
			Timeline:                   append([]AccountMonitorTimelinePoint{}, timelines[account.ID]...),
		}
		projectAccountMonitorEffectiveCost(&row, &account)
		row.UpstreamMultiplier = &row.Multiplier
		if s.modelDetection != nil {
			if detection, detectionErr := s.modelDetection.ProjectionForAccount(ctx, &account); detectionErr == nil {
				row.ModelDetection = &detection
			}
		}
		row.EquivalentSiteMultiplier = accountMonitorEquivalentSiteMultiplier(account, row.ModelID, s.costPricing)
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
		projectAccountMonitorProbe(&row, aggregate, latest[account.ID], timelines[account.ID], settings, observedAt, row.ManagementState)
		projectLegacyAccountMonitorScore(&row, account, aggregate, latest[account.ID], groups, observedAt)
		row.ServiceState = accountMonitorLegacyServiceState(row.AvailabilityStatus)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		rows = append(rows, row)
	}
	groups = s.projectGroupQualityEvidence(ctx, groups, accounts, rows, aggregates, latest, settings, observedAt)
	health := summarizeAccountMonitorHealth(rows)

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

// ProjectMonitorV2Groups returns a read-only native probe projection for the
// requested groups. Eligibility is evaluated at snapshot time so Monitor V2
// shares the scheduler's Account.IsSchedulable semantics.
func (s *AccountMonitorService) ProjectMonitorV2Groups(
	ctx context.Context,
	groupIDs []int64,
	start, end time.Time,
	bucketSize time.Duration,
) (map[int64]MonitorV2NativeGroupProjection, error) {
	result := make(map[int64]MonitorV2NativeGroupProjection)
	if s == nil || s.repo == nil || s.accountRepo == nil {
		return nil, errors.New("account monitor native projection unavailable")
	}
	if len(groupIDs) == 0 || !end.After(start) || bucketSize <= 0 {
		return result, nil
	}
	nativeRepo, ok := s.repo.(AccountMonitorGroupProbeRepository)
	if !ok {
		return nil, errors.New("account monitor native projection repository unavailable")
	}
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load account monitor settings for monitor v2: %w", err)
	}
	scopes, err := s.monitorGroupScopes(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	freshSince := end.Add(-2 * time.Duration(settings.IntervalSeconds) * time.Second)
	projection, err := nativeRepo.ProjectMonitorV2Groups(ctx, scopes, start, end, freshSince, bucketSize)
	if err != nil {
		return nil, fmt.Errorf("project native monitor v2 groups: %w", err)
	}
	return projection, nil
}

// monitorGroupScopes is shared by the native and hybrid projections. It keeps
// the scheduler's exact schedulable-account semantics at snapshot time.
func (s *AccountMonitorService) monitorGroupScopes(ctx context.Context, groupIDs []int64) ([]MonitorV2GroupAccountScope, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account monitor account repository unavailable")
	}
	accounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return nil, err
	}
	requestedGroups := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID > 0 {
			requestedGroups[groupID] = struct{}{}
		}
	}
	scopes := make([]MonitorV2GroupAccountScope, 0, len(accounts))
	seen := make(map[MonitorV2GroupAccountScope]struct{})
	for i := range accounts {
		account := &accounts[i]
		if !account.IsSchedulable() {
			continue
		}
		accountGroups := make(map[int64]struct{}, len(account.GroupIDs)+len(account.Groups))
		for _, groupID := range account.GroupIDs {
			accountGroups[groupID] = struct{}{}
		}
		for _, group := range account.Groups {
			if group != nil {
				accountGroups[group.ID] = struct{}{}
			}
		}
		for groupID := range accountGroups {
			if _, requested := requestedGroups[groupID]; !requested {
				continue
			}
			scope := MonitorV2GroupAccountScope{GroupID: groupID, AccountID: account.ID}
			if _, exists := seen[scope]; exists {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].GroupID == scopes[j].GroupID {
			return scopes[i].AccountID < scopes[j].AccountID
		}
		return scopes[i].GroupID < scopes[j].GroupID
	})
	return scopes, nil
}

// ProjectMonitorV4Groups returns the unified active-probe and strict
// successful-user-request projection for the hybrid monitor. V4 keeps all
// current account/group facts in scope so a temporary scheduling block cannot
// erase historical real-request evidence; IsSchedulableAt is reserved for the
// automatic probe pool.
func (s *AccountMonitorService) ProjectMonitorV4Groups(
	ctx context.Context,
	groupIDs []int64,
	start, end time.Time,
	bucketSize time.Duration,
) (map[int64]MonitorV4GroupProjection, error) {
	result := make(map[int64]MonitorV4GroupProjection)
	if s == nil || s.repo == nil || s.accountRepo == nil {
		return nil, errors.New("account monitor hybrid projection unavailable")
	}
	if len(groupIDs) == 0 || !end.After(start) || bucketSize <= 0 {
		return result, nil
	}
	hybridRepo, ok := s.repo.(AccountMonitorHybridProjectionRepository)
	if !ok {
		return nil, errors.New("account monitor hybrid projection repository unavailable")
	}
	scopes, err := s.monitorGroupFactScopes(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	if groupsRepo, ok := hybridRepo.(AccountMonitorHybridProjectionGroupsRepository); ok {
		projection, err := groupsRepo.ProjectMonitorV4GroupsForGroups(ctx, groupIDs, scopes, start, end, bucketSize)
		if err != nil {
			return nil, fmt.Errorf("project hybrid monitor v4 groups: %w", err)
		}
		logMonitorV4ProbeCompleteness(ctx, projection)
		return projection, nil
	}
	projection, err := hybridRepo.ProjectMonitorV4Groups(ctx, scopes, start, end, bucketSize)
	if err != nil {
		return nil, fmt.Errorf("project hybrid monitor v4 groups: %w", err)
	}
	logMonitorV4ProbeCompleteness(ctx, projection)
	return projection, nil
}

func logMonitorV4ProbeCompleteness(ctx context.Context, projection map[int64]MonitorV4GroupProjection) {
	for groupID, group := range projection {
		if group.MissingProbeTerminalCount <= 0 {
			continue
		}
		slog.WarnContext(ctx, "monitor_v4_probe_terminal_missing_fail_closed", "group_id", groupID, "bucket_count", group.MissingProbeTerminalCount)
	}
}

// monitorGroupFactScopes maps visible groups to all currently known account
// memberships. Unlike monitorGroupScopes (used by native availability), it
// intentionally does not apply transient schedulability gates: usage_logs and
// ops_error_logs remain valid historical request facts after an account enters
// cooldown, is rate-limited, or exhausts quota.
func (s *AccountMonitorService) monitorGroupFactScopes(ctx context.Context, groupIDs []int64) ([]MonitorV2GroupAccountScope, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account monitor account repository unavailable")
	}
	accounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return nil, err
	}
	requestedGroups := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID > 0 {
			requestedGroups[groupID] = struct{}{}
		}
	}
	scopes := make([]MonitorV2GroupAccountScope, 0, len(accounts))
	seen := make(map[MonitorV2GroupAccountScope]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.ID <= 0 {
			continue
		}
		accountGroups := make(map[int64]struct{}, len(account.GroupIDs)+len(account.Groups))
		for _, groupID := range account.GroupIDs {
			accountGroups[groupID] = struct{}{}
		}
		for _, group := range account.Groups {
			if group != nil {
				accountGroups[group.ID] = struct{}{}
			}
		}
		for groupID := range accountGroups {
			if _, requested := requestedGroups[groupID]; !requested {
				continue
			}
			scope := MonitorV2GroupAccountScope{GroupID: groupID, AccountID: account.ID}
			if _, exists := seen[scope]; exists {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].GroupID == scopes[j].GroupID {
			return scopes[i].AccountID < scopes[j].AccountID
		}
		return scopes[i].GroupID < scopes[j].GroupID
	})
	return scopes, nil
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

// ListWindow combines real request evidence with active probe observations for
// quality, scoring, and ranking. Availability and freshness remain gated by
// active probes so a request-only account cannot be reported as operational.
func (s *AccountMonitorService) ListWindow(ctx context.Context, rawRange string) (AccountMonitorPage, error) {
	if err := ctx.Err(); err != nil {
		return AccountMonitorPage{}, err
	}
	rangeValue, duration, err := ParseAccountMonitorRange(rawRange)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	globalWeightsResponse, err := s.GetGlobalScoreWeights(ctx)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	globalWeights := normalizeAccountMonitorScoreWeights(AccountMonitorScoreWeights{
		Cost: globalWeightsResponse.Cost, Success: globalWeightsResponse.Success,
		TTFT: globalWeightsResponse.TTFT, Latency: globalWeightsResponse.Latency,
	})
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
	var windowAggregates map[int64]AccountMonitorWindowAggregate
	// Prefer the dedicated real-request projection when the native repository
	// provides it. Legacy adapters continue to use the existing window query.
	if realRepo, ok := s.repo.(AccountMonitorRealRequestRepository); ok {
		if real, realErr := realRepo.ListRealRequestAggregates(ctx, ids, since, observedAt); realErr == nil {
			windowAggregates = real
		} else {
			return AccountMonitorPage{}, fmt.Errorf("list real request aggregates: %w", realErr)
		}
	} else {
		windowAggregates, err = s.repo.ListWindowAggregates(ctx, ids, since, observedAt)
		if err != nil {
			return AccountMonitorPage{}, fmt.Errorf("list account monitor window aggregates: %w", err)
		}
	}
	var realTimelines map[int64][]AccountMonitorRealRequestTimelinePoint
	if timelineRepo, ok := s.repo.(AccountMonitorRealRequestTimelineRepository); ok {
		realTimelines, err = timelineRepo.ListRealRequestTimelines(ctx, ids, since, observedAt, AccountMonitorTimelineLimit)
		if err != nil {
			return AccountMonitorPage{}, fmt.Errorf("list real request timelines: %w", err)
		}
	}
	probeAggregates, err := s.repo.ListAggregates(ctx, ids, since, observedAt)
	if err != nil {
		return AccountMonitorPage{}, fmt.Errorf("list account monitor probe aggregates: %w", err)
	}
	recommendationAggregates := probeAggregates
	historicalAggregates := map[int64]AccountMonitorAggregate(nil)
	historicalSince := observedAt.Add(-7 * 24 * time.Hour)
	if rangeValue != AccountMonitorRange7Days {
		recommendationSince := historicalSince
		recommendationAggregates, err = s.repo.ListAggregates(ctx, ids, recommendationSince, observedAt)
		if err != nil {
			slog.WarnContext(ctx, "account monitor recommendation probe aggregates unavailable", "error", err)
			recommendationAggregates = nil
		}
		historicalAggregates = recommendationAggregates
	} else {
		historicalSince = observedAt.Add(-30 * 24 * time.Hour)
		historicalAggregates, err = s.repo.ListAggregates(ctx, ids, historicalSince, observedAt)
		if err != nil {
			slog.WarnContext(ctx, "account monitor historical score aggregates unavailable", "error", err)
			historicalAggregates = nil
		}
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
	var groupReal map[int64]map[int64]AccountMonitorWindowAggregate
	if groupRepo, ok := s.repo.(AccountMonitorGroupRealRequestRepository); ok {
		groupIDs := make([]int64, 0, len(groups))
		for _, group := range groups {
			groupIDs = append(groupIDs, group.ID)
		}
		groupReal, err = groupRepo.ListGroupRealRequestAggregates(ctx, groupIDs, ids, since, observedAt)
		if err != nil {
			return AccountMonitorPage{}, fmt.Errorf("list group real request aggregates: %w", err)
		}
	}

	rows := make([]AccountMonitorAccount, 0, len(accounts))
	for _, account := range accounts {
		window := windowAggregates[account.ID]
		resolvedMultiplier := s.resolveMultiplier(&account, observedAt)
		modelID := monitorModelForAccount(&account)
		if current, ok := latest[account.ID]; ok {
			modelID = current.ModelID
		}
		row := AccountMonitorAccount{
			AccountID: account.ID, Name: account.Name, Platform: account.Platform, AccountType: account.Type,
			Status: account.Status, Schedulable: account.Schedulable, Priority: account.Priority,
			HomepageURL: accountMonitorHomepageURL(account), GroupIDs: append([]int64{}, account.GroupIDs...),
			GroupNames: accountGroupNames(account), ModelID: modelID,
			ConnectionProbeModel: s.connectionProbeModel(ctx, &account),
			LatestStatus:         "unavailable", Multiplier: resolvedMultiplier,
			Balance:                    s.resolveBalance(&account, observedAt),
			ProcurementCostCNY:         account.ProcurementCostCNY,
			EstimatedUsableQuotaUSD:    account.EstimatedUsableQuotaUSD,
			ProcurementCostEffectiveAt: account.ProcurementCostEffectiveAt,
			ExpiresAt:                  account.ExpiresAt,
			Range:                      rangeValue, RequestCount: window.RequestCount, ErrorCount: window.ErrorCount, BaseCost: window.BaseCost,
			Timeline: append([]AccountMonitorTimelinePoint{}, timelines[account.ID]...),
		}
		projectAccountMonitorEffectiveCost(&row, &account)
		if realTimelines != nil {
			row.RealRequestTimeline = realTimelines[account.ID]
		}
		if s.modelDetection != nil {
			if detection, detectionErr := s.modelDetection.ProjectionForAccount(ctx, &account); detectionErr == nil {
				row.ModelDetection = &detection
			}
		}
		row.EquivalentSiteMultiplier = accountMonitorEquivalentSiteMultiplier(account, row.ModelID, s.costPricing)
		cost := accountMonitorProjectedEffectiveCost(account, resolvedMultiplier, since, observedAt, window.BaseCost)
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
		projectAccountMonitorProbe(&row, probeAggregates[account.ID], latest[account.ID], timelines[account.ID], settings, observedAt, row.ManagementState)
		row.ServiceState = accountMonitorLegacyServiceState(row.AvailabilityStatus)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		rows = append(rows, row)
	}

	rows = s.projectGlobalWindowQuality(accounts, rows, windowAggregates, probeAggregates, historicalAggregates, latest, settings, observedAt, globalWeights)
	applyRealRequestEvidence(rows, windowAggregates, observedAt)
	applyGlobalRealRequestRanks(rows)
	rows = s.projectGroupRecommendations(ctx, accounts, rows, recommendationAggregates, latest, groups, settings, observedAt)
	groups = s.projectGroupWindowQuality(ctx, groups, accounts, rows, probeAggregates, historicalAggregates, latest, settings, since, observedAt)
	if groupReal != nil {
		applyGroupProfitabilityByGroup(groups, groupReal)
	} else {
		applyGroupProfitability(groups, windowAggregates)
	}
	applyGroupSchedulerOrder(groups)
	return AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		SchemaVersion: AccountMonitorSchemaVersion, Range: rangeValue, ObservedAt: observedAt,
		Stale: len(rows) == 0 || anyMonitorRowStale(rows), Settings: settings,
		Health: summarizeAccountMonitorHealth(rows), Groups: groups, Accounts: rows,
	}}, nil
}

func projectAccountMonitorEffectiveCost(row *AccountMonitorAccount, account *Account) {
	if row == nil || account == nil {
		return
	}
	cost := EffectiveCostForAccount(account)
	row.EffectiveCostModel = cost.Model
	row.UpstreamActualCost = account.UpstreamActualCost
	row.UpstreamObtainedQuota = account.UpstreamObtainedQuota
	row.EffectiveCostA = cost.A
	row.EffectiveCostR = cost.R
	row.EffectiveCostU = cost.U
	row.EffectiveCostStatus = cost.Status
}

func (s *AccountMonitorService) projectGroupRecommendations(
	ctx context.Context,
	accounts []Account,
	rows []AccountMonitorAccount,
	probes map[int64]AccountMonitorAggregate,
	latest map[int64]AccountMonitorLatest,
	groups []AccountMonitorGroup,
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
		_, isTestGroup := accountMonitorRecommendationCurrentTarget(row.GroupNames)
		if !isTestGroup && accountMonitorAccountPaused(account, now) {
			continue
		}
		evidence := accountMonitorWindowEvidence(AccountMonitorWindowAggregate{}, probes[account.ID], latest[account.ID], settings, now)
		recommendationAccount := account
		if account.Type != AccountTypeAPIKey {
			if row.Multiplier.Status == AccountMonitorMultiplierStatusOK && row.Multiplier.Value != nil {
				recommendationAccount.RateMultiplier = row.Multiplier.Value
			} else {
				recommendationAccount.RateMultiplier = nil
			}
		}
		row.GroupRecommendation = s.evaluateGroupRecommendation(
			ctx,
			recommendationAccount,
			row.GroupNames,
			groups,
			evidence,
			latest[account.ID],
			now,
		)
	}
	return rows
}

func applyRealRequestEvidence(rows []AccountMonitorAccount, aggregates map[int64]AccountMonitorWindowAggregate, observedAt time.Time) {
	for i := range rows {
		a := aggregates[rows[i].AccountID]
		rows[i].RealRequestEvidence = &AccountMonitorRealRequestEvidence{
			RequestCount: a.RequestCount, SuccessCount: a.SuccessCount, FailureCount: a.ErrorCount,
			SuccessRate: a.SuccessRate, TTFTP95MS: a.TTFTP95MS, TTFTSampleCount: a.TTFTSampleCount, ObservedAt: observedAt,
		}
	}
}

func applyGlobalRealRequestRanks(rows []AccountMonitorAccount) {
	eligible := make([]int, 0, len(rows))
	for i := range rows {
		if rows[i].RealRequestEvidence != nil && rows[i].RealRequestEvidence.RequestCount > 0 {
			eligible = append(eligible, i)
		}
	}
	bySuccess := append([]int(nil), eligible...)
	sort.SliceStable(bySuccess, func(i, j int) bool {
		l, r := rows[bySuccess[i]].RealRequestEvidence, rows[bySuccess[j]].RealRequestEvidence
		if l.SuccessRate != r.SuccessRate {
			return l.SuccessRate > r.SuccessRate
		}
		return rows[bySuccess[i]].AccountID < rows[bySuccess[j]].AccountID
	})
	byTTFT := append([]int(nil), eligible...)
	sort.SliceStable(byTTFT, func(i, j int) bool {
		l, r := rows[byTTFT[i]].RealRequestEvidence.TTFTP95MS, rows[byTTFT[j]].RealRequestEvidence.TTFTP95MS
		if l == nil && r != nil {
			return false
		}
		if l != nil && r == nil {
			return true
		}
		if l != nil && r != nil && *l != *r {
			return *l < *r
		}
		return rows[byTTFT[i]].AccountID < rows[byTTFT[j]].AccountID
	})
	for rank, index := range bySuccess {
		value := AccountMonitorRank{Rank: rank + 1, Total: len(bySuccess)}
		rows[index].GlobalRanks.SuccessRate = &value
	}
	for rank, index := range byTTFT {
		value := AccountMonitorRank{Rank: rank + 1, Total: len(byTTFT)}
		rows[index].GlobalRanks.TTFTP95 = &value
	}
}

func applyGroupProfitability(groups []AccountMonitorGroup, aggregates map[int64]AccountMonitorWindowAggregate) {
	for gi := range groups {
		members := make([]int, 0, len(groups[gi].Accounts))
		for i := range groups[gi].Accounts {
			row := &groups[gi].Accounts[i]
			a := aggregates[row.AccountID]
			profit := &AccountMonitorGroupProfitability{GroupID: groups[gi].ID, GroupName: groups[gi].Name, Revenue: a.Revenue, AccountCost: a.AccountCost, SampleCount: a.RequestCount, Status: "no_real_request"}
			if a.RequestCount > 0 {
				if a.Revenue <= 0 {
					profit.Status = "no_revenue"
				} else if !a.CostComplete {
					profit.Status = "cost_incomplete"
				} else {
					value := (a.Revenue - a.AccountCost) / a.Revenue
					profit.ProfitRate = &value
					profit.Status = "confirmed"
					members = append(members, i)
				}
			} else if estimated, ok := accountMonitorEstimatedProfitRate(groups[gi].RateMultiplier, row.Multiplier); ok {
				profit.ProfitRate = &estimated
				profit.Status = "estimated"
				members = append(members, i)
			}
			row.GroupProfitability = profit
		}
		sort.SliceStable(members, func(i, j int) bool {
			l, r := groups[gi].Accounts[members[i]].GroupProfitability.ProfitRate, groups[gi].Accounts[members[j]].GroupProfitability.ProfitRate
			if *l != *r {
				return *l > *r
			}
			return groups[gi].Accounts[members[i]].AccountID < groups[gi].Accounts[members[j]].AccountID
		})
		for rank, index := range members {
			value := AccountMonitorRank{Rank: rank + 1, Total: len(members)}
			groups[gi].Accounts[index].GroupProfitability.Rank = &value
		}
	}
}

func applyGroupProfitabilityByGroup(groups []AccountMonitorGroup, aggregates map[int64]map[int64]AccountMonitorWindowAggregate) {
	for gi := range groups {
		members := make([]int, 0, len(groups[gi].Accounts))
		for i := range groups[gi].Accounts {
			row := &groups[gi].Accounts[i]
			a := aggregates[groups[gi].ID][row.AccountID]
			profit := &AccountMonitorGroupProfitability{GroupID: groups[gi].ID, GroupName: groups[gi].Name, Revenue: a.Revenue, AccountCost: a.AccountCost, SampleCount: a.RequestCount, Status: "no_real_request"}
			if a.RequestCount > 0 {
				switch {
				case a.Revenue <= 0:
					profit.Status = "no_revenue"
				case !a.CostComplete:
					profit.Status = "cost_incomplete"
				default:
					value := (a.Revenue - a.AccountCost) / a.Revenue
					profit.ProfitRate = &value
					profit.Status = "confirmed"
					members = append(members, i)
				}
			} else if estimated, ok := accountMonitorEstimatedProfitRate(groups[gi].RateMultiplier, row.Multiplier); ok {
				profit.ProfitRate = &estimated
				profit.Status = "estimated"
				members = append(members, i)
			}
			row.GroupProfitability = profit
		}
		sort.SliceStable(members, func(i, j int) bool {
			l := groups[gi].Accounts[members[i]].GroupProfitability.ProfitRate
			r := groups[gi].Accounts[members[j]].GroupProfitability.ProfitRate
			if *l != *r {
				return *l > *r
			}
			return groups[gi].Accounts[members[i]].AccountID < groups[gi].Accounts[members[j]].AccountID
		})
		for rank, index := range members {
			value := AccountMonitorRank{Rank: rank + 1, Total: len(members)}
			groups[gi].Accounts[index].GroupProfitability.Rank = &value
		}
	}
}

func accountMonitorEstimatedProfitRate(groupMultiplier float64, multiplier AccountMonitorMultiplier) (float64, bool) {
	if groupMultiplier <= 0 || multiplier.Status != AccountMonitorMultiplierStatusOK || multiplier.Value == nil {
		return 0, false
	}
	value := (groupMultiplier - *multiplier.Value) / groupMultiplier
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func applyGroupSchedulerOrder(groups []AccountMonitorGroup) {
	for gi := range groups {
		sort.SliceStable(groups[gi].Accounts, func(i, j int) bool {
			l, r := groups[gi].Accounts[i].SchedulerRank, groups[gi].Accounts[j].SchedulerRank
			if l == nil && r != nil {
				return false
			}
			if l != nil && r == nil {
				return true
			}
			if l != nil && r != nil && *l != *r {
				return *l < *r
			}
			return groups[gi].Accounts[i].AccountID < groups[gi].Accounts[j].AccountID
		})
	}
}

func (s *AccountMonitorService) evaluateGroupRecommendation(
	ctx context.Context,
	account Account,
	currentGroupNames []string,
	groups []AccountMonitorGroup,
	evidence AccountMonitorQualityEvidence,
	latest AccountMonitorLatest,
	now time.Time,
) (recommendation *AccountMonitorGroupRecommendation) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.WarnContext(ctx, "account monitor recommendation evaluation failed", "account_id", account.ID, "error", recovered)
			recommendation = nil
		}
	}()
	evaluator := s.recommend
	if evaluator == nil {
		evaluator = EvaluateAccountMonitorGroupRecommendation
	}
	return evaluator(account, currentGroupNames, groups, evidence, latest, now)
}

func (s *AccountMonitorService) projectGlobalWindowQuality(
	accounts []Account,
	rows []AccountMonitorAccount,
	windows map[int64]AccountMonitorWindowAggregate,
	probes map[int64]AccountMonitorAggregate,
	historicalProbes map[int64]AccountMonitorAggregate,
	latest map[int64]AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
	weights AccountMonitorScoreWeights,
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
		probe := probes[row.AccountID]
		evidence := accountMonitorWindowEvidence(windows[row.AccountID], probe, latest[row.AccountID], settings, now)
		row.EvidenceSource = evidence.Source
		row.SampleCount = evidence.SampleCount
		row.SuccessSampleCount = evidence.SuccessSampleCount
		row.TTFTSampleCount = evidence.TTFTSampleCount
		row.LatencySampleCount = evidence.LatencySampleCount
		row.SuccessRate = evidence.SuccessRate
		row.TTFTP50MS = evidence.TTFTP50MS
		row.LatencyP95MS = evidence.LatencyP95MS
		row.OutputRateTokensPerSecond = evidence.OutputRateTokensPerSecond
		row.OutputRateSampleCount = evidence.OutputRateSampleCount
		row.CheckedAt = accountMonitorWindowCheckedAt(latest[row.AccountID], evidence)
		row.ManagementState = accountMonitorManagementState(account, now)
		projectAccountMonitorProbe(row, probe, latest[row.AccountID], row.Timeline, settings, now, row.ManagementState)
		projectAccountMonitorWindowState(row, evidence)
		row.ServiceState = accountMonitorLegacyServiceState(row.AvailabilityStatus)
		row.GroupEligibility = accountMonitorEligibilityNotApplicable
		row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
		scoreEvidence, scoreStatus, scoreEligible := accountMonitorWindowScoreProjection(account, row.ScoreStatus, evidence)
		if !scoreEligible {
			if historical := accountMonitorHistoricalEvidence(historicalProbes[row.AccountID]); historical.SampleCount > 0 {
				scoreEvidence, scoreStatus, scoreEligible = accountMonitorWindowScoreProjection(account, row.ScoreStatus, historical)
			}
		}
		row.ScoreStatus = scoreStatus
		row.Eligible = scoreEligible
		if row.Eligible {
			row.EvidenceSource = scoreEvidence.Source
			breakdown, score := accountMonitorWindowScoreBreakdown(1, row.EffectiveMultiplier, weights, scoreEvidence)
			row.ScoreBreakdown = &breakdown
			row.QualityScore = score
			capAccountMonitorAbnormalScore(row)
			row.QualityExplanation = &AccountMonitorQualityExplanation{
				Score: row.QualityScore, Breakdown: qualityExplanationBreakdown(breakdown, weights),
				Window: string(row.Range), SampleCount: scoreEvidence.SampleCount,
				Source: scoreEvidence.Source, ObservedAt: scoreEvidence.ObservedAt, ExperienceLabel: accountMonitorExperienceLabel(scoreEvidence),
			}
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
	rankTotal := 0
	for i := range rows {
		if rows[i].Eligible {
			rankTotal++
		}
	}
	rank := 0
	for i := range rows {
		if rows[i].Eligible {
			rank++
			value := rank
			rows[i].GroupRank = &value
			rows[i].QualityRank = &value
		}
		rows[i].QualityRankTotal = rankTotal
		if rows[i].QualityExplanation != nil {
			rows[i].QualityExplanation.Rank = rows[i].QualityRank
			rows[i].QualityExplanation.RankTotal = rankTotal
		}
	}
	return rows
}

func (s *AccountMonitorService) projectGroupWindowQuality(
	ctx context.Context,
	groups []AccountMonitorGroup,
	accounts []Account,
	rows []AccountMonitorAccount,
	probes map[int64]AccountMonitorAggregate,
	historicalProbes map[int64]AccountMonitorAggregate,
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
		members := make([]int64, 0)
		memberAccounts := make([]*Account, 0)
		for j := range accounts {
			if accountMonitorAccountInGroup(accounts[j], group.ID) {
				members = append(members, accounts[j].ID)
				member := accounts[j]
				memberAccounts = append(memberAccounts, &member)
			}
		}
		groupWindows := map[int64]AccountMonitorWindowAggregate{}
		groupWindowProvider, hasGroupWindowProvider := s.repo.(AccountMonitorGroupWindowAggregateRepository)
		groupWindowReadError := false
		groupWindowResultAvailable := false
		if hasGroupWindowProvider && len(members) > 0 {
			if loaded, loadErr := groupWindowProvider.ListGroupWindowAggregates(ctx, group.ID, members, windowStart, now); loadErr == nil {
				if loaded != nil {
					groupWindows = loaded
					groupWindowResultAvailable = true
				}
			} else {
				groupWindowReadError = true
			}
		}
		groupProbes := probes
		groupHistoricalProbes := historicalProbes
		if !hasGroupWindowProvider {
			// Group quality requires group-scoped request evidence. Do not let
			// account-scoped probes populate legacy adapters that lack it.
			groupProbes = nil
			groupHistoricalProbes = nil
		}
		projected := make([]AccountMonitorGroupAccount, 0)
		for _, account := range accounts {
			if !accountMonitorAccountInGroup(account, group.ID) {
				continue
			}
			base, ok := rowsByID[account.ID]
			if !ok {
				continue
			}
			window, hasGroupWindow := groupWindows[account.ID]
			probe := AccountMonitorAggregate{}
			historicalProbe := AccountMonitorAggregate{}
			if hasGroupWindow {
				probe = groupProbes[account.ID]
				historicalProbe = groupHistoricalProbes[account.ID]
			}
			evidence := accountMonitorWindowEvidence(window, probe, latest[account.ID], settings, now)
			if groupWindowReadError {
				evidence = accountMonitorUnknownQualityEvidence("read_error")
			} else if !groupWindowResultAvailable {
				evidence = accountMonitorUnknownQualityEvidence("missing")
			} else if !hasGroupWindow {
				evidence = accountMonitorStaleQualityEvidence()
			}
			row := AccountMonitorGroupAccount{AccountMonitorAccount: base, Evidence: evidence}
			// The embedded base row carries the full-site projection. Clear its
			// ranking fields before applying this group's evidence so a valid
			// global score cannot leak into a group row with no group evidence.
			row.QualityScore = nil
			row.ScoreBreakdown = nil
			row.GroupRank = nil
			row.QualityRank = nil
			row.QualityRankTotal = 0
			row.QualityExplanation = nil
			row.SchedulerRank = nil
			row.SchedulerRankTotal = 0
			row.SchedulerExplanation = nil
			row.EvidenceSource = evidence.Source
			row.SampleCount = evidence.SampleCount
			row.SuccessSampleCount = evidence.SuccessSampleCount
			row.TTFTSampleCount = evidence.TTFTSampleCount
			row.LatencySampleCount = evidence.LatencySampleCount
			row.SuccessRate = evidence.SuccessRate
			row.TTFTP50MS = evidence.TTFTP50MS
			row.LatencyP95MS = evidence.LatencyP95MS
			row.OutputRateTokensPerSecond = evidence.OutputRateTokensPerSecond
			row.OutputRateSampleCount = evidence.OutputRateSampleCount
			row.CheckedAt = accountMonitorWindowCheckedAt(latest[account.ID], evidence)
			row.ManagementState = accountMonitorManagementState(account, now)
			projectAccountMonitorProbe(&row.AccountMonitorAccount, probe, latest[account.ID], row.Timeline, settings, now, row.ManagementState)
			projectAccountMonitorWindowState(&row.AccountMonitorAccount, evidence)
			row.ServiceState = accountMonitorLegacyServiceState(row.AvailabilityStatus)
			row.GroupEligibility = accountMonitorEligibilityEligible
			row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
			cost := accountMonitorProjectedEffectiveCost(account, base.Multiplier, windowStart, now, window.BaseCost)
			row.CostMode = cost.Mode
			row.EffectiveMultiplier = cost.EffectiveMultiplier
			row.CostScore = accountMonitorCostScore(group.RateMultiplier, cost.EffectiveMultiplier, group.ScoreWeights)
			scoreEvidence, scoreStatus, scoreEligible := accountMonitorWindowScoreProjection(account, row.ScoreStatus, evidence)
			if !scoreEligible {
				if historical := accountMonitorHistoricalEvidence(historicalProbe); historical.SampleCount > 0 {
					scoreEvidence, scoreStatus, scoreEligible = accountMonitorWindowScoreProjection(account, row.ScoreStatus, historical)
				}
			}
			row.ScoreStatus = scoreStatus
			row.Eligible = scoreEligible && row.GroupEligibility == accountMonitorEligibilityEligible
			if row.Eligible {
				row.Evidence = scoreEvidence
				row.EvidenceSource = scoreEvidence.Source
				breakdown, score := accountMonitorWindowScoreBreakdown(group.RateMultiplier, cost.EffectiveMultiplier, group.ScoreWeights, scoreEvidence)
				row.ScoreBreakdown = &breakdown
				row.QualityScore = score
				capAccountMonitorAbnormalScore(&row.AccountMonitorAccount)
				row.QualityExplanation = &AccountMonitorQualityExplanation{
					Score: row.QualityScore, Breakdown: qualityExplanationBreakdown(breakdown, group.ScoreWeights),
					Window: string(row.Range), SampleCount: scoreEvidence.SampleCount,
					Source: scoreEvidence.Source, ObservedAt: scoreEvidence.ObservedAt, ExperienceLabel: accountMonitorExperienceLabel(scoreEvidence),
				}
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
		qualityRankTotal := 0
		for j := range projected {
			if projected[j].Eligible {
				qualityRankTotal++
				rank := qualityRankTotal
				projected[j].GroupRank = &rank
				projected[j].QualityRank = &rank
			}
			projected[j].QualityRankTotal = qualityRankTotal
			if projected[j].QualityExplanation != nil {
				projected[j].QualityExplanation.Rank = projected[j].QualityRank
			}
		}
		for j := range projected {
			projected[j].QualityRankTotal = qualityRankTotal
			if projected[j].QualityExplanation != nil {
				projected[j].QualityExplanation.RankTotal = qualityRankTotal
			}
		}
		qualityOrder := make([]int64, 0, len(projected))
		for _, row := range projected {
			if row.Eligible {
				qualityOrder = append(qualityOrder, row.AccountID)
			}
		}
		s.attachSchedulerProjection(ctx, group, memberAccounts, projected, qualityOrder, now)
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

func qualityExplanationBreakdown(breakdown AccountMonitorScoreBreakdown, weights AccountMonitorScoreWeights) AccountMonitorQualityExplanationBreakdown {
	return AccountMonitorQualityExplanationBreakdown{
		Cost:    AccountMonitorQualityScoreComponent{Score: breakdown.Cost, Max: float64(weights.Cost)},
		Success: AccountMonitorQualityScoreComponent{Score: breakdown.Success, Max: float64(weights.Success)},
		TTFT:    AccountMonitorQualityScoreComponent{Score: breakdown.TTFT, Max: float64(weights.TTFT)},
		Latency: AccountMonitorQualityScoreComponent{Score: breakdown.Latency, Max: float64(weights.Latency)},
	}
}

func accountMonitorReasonLabel(code AccountMonitorReasonCode) string {
	switch code {
	case AccountMonitorReasonStrategy:
		return "当前分组策略改变候选顺序"
	case AccountMonitorReasonQualityGate:
		return "质量资格影响候选资格"
	case AccountMonitorReasonRuntimeLoad:
		return "实时负载或排队改变候选顺序"
	case AccountMonitorReasonCooldown:
		return "短时错误或冷却改变候选资格"
	case AccountMonitorReasonTieBreak:
		return "调度分相同，使用稳定次级排序"
	case AccountMonitorReasonDataFreshness:
		return "质量证据与调度快照存在时效差异"
	case AccountMonitorReasonNotEligible:
		return "账号不具备当前分组调度资格"
	default:
		return ""
	}
}

func (s *AccountMonitorService) attachSchedulerProjection(
	ctx context.Context,
	group *AccountMonitorGroup,
	accounts []*Account,
	rows []AccountMonitorGroupAccount,
	qualityOrder []int64,
	now time.Time,
) {
	if s == nil || group == nil || group.ID <= 0 || len(accounts) == 0 {
		return
	}
	platform := NormalizeGroupPlatform(group.Platform)
	if platform != PlatformOpenAI && platform != PlatformGrok {
		return
	}
	if s.schedulerProjection == nil {
		markSchedulerProjectionUnavailable(rows)
		return
	}
	if s.concurrencyService != nil && s.concurrencyService.cache == nil {
		slog.WarnContext(ctx, "account monitor scheduler load snapshot unavailable", "group_id", group.ID, "reason", "concurrency cache unavailable")
		markSchedulerProjectionUnavailable(rows)
		return
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if s.concurrencyService != nil {
		loadReq := buildOpenAIAccountLoadRequest(accounts)
		loaded, err := s.concurrencyService.GetAccountsLoadBatch(ctx, loadReq)
		if err != nil {
			slog.WarnContext(ctx, "account monitor scheduler load snapshot unavailable", "group_id", group.ID, "error", err)
			markSchedulerProjectionUnavailable(rows)
			return
		}
		loadMap = loaded
	}
	projection, err := s.schedulerProjection.Project(ctx, OpenAIAccountSchedulerProjectionRequest{
		GroupID:           group.ID,
		Platform:          platform,
		RequiredTransport: OpenAIUpstreamTransportAny,
		RequirePrivacySet: group.RequirePrivacySet,
		QualityOrder:      append([]int64(nil), qualityOrder...),
		SnapshotAt:        now.UTC(),
		Accounts:          accounts,
		LoadMap:           loadMap,
	})
	if err != nil || projection == nil {
		if err != nil {
			slog.WarnContext(ctx, "account monitor scheduler projection unavailable", "group_id", group.ID, "error", err)
		} else {
			slog.WarnContext(ctx, "account monitor scheduler projection unavailable", "group_id", group.ID, "reason", "nil projection")
		}
		markSchedulerProjectionUnavailable(rows)
		return
	}
	candidates := make(map[int64]OpenAIAccountSchedulerProjectionCandidate, len(projection.Candidates))
	eligibleTotal := 0
	for _, candidate := range projection.Candidates {
		candidates[candidate.AccountID] = candidate
		if candidate.Eligible && candidate.Rank != nil {
			eligibleTotal++
		}
	}
	for i := range rows {
		candidate, ok := candidates[rows[i].AccountID]
		if !ok {
			continue
		}
		snapshotAt := projection.SnapshotAt.UTC()
		rows[i].SchedulerExplanation = &AccountMonitorSchedulerExplanation{
			Rank: candidate.Rank, RankTotal: eligibleTotal, CandidateTotal: projection.CandidateCount,
			Eligible: candidate.Eligible, PolicyKey: projection.PolicyKey, PolicyLabel: projection.PolicyLabel,
			EffectiveWeights: projection.EffectiveWeights, EffectiveFacts: projection.EffectiveFacts,
			ModelQuotaParity: projection.ModelQuotaParity, CandidateScope: "group", SnapshotAt: &snapshotAt,
			PrimaryReasonCode: candidate.PrimaryReasonCode, PrimaryReasonLabel: accountMonitorReasonLabel(candidate.PrimaryReasonCode),
		}
		if candidate.Eligible && candidate.Rank != nil {
			rows[i].SchedulerRank = candidate.Rank
			rows[i].SchedulerRankTotal = eligibleTotal
		}
	}
}

func markSchedulerProjectionUnavailable(rows []AccountMonitorGroupAccount) {
	for i := range rows {
		rows[i].SchedulerUnavailable = true
	}
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
			evidence, _ := accountMonitorEvidence(groupAggregate, globalAggregate, groupEvidenceAvailable, latest[accountID], settings, now)
			row := AccountMonitorGroupAccount{AccountMonitorAccount: base, Evidence: evidence}
			row.QualityScore = nil
			row.ScoreBreakdown = nil
			row.GroupRank = nil
			row.QualityRank = nil
			row.QualityRankTotal = 0
			row.QualityExplanation = nil
			row.SchedulerRank = nil
			row.SchedulerRankTotal = 0
			row.SchedulerExplanation = nil
			row.EvidenceSource = evidence.Source
			row.SampleCount = evidence.SampleCount
			row.SuccessSampleCount = evidence.SuccessSampleCount
			row.TTFTSampleCount = evidence.TTFTSampleCount
			row.LatencySampleCount = evidence.LatencySampleCount
			row.SuccessRate = evidence.SuccessRate
			row.TTFTP50MS = evidence.TTFTP50MS
			row.LatencyP95MS = evidence.LatencyP95MS
			row.OutputRateTokensPerSecond = evidence.OutputRateTokensPerSecond
			row.OutputRateSampleCount = evidence.OutputRateSampleCount
			if checkedAt := evidence.ObservedAt; !checkedAt.IsZero() {
				checkedAt = checkedAt.UTC()
				row.CheckedAt = &checkedAt
			}
			row.ManagementState = managementState
			row.ServiceState = accountMonitorGroupServiceState(row, evidence, row.ManagementState)
			row.GroupEligibility = accountMonitorGroupEligibility(row, group.RateMultiplier)
			row.MonitorBucket = accountMonitorBucket(row.ManagementState, row.ServiceState, row.GroupEligibility)
			row.Eligible = evidence.Known && (row.ScoreStatus == accountMonitorScoreEligible || row.ScoreStatus == accountMonitorScoreCapped) &&
				row.GroupEligibility == accountMonitorEligibilityEligible &&
				(row.ManagementState == accountMonitorManagementPaused || row.ServiceState == accountMonitorServiceAvailable)
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

// projectAccountMonitorProbe projects the active-probe-specific fields kept for
// backwards-compatible clients. Window quality is applied separately so real
// requests can participate without changing the probe aliases.
func projectAccountMonitorProbe(
	row *AccountMonitorAccount,
	aggregate AccountMonitorAggregate,
	latest AccountMonitorLatest,
	timeline []AccountMonitorTimelinePoint,
	settings AccountMonitorSettings,
	now time.Time,
	managementState string,
) {
	if row == nil {
		return
	}
	row.ProbeSampleCount = aggregate.SampleCount
	row.ProbeSuccessCount = aggregate.SuccessCount
	row.ProbeSuccessRate = aggregate.SuccessRate
	row.ProbeTTFTP50MS = aggregate.TTFTP50MS
	row.ProbeLatencyP95MS = aggregate.LatencyP95MS
	// Legacy quality fields remain aliases for clients that have not migrated.
	row.SampleCount = aggregate.SampleCount
	row.SuccessSampleCount = aggregate.SuccessCount
	row.SuccessRate = aggregate.SuccessRate
	row.TTFTSampleCount = aggregate.TTFTSampleCount
	row.LatencySampleCount = aggregate.LatencySampleCount
	row.TTFTP50MS = aggregate.TTFTP50MS
	row.LatencyP95MS = aggregate.LatencyP95MS
	row.CheckedAt = nil
	if aggregate.LastCheckedAt != nil && !aggregate.LastCheckedAt.IsZero() {
		checkedAt := aggregate.LastCheckedAt.UTC()
		row.CheckedAt = &checkedAt
	} else if !latest.CheckedAt.IsZero() {
		checkedAt := latest.CheckedAt.UTC()
		row.CheckedAt = &checkedAt
	}
	interval := time.Duration(settings.IntervalSeconds*2) * time.Second
	if interval <= 0 {
		interval = AccountMonitorDefaultIntervalSeconds * 2 * time.Second
	}
	row.Stale = aggregate.SampleCount == 0 || row.CheckedAt == nil || now.Sub(*row.CheckedAt) > interval
	consecutiveFailed := aggregate.ConsecutiveFailed
	if consecutiveFailed == 0 && latest.Status != "success" {
		for index := len(timeline) - 1; index >= 0; index-- {
			if timeline[index].Status == "success" {
				break
			}
			consecutiveFailed++
		}
		if consecutiveFailed == 0 {
			consecutiveFailed = 1
		}
	}
	row.AvailabilityStatus = accountMonitorAvailabilityStatus(managementState, row.Stale, aggregate.SampleCount, consecutiveFailed, latest)
	row.ScoreStatus = accountMonitorScoreStatus(row.AvailabilityStatus)
	row.Eligible = row.ScoreStatus == accountMonitorScoreEligible || row.ScoreStatus == accountMonitorScoreCapped
	if !row.Eligible {
		row.QualityScore = nil
		row.GroupRank = nil
	}
	if row.AvailabilityStatus == accountMonitorAvailabilityAbnormal && row.QualityScore != nil && *row.QualityScore > accountMonitorAbnormalScoreCap {
		capped := accountMonitorAbnormalScoreCap
		row.QualityScore = &capped
	}
}

func projectAccountMonitorWindowState(
	row *AccountMonitorAccount,
	evidence AccountMonitorQualityEvidence,
) {
	if row == nil {
		return
	}
	row.SampleCount = evidence.SampleCount
	row.SuccessSampleCount = evidence.SuccessSampleCount
	row.SuccessRate = evidence.SuccessRate
	row.TTFTSampleCount = evidence.TTFTSampleCount
	row.LatencySampleCount = evidence.LatencySampleCount
	row.TTFTP50MS = evidence.TTFTP50MS
	row.LatencyP95MS = evidence.LatencyP95MS
	row.OutputRateTokensPerSecond = evidence.OutputRateTokensPerSecond
	row.OutputRateSampleCount = evidence.OutputRateSampleCount
	// Availability and service state remain probe-driven; this helper only
	// replaces the quality fields with the hybrid window evidence. The probe
	// aliases and current-state gate are preserved for compatibility.
}

func accountMonitorAvailabilityStatus(managementState string, stale bool, sampleCount, consecutiveFailed int, latest AccountMonitorLatest) string {
	if sampleCount == 0 || stale {
		return accountMonitorAvailabilityStale
	}
	if managementState == accountMonitorManagementPaused {
		if accountMonitorHTTPFailure(latest) {
			return accountMonitorAvailabilityUnavailable
		}
	} else if consecutiveFailed >= 3 || accountMonitorFatalProbeError(latest) {
		return accountMonitorAvailabilityUnavailable
	}
	if latest.Status == "success" {
		return accountMonitorAvailabilityNormal
	}
	return accountMonitorAvailabilityAbnormal
}

func accountMonitorHTTPFailure(latest AccountMonitorLatest) bool {
	return latest.Status == "failed" && latest.HTTPStatus != nil && *latest.HTTPStatus >= http.StatusBadRequest && *latest.HTTPStatus <= 599
}

func accountMonitorProbeShouldStop(account Account, latest AccountMonitorLatest) bool {
	if !accountMonitorAccountPaused(account, time.Now().UTC()) {
		return false
	}
	// API-key 401 is a recoverable credential outage. Keep it in the monitor
	// pool so the five-minute recovery probe can re-admit the account after a
	// successful check instead of permanently suppressing all probes.
	if account.Type == AccountTypeAPIKey && latest.HTTPStatus != nil && *latest.HTTPStatus == http.StatusUnauthorized {
		return false
	}
	return accountMonitorHTTPFailure(latest)
}

func accountMonitorScoreStatus(availabilityStatus string) string {
	switch availabilityStatus {
	case accountMonitorAvailabilityNormal:
		return accountMonitorScoreEligible
	case accountMonitorAvailabilityAbnormal:
		return accountMonitorScoreCapped
	default:
		return accountMonitorScoreIneligible
	}
}

func accountMonitorLegacyServiceState(availabilityStatus string) string {
	switch availabilityStatus {
	case accountMonitorAvailabilityNormal:
		return accountMonitorServiceAvailable
	case accountMonitorAvailabilityDisabled:
		return accountMonitorServiceNotMonitored
	case accountMonitorAvailabilityStale:
		return accountMonitorServicePending
	default:
		return accountMonitorServiceUnavailable
	}
}

func capAccountMonitorAbnormalScore(row *AccountMonitorAccount) {
	if row == nil || row.AvailabilityStatus != accountMonitorAvailabilityAbnormal || row.QualityScore == nil || *row.QualityScore <= accountMonitorAbnormalScoreCap {
		return
	}
	capped := accountMonitorAbnormalScoreCap
	row.QualityScore = &capped
}

func accountMonitorFatalProbeError(latest AccountMonitorLatest) bool {
	if latest.HTTPStatus != nil {
		switch *latest.HTTPStatus {
		case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
			return true
		}
	}
	code := strings.ToLower(strings.TrimSpace(latest.ErrorCode))
	if code == "" {
		return false
	}
	for _, fatal := range []string{"auth", "unauthorized", "forbidden", "invalid_api_key", "invalid api key", "balance_exhausted", "quota", "insufficient_quota", "billing"} {
		if strings.Contains(code, fatal) {
			return true
		}
	}
	return false
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
	_ = globalAggregate
	aggregate := groupAggregate
	source := "group"
	if !groupEvidenceAvailable {
		return accountMonitorUnknownQualityEvidence("missing"), AccountMonitorAggregate{}
	}
	observedAt := time.Time{}
	if aggregate.LastCheckedAt != nil {
		observedAt = *aggregate.LastCheckedAt
	}
	if observedAt.IsZero() {
		observedAt = latest.CheckedAt
	}
	fresh := !observedAt.IsZero() && now.Sub(observedAt) <= time.Duration(settings.IntervalSeconds*2)*time.Second
	if !fresh || aggregate.SampleCount < AccountMonitorGroupEvidenceMinSamples {
		source = "stale"
	}
	evidence := AccountMonitorQualityEvidence{
		Source: source, Known: aggregate.SampleCount >= AccountMonitorGroupEvidenceMinSamples && fresh,
		Freshness:          accountMonitorQualityFreshnessFresh,
		SampleCount:        aggregate.SampleCount,
		SuccessSampleCount: legacyAggregateSuccessSamples(aggregate), TTFTSampleCount: aggregate.TTFTSampleCount,
		LatencySampleCount: aggregate.LatencySampleCount, SuccessRate: aggregate.SuccessRate,
		TTFTP50MS: aggregate.TTFTP50MS, LatencyP95MS: aggregate.LatencyP95MS, ObservedAt: observedAt.UTC(),
	}
	if source == "stale" {
		evidence.Freshness = accountMonitorQualityFreshnessStale
	}
	return evidence, aggregate
}

func legacyAggregateSuccessSamples(aggregate AccountMonitorAggregate) int {
	if aggregate.SuccessSampleCount > 0 {
		return aggregate.SuccessSampleCount
	}
	if aggregate.SuccessCount > 0 {
		return aggregate.SuccessCount
	}
	if aggregate.SampleCount > 0 && aggregate.SuccessRate > 0 {
		return int(math.Round(float64(aggregate.SampleCount) * aggregate.SuccessRate))
	}
	return 0
}

func projectLegacyAccountMonitorScore(row *AccountMonitorAccount, account Account, aggregate AccountMonitorAggregate, latest AccountMonitorLatest, groups []AccountMonitorGroup, now time.Time) {
	if row == nil || aggregate.SampleCount <= 0 || legacyAggregateSuccessSamples(aggregate) <= 0 || row.Multiplier.Value == nil {
		return
	}
	evidence := accountMonitorLegacyQualityEvidence(aggregate, latest, now)
	row.EvidenceSource = evidence.Source
	row.QualityScore = CalculateAccountMonitorQualityScore(1, *row.Multiplier.Value, normalizeAccountMonitorScoreWeights(DefaultAccountMonitorScoreWeights), evidence)
	if row.QualityScore == nil {
		return
	}
	breakdown, _ := accountMonitorScoreBreakdown(1, row.Multiplier.Value, normalizeAccountMonitorScoreWeights(DefaultAccountMonitorScoreWeights), evidence)
	row.ScoreBreakdown = &breakdown
	row.QualityExplanation = &AccountMonitorQualityExplanation{Score: row.QualityScore, Window: "legacy", SampleCount: evidence.SampleCount, Source: evidence.Source, ObservedAt: evidence.ObservedAt, ExperienceLabel: accountMonitorExperienceLabel(evidence)}
	row.Eligible = true
	_ = account
	_ = groups
}

func accountMonitorLegacyQualityEvidence(aggregate AccountMonitorAggregate, latest AccountMonitorLatest, now time.Time) AccountMonitorQualityEvidence {
	observedAt := time.Time{}
	if aggregate.LastCheckedAt != nil {
		observedAt = aggregate.LastCheckedAt.UTC()
	} else {
		observedAt = latest.CheckedAt.UTC()
	}
	freshness := accountMonitorQualityFreshnessFresh
	if observedAt.IsZero() || now.Sub(observedAt) > 2*AccountMonitorGroupEvidenceWindow {
		freshness = accountMonitorQualityFreshnessStale
	}
	return AccountMonitorQualityEvidence{Source: "legacy", Known: true, Freshness: freshness, SampleCount: aggregate.SampleCount, SuccessSampleCount: aggregate.SuccessSampleCount, TTFTSampleCount: aggregate.TTFTSampleCount, LatencySampleCount: aggregate.LatencySampleCount, SuccessRate: aggregate.SuccessRate, TTFTP50MS: aggregate.TTFTP50MS, LatencyP95MS: aggregate.LatencyP95MS, ObservedAt: observedAt}
}

// CalculateAccountMonitorQualityScore is deliberately pure: it only uses the
// group/account billing multipliers, score weights, and supplied evidence.
func CalculateAccountMonitorQualityScore(
	groupMultiplier float64,
	accountMultiplier float64,
	weights AccountMonitorScoreWeights,
	evidence AccountMonitorQualityEvidence,
) *float64 {
	_, score := accountMonitorScoreBreakdown(groupMultiplier, &accountMultiplier, weights, evidence)
	return score
}

func accountMonitorScoreBreakdown(
	groupMultiplier float64,
	accountMultiplier *float64,
	weights AccountMonitorScoreWeights,
	evidence AccountMonitorQualityEvidence,
) (AccountMonitorScoreBreakdown, *float64) {
	if groupMultiplier <= 0 || !accountMonitorQualityEvidenceUsableForScore(evidence) {
		return AccountMonitorScoreBreakdown{}, nil
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
	breakdown := AccountMonitorScoreBreakdown{
		Cost:    accountMonitorCostScore(groupMultiplier, accountMultiplier, weights),
		Success: float64(weights.Success) * clamp01(evidence.SuccessRate),
		TTFT:    float64(weights.TTFT) * latencyScore(evidence.TTFTP50MS, weights.TTFTTargetMS, weights.TTFTLimitMS),
		Latency: float64(weights.Latency) * accountMonitorGenerationExperienceScore(evidence, weights, latencyScore),
	}
	total := math.Round(breakdown.Cost + breakdown.Success + breakdown.TTFT + breakdown.Latency)
	return breakdown, &total
}

const accountMonitorExperienceReferenceOutputTokens = 256

// AccountMonitorOutputRateTokensPerSecond derives a weighted real-request
// generation rate from usage_logs aggregates. Invalid or incomplete input is
// unknown, never treated as a fast zero-rate response.
func AccountMonitorOutputRateTokensPerSecond(outputTokens int64, generationMS float64, sampleCount int) *float64 {
	if outputTokens <= 0 || generationMS <= 0 || sampleCount <= 0 || math.IsNaN(generationMS) || math.IsInf(generationMS, 0) {
		return nil
	}
	rate := float64(outputTokens) * 1000 / generationMS
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil
	}
	return &rate
}

func validAccountMonitorOutputRate(rate *float64) bool {
	return rate != nil && *rate > 0 && !math.IsNaN(*rate) && !math.IsInf(*rate, 0)
}

func accountMonitorGenerationExperienceScore(
	evidence AccountMonitorQualityEvidence,
	weights AccountMonitorScoreWeights,
	latencyScore func(*float64, int, int) float64,
) float64 {
	if evidence.OutputRateSampleCount > 0 && validAccountMonitorOutputRate(evidence.OutputRateTokensPerSecond) && weights.LatencyTargetMS > 0 && weights.LatencyLimitMS > weights.LatencyTargetMS {
		// The persisted latency thresholds retain their role as the experience
		// budget, translated through a fixed generic output size rather than a
		// model-specific rank or a new persisted policy dimension.
		targetRate := float64(accountMonitorExperienceReferenceOutputTokens*1000) / float64(weights.LatencyTargetMS)
		limitRate := float64(accountMonitorExperienceReferenceOutputTokens*1000) / float64(weights.LatencyLimitMS)
		rate := *evidence.OutputRateTokensPerSecond
		if rate >= targetRate {
			return 1
		}
		if rate <= limitRate {
			return 0
		}
		return (rate - limitRate) / (targetRate - limitRate)
	}
	return latencyScore(evidence.LatencyP95MS, weights.LatencyTargetMS, weights.LatencyLimitMS)
}

func accountMonitorExperienceLabel(evidence AccountMonitorQualityEvidence) string {
	if evidence.OutputRateSampleCount > 0 && validAccountMonitorOutputRate(evidence.OutputRateTokensPerSecond) {
		return "生成体验（输出速率）"
	}
	return "生成体验（完整响应 P95 兼容回退）"
}

func accountMonitorQualityEvidenceUsableForScore(evidence AccountMonitorQualityEvidence) bool {
	if evidence.Known {
		return true
	}
	return evidence.Source != "" && evidence.Source != accountMonitorQualitySourceUnknown &&
		(evidence.SampleCount > 0 || evidence.SuccessRate > 0 || evidence.TTFTP50MS != nil || evidence.LatencyP95MS != nil || validAccountMonitorOutputRate(evidence.OutputRateTokensPerSecond))
}

type accountMonitorWindowCostResult struct {
	Mode                string
	WindowCost          float64
	EffectiveMultiplier *float64
	CostScoreEligible   bool
}

func accountMonitorEffectiveCost(account Account, windowStart, windowEnd time.Time, baseCost float64) accountMonitorWindowCostResult {
	switch {
	case account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey:
		return accountMonitorMultiplierValueCost(account.BillingRateMultiplier())
	case account.Platform == PlatformOpenAI:
		return accountMonitorProcurementQuotaCost(account.ProcurementCostCNY, account.EstimatedUsableQuotaUSD)
	default:
		return accountMonitorLegacyWindowCost(account, windowStart, windowEnd, baseCost)
	}
}

func accountMonitorEquivalentSiteMultiplier(account Account, model string, pricing accountMonitorModelPricingReader) *float64 {
	model = strings.TrimSpace(model)
	if pricing == nil || model == "" {
		return nil
	}
	sitePricing, err := pricing.GetModelPricing(model)
	if err != nil || sitePricing == nil {
		return nil
	}
	upstreamModel := strings.TrimSpace(resolveAccountUpstreamModel(&account, model))
	if upstreamModel == "" {
		upstreamModel = model
	}
	upstreamPricing, err := pricing.GetModelPricing(upstreamModel)
	if err != nil || upstreamPricing == nil {
		return nil
	}
	siteUnitPrice := sitePricing.InputPricePerToken + sitePricing.OutputPricePerToken
	upstreamUnitPrice := upstreamPricing.InputPricePerToken + upstreamPricing.OutputPricePerToken
	if siteUnitPrice <= 0 || upstreamUnitPrice < 0 || math.IsNaN(siteUnitPrice) || math.IsNaN(upstreamUnitPrice) || math.IsInf(siteUnitPrice, 0) || math.IsInf(upstreamUnitPrice, 0) {
		return nil
	}
	accountMultiplier := 0.0
	if rate := accountMonitorMultiplierCost(account.RateMultiplier).EffectiveMultiplier; rate != nil {
		accountMultiplier = *rate
	} else if procurement := accountMonitorProcurementQuotaCost(account.ProcurementCostCNY, account.EstimatedUsableQuotaUSD).EffectiveMultiplier; procurement != nil {
		accountMultiplier = *procurement
	}
	result := accountMultiplier * upstreamUnitPrice / siteUnitPrice
	if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
		return nil
	}
	return &result
}

func accountMonitorProjectedEffectiveCost(
	account Account,
	_ AccountMonitorMultiplier,
	windowStart, windowEnd time.Time,
	baseCost float64,
) accountMonitorWindowCostResult {
	return accountMonitorEffectiveCost(account, windowStart, windowEnd, baseCost)
}

func accountMonitorProcurementQuotaCost(cost, quota *float64) accountMonitorWindowCostResult {
	result := accountMonitorWindowCostResult{Mode: "procurement"}
	if cost == nil || quota == nil || math.IsNaN(*cost) || math.IsInf(*cost, 0) || *cost < 0 ||
		math.IsNaN(*quota) || math.IsInf(*quota, 0) || *quota <= 0 {
		return result
	}
	effectiveMultiplier := *cost / *quota
	result.EffectiveMultiplier = &effectiveMultiplier
	result.CostScoreEligible = true
	return result
}

func accountMonitorMultiplierCost(multiplier *float64) accountMonitorWindowCostResult {
	result := accountMonitorWindowCostResult{Mode: "multiplier"}
	if multiplier == nil || math.IsNaN(*multiplier) || math.IsInf(*multiplier, 0) || *multiplier < 0 {
		return result
	}
	effectiveMultiplier := *multiplier
	result.EffectiveMultiplier = &effectiveMultiplier
	result.CostScoreEligible = true
	return result
}

func accountMonitorMultiplierValueCost(multiplier float64) accountMonitorWindowCostResult {
	return accountMonitorMultiplierCost(&multiplier)
}

func accountMonitorLegacyWindowCost(account Account, windowStart, windowEnd time.Time, baseCost float64) accountMonitorWindowCostResult {
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

	return accountMonitorMultiplierValueCost(account.BillingRateMultiplier())
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
	_, score := accountMonitorWindowScoreBreakdown(groupMultiplier, effectiveMultiplier, weights, evidence)
	return score
}

func accountMonitorWindowScoreBreakdown(
	groupMultiplier float64,
	effectiveMultiplier *float64,
	weights AccountMonitorScoreWeights,
	evidence AccountMonitorQualityEvidence,
) (AccountMonitorScoreBreakdown, *float64) {
	return accountMonitorScoreBreakdown(groupMultiplier, effectiveMultiplier, weights, evidence)
}

func accountMonitorWindowEvidence(
	window AccountMonitorWindowAggregate,
	probe AccountMonitorAggregate,
	latest AccountMonitorLatest,
	settings AccountMonitorSettings,
	now time.Time,
) AccountMonitorQualityEvidence {
	evidence := fuseAccountMonitorQualityEvidence(window, probe, latest, settings, now)
	if window.RequestCount > 0 && probe.SampleCount == 0 && evidence.Known {
		if accountMonitorWindowObservedAt(window).IsZero() {
			evidence = accountMonitorUnknownQualityEvidence("missing")
			evidence.ObservedAt = accountMonitorProbeObservedAt(probe, latest)
			return evidence
		}
		return accountMonitorStaleQualityEvidenceAt(latestEvidenceTime(accountMonitorWindowObservedAt(window), accountMonitorProbeObservedAt(probe, latest)))
	}
	if !evidence.Known && evidence.Freshness == accountMonitorQualityFreshnessStale {
		evidence.ObservedAt = latestEvidenceTime(accountMonitorWindowObservedAt(window), accountMonitorProbeObservedAt(probe, latest))
	}
	return evidence
}

func accountMonitorStaleQualityEvidence() AccountMonitorQualityEvidence {
	return AccountMonitorQualityEvidence{Source: "stale", Freshness: accountMonitorQualityFreshnessStale, UnknownReason: "stale"}
}

func accountMonitorStaleQualityEvidenceAt(observedAt time.Time) AccountMonitorQualityEvidence {
	evidence := accountMonitorStaleQualityEvidence()
	evidence.ObservedAt = observedAt.UTC()
	return evidence
}

func accountMonitorHistoricalEvidence(aggregate AccountMonitorAggregate) AccountMonitorQualityEvidence {
	if aggregate.SampleCount <= 0 || legacyAggregateSuccessSamples(aggregate) <= 0 {
		return AccountMonitorQualityEvidence{Source: "historical_final", Freshness: accountMonitorQualityFreshnessUnknown}
	}
	observedAt := time.Time{}
	if aggregate.LastCheckedAt != nil {
		observedAt = aggregate.LastCheckedAt.UTC()
	}
	return AccountMonitorQualityEvidence{
		Source: "historical_final", Known: true, Freshness: accountMonitorQualityFreshnessFresh, SampleCount: aggregate.SampleCount,
		SuccessSampleCount: legacyAggregateSuccessSamples(aggregate), TTFTSampleCount: aggregate.TTFTSampleCount,
		LatencySampleCount: aggregate.LatencySampleCount, SuccessRate: aggregate.SuccessRate,
		TTFTP50MS: aggregate.TTFTP50MS, LatencyP95MS: aggregate.LatencyP95MS, ObservedAt: observedAt,
	}
}

func accountMonitorWindowScoreProjection(
	account Account,
	currentScoreStatus string,
	evidence AccountMonitorQualityEvidence,
) (AccountMonitorQualityEvidence, string, bool) {
	if !accountMonitorQualityEvidenceUsableForScore(evidence) || evidence.SampleCount <= 0 || evidence.SuccessSampleCount <= 0 {
		return evidence, accountMonitorScoreIneligible, false
	}
	if currentScoreStatus == accountMonitorScoreEligible || currentScoreStatus == accountMonitorScoreCapped {
		if evidence.Source == "stale" {
			evidence.Source = accountMonitorQualitySourceProbe
		}
		return evidence, currentScoreStatus, true
	}
	if !account.IsSchedulable() && account.Schedulable {
		return evidence, accountMonitorScoreIneligible, false
	}
	if evidence.Source == "stale" {
		evidence.Source = accountMonitorQualitySourceProbe
	}
	return evidence, accountMonitorScoreEligible, true
}

func accountMonitorWindowObservedAt(window AccountMonitorWindowAggregate) time.Time {
	if window.LastObservedAt != nil {
		return window.LastObservedAt.UTC()
	}
	return time.Time{}
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

func (s *AccountMonitorService) resolveBalance(account *Account, now time.Time) *AccountMonitorBalance {
	if s == nil || s.multiplier == nil || !isAccountMonitorBalanceEligible(account) {
		return nil
	}
	balanceResolver, ok := s.multiplier.(interface {
		ResolveBalance(*Account, time.Time) AccountMonitorBalance
	})
	if !ok {
		return nil
	}
	balance := balanceResolver.ResolveBalance(account, now)
	return &balance
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

// SettleDueProbeBuckets is a lightweight watchdog pass used by the runner.
// It does not execute account probes; it only closes any due group/bucket
// terminal that a prior monitor run may have missed. The unique ledger key
// keeps this safe to call concurrently with RunAll.
func (s *AccountMonitorService) SettleDueProbeBuckets(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	accounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list monitor accounts for probe terminal watchdog: %w", err)
	}
	if err := s.settleDueProbeBuckets(ctx, monitorGroupAccountIDs(accounts), time.Now().UTC(), uuid.NewString()); err != nil {
		return fmt.Errorf("settle monitor probe terminal watchdog: %w", err)
	}
	return nil
}

func (s *AccountMonitorService) runAll(ctx context.Context, actorID int64) (int, error) {
	accounts, err := s.listPool(ctx)
	if err != nil {
		return 0, err
	}
	allAccounts, err := s.listMonitorAccounts(ctx)
	if err != nil {
		return 0, err
	}
	runID := uuid.NewString()
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	latest, err := s.repo.ListLatest(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("list latest account monitor results: %w", err)
	}
	var mu sync.Mutex
	var completed int
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, account := range accounts {
		account := account
		if accountMonitorProbeShouldStop(account, latest[account.ID]) {
			continue
		}
		if s.shouldSkipAPIKeyRecoveryProbe(account, time.Now().UTC()) {
			continue
		}
		g.Go(func() error {
			reader := s.activeProbeUsageReader()
			if reader != nil {
				bucketStart, bucketEnd := currentActiveProbeBucket(time.Now())
				used, usageErr := reader.HasAccountUsageInWindow(gctx, account.ID, bucketStart, bucketEnd)
				if usageErr != nil {
					// A usage-read failure must not silently turn into a skipped
					// probe. Real requests still win in the projection; when the
					// reader is unavailable, execute the probe and let persistence
					// or the read-side fail-closed path record the failure.
					slog.WarnContext(gctx, "account_monitor.active_probe_usage_read_failed", "account_id", account.ID, "error", usageErr)
				} else if used {
					return nil
				}
			}
			if reader == nil {
				slog.WarnContext(gctx, "account_monitor.active_probe_usage_reader_unavailable", "account_id", account.ID)
			}
			if err := gctx.Err(); err != nil {
				return err
			}
			result := s.probeAccount(gctx, account)
			if err := s.repo.InsertResult(gctx, result, runID); err != nil {
				return fmt.Errorf("persist account %d monitor result: %w", account.ID, err)
			}
			s.recordAPIKeyRecoveryProbe(gctx, account, result)
			s.refreshAuxiliary(gctx, &account, AccountMonitorRefreshOptions{
				RefreshDeclaration: true, RefreshBalance: true,
			})
			mu.Lock()
			completed++
			mu.Unlock()
			return nil
		})
	}
	probeRunErr := g.Wait()
	terminalErr := s.settleDueProbeBuckets(ctx, monitorGroupAccountIDs(allAccounts), time.Now().UTC(), runID)
	if probeRunErr != nil || terminalErr != nil {
		return completed, errors.Join(probeRunErr, terminalErr)
	}
	if err := s.repo.DeleteBefore(ctx, time.Now().Add(-AccountMonitorResultRetentionDays*24*time.Hour)); err != nil {
		return completed, fmt.Errorf("delete account monitor history: %w", err)
	}
	_ = actorID
	return completed, nil
}

// settleDueProbeBuckets closes the previous bucket on every run and closes the
// current bucket only during its final minute. The unique repository key makes
// retries and overlapping scheduler runs idempotent. A usage-reader error does
// not skip the terminal write: real usage remains authoritative in V4 and will
// suppress this probe terminal at read time.
func (s *AccountMonitorService) settleDueProbeBuckets(
	ctx context.Context,
	groupAccounts map[int64][]int64,
	now time.Time,
	runID string,
) error {
	if s == nil || s.repo == nil {
		return nil
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("list monitor groups for probe terminal settlement: %w", err)
	}
	for _, group := range groups {
		if group.ID <= 0 {
			continue
		}
		if group.Status != StatusActive {
			continue
		}
		if _, exists := groupAccounts[group.ID]; !exists {
			groupAccounts[group.ID] = nil
		}
	}
	if len(groupAccounts) == 0 {
		return nil
	}
	currentStart, _ := currentActiveProbeBucket(now)
	currentLocal := now.In(activeProbeLocation)
	currentStartLocal := currentStart.In(activeProbeLocation)
	buckets := []time.Time{currentStart.Add(-5 * time.Minute)}
	if currentLocal.Sub(currentStartLocal) >= 4*time.Minute && currentLocal.Sub(currentStartLocal) < 5*time.Minute {
		buckets = append(buckets, currentStart)
	}
	reader := s.activeProbeUsageReader()
	groupIDs := make([]int64, 0, len(groupAccounts))
	for groupID := range groupAccounts {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	var errs []error
	for _, groupID := range groupIDs {
		accountIDs := groupAccounts[groupID]
		for _, bucketStart := range buckets {
			if reader != nil {
				used, err := reader.HasGroupUsageInWindow(ctx, groupID, bucketStart, bucketStart.Add(5*time.Minute))
				if err != nil {
					slog.WarnContext(ctx, "account_monitor.group_usage_read_failed_for_terminal", "group_id", groupID, "bucket_start", bucketStart.UTC(), "error", err)
				} else if used {
					continue
				}
			}
			if err := s.repo.EnsureProbeBucketTerminal(ctx, groupID, accountIDs, bucketStart.UTC(), runID); err != nil {
				errs = append(errs, fmt.Errorf("group %d bucket %s: %w", groupID, bucketStart.UTC().Format(time.RFC3339), err))
			}
		}
	}
	return errors.Join(errs...)
}

func monitorGroupAccountIDs(accounts []Account) map[int64][]int64 {
	byGroup := make(map[int64][]int64)
	seen := make(map[int64]map[int64]struct{})
	for _, account := range accounts {
		if account.ID <= 0 {
			continue
		}
		groups := make(map[int64]struct{}, len(account.GroupIDs)+len(account.Groups))
		for _, groupID := range account.GroupIDs {
			if groupID > 0 {
				groups[groupID] = struct{}{}
			}
		}
		for _, group := range account.Groups {
			if group != nil && group.ID > 0 {
				groups[group.ID] = struct{}{}
			}
		}
		for groupID := range groups {
			if seen[groupID] == nil {
				seen[groupID] = make(map[int64]struct{})
			}
			if _, exists := seen[groupID][account.ID]; exists {
				continue
			}
			seen[groupID][account.ID] = struct{}{}
			byGroup[groupID] = append(byGroup[groupID], account.ID)
		}
	}
	for groupID := range byGroup {
		sort.Slice(byGroup[groupID], func(i, j int) bool { return byGroup[groupID][i] < byGroup[groupID][j] })
	}
	return byGroup
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

	accounts, err := s.listActiveAccounts(ctx)
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
		runErr = fmt.Errorf("account %d is not active", accountID)
		return AccountMonitorProbeResult{}, runErr
	}
	latest, err := s.repo.ListLatest(ctx, []int64{target.ID})
	if err != nil {
		runErr = fmt.Errorf("list latest account monitor results: %w", err)
		return AccountMonitorProbeResult{}, runErr
	}
	if accountMonitorProbeShouldStop(*target, latest[target.ID]) {
		runErr = fmt.Errorf("account %d monitor probe stopped after closed-scheduling HTTP error", accountID)
		return AccountMonitorProbeResult{}, runErr
	}
	if s.shouldSkipAPIKeyRecoveryProbe(*target, time.Now().UTC()) {
		runErr = fmt.Errorf("account %d API-key recovery probe is not due", accountID)
		return AccountMonitorProbeResult{}, runErr
	}
	result := s.probeAccount(ctx, *target)
	if err := s.repo.InsertResult(ctx, result, uuid.NewString()); err != nil {
		runErr = err
		return AccountMonitorProbeResult{}, err
	}
	completed = 1
	s.recordAPIKeyRecoveryProbe(ctx, *target, result)
	s.refreshAuxiliary(ctx, target, AccountMonitorRefreshOptions{
		RefreshDeclaration: true, RefreshBalance: true,
	})
	_ = s.repo.DeleteBefore(ctx, time.Now().Add(-AccountMonitorResultRetentionDays*24*time.Hour))
	_ = actorID
	return result, nil
}

type accountMonitorTempUnschedulableRecoveryRepository interface {
	ClearTempUnschedulable(context.Context, int64) error
}

func (s *AccountMonitorService) shouldSkipAPIKeyRecoveryProbe(account Account, now time.Time) bool {
	if s == nil || account.Type != AccountTypeAPIKey || account.TempUnschedulableUntil == nil {
		return false
	}
	s.probeRecoveryMu.Lock()
	defer s.probeRecoveryMu.Unlock()
	if s.apiKeyProbeNextAt == nil {
		s.apiKeyProbeNextAt = make(map[int64]time.Time)
	}
	next := s.apiKeyProbeNextAt[account.ID]
	return !next.IsZero() && now.Before(next)
}

func (s *AccountMonitorService) recordAPIKeyRecoveryProbe(ctx context.Context, account Account, result AccountMonitorProbeResult) {
	if s == nil || account.Type != AccountTypeAPIKey || account.TempUnschedulableUntil == nil {
		return
	}
	now := result.CheckedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.probeRecoveryMu.Lock()
	if s.apiKeyProbeFailures == nil {
		s.apiKeyProbeFailures = make(map[int64]int)
	}
	if s.apiKeyProbeNextAt == nil {
		s.apiKeyProbeNextAt = make(map[int64]time.Time)
	}
	if strings.EqualFold(strings.TrimSpace(result.Status), "success") {
		delete(s.apiKeyProbeFailures, account.ID)
		delete(s.apiKeyProbeNextAt, account.ID)
		s.probeRecoveryMu.Unlock()
		if repo, ok := s.accountRepo.(accountMonitorTempUnschedulableRecoveryRepository); ok {
			if err := repo.ClearTempUnschedulable(ctx, account.ID); err != nil {
				slog.Warn("account_monitor.api_key_recovery_clear_temp_failed", "account_id", account.ID, "error", err)
				return
			}
			if s.runtimeBlocker != nil {
				s.runtimeBlocker.ClearAccountSchedulingBlock(account.ID)
			}
			slog.Info("account_monitor.api_key_recovered", "account_id", account.ID)
		}
		return
	}
	failures := s.apiKeyProbeFailures[account.ID] + 1
	s.apiKeyProbeFailures[account.ID] = failures
	interval := 5 * time.Minute
	if failures >= 3 {
		interval = 15 * time.Minute
	}
	s.apiKeyProbeNextAt[account.ID] = now.Add(interval)
	s.probeRecoveryMu.Unlock()
}

// ProbeOpenAIAccountModel performs one real account-test request for the
// supplied canonical model. It is intentionally narrow so admin recovery can
// reuse the tested upstream transport instead of fabricating a probe result.
func (s *AccountMonitorService) ProbeOpenAIAccountModel(ctx context.Context, accountID int64, canonicalModel string) (bool, error) {
	if s == nil || s.testService == nil {
		return false, fmt.Errorf("account test service is unavailable")
	}
	canonicalModel = strings.TrimSpace(canonicalModel)
	if accountID <= 0 || canonicalModel == "" {
		return false, fmt.Errorf("account_id and canonical_scheduling_model are required")
	}
	result, err := s.testService.RunTestBackground(ctx, accountID, canonicalModel)
	if err != nil {
		return false, err
	}
	return result != nil && result.Status == "success", nil
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

func (s *AccountMonitorService) refreshAuxiliary(ctx context.Context, account *Account, options AccountMonitorRefreshOptions) {
	if s == nil || s.multiplier == nil || account == nil {
		return
	}
	refreshCtx, cancel := context.WithTimeout(ctx, accountMonitorMultiplierRefreshTimeout)
	defer cancel()
	if err := s.multiplier.Refresh(refreshCtx, account, options); err != nil {
		slog.Warn("account_monitor: auxiliary refresh failed",
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

func (s *AccountMonitorService) GetGlobalScoreWeights(ctx context.Context) (AccountMonitorGlobalScoreWeightsResponse, error) {
	weights, err := s.repo.LoadGlobalScoreWeights(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultGlobalScoreWeightsResponse(), nil
	}
	if err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("load global score weights: %w", err)
	}
	return globalScoreWeightsResponse(weights, false), nil
}

func (s *AccountMonitorService) UpdateGlobalScoreWeights(ctx context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorGlobalScoreWeightsResponse, error) {
	if actorID <= 0 {
		return AccountMonitorGlobalScoreWeightsResponse{}, errors.New("invalid actor id")
	}
	weights = fourAccountMonitorScoreWeights(weights)
	if err := validateAccountMonitorFourScoreWeights(weights); err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, err
	}
	saved, err := s.repo.SaveGlobalScoreWeights(ctx, actorID, weights)
	if err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("save global score weights: %w", err)
	}
	return globalScoreWeightsResponse(saved, false), nil
}

func (s *AccountMonitorService) ResetGlobalScoreWeights(ctx context.Context, actorID int64) (AccountMonitorGlobalScoreWeightsResponse, error) {
	if actorID <= 0 {
		return AccountMonitorGlobalScoreWeightsResponse{}, errors.New("invalid actor id")
	}
	if err := s.repo.ResetGlobalScoreWeights(ctx); err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("reset global score weights: %w", err)
	}
	return defaultGlobalScoreWeightsResponse(), nil
}

func defaultGlobalScoreWeightsResponse() AccountMonitorGlobalScoreWeightsResponse {
	return globalScoreWeightsResponse(fourAccountMonitorScoreWeights(DefaultAccountMonitorScoreWeights), true)
}

func globalScoreWeightsResponse(weights AccountMonitorScoreWeights, isDefault bool) AccountMonitorGlobalScoreWeightsResponse {
	response := AccountMonitorGlobalScoreWeightsResponse{
		Cost:      weights.Cost,
		Success:   weights.Success,
		TTFT:      weights.TTFT,
		Latency:   weights.Latency,
		UpdatedBy: weights.UpdatedBy,
		IsDefault: isDefault,
	}
	if !weights.UpdatedAt.IsZero() {
		updatedAt := weights.UpdatedAt
		response.UpdatedAt = &updatedAt
	}
	return response
}

func fourAccountMonitorScoreWeights(weights AccountMonitorScoreWeights) AccountMonitorScoreWeights {
	return AccountMonitorScoreWeights{Cost: weights.Cost, Success: weights.Success, TTFT: weights.TTFT, Latency: weights.Latency}
}

func validateAccountMonitorFourScoreWeights(weights AccountMonitorScoreWeights) error {
	if weights.Cost < 0 || weights.Cost > 100 ||
		weights.Success < 0 || weights.Success > 100 ||
		weights.TTFT < 0 || weights.TTFT > 100 ||
		weights.Latency < 0 || weights.Latency > 100 {
		return fmt.Errorf("%w: score weights must be between 0 and 100", ErrAccountMonitorInvalidScoreWeights)
	}
	if weights.Cost+weights.Success+weights.TTFT+weights.Latency != 100 {
		return fmt.Errorf("%w: score weights must sum to 100", ErrAccountMonitorInvalidScoreWeights)
	}
	return nil
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
	accounts, err := s.listActiveAccounts(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	filtered := accounts[:0]
	for _, account := range accounts {
		nativeEligible := account.IsSchedulableAt(now)
		// API-key recovery probes are the native half-open exception: they are
		// rate-limited by shouldSkipAPIKeyRecoveryProbe and only test an explicit
		// 401/authentication temporary block. Balance, quota, and permission blocks
		// must stay out of the automatic probe pool.
		recoveryEligible := accountMonitorAPIKey401RecoveryEligible(&account, now)
		if (nativeEligible || recoveryEligible) && account.ActiveProbeEnabled() && accountActiveProbeEnabledByGroups(&account) {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

func accountMonitorAPIKey401RecoveryEligible(account *Account, now time.Time) bool {
	if account == nil || account.Type != AccountTypeAPIKey || account.TempUnschedulableUntil == nil || !now.Before(*account.TempUnschedulableUntil) {
		return false
	}
	if account.IsQuotaExceeded() || (account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt)) || (account.OverloadUntil != nil && now.Before(*account.OverloadUntil)) {
		return false
	}

	reason := strings.ToLower(strings.TrimSpace(account.TempUnschedulableReason))
	if reason == "" || strings.Contains(reason, "403") || strings.Contains(reason, "forbidden") {
		return false
	}
	for _, blocked := range []string{"balance", "quota", "insufficient", "billing", "payment", "precharge"} {
		if strings.Contains(reason, blocked) {
			return false
		}
	}
	for _, authMarker := range []string{"401", "unauthorized", "authentication", "invalid_auth", "invalid_api_key", "invalid api key", "credential_invalid", "credential"} {
		if strings.Contains(reason, authMarker) {
			return true
		}
	}
	return false
}

func accountActiveProbeEnabledByGroups(account *Account) bool {
	if account == nil {
		return false
	}
	for _, group := range account.Groups {
		if group != nil && !group.ActiveProbeEnabled {
			return false
		}
	}
	return true
}

func (s *AccountMonitorService) listActiveAccounts(ctx context.Context) ([]Account, error) {
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return nil, fmt.Errorf("list active monitor accounts: %w", err)
	}
	filtered := accounts[:0]
	for _, account := range accounts {
		if account.Status == StatusActive {
			filtered = append(filtered, account)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered, nil
}

func (s *AccountMonitorService) activeProbeUsageReader() ActiveProbeUsageWindowReader {
	if s == nil {
		return nil
	}
	if s.activeProbeUsage != nil {
		return s.activeProbeUsage
	}
	if s.usage != nil {
		return s.usage
	}
	return nil
}

func (s *AccountMonitorService) probeAccount(ctx context.Context, account Account) AccountMonitorProbeResult {
	modelID := s.connectionProbeModel(ctx, &account)
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
	result, err := probeConnection(probeCtx, account.ID, modelID, accountMonitorProbePrompt(), AccountTestModeDefault)
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

func (s *AccountMonitorService) connectionProbeModel(ctx context.Context, account *Account) string {
	models := nativeAccountTextModels(account)
	saved := ""
	if s != nil && s.modelDetection != nil && account != nil {
		if settings, err := s.modelDetection.LoadSettings(ctx, account.ID); err == nil {
			saved = settings.ConnectionProbeModel
		}
	}
	if selected := selectConnectionProbeModel(models, saved); selected != "" {
		return selected
	}
	return monitorModelForAccount(account)
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
