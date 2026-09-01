package service

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// OpenAIAccountQuality is the account-level, non-image quality projection used
// by the unified scheduler. U is deliberately absent: effective cost is read
// live from EffectiveCostForAccount at candidate-build time.
type OpenAIAccountQuality struct {
	AccountID            int64
	AttemptCount         int64
	SuccessCount         int64
	SuccessRate          *float64
	TTFTSampleCount      int64
	TTFTTrimmedMeanMS    *float64
	LatencySampleCount   int64
	LatencyTrimmedMeanMS *float64
}

// OpenAIAccountQualityRepository is the narrow read-only repository contract
// consumed by the scheduler. The broader UsageLogRepository remains unchanged
// for compatibility with existing test doubles and services.
type OpenAIAccountQualityRepository interface {
	ListOpenAIAccountQuality(ctx context.Context, start, end time.Time) ([]OpenAIAccountQuality, error)
}

type OpenAIAccountQualitySnapshot struct {
	WindowStart time.Time
	WindowEnd   time.Time
	SnapshotAt  time.Time
	Stale       bool
	Accounts    map[int64]OpenAIAccountQuality
}

type OpenAIAccountQualitySnapshotProvider interface {
	Snapshot(ctx context.Context) OpenAIAccountQualitySnapshot
}

type openAIAccountQualitySnapshotProvider struct {
	repo OpenAIAccountQualityRepository
	ttl  time.Duration
	now  func() time.Time

	mu      sync.Mutex
	last    OpenAIAccountQualitySnapshot
	hasLast bool
	refresh singleflight.Group
}

func NewOpenAIAccountQualitySnapshotProvider(repo OpenAIAccountQualityRepository, ttl time.Duration, now func() time.Time) OpenAIAccountQualitySnapshotProvider {
	if ttl <= 0 {
		ttl = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &openAIAccountQualitySnapshotProvider{repo: repo, ttl: ttl, now: now}
}

func (p *openAIAccountQualitySnapshotProvider) Snapshot(ctx context.Context) OpenAIAccountQualitySnapshot {
	if p == nil {
		return OpenAIAccountQualitySnapshot{Stale: true, Accounts: map[int64]OpenAIAccountQuality{}}
	}
	if snapshot, ok := p.cached(p.now()); ok {
		return snapshot
	}

	value, _, _ := p.refresh.Do("openai-account-quality", func() (any, error) {
		if snapshot, ok := p.cached(p.now()); ok {
			return snapshot, nil
		}
		if p.repo == nil {
			return p.coldStartFailure(), nil
		}

		end := p.now()
		start := end.Add(-7 * 24 * time.Hour)
		rows, err := p.repo.ListOpenAIAccountQuality(ctx, start, end)
		if err != nil {
			return p.staleOrColdStart(), nil
		}
		accounts := make(map[int64]OpenAIAccountQuality, len(rows))
		for _, row := range rows {
			if row.AccountID <= 0 {
				continue
			}
			accounts[row.AccountID] = row
		}
		snapshot := OpenAIAccountQualitySnapshot{
			WindowStart: start,
			WindowEnd:   end,
			SnapshotAt:  end,
			Accounts:    accounts,
		}
		p.mu.Lock()
		p.last = snapshot
		p.hasLast = true
		p.mu.Unlock()
		return cloneOpenAIAccountQualitySnapshot(snapshot), nil
	})
	if snapshot, ok := value.(OpenAIAccountQualitySnapshot); ok {
		return snapshot
	}
	return p.staleOrColdStart()
}

func (p *openAIAccountQualitySnapshotProvider) cached(now time.Time) (OpenAIAccountQualitySnapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasLast || now.Sub(p.last.SnapshotAt) < 0 || now.Sub(p.last.SnapshotAt) >= p.ttl {
		return OpenAIAccountQualitySnapshot{}, false
	}
	return cloneOpenAIAccountQualitySnapshot(p.last), true
}

func (p *openAIAccountQualitySnapshotProvider) staleOrColdStart() OpenAIAccountQualitySnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasLast {
		return OpenAIAccountQualitySnapshot{Stale: true, Accounts: map[int64]OpenAIAccountQuality{}}
	}
	snapshot := cloneOpenAIAccountQualitySnapshot(p.last)
	snapshot.Stale = true
	return snapshot
}

func (p *openAIAccountQualitySnapshotProvider) coldStartFailure() OpenAIAccountQualitySnapshot {
	return OpenAIAccountQualitySnapshot{Stale: true, Accounts: map[int64]OpenAIAccountQuality{}}
}

func cloneOpenAIAccountQualitySnapshot(snapshot OpenAIAccountQualitySnapshot) OpenAIAccountQualitySnapshot {
	accounts := make(map[int64]OpenAIAccountQuality, len(snapshot.Accounts))
	for id, quality := range snapshot.Accounts {
		accounts[id] = quality
	}
	snapshot.Accounts = accounts
	return snapshot
}
