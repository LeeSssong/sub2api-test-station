package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const maxFeishuResponseBytes = 1 << 20

type Alert struct {
	Severity string
	Code     string
	Message  string
	At       time.Time
}

type AppBotAlerter struct {
	baseURL   string
	appID     string
	appSecret string
	chatID    string
	http      *http.Client
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewAppBotAlerter(baseURL, appIDFile, appSecretFile, chatIDFile string) (*AppBotAlerter, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !isLoopbackHTTP(parsed)) {
		return nil, errors.New("invalid Feishu OpenAPI base URL")
	}
	appID, err := readSecret(appIDFile)
	if err != nil {
		return nil, errors.New("Feishu App ID is unavailable")
	}
	appSecret, err := readSecret(appSecretFile)
	if err != nil {
		return nil, errors.New("Feishu App Secret is unavailable")
	}
	chatID, err := readSecret(chatIDFile)
	if err != nil {
		return nil, errors.New("Feishu alert chat is unavailable")
	}
	return &AppBotAlerter{baseURL: baseURL, appID: appID, appSecret: appSecret, chatID: chatID, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (a *AppBotAlerter) Send(ctx context.Context, alert Alert) error {
	code := "d04_operational_alert"
	if alert.Code == "scheduler_tick_failed" {
		code = alert.Code
	}
	severity := "高"
	if alert.Severity != "high" {
		severity = "中"
	}
	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{"template": "red", "title": map[string]string{"tag": "plain_text", "content": "首发计划后台异常"}},
		"elements": []any{
			map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": fmt.Sprintf("**错误码**：%s\n**级别**：%s\n**时间**：%s", code, severity, alert.At.UTC().Format(time.RFC3339))}},
			map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": "**处理建议**：检查 D04 健康、调度状态与只读原因；不要在群消息中发送凭据。"}},
		},
	}
	content, err := json.Marshal(card)
	if err != nil {
		return errors.New("encode Feishu alert card")
	}
	token, err := a.tenantToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{"receive_id": a.chatID, "msg_type": "interactive", "content": string(content)}
	var response struct {
		Code int `json:"code"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := a.postJSON(ctx, "/open-apis/im/v1/messages?receive_id_type=chat_id", token, body, &response); err != nil {
		return err
	}
	if response.Code != 0 || response.Data.MessageID == "" {
		return errors.New("Feishu message response schema mismatch")
	}
	return nil
}

func (a *AppBotAlerter) tenantToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Now().Before(a.expiresAt) {
		return a.token, nil
	}
	var response struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := a.postJSON(ctx, "/open-apis/auth/v3/tenant_access_token/internal", "", map[string]string{"app_id": a.appID, "app_secret": a.appSecret}, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.TenantAccessToken == "" || response.Expire <= 0 {
		return "", errors.New("Feishu token response schema mismatch")
	}
	cacheSeconds := response.Expire - 60
	if cacheSeconds < 1 {
		cacheSeconds = 1
	}
	a.token = response.TenantAccessToken
	a.expiresAt = time.Now().Add(time.Duration(cacheSeconds) * time.Second)
	return a.token, nil
}

func (a *AppBotAlerter) postJSON(ctx context.Context, path, token string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return errors.New("encode Feishu OpenAPI request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return errors.New("build Feishu OpenAPI request")
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("Feishu OpenAPI request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxFeishuResponseBytes+1))
	if err != nil || len(data) > maxFeishuResponseBytes {
		return errors.New("Feishu OpenAPI response unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Feishu OpenAPI returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("Feishu OpenAPI response schema mismatch")
	}
	return nil
}

func readSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("secret file is empty")
	}
	return value, nil
}

func isLoopbackHTTP(parsed *url.URL) bool {
	if parsed.Scheme != "http" {
		return false
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}
