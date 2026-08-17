//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAISharedHealthIntegrationConcurrentHalfOpenHasOneWinner(t *testing.T) {
	rdb := testRedis(t)
	store := NewOpenAISharedHealthStore(rdb)
	key, err := service.NewOpenAISharedHealthKey(153, "gpt-5.6-sol")
	require.NoError(t, err)

	const contenders = 16
	var winners atomic.Int32
	start := make(chan struct{})
	errs := make(chan error, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, acquireErr := store.AcquireHalfOpen(context.Background(), key, t.Name(), 15*time.Second)
			errs <- acquireErr
			if ok {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for acquireErr := range errs {
		require.NoError(t, acquireErr)
	}
	require.Equal(t, int32(1), winners.Load())
}
