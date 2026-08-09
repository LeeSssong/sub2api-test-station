package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountExternalCommandV1EnforcesIdempotencyKeyAndPayload(t *testing.T) {
	previous := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	gin.SetMode(gin.TestMode)
	stub := newStubAdminService()
	stub.updateAccountResult = &service.Account{ID: 7}
	handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/accounts/:id/external-command/v1", handler.UpdateExternalCommandV1)
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/accounts/7/external-command/v1", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "relay-command-1")
		router.ServeHTTP(rec, req)
		return rec
	}
	require.Equal(t, http.StatusOK, call(`{"command_id":"cmd-1","fields":{"priority":2}}`).Code)
	replay := call(`{"command_id":"cmd-1","fields":{"priority":2}}`)
	require.Equal(t, http.StatusOK, replay.Code)
	require.Equal(t, "true", replay.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, stub.updateAccountCalls)
	require.NotEqual(t, http.StatusOK, call(`{"command_id":"cmd-1","fields":{"priority":3}}`).Code)
	require.NotEqual(t, http.StatusOK, call(`{"command_id":"cmd-2","fields":{"priority":2}}`).Code)
}
