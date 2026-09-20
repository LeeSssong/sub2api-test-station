package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type fakeDatabasePinger struct {
	err   error
	calls int
}

func (f *fakeDatabasePinger) PingContext(context.Context) error {
	f.calls++
	return f.err
}

type fakeRedisPinger struct {
	err   error
	calls int
}

func (f *fakeRedisPinger) Ping(context.Context) *redis.StatusCmd {
	f.calls++
	return redis.NewStatusResult("PONG", f.err)
}

func TestDependencyReadinessCheckerFailsClosedForNilProductionDependencies(t *testing.T) {
	require.NotPanics(t, func() {
		err := NewDependencyReadinessChecker(nil, nil).Check(context.Background())
		require.Error(t, err)
	})
}

func TestDependencyReadinessChecker(t *testing.T) {
	t.Run("ready when database and redis respond", func(t *testing.T) {
		database := &fakeDatabasePinger{}
		redisClient := &fakeRedisPinger{}

		err := newDependencyReadinessChecker(database, redisClient).Check(context.Background())

		require.NoError(t, err)
		require.Equal(t, 1, database.calls)
		require.Equal(t, 1, redisClient.calls)
	})

	t.Run("database failure short circuits redis", func(t *testing.T) {
		databaseErr := errors.New("database unavailable")
		database := &fakeDatabasePinger{err: databaseErr}
		redisClient := &fakeRedisPinger{}

		err := newDependencyReadinessChecker(database, redisClient).Check(context.Background())

		require.ErrorIs(t, err, databaseErr)
		require.Equal(t, 1, database.calls)
		require.Zero(t, redisClient.calls)
	})

	t.Run("redis failure reports not ready", func(t *testing.T) {
		redisErr := errors.New("redis unavailable")
		database := &fakeDatabasePinger{}
		redisClient := &fakeRedisPinger{err: redisErr}

		err := newDependencyReadinessChecker(database, redisClient).Check(context.Background())

		require.ErrorIs(t, err, redisErr)
		require.Equal(t, 1, database.calls)
		require.Equal(t, 1, redisClient.calls)
	})
}

type fakeReadinessChecker struct {
	check func(context.Context) error
}

func (f fakeReadinessChecker) Check(ctx context.Context) error {
	return f.check(ctx)
}

func TestCommonRoutesHealthAndReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("health remains liveness", func(t *testing.T) {
		router := gin.New()
		RegisterCommonRoutes(router, fakeReadinessChecker{check: func(context.Context) error {
			return errors.New("dependency unavailable")
		}})

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Header().Get("Content-Type"), "application/json")
		require.JSONEq(t, `{"status":"ok"}`, response.Body.String())
	})

	t.Run("ready dependencies return ready", func(t *testing.T) {
		router := gin.New()
		RegisterCommonRoutes(router, fakeReadinessChecker{check: func(context.Context) error { return nil }})

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Header().Get("Content-Type"), "application/json")
		require.JSONEq(t, `{"status":"ready"}`, response.Body.String())
	})

	t.Run("dependency error returns sanitized not ready", func(t *testing.T) {
		router := gin.New()
		RegisterCommonRoutes(router, fakeReadinessChecker{check: func(context.Context) error {
			return errors.New("postgresql://secret@example.invalid")
		}})

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.Contains(t, response.Header().Get("Content-Type"), "application/json")
		require.JSONEq(t, `{"status":"not_ready"}`, response.Body.String())
		require.NotContains(t, response.Body.String(), "postgresql")
		require.NotContains(t, response.Body.String(), "secret")
	})

	t.Run("nil checker fails closed", func(t *testing.T) {
		router := gin.New()
		RegisterCommonRoutes(router, nil)

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.JSONEq(t, `{"status":"not_ready"}`, response.Body.String())
	})

	t.Run("post is not a successful readiness response", func(t *testing.T) {
		router := gin.New()
		RegisterCommonRoutes(router, fakeReadinessChecker{check: func(context.Context) error { return nil }})

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/readyz", nil))

		require.NotEqual(t, http.StatusOK, response.Code)
	})
}

func TestReadinessHandlerTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", readinessHandler(fakeReadinessChecker{check: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}, 10*time.Millisecond))

	started := time.Now()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"status":"not_ready"}`, response.Body.String())
	require.Less(t, time.Since(started), 500*time.Millisecond)
}
