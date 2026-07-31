package securityaudit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lifecycleConfigStore struct {
	fakeConfigStore

	mu            sync.Mutex
	startCalls    int
	shutdownCalls int
}

func (s *lifecycleConfigStore) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	return nil
}

func (s *lifecycleConfigStore) Shutdown(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdownCalls++
	return nil
}

func (s *lifecycleConfigStore) calls() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalls, s.shutdownCalls
}

func TestPromptServiceAPIRoleStartsConfigWithoutSharedRunnerAndKeepsEnqueue(t *testing.T) {
	store := &lifecycleConfigStore{fakeConfigStore: fakeConfigStore{cfg: asyncConfig(), active: true}}
	metrics := NewAtomicMetrics()
	runnerStarts, runnerStops := 0, 0
	svc := &PromptService{
		config:       store,
		runner:       &Runner{},
		enqueuer:     NewEnqueuer(store, &fakeJobRepository{}, &fakePayloadStore{}, metrics),
		metrics:      metrics,
		enqueueSlots: make(chan struct{}, 1),
		probes:       map[string]ProbeResult{},
		runnerStart: func(context.Context) error {
			runnerStarts++
			return nil
		},
		runnerShutdown: func(context.Context) error {
			runnerStops++
			return nil
		},
	}

	require.NoError(t, svc.Start(context.Background(), PromptStartMode{ConsumeSharedQueue: false}))
	configStarts, configStops := store.calls()
	require.Equal(t, 1, configStarts)
	require.Zero(t, configStops)
	require.Zero(t, runnerStarts)
	require.NotNil(t, svc.background, "API role needs a retained background context for asynchronous enqueue")

	require.NoError(t, svc.Enqueue(context.Background(), Request{
		RequestID: "api-role-enqueue",
		Protocol:  "openai_chat_completions",
		Body:      []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	}))
	require.Eventually(t, func() bool {
		return metrics.AuditSnapshot().Enqueued == 1
	}, time.Second, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, svc.Shutdown(ctx))
	configStarts, configStops = store.calls()
	require.Equal(t, 1, configStarts)
	require.Equal(t, 1, configStops)
	require.Zero(t, runnerStops, "Shutdown must not stop a runner that was never started")
}

func TestPromptServiceWorkerRoleStartsAndStopsSharedRunner(t *testing.T) {
	store := &lifecycleConfigStore{}
	runnerStarts, runnerStops := 0, 0
	svc := &PromptService{
		config:       store,
		runner:       &Runner{},
		enqueueSlots: make(chan struct{}, 1),
		probes:       map[string]ProbeResult{},
		runnerStart: func(context.Context) error {
			runnerStarts++
			return nil
		},
		runnerShutdown: func(context.Context) error {
			runnerStops++
			return nil
		},
	}

	require.NoError(t, svc.Start(context.Background(), PromptStartMode{ConsumeSharedQueue: true}))
	require.Equal(t, 1, runnerStarts)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, svc.Shutdown(ctx))
	configStarts, configStops := store.calls()
	require.Equal(t, 1, configStarts)
	require.Equal(t, 1, configStops)
	require.Equal(t, 1, runnerStops)
}
