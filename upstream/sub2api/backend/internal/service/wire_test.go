package service

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/collection"
)

type lifecyclePricingRemoteClient struct {
	pricingCalls atomic.Int64
	hashCalls    atomic.Int64
	pricing      []byte
}

func (c *lifecyclePricingRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	c.pricingCalls.Add(1)
	if c.pricing == nil {
		return nil, errors.New("unexpected pricing fetch")
	}
	return c.pricing, nil
}

func (c *lifecyclePricingRemoteClient) FetchHashText(context.Context, string) (string, error) {
	c.hashCalls.Add(1)
	return "", errors.New("unexpected hash fetch")
}

func TestProvidePricingService_APIRoleLoadsReadOnlyWithoutRemoteOrDirectoryWrites(t *testing.T) {
	dataDir := t.TempDir() + "/missing-pricing"
	remote := &lifecyclePricingRemoteClient{}
	cfg := &config.Config{
		Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI},
		Pricing: config.PricingConfig{
			DataDir:      dataDir,
			RemoteURL:    "https://pricing.invalid/catalog.json",
			HashURL:      "https://pricing.invalid/catalog.sha256",
			FallbackFile: "",
		},
	}
	pricing, err := ProvidePricingService(cfg, remote)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	t.Cleanup(pricing.Stop)
	require.Equal(t, int64(0), remote.pricingCalls.Load())
	require.Equal(t, int64(0), remote.hashCalls.Load())
	_, statErr := os.Stat(dataDir)
	require.ErrorIs(t, statErr, os.ErrNotExist, "API pricing initialization must not create its data directory")
}

func TestProvidePricingService_SingletonRolesRefreshAndAPIRoleUsesLocalData(t *testing.T) {
	const remotePricing = `{"remote-model":{"input_cost_per_token":0.1}}`
	const localPricing = `{"local-model":{"input_cost_per_token":0.2}}`

	for _, tt := range []struct {
		name       string
		role       config.ProcessRole
		wantRemote bool
	}{
		{name: "all refreshes shared catalog", role: config.ProcessRoleAll, wantRemote: true},
		{name: "api loads the shared catalog without refreshing", role: config.ProcessRoleAPI},
		{name: "worker refreshes shared catalog", role: config.ProcessRoleWorker, wantRemote: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			if !tt.wantRemote {
				require.NoError(t, os.WriteFile(dataDir+"/model_pricing.json", []byte(localPricing), 0600))
			}
			remote := &lifecyclePricingRemoteClient{pricing: []byte(remotePricing)}
			cfg := &config.Config{
				Server: config.ServerConfig{ProcessRole: tt.role},
				Pricing: config.PricingConfig{
					DataDir:   dataDir,
					RemoteURL: "https://pricing.invalid/catalog.json",
				},
			}

			pricing, err := ProvidePricingService(cfg, remote)
			require.NoError(t, err)
			t.Cleanup(pricing.Stop)
			if tt.wantRemote {
				require.Equal(t, int64(1), remote.pricingCalls.Load())
				require.NotNil(t, pricing.GetModelPricing("remote-model"))
				return
			}
			require.Equal(t, int64(0), remote.pricingCalls.Load())
			require.NotNil(t, pricing.GetModelPricing("local-model"))
		})
	}
}

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService(&config.Config{Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI}})
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService(&config.Config{Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI}})
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}

func TestProvideUpstreamBillingProbeService_APIRoleDoesNotStartSingleton(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI}}
	svc := ProvideUpstreamBillingProbeService(nil, nil, nil, nil, nil, cfg)
	t.Cleanup(svc.Stop)

	svc.mu.Lock()
	started := svc.started
	svc.mu.Unlock()
	require.False(t, started, "API role must not start the shared billing-probe runner")
}

func TestProvideUsageRecordWorkerPool_APIRoleStartsRequestLocalWorkers(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{ProcessRole: config.ProcessRoleAPI}}
	svc := ProvideUsageRecordWorkerPool(cfg)
	t.Cleanup(svc.Stop)

	executed := make(chan struct{})
	mode := svc.Submit(func(context.Context) {
		close(executed)
	})
	require.Equal(t, UsageRecordSubmitModeEnqueued, mode)

	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("API role request-local usage worker did not execute submitted work")
	}
}

