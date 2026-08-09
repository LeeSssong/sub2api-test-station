package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountMonitorRuntimeCooldownActionAndListContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &service.OpenAIGatewayService{}
	h := NewAccountMonitorHandler(nil, nil, nil, nil)
	h.SetOpenAIGatewayService(gateway)

	post := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-monitors/runtime/cooldown", strings.NewReader(`{"account_id":81,"canonical_scheduling_model":"gpt-5.5","cooldown_seconds":30}`))
	post.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	postCtx, _ := gin.CreateTestContext(postRec)
	postCtx.Request = post
	h.ImmediatelyCooldownAccountModel(postCtx)
	require.Equal(t, http.StatusOK, postRec.Code, postRec.Body.String())

	list := httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-monitors/runtime", nil)
	listRec := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(listRec)
	listCtx.Request = list
	h.ListAccountModelRuntime(listCtx)
	require.Equal(t, http.StatusOK, listRec.Code, listRec.Body.String())
	for _, field := range []string{"account_id", "canonical_scheduling_model", "state", "failure_streak", "last_failure_at", "cooldown_until", "half_open_in_flight", "last_status_code", "last_error_type", "output_started", "sticky_reference_count"} {
		require.Contains(t, listRec.Body.String(), `"`+field+`"`)
	}
}

func TestAccountMonitorRuntimeActionsRejectInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAccountMonitorHandler(nil, nil, nil, nil)
	h.SetOpenAIGatewayService(&service.OpenAIGatewayService{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-monitors/runtime/restore", strings.NewReader(`{"account_id":0,"canonical_scheduling_model":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.RestoreAccountModelScheduling(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccountMonitorRuntimeSoftFailureKeepsAllDTOKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &service.OpenAIGatewayService{}
	gateway.RecordOpenAIAccountModelFailure(nil, service.OpenAIAccountModelFailureEvent{AccountID: 82, CanonicalModel: "gpt-5.5", StatusCode: 502, ErrorType: "transient_upstream"})
	h := NewAccountMonitorHandler(nil, nil, nil, nil)
	h.SetOpenAIGatewayService(gateway)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-monitors/runtime", nil)
	h.ListAccountModelRuntime(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"cooldown_until":null`)
	require.Contains(t, recorder.Body.String(), `"last_error_type":"transient_upstream"`)
}
