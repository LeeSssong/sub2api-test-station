package service

import (
	"context"
	"errors"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
	"math/rand"
	"time"
)

type monitorV4ProbeGroupKey struct{}
type accountMonitorConnectionModelOverrideKey struct{}

const monitorV4CheckCooldown = 30 * time.Second

type MonitorV4CheckResult struct {
	GroupID   int64     `json:"group_id"`
	Status    string    `json:"status"`
	TTFTMS    *float64  `json:"ttft_ms"`
	CheckedAt time.Time `json:"checked_at"`
}
type monitorV4GroupChecker interface {
	CheckMonitorGroup(context.Context, int64) (AccountMonitorProbeResult, error)
}
type monitorV4GroupProbeWriter interface {
	InsertGroupProbeResult(context.Context, int64, AccountMonitorProbeResult, string) error
}
type monitorV4LinkedGroups interface {
	LinkedMonitorGroups(context.Context, int64) ([]Group, error)
}

// LinkedMonitorGroups includes disabled keys; no credential is exposed to callers.
func (s *APIKeyService) LinkedMonitorGroups(ctx context.Context, userID int64) ([]Group, error) {
	seen := map[int64]bool{}
	groups := []Group{}
	for page := 1; ; page++ {
		keys, paging, err := s.apiKeyRepo.ListByUserID(ctx, userID, pagination.PaginationParams{Page: page, PageSize: 1000}, APIKeyListFilters{})
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if key.Group != nil && !seen[key.Group.ID] {
				seen[key.Group.ID] = true
				groups = append(groups, *key.Group)
			}
		}
		if len(keys) < 1000 || paging == nil || page >= paging.Pages {
			break
		}
	}
	return groups, nil
}

func (s *MonitorV4Service) beginCheck(userID int64, now time.Time) error {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if s.checkNext == nil {
		s.checkNext = map[int64]time.Time{}
		s.checkActive = map[int64]bool{}
	}
	if s.checkActive[userID] || now.Before(s.checkNext[userID]) || s.checkCount >= 4 {
		return infraerrors.TooManyRequests("LINE_CHECK_RATE_LIMIT", "线路检查进行中或过于频繁，请稍后重试")
	}
	for id, next := range s.checkNext {
		if !s.checkActive[id] && !now.Before(next) {
			delete(s.checkNext, id)
			delete(s.checkActive, id)
		}
	}
	s.checkNext[userID] = now.Add(monitorV4CheckCooldown)
	s.checkActive[userID] = true
	s.checkCount++
	return nil
}
func (s *MonitorV4Service) finishCheck(userID int64) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	s.checkActive[userID] = false
	s.checkCount--
}

func (s *MonitorV4Service) Check(ctx context.Context, userID int64, ids []int64) ([]MonitorV4CheckResult, error) {
	if userID <= 0 {
		return nil, infraerrors.Unauthorized("UNAUTHENTICATED", "User not authenticated")
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, infraerrors.BadRequest("INVALID_GROUPS", "请选择 1 至 100 条线路")
	}
	checker, ok := s.native.(monitorV4GroupChecker)
	if !ok {
		return nil, errors.New("line checker unavailable")
	}
	groups, err := s.available.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	allowed := map[int64]Group{}
	for _, g := range groups {
		allowed[g.ID] = g
	}
	if linked, ok := s.available.(monitorV4LinkedGroups); ok {
		items, e := linked.LinkedMonitorGroups(ctx, userID)
		if e != nil {
			return nil, e
		}
		for _, g := range items {
			if !g.IsActive() {
				allowed[g.ID] = g
			}
		}
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if _, ok := allowed[id]; !ok || id <= 0 {
			return nil, infraerrors.Forbidden("GROUP_NOT_AVAILABLE", "线路不可访问")
		}
		if seen[id] {
			return nil, infraerrors.BadRequest("DUPLICATE_GROUP", "线路重复")
		}
		seen[id] = true
	}
	if err := s.beginCheck(userID, time.Now()); err != nil {
		return nil, err
	}
	defer s.finishCheck(userID)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results := make([]MonitorV4CheckResult, len(ids))
	for i, id := range ids {
		status := "failed"
		if allowed[id].Status != StatusActive {
			status = "disabled"
		}
		results[i] = MonitorV4CheckResult{GroupID: id, Status: status, CheckedAt: time.Now().UTC()}
	}
	// Bounded workers keep one user's batch from multiplying upstream requests without limit.
	jobs := make(chan int)
	done := make(chan struct{}, 2)
	for worker := 0; worker < 2; worker++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := range jobs {
				id := ids[i]
				result := results[i]
				if allowed[id].Status == StatusActive && ctx.Err() == nil {
					probe, e := checker.CheckMonitorGroup(context.WithValue(ctx, monitorV4ProbeGroupKey{}, allowed[id]), id)
					result.Status = "failed"
					if e == nil {
						result.Status = monitorV4CheckStatus(probe)
						result.TTFTMS = probe.TTFTMS
						result.CheckedAt = probe.CheckedAt
					}
				}
				results[i] = result
			}
		}()
	}
