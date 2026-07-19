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
	reportCalls := 0
	s := Scheduler{Mode: config.ModeProbe, Store: store, Timezone: shanghai, Clock: func() time.Time { return now },
		Production: func(context.Context) error { productionCalls++; return nil },
		Candidates: func(context.Context) ([]domain.UpstreamID, error) { return []domain.UpstreamID{17}, nil },
		Candidate: func(_ context.Context, id domain.UpstreamID, probe bool) error {
			candidateCalls[id] = append(candidateCalls[id], probe)
			return nil
		},
		DailyReport: func(context.Context) error { reportCalls++; return nil },
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute)
	_ = s.Tick(context.Background())
	if productionCalls != 1 || len(candidateCalls[17]) != 1 || !candidateCalls[17][0] || reportCalls != 1 {
		t.Fatalf("calls production=%d candidate=%v report=%d", productionCalls, candidateCalls, reportCalls)
	}
	now = now.Add(time.Minute)
	_ = s.Tick(context.Background())
	if productionCalls != 2 || len(candidateCalls[17]) != 1 {
		t.Fatalf("five minute calls production=%d candidate=%v", productionCalls, candidateCalls)
	}
	now = time.Date(2026, 7, 19, 7, 0, 0, 0, time.UTC)
	_ = s.Tick(context.Background())
	if len(candidateCalls[17]) != 2 || reportCalls != 1 {
		t.Fatalf("six hour calls candidate=%v report=%d", candidateCalls, reportCalls)
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
