package notify

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderFeishuIncludesFiveOperatorSections(t *testing.T) {
	t.Parallel()
	message := RenderFeishu(IncidentView{
		Title: "GPT-Pro 倍率变化", WhatWasDone: []string{"读取价格页 1 次"}, Results: []string{"当前倍率 0.10x"},
		Change: "相对 0.07x 上升 42.9%", Focus: "核对利润但不自动改价", Links: []Link{{Label: "事件详情", URL: "https://ops.example/incidents/1"}},
	})
	text := message.Content.Text
	for _, section := range []string{"做了什么", "得到什么", "发生什么变化", "需要关注什么", "链接"} {
		if !strings.Contains(text, section) {
			t.Fatalf("missing section %q in %s", section, text)
		}
	}
}

func TestRenderSessionExpiredUsesExactLoginLink(t *testing.T) {
	t.Parallel()
	message := RenderSessionExpired("wawazz", "https://wawazz.example/login")
	if !strings.Contains(message.Content.Text, "上游用量读取会话失效") || !strings.Contains(message.Content.Text, "质量和公开价格监控正常") || !strings.Contains(message.Content.Text, "https://wawazz.example/login") {
		t.Fatalf("message=%s", message.Content.Text)
	}
}

func TestFeishuClientReadsWebhookSecretWithoutLeakingFailures(t *testing.T) {
	t.Parallel()
	secret := filepath.Join(t.TempDir(), "webhook")
	if err := os.WriteFile(secret, []byte("https://open.feishu.example/hook/secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := Client{WebhookFile: secret, Resolver: notifyResolver{}, HTTP: &http.Client{Transport: notifyTransport(func(request *http.Request) *http.Response {
		if strings.Contains(request.URL.String(), "secret-value") {
			return notifyResponse(http.StatusInternalServerError, "secret body")
		}
		return notifyResponse(http.StatusNoContent, "")
	})}}
	err := client.Send(context.Background(), RenderFeishu(IncidentView{Title: "test"}))
	if err == nil || strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "secret body") {
		t.Fatalf("error=%v", err)
	}
}

type notifyResolver struct{}

func (notifyResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.40")}}, nil
}

type notifyTransport func(*http.Request) *http.Response

func (fn notifyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request), nil
}
func notifyResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
