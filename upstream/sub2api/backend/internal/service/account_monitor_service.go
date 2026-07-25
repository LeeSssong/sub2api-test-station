package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type AccountMonitorService struct {
	repo        AccountMonitorRepository
	accountRepo AccountMonitorAccountRepository
	testService *AccountTestService
	usage       *AccountUsageService
	runMu       sync.Mutex
}

type AccountMonitorAccountRepository interface {
	ListSchedulable(ctx context.Context) ([]Account, error)
}

func NewAccountMonitorService(
	repo AccountMonitorRepository,
	accountRepo AccountMonitorAccountRepository,
	testService *AccountTestService,
	usage *AccountUsageService,
) *AccountMonitorService {
	return &AccountMonitorService{
		repo:        repo,
		accountRepo: accountRepo,
		testService: testService,
		usage:       usage,
	}
}

func (s *AccountMonitorService) List(ctx context.Context) (AccountMonitorPage, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return AccountMonitorPage{}, err
	}
	accounts, err := s.listPool(ctx)
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

	observedAt := time.Now().UTC()
	rows := make([]AccountMonitorAccount, 0, len(accounts))
	for _, account := range accounts {
		aggregate := aggregates[account.ID]
		row := AccountMonitorAccount{
			AccountID:    account.ID,
			Name:         account.Name,
			Platform:     account.Platform,
			AccountType:  account.Type,
			Status:       account.Status,
			Schedulable:  account.Schedulable,
			GroupIDs:     append([]int64(nil), account.GroupIDs...),
			GroupNames:   accountGroupNames(account),
			ModelID:      monitorModelForAccount(&account),
			LatestStatus: "unavailable",
			SuccessRate:  aggregate.SuccessRate,
			SampleCount:  aggregate.SampleCount,
			TTFTP50MS:    aggregate.TTFTP50MS,
			TTFTP95MS:    aggregate.TTFTP95MS,
			LatencyP95MS: aggregate.LatencyP95MS,
			Multiplier:   account.BillingRateMultiplier(),
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
		rows = append(rows, row)
	}

	return AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		SchemaVersion: AccountMonitorSchemaVersion,
		ObservedAt:    observedAt,
		Stale:         len(rows) == 0 || anyMonitorRowStale(rows),
		Settings:      settings,
		Accounts:      rows,
	}}, nil
}

func (s *AccountMonitorService) RunAll(ctx context.Context, actorID int64) (int, error) {
	if !s.runMu.TryLock() {
		return 0, errors.New("account monitor run already in progress")
	}
	defer s.runMu.Unlock()

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
	if !s.runMu.TryLock() {
		return AccountMonitorProbeResult{}, errors.New("account monitor run already in progress")
	}
	defer s.runMu.Unlock()

	accounts, err := s.listPool(ctx)
	if err != nil {
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
		return AccountMonitorProbeResult{}, fmt.Errorf("account %d is not active and schedulable", accountID)
	}
	result := s.probeAccount(ctx, *target)
	if err := s.repo.InsertResult(ctx, result, uuid.NewString()); err != nil {
		return AccountMonitorProbeResult{}, err
	}
	_ = s.repo.DeleteBefore(ctx, time.Now().Add(-AccountMonitorHistoryDays*24*time.Hour))
	_ = actorID
	return result, nil
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
	accounts, err := s.accountRepo.ListSchedulable(ctx)
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
	if s.testService == nil {
		return AccountMonitorProbeResult{
			AccountID: account.ID,
			ModelID:   modelID,
			Status:    "failed",
			ErrorCode: "account_test_error",
			CheckedAt: time.Now().UTC(),
		}
	}
	result, err := s.testService.ProbeAccountConnection(ctx, account.ID, modelID, "hi", AccountTestModeDefault)
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
		return "claude-sonnet-4-5"
	}
	switch strings.ToLower(strings.TrimSpace(account.Platform)) {
	case PlatformOpenAI:
		return "gpt-4o-mini"
	case PlatformGemini:
		return "gemini-2.5-flash"
	case PlatformGrok:
		return "grok-3-mini"
	case PlatformAntigravity:
		return "claude-sonnet-4-5"
	default:
		return "claude-sonnet-4-5"
	}
}

func anyMonitorRowStale(rows []AccountMonitorAccount) bool {
	for _, row := range rows {
		if row.Stale {
			return true
		}
	}
	return false
}