func TestProvideContentModerationService_RolesSplitRequestWorkersFromCleanup(t *testing.T) {
	originalDelay := contentModerationCleanupDelay
	originalInterval := contentModerationCleanupInterval
	contentModerationCleanupDelay = time.Millisecond
	contentModerationCleanupInterval = time.Hour
	t.Cleanup(func() {
		contentModerationCleanupDelay = originalDelay
		contentModerationCleanupInterval = originalInterval
	})

	tests := []struct {
		name               string
		role               config.ProcessRole
		wantRequestWorkers bool
		wantCleanupWorker  bool
	}{
		{name: "all starts both work classes", role: config.ProcessRoleAll, wantRequestWorkers: true, wantCleanupWorker: true},
		{name: "api starts only request workers", role: config.ProcessRoleAPI, wantRequestWorkers: true, wantCleanupWorker: false},
		{name: "worker starts only cleanup", role: config.ProcessRoleWorker, wantRequestWorkers: false, wantCleanupWorker: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Server: config.ServerConfig{ProcessRole: tt.role}}
			repo := &lifecycleContentModerationRepo{}
			svc := ProvideContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{}}, repo, nil, nil, nil, nil, nil, cfg)
			t.Cleanup(svc.Stop)
			svc.enqueueRecord(ContentModerationCheckInput{}, nil, &ContentModerationLog{Action: ContentModerationActionAllow}, "", false, false)

			if tt.wantRequestWorkers {
				require.Eventually(t, func() bool { return svc.asyncProcessed.Load() == 1 }, time.Second, 10*time.Millisecond)
			} else {
				require.Never(t, func() bool { return svc.asyncProcessed.Load() != 0 }, 100*time.Millisecond, 10*time.Millisecond)
			}
			if tt.wantCleanupWorker {
				require.Eventually(t, func() bool { return repo.cleanupCalls.Load() == 1 }, time.Second, 10*time.Millisecond)
			} else {
				require.Never(t, func() bool { return repo.cleanupCalls.Load() != 0 }, 100*time.Millisecond, 10*time.Millisecond)
			}
		})
	}
}

func TestContentModerationService_StopWaitsForCleanupWorker(t *testing.T) {
	repo := &blockingLifecycleContentModerationRepo{
		cleanupStarted: make(chan struct{}),
		releaseCleanup: make(chan struct{}),
	}
	svc := &ContentModerationService{
		settingRepo: &contentModerationTestSettingRepo{values: map[string]string{}},
		repo:        repo,
	}
	svc.startCleanupWorker(contentModerationCleanupSchedule{
		delay:    time.Millisecond,
		interval: time.Hour,
		timeout:  time.Hour,
	})
	released := false
	defer func() {
		if !released {
			close(repo.releaseCleanup)
		}
		svc.Stop()
	}()

	select {
	case <-repo.cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup worker did not start")
	}

	stopped := make(chan struct{})
	go func() {
		svc.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the active cleanup worker exited")
	case <-time.After(20 * time.Millisecond):
	}

	close(repo.releaseCleanup)
	released = true
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after the cleanup worker exited")
	}
	svc.Stop()
}

type lifecycleContentModerationRepo struct {
	contentModerationTestRepo
	cleanupCalls atomic.Int64
}

func (r *lifecycleContentModerationRepo) CleanupExpiredLogs(context.Context, time.Time, time.Time) (*ContentModerationCleanupResult, error) {
	r.cleanupCalls.Add(1)
	return &ContentModerationCleanupResult{}, nil
}

type blockingLifecycleContentModerationRepo struct {
	contentModerationTestRepo
	cleanupStarted chan struct{}
	releaseCleanup chan struct{}
}

func (r *blockingLifecycleContentModerationRepo) CleanupExpiredLogs(context.Context, time.Time, time.Time) (*ContentModerationCleanupResult, error) {
	close(r.cleanupStarted)
	<-r.releaseCleanup
	return &ContentModerationCleanupResult{}, nil
}