queue:
	for i := range ids {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break queue
		}
	}
	close(jobs)
	<-done
	<-done
	return results, nil
}
func monitorV4CheckStatus(result AccountMonitorProbeResult) string {
	if (result.ErrorCode == "timeout" && result.TTFTMS == nil) || (result.TTFTMS != nil && *result.TTFTMS > 15000) {
		return "timeout"
	}
	if result.Status == "success" && result.TTFTMS != nil {
		return "success"
	}
	return "failed"
}

// CheckMonitorGroup chooses only a currently schedulable account belonging to the
// requested group. It reuses native probe models, credentials and persistence.
func (s *AccountMonitorService) CheckMonitorGroup(ctx context.Context, groupID int64) (AccountMonitorProbeResult, error) {
	accounts, err := s.listActiveAccounts(ctx)
	if err != nil {
		return AccountMonitorProbeResult{}, err
	}
	type probeCandidate struct {
		account  Account
		models   []string
		priority int
	}
	var candidates []probeCandidate
	var accountIDs []int64
	group, hasGroup := ctx.Value(monitorV4ProbeGroupKey{}).(Group)
	for _, account := range accounts {
		belongs := false
		for _, id := range account.GroupIDs {
			if id == groupID {
				belongs = true
			}
		}
		for _, group := range account.Groups {
			if group != nil && group.ID == groupID {
				belongs = true
			}
		}
		if !belongs || !account.IsSchedulableAt(time.Now()) {
			continue
		}
		models := make([]string, 0)
		for _, model := range nativeAccountTextModels(&account) {
			if (!hasGroup || group.ModelAllowlist.Allows(model)) && account.IsModelSupported(model) {
				models = append(models, model)
			}
		}
		if len(models) == 0 {
			continue
		}
		candidates = append(candidates, probeCandidate{account: account, models: models, priority: accountSchedulingPriorityForGroup(&account, &groupID)})
		accountIDs = append(accountIDs, account.ID)
	}
	if len(candidates) == 0 {
		return AccountMonitorProbeResult{}, errors.New("no schedulable account for requested group")
	}
	latest, err := s.repo.ListLatest(ctx, accountIDs)
	if err != nil {
		return AccountMonitorProbeResult{}, err
	}
	bestPriority := 0
	var best []probeCandidate
	for _, candidate := range candidates {
		if accountMonitorProbeShouldStop(candidate.account, latest[candidate.account.ID]) || s.shouldSkipAPIKeyRecoveryProbe(candidate.account, time.Now()) {
			continue
		}
		if len(best) == 0 || candidate.priority < bestPriority {
			bestPriority = candidate.priority
			best = best[:0]
		}
		if candidate.priority == bestPriority {
			best = append(best, candidate)
		}
	}
	if len(best) == 0 {
		return AccountMonitorProbeResult{}, errors.New("no schedulable account for requested group")
	}
	selected := best[rand.Intn(len(best))]
	model := selected.models[rand.Intn(len(selected.models))]
	modelCtx := context.WithValue(ctx, accountMonitorConnectionModelOverrideKey{}, model)
	probeCtx, cancel := context.WithTimeout(context.WithValue(modelCtx, accountMonitorFirstTokenTimeoutKey{}, 15*time.Second), 60*time.Second)
	result := s.probeAccount(probeCtx, selected.account)
	cancel()
	if result.TTFTMS != nil && *result.TTFTMS > 15000 {
		result.Status = "failed"
		result.ErrorCode = "timeout"
	}
	writer, ok := s.repo.(monitorV4GroupProbeWriter)
	if !ok {
		return AccountMonitorProbeResult{}, errors.New("group probe persistence unavailable")
	}
	if err := writer.InsertGroupProbeResult(ctx, groupID, result, uuid.NewString()); err != nil {
		return AccountMonitorProbeResult{}, err
	}
	return result, nil
}

func (s *MonitorV4Service) withLinkedMonitorGroups(ctx context.Context, userID int64, groups []Group) ([]Group, error) {
	reader, ok := s.available.(monitorV4LinkedGroups)
	if !ok {
		return groups, nil
	}
	linked, err := reader.LinkedMonitorGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, g := range groups {
		seen[g.ID] = true
	}
	for _, g := range linked {
		if !seen[g.ID] {
			groups = append(groups, g)
			seen[g.ID] = true
		}
	}
	return groups, nil
}
