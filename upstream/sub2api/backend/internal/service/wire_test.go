package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/collection"
)

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
