package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountModelRuntimeAuditRepo struct {
	mu   sync.Mutex
	logs []*service.AuditLog
}

func (r *accountModelRuntimeAuditRepo) BatchInsert(_ context.Context, logs []*service.AuditLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}
func (*accountModelRuntimeAuditRepo) Insert(context.Context, *service.AuditLog) error { return nil }
func (*accountModelRuntimeAuditRepo) List(context.Context, *service.AuditLogFilter) (*service.AuditLogList, error) {
	return &service.AuditLogList{}, nil
}
func (*accountModelRuntimeAuditRepo) GetByID(context.Context, int64) (*service.AuditLog, error) {
	return nil, service.ErrAuditLogNotFound
}
func (*accountModelRuntimeAuditRepo) Count(context.Context) (int64, error) { return 0, nil }
func (*accountModelRuntimeAuditRepo) TruncateAll(context.Context) error    { return nil }
func (*accountModelRuntimeAuditRepo) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestAccountMonitorRuntimeRouteRequiresAdminAndAuditsSensitiveRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &service.OpenAIGatewayService{}
	gateway.RecordOpenAIAccountModelFailure(context.Background(), service.OpenAIAccountModelFailureEvent{
		AccountID: 81, CanonicalModel: "gpt-5.5", StatusCode: 502, ErrorType: "transient_upstream",
	})
	runtimeHandler := adminhandler.NewAccountMonitorHandler(nil, nil, nil, nil)
	runtimeHandler.SetOpenAIGatewayService(gateway)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{AccountMonitor: runtimeHandler}}

	repository := &accountModelRuntimeAuditRepo{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()
	defer auditService.Stop()

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
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, servermiddleware.NewAuditLogMiddleware(auditService), stepUp, nil, nil)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-monitors/runtime", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-monitors/runtime", nil)
	request.Header.Set("Authorization", "Bearer admin")
	router.ServeHTTP(authorized, request)
	require.Equal(t, http.StatusOK, authorized.Code)
	require.Contains(t, authorized.Body.String(), `"sticky_reference_count"`)

	auditService.Stop()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.Len(t, repository.logs, 1)
	require.Equal(t, "admin.account_model_runtime.read", repository.logs[0].Action)
}
