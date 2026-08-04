package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMonitorRouteRepoStub struct {
	service.AccountMonitorRepository
}

func (*accountMonitorRouteRepoStub) LoadSettings(context.Context) (service.AccountMonitorSettings, error) {
	return service.AccountMonitorSettings{IntervalSeconds: 300}, nil
}

func (*accountMonitorRouteRepoStub) ListWindowAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorWindowAggregate, error) {
	return nil, nil
}

func (*accountMonitorRouteRepoStub) ListAggregates(context.Context, []int64, time.Time, time.Time) (map[int64]service.AccountMonitorAggregate, error) {
	return nil, nil
}

func (*accountMonitorRouteRepoStub) ListLatest(context.Context, []int64) (map[int64]service.AccountMonitorLatest, error) {
	return nil, nil
}

func (*accountMonitorRouteRepoStub) ListGroups(context.Context) ([]service.AccountMonitorGroup, error) {
	return nil, nil
}

type accountMonitorRouteAccountRepoStub struct{}

func (*accountMonitorRouteAccountRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]service.Account, error) {
	return nil, nil
}

func (*accountMonitorRouteAccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	accounts := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		accounts = append(accounts, &service.Account{ID: id, Concurrency: 10})
	}
	return accounts, nil
}

type accountMonitorRouteConcurrencyCacheStub struct {
	service.ConcurrencyCache
}

func (*accountMonitorRouteConcurrencyCacheStub) GetAccountConcurrencyBatch(_ context.Context, ids []int64) (map[int64]int, error) {
	current := make(map[int64]int, len(ids))
	for _, id := range ids {
		current[id] = int(id)
	}
	return current, nil
}

func TestAccountMonitorRoutesRegisterWindowEndpointAndKeepLegacyEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &accountMonitorRouteRepoStub{}
	monitorService := service.NewAccountMonitorService(repo, &accountMonitorRouteAccountRepoStub{}, nil, nil, nil)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		AccountMonitor: adminhandler.NewAccountMonitorHandler(monitorService, nil, nil, nil),
	}}
	router := gin.New()
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer admin" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
		}
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	for _, tt := range []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
	}{
		{name: "new endpoint defaults to 24h", path: "/api/v1/admin/accounts/monitor", wantStatus: http.StatusOK, wantContains: `"range":"24h"`},
		{name: "new endpoint accepts selected range", path: "/api/v1/admin/accounts/monitor?range=7d", wantStatus: http.StatusOK, wantContains: `"range":"7d"`},
		{name: "new endpoint rejects invalid range in handler", path: "/api/v1/admin/accounts/monitor?range=48h", wantStatus: http.StatusBadRequest, wantContains: "INVALID_ACCOUNT_MONITOR_RANGE"},
		{name: "legacy endpoint remains registered", path: "/api/v1/admin/account-monitors", wantStatus: http.StatusOK, wantContains: `"range":"24h"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("Authorization", "Bearer admin")
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if tt.wantContains != "" && !strings.Contains(recorder.Body.String(), tt.wantContains) {
				t.Fatalf("body = %s, want %s", recorder.Body.String(), tt.wantContains)
			}
		})
	}
}

func TestAccountMonitorConcurrencyRouteRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountRepo := &accountMonitorRouteAccountRepoStub{}
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		AccountMonitor: adminhandler.NewAccountMonitorHandler(
			nil,
			nil,
			accountRepo,
			service.NewConcurrencyService(&accountMonitorRouteConcurrencyCacheStub{}),
		),
	}}
	router := gin.New()
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer admin" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
		}
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	for _, tt := range []struct {
		name       string
		authorized bool
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "authenticated admin", authorized: true, wantStatus: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/monitor/concurrency", strings.NewReader(`{"account_ids":[1]}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.authorized {
				req.Header.Set("Authorization", "Bearer admin")
			}
			router.ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
			}
			if tt.authorized && !strings.Contains(res.Body.String(), `"items":[{"account_id":1,"current":1,"limit":10}]`) {
				t.Fatalf("body = %s", res.Body.String())
			}
		})
	}
}
