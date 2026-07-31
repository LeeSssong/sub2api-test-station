package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type monitorV2RouteSnapshotter struct{}

type monitorV2RouteSettingRepo struct {
	values map[string]string
}

func (r *monitorV2RouteSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (r *monitorV2RouteSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *monitorV2RouteSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *monitorV2RouteSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *monitorV2RouteSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *monitorV2RouteSettingRepo) GetAll(context.Context) (map[string]string, error) {
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *monitorV2RouteSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func (monitorV2RouteSnapshotter) Snapshot(
	context.Context,
	service.MonitorV2Window,
	time.Time,
	...service.MonitorV2Scope,
) (*service.MonitorV2Snapshot, error) {
	return &service.MonitorV2Snapshot{
		ContractVersion: service.MonitorV2ContractVersion,
		Window:          service.MonitorV2Window7D,
		RefreshIntervalSeconds: 300,
		GeneratedAt:     time.Now().UTC(),
		Groups:          []service.MonitorV2Group{},
	}, nil
}

func TestMonitorV2RouteUsesHeavyQueryLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	settingService := service.NewSettingService(&monitorV2RouteSettingRepo{values: map[string]string{
		service.SettingKeyPanelRateLimitSettings: `{"enabled":true,"user_rpm":100,"heavy_rpm":1,"exempt_admin":false,"public_ip_rpm":0}`,
	}}, &config.Config{})
	panelLimiter := middleware.NewPanelRateLimiter(redisClient, settingService)

	engine := gin.New()
	v1 := engine.Group("/api/v1")
	jwt := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	RegisterUserRoutes(v1, &handler.Handlers{
		MonitorV2: handler.NewMonitorV2Handler(monitorV2RouteSnapshotter{}),
	}, jwt, audit, settingService, panelLimiter)

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil))
	second := httptest.NewRecorder()
	engine.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil))

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}

func TestMonitorV2RouteUsesAuthenticatedUserBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	authCalled := false
	jwt := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		authCalled = true
		c.Next()
	})
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	RegisterUserRoutes(v1, &handler.Handlers{
		MonitorV2: handler.NewMonitorV2Handler(monitorV2RouteSnapshotter{}),
	}, jwt, audit, nil, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/monitor-v2?window=7d", nil)
	engine.ServeHTTP(recorder, request)

	require.True(t, authCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"contract_version":"`+service.MonitorV2ContractVersion+`"`)
	require.Contains(t, recorder.Body.String(), `"refresh_interval_seconds":300`)
}
