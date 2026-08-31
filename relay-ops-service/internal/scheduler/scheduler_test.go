package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
)

func TestTickUsesFixedProductionCandidateAndDailySchedules(t *testing.T) {
	t.Parallel()
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)
	store := newFakeJobStore()
	productionCalls := 0
	candidateCalls := map[domain.UpstreamID][]bool{}
	s := Scheduler{Mode: config.ModeProbe, Store: store, Timezone: shanghai, Clock: func() time.Time { return now },
		Production: func(context.Context) error { productionCalls++; return nil },
		Candidates: func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{17}, nil },
		Candidate: func(_ context.Context, id domain.UpstreamID, probe bool) error {
			candidateCalls[id] = append(candidateCalls[id], probe)
			return nil
		},
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute)
	_ = s.Tick(context.Background())
	if productionCalls != 1 || len(candidateCalls[17]) != 1 || !candidateCalls[17][0] {
		t.Fatalf("calls production=%d candidate=%v", productionCalls, candidateCalls)
	}
	now = now.Add(time.Minute)
	_ = s.Tick(context.Background())
	if productionCalls != 2 || len(candidateCalls[17]) != 1 {
		t.Fatalf("five minute calls production=%d candidate=%v", productionCalls, candidateCalls)
	}
	now = time.Date(2026, 7, 19, 7, 0, 0, 0, time.UTC)
	_ = s.Tick(context.Background())
	if len(candidateCalls[17]) != 2 {
		t.Fatalf("six hour calls candidate=%v", candidateCalls)
	}
}

func TestReadOnlySkipsPaidProbeAndClosedSkipsCollection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		mode      string
		wantCalls int
		wantProbe bool
	}{{config.ModeReadOnly, 1, false}, {config.ModeClosed, 0, false}} {
		calls := 0
		probed := false
		s := Scheduler{Mode: test.mode, Store: newFakeJobStore(), Clock: time.Now,
			Production: func(context.Context) error { return nil }, Candidates: func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{17}, nil },
			Candidate: func(_ context.Context, _ domain.UpstreamID, probe bool) error { calls++; probed = probe; return nil },
		}
		_ = s.Tick(context.Background())
		if calls != test.wantCalls || probed != test.wantProbe {
			t.Fatalf("mode=%s calls=%d probed=%v", test.mode, calls, probed)
		}
	}
}

func TestCandidateFailuresAreIsolated(t *testing.T) {
	t.Parallel()
	visited := []domain.UpstreamID{}
	s := Scheduler{Mode: config.ModeProbe, Store: newFakeJobStore(), Clock: time.Now,
		Production: func(context.Context) error { return nil }, Candidates: func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{1, 2}, nil },
		Candidate: func(_ context.Context, id domain.UpstreamID, _ bool) error {
			visited = append(visited, id)
			if id == 1 {
				return errors.New("failed")
			}
			return nil
		},
	}
	if err := s.Tick(context.Background()); err == nil || len(visited) != 2 {
		t.Fatalf("err=%v visited=%v", err, visited)
	}
}

func TestProbeModeUsesQualityFirstCandidateCadence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	store := newFakeJobStore()
	calls := map[string]int{}
	s := Scheduler{
		Mode: config.ModeProbe, Store: store, Clock: func() time.Time { return now },
		Candidates: func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{73}, nil },
		FastCandidate: func(_ context.Context, id domain.UpstreamID, job string, paid bool) error {
			if id != 73 || !paid {
				t.Fatalf("id=%d paid=%v", id, paid)
			}
			calls[job]++
			return nil
		},
	}

	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{JobHealthPulse, JobCatalogQuick, JobCapacityCheck} {
		if calls[job] != 1 {
			t.Fatalf("initial calls=%v", calls)
		}
	}
	now = now.Add(14 * time.Minute)
	_ = s.Tick(context.Background())
	if calls[JobHealthPulse] != 1 || calls[JobCatalogQuick] != 1 || calls[JobCapacityCheck] != 1 {
		t.Fatalf("fourteen-minute calls=%v", calls)
	}
	now = now.Add(time.Minute)
	_ = s.Tick(context.Background())
	if calls[JobHealthPulse] != 2 || calls[JobCatalogQuick] != 1 || calls[JobCapacityCheck] != 1 {
		t.Fatalf("fifteen-minute calls=%v", calls)
	}
	now = time.Date(2026, 7, 22, 6, 0, 0, 0, time.UTC)
	_ = s.Tick(context.Background())
	if calls[JobCatalogQuick] != 2 || calls[JobCapacityCheck] != 1 {
		t.Fatalf("six-hour calls=%v", calls)
	}
	now = time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	_ = s.Tick(context.Background())
	if calls[JobCapacityCheck] != 2 {
		t.Fatalf("daily calls=%v", calls)
	}
}

func TestReadOnlyNeverRunsPaidFastCandidateJobs(t *testing.T) {
	t.Parallel()
	fastCalls := 0
	legacyCalls := 0
	s := Scheduler{
		Mode: config.ModeReadOnly, Store: newFakeJobStore(), Clock: time.Now,
		Candidates:    func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{73}, nil },
		Candidate:     func(context.Context, domain.UpstreamID, bool) error { legacyCalls++; return nil },
		FastCandidate: func(context.Context, domain.UpstreamID, string, bool) error { fastCalls++; return nil },
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fastCalls != 0 || legacyCalls != 1 {
		t.Fatalf("fast=%d legacy=%d", fastCalls, legacyCalls)
	}
}

func TestTickDoesNotRunAccountingBeforeShanghai0010(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := Scheduler{
		Store:    newFakeJobStore(),
		Timezone: shanghai,
		Clock:    func() time.Time { return time.Date(2026, 8, 3, 0, 9, 0, 0, shanghai) },
		AccountingDaily: func(context.Context) error {
			calls++
			return nil
		},
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("accounting calls = %d, want 0", calls)
	}
}

func TestTickRunsAccountingOnceAfterShanghai0010(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := Scheduler{
		Store:    newFakeJobStore(),
		Timezone: shanghai,
		Clock:    func() time.Time { return time.Date(2026, 8, 3, 0, 10, 0, 0, shanghai) },
		AccountingDaily: func(context.Context) error {
			calls++
			return nil
		},
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("accounting calls = %d, want 1", calls)
	}
}

type fakeJobStore struct {
	mu  sync.Mutex
	due map[string]time.Time
}

func newFakeJobStore() *fakeJobStore { return &fakeJobStore{due: map[string]time.Time{}} }
func (s *fakeJobStore) Claim(_ context.Context, key string, now time.Time, interval time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	due, ok := s.due[key]
	if ok && now.Before(due) {
		return false, nil
	}
	s.due[key] = now.Add(interval)
	return true, nil
}
func (s *fakeJobStore) Finish(context.Context, string, time.Time, error) error { return nil }
