//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// This repository records the observable history/checked side effects while
// the real service, HTTP checker and runner execute against an HTTP fixture.
type upstreamMonitorRuntimeRepo struct {
	ChannelMonitorRepository
	mu      sync.Mutex
	monitor *ChannelMonitor
	history []*ChannelMonitorHistoryRow
	checked int
}

func (r *upstreamMonitorRuntimeRepo) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	copy := *r.monitor
	return &copy, nil
}

func (r *upstreamMonitorRuntimeRepo) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	copy := *r.monitor
	return []*ChannelMonitor{&copy}, nil
}

func (r *upstreamMonitorRuntimeRepo) InsertHistoryBatch(_ context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, rows...)
	return nil
}

func (r *upstreamMonitorRuntimeRepo) MarkChecked(context.Context, int64, time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checked++
	return nil
}

func (r *upstreamMonitorRuntimeRepo) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.history), r.checked
}

func upstreamRuntimeFixture(t *testing.T, fail bool) (*ChannelMonitorService, *upstreamMonitorRuntimeRepo, *atomic.Int64) {
	t.Helper()
	swapMonitorHTTPClient(t)
	requests := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		requests.Add(1)
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		answer := answerFromChallengePrompt(body.Messages[len(body.Messages)-1].Content)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": answer}}}})
	}))
	t.Cleanup(server.Close)
	repo := &upstreamMonitorRuntimeRepo{monitor: &ChannelMonitor{
		ID: 17, Name: "upstream-runtime", Enabled: true, IntervalSeconds: 1,
		Provider: MonitorProviderOpenAI, APIMode: MonitorAPIModeChatCompletions,
		Endpoint: server.URL, APIKey: "NEW:test-key", PrimaryModel: "test-primary",
		ExtraModels: []string{"test-extra"},
	}}
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	return svc, repo, requests
}

func TestUpstreamMonitorRuntime_ManualCheckPersistsEveryModel(t *testing.T) {
	svc, repo, requests := upstreamRuntimeFixture(t, false)
	results, err := svc.RunCheck(context.Background(), 17)
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, result := range results {
		require.Equal(t, MonitorStatusOperational, result.Status)
	}
	rows, checked := repo.counts()
	require.Equal(t, 2, rows)
	require.Equal(t, 1, checked)
	require.EqualValues(t, 2, requests.Load())
}

func TestUpstreamMonitorRuntime_FailedModelIsRecordedWithoutCustomRetries(t *testing.T) {
	svc, repo, requests := upstreamRuntimeFixture(t, true)
	results, err := svc.RunCheck(context.Background(), 17)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.EqualValues(t, 2, requests.Load(), "upstream sends one request per configured model")
	rows, checked := repo.counts()
	require.Equal(t, 2, rows)
	require.Equal(t, 1, checked)
	for _, result := range results {
		require.NotEqual(t, MonitorStatusOperational, result.Status)
	}
}

func TestUpstreamMonitorRuntime_PeriodicChecksPersistWithoutGroupAssociation(t *testing.T) {
	svc, repo, requests := upstreamRuntimeFixture(t, false)
	runner := NewChannelMonitorRunner(svc, nil)
	defer runner.Stop()
	runner.Start()
	require.Eventually(t, func() bool {
		rows, checked := repo.counts()
		return rows >= 4 && checked >= 2
	}, 3*time.Second, 10*time.Millisecond)
	require.GreaterOrEqual(t, requests.Load(), int64(4))
}

func TestUpstreamMonitorRuntime_ProviderStartsOnlyBackgroundRole(t *testing.T) {
	for _, role := range []config.ProcessRole{config.ProcessRoleAPI, config.ProcessRoleWorker} {
		t.Run(string(role), func(t *testing.T) {
			repo := &upstreamMonitorRuntimeRepo{monitor: &ChannelMonitor{ID: 17, IntervalSeconds: 60}}
			svc := NewChannelMonitorService(repo, nil)
			runner := ProvideChannelMonitorRunner(svc, nil, nil, &config.Config{Server: config.ServerConfig{ProcessRole: role}})
			defer runner.Stop()
			require.Equal(t, role == config.ProcessRoleWorker, runner.started)
		})
	}
}

func TestUpstreamMonitorRuntime_WorkerReconcilesDurableCRUDWithoutResettingCheckedTimers(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 8)}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()

	monitor := &ChannelMonitor{ID: 21, Enabled: true, IntervalSeconds: 60, APIKey: "test-key"}
	svc.enabled = []*ChannelMonitor{monitor}
	runner.reconcileEnabledMonitors()
	select {
	case <-svc.runCalled:
	case <-time.After(time.Second):
		t.Fatal("worker did not discover monitor created by API")
	}
	original := runnerTaskPtr(runner, 21)
	checked := time.Now()
	monitor.LastCheckedAt = &checked
	monitor.UpdatedAt = checked
	runner.reconcileEnabledMonitors()
	require.Same(t, original, runnerTaskPtr(runner, 21), "recording a check must preserve the original interval")

	svc.listErr = errors.New("temporary database failure")
	runner.reconcileEnabledMonitors()
	require.Same(t, original, runnerTaskPtr(runner, 21))
	svc.listErr = nil

	monitor.IntervalSeconds = 120
	runner.reconcileEnabledMonitors()
	require.NotSame(t, original, runnerTaskPtr(runner, 21), "API edits must reach the worker")
	monitor.Enabled = false
	runner.reconcileEnabledMonitors()
	require.Zero(t, runnerTaskCount(runner))
	monitor.Enabled = true
	runner.reconcileEnabledMonitors()
	require.Equal(t, 1, runnerTaskCount(runner))
	svc.enabled = nil
	runner.reconcileEnabledMonitors()
	require.Zero(t, runnerTaskCount(runner), "API deletion must cancel the timer")
}

func TestUpstreamMonitorRuntime_UserViewKeepsLatestQuota(t *testing.T) {
	quota := &domain.MonitorQuotaSnapshot{Success: true, Source: "cn_quota"}
	view := buildUserViewFromSummary(&ChannelMonitor{ID: 21}, MonitorStatusSummary{}, &ChannelMonitorLatest{Quota: quota}, nil)
	require.Equal(t, quota, view.LatestQuota)
}
