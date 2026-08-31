package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
)

const (
	JobHealthPulse   = "health_pulse"
	JobCatalogQuick  = "catalog_quick"
	JobCapacityCheck = "capacity_check"
)

type JobStore interface {
	Claim(context.Context, string, time.Time, time.Duration) (bool, error)
	Finish(context.Context, string, time.Time, error) error
}

type Scheduler struct {
	Mode                string
	Store               JobStore
	Timezone            *time.Location
	Clock               func() time.Time
	Production          func(context.Context) error
	Candidates          func(context.Context) ([]domain.UpstreamID, error)
	Candidate           func(context.Context, domain.UpstreamID, bool) error
	FastCandidate       func(context.Context, domain.UpstreamID, string, bool) error
	UsageSessions       func(context.Context) ([]billing.SessionConfig, error)
	Usage               func(context.Context, billing.SessionConfig) error
	AccountingDaily     func(context.Context) error
	ReconciliationSweep func(context.Context) error
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			// Individual failures are persisted by Finish; later jobs still run.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) Tick(ctx context.Context) error {
	if s.Mode == config.ModeClosed {
		return nil
	}
	if s.Store == nil {
		return fmt.Errorf("scheduler store is required")
	}
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock().UTC()
	}
	var failures []error
	if s.ReconciliationSweep != nil {
		if err := s.runDue(ctx, "reconciliation-sweep", now, time.Minute, s.ReconciliationSweep); err != nil {
			failures = append(failures, err)
		}
	}
	if err := s.runDue(ctx, "production-collection", now, 5*time.Minute, s.RunProductionCollection); err != nil {
		failures = append(failures, err)
	}
	if s.Candidates != nil {
		ids, err := s.Candidates(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("list candidates: %w", err))
		} else {
			for _, id := range ids {
				candidateID := id
				if s.Mode == config.ModeProbe && s.FastCandidate != nil {
					jobs := []struct {
						kind     string
						interval time.Duration
					}{
						{JobHealthPulse, 15 * time.Minute},
						{JobCatalogQuick, 6 * time.Hour},
						{JobCapacityCheck, 24 * time.Hour},
					}
					for _, job := range jobs {
						current := job
						key := "candidate-fast:" + strconv.FormatInt(int64(candidateID), 10) + ":" + current.kind
						if err := s.runDue(ctx, key, now, current.interval, func(runCtx context.Context) error {
							return s.FastCandidate(runCtx, candidateID, current.kind, true)
						}); err != nil {
							failures = append(failures, err)
						}
					}
				} else {
					key := "candidate-cycle:" + strconv.FormatInt(int64(candidateID), 10)
					if err := s.runDue(ctx, key, now, 6*time.Hour, func(runCtx context.Context) error { return s.RunCandidateCycle(runCtx, candidateID) }); err != nil {
						failures = append(failures, err)
					}
				}
			}
		}
	}
	if s.UsageSessions != nil && s.Usage != nil {
		sessions, err := s.UsageSessions(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("list usage sessions: %w", err))
		} else {
			for _, session := range sessions {
				current := session
				key := "usage-session:" + strconv.FormatInt(int64(session.UpstreamID), 10)
				if err := s.runDue(ctx, key, now, time.Hour, func(runCtx context.Context) error { return s.Usage(runCtx, current) }); err != nil {
					failures = append(failures, err)
				}
			}
		}
	}
	location := s.Timezone
	if location == nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	local := now.In(location)
	if s.AccountingDaily != nil &&
		(local.Hour() > 0 || (local.Hour() == 0 && local.Minute() >= 10)) {
		if err := s.runDue(ctx, "accounting-daily", now, 24*time.Hour, s.AccountingDaily); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Scheduler) RunProductionCollection(ctx context.Context) error {
	if s.Production == nil {
		return nil
	}
	return s.Production(ctx)
}

func (s *Scheduler) RunCandidateCycle(ctx context.Context, upstreamID domain.UpstreamID) error {
	if s.Candidate == nil {
		return nil
	}
	return s.Candidate(ctx, upstreamID, s.Mode == config.ModeProbe)
}

func (s *Scheduler) runDue(ctx context.Context, key string, now time.Time, interval time.Duration, run func(context.Context) error) error {
	claimed, err := s.Store.Claim(ctx, key, now, interval)
	if err != nil {
		return fmt.Errorf("claim %s: %w", key, err)
	}
	if !claimed {
		return nil
	}
	runErr := run(ctx)
	if finishErr := s.Store.Finish(ctx, key, now, runErr); finishErr != nil {
		return fmt.Errorf("finish %s: %w", key, finishErr)
	}
	if runErr != nil {
		return fmt.Errorf("run %s: %w", key, runErr)
	}
	return nil
}
