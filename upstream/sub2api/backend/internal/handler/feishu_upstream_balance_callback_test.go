package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type feishuSilenceRepoStub struct{ calls int }

func (s *feishuSilenceRepoStub) SilenceByActionToken(context.Context, string, time.Time, time.Time) (bool, error) {
	s.calls++
	return true, nil
}

func TestFeishuUpstreamBalanceCallbackAuthorizesRecipientAndSilences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &feishuSilenceRepoStub{}
	svc := service.NewUpstreamBalanceNotificationService(nil, nil, nil, nil, []string{"ou_allowed"})
	svc.ConfigureFeishuSilence(repo, "callback-secret")
	h := NewFeishuUpstreamBalanceCallbackHandler(svc)
	r := gin.New()
	r.POST("/callback", h.Handle)
	body := `{"event":{"operator":{"open_id":"ou_allowed"},"action":{"value":{"duration":"6h","token":"0123456789012345678901234567890123456789"}}}}`
	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
	req.Header.Set("X-Feishu-Callback-Token", "callback-secret")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK || repo.calls != 1 {
		t.Fatalf("status=%d body=%s calls=%d", res.Code, res.Body.String(), repo.calls)
	}
}

func TestFeishuUpstreamBalanceCallbackRejectsUnconfiguredOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &feishuSilenceRepoStub{}
	svc := service.NewUpstreamBalanceNotificationService(nil, nil, nil, nil, []string{"ou_allowed"})
	svc.ConfigureFeishuSilence(repo, "callback-secret")
	h := NewFeishuUpstreamBalanceCallbackHandler(svc)
	r := gin.New()
	r.POST("/callback", h.Handle)
	body := `{"event":{"operator":{"open_id":"ou_other"},"action":{"value":{"duration":"24h","token":"0123456789012345678901234567890123456789"}}}}`
	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
	req.Header.Set("X-Feishu-Callback-Token", "callback-secret")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || repo.calls != 0 {
		t.Fatalf("status=%d body=%s calls=%d", res.Code, res.Body.String(), repo.calls)
	}
}
