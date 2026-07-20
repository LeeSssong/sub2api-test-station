package feishuapi

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

const maxResponseBytes = 1 << 20

var errUnauthorized = errors.New("Feishu OpenAPI token was rejected")

type Client struct {
	baseURL   string
	appID     string
	appSecret string
	http      *http.Client
	now       func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewClient(baseURL, appIDFile, appSecretFile string) (*Client, error) {
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
	return &Client{
		baseURL: baseURL, appID: appID, appSecret: appSecret,
		http: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}, nil
}

func (c *Client) SendText(ctx context.Context, chatID, text string) (string, error) {
	if chatID == "" || len(chatID) > 256 || text == "" || len([]byte(text)) > 16<<10 {
		return "", errors.New("invalid Feishu message input")
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.tenantToken(ctx)
		if err != nil {
			return "", err
		}
		messageID, err := c.sendText(ctx, token, chatID, text)
		if !errors.Is(err, errUnauthorized) {
			return messageID, err
		}
		c.invalidateToken(token)
	}
	return "", errUnauthorized
}

func (c *Client) tenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.expiresAt) {
		return c.token, nil
	}
	body := struct {
		AppID     string `json:"app_id"`
		AppSecret string `json:"app_secret"`
	}{AppID: c.appID, AppSecret: c.appSecret}
	var response struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := c.postJSON(ctx, "/open-apis/auth/v3/tenant_access_token/internal", "", body, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.TenantAccessToken == "" || response.Expire <= 0 {
		return "", errors.New("Feishu token response schema mismatch")
	}
	cacheSeconds := response.Expire - 60
	if cacheSeconds <= 0 {
		cacheSeconds = 1
	}
	c.token = response.TenantAccessToken
	c.expiresAt = c.now().Add(time.Duration(cacheSeconds) * time.Second)
	return c.token, nil
}

func (c *Client) sendText(ctx context.Context, token, chatID, text string) (string, error) {
	content, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return "", errors.New("encode Feishu message content")
	}
	body := struct {
		ReceiveID string `json:"receive_id"`
		MsgType   string `json:"msg_type"`
		Content   string `json:"content"`
	}{ReceiveID: chatID, MsgType: "text", Content: string(content)}
	var response struct {
		Code int `json:"code"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	path := "/open-apis/im/v1/messages?receive_id_type=chat_id"
	if err := c.postJSON(ctx, path, token, body, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.Data.MessageID == "" {
		return "", errors.New("Feishu message response schema mismatch")
	}
	return response.Data.MessageID, nil
}

func (c *Client) postJSON(ctx context.Context, path, token string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return errors.New("encode Feishu OpenAPI request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return errors.New("build Feishu OpenAPI request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Feishu OpenAPI request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("read Feishu OpenAPI response")
	}
	if len(data) > maxResponseBytes {
		return errors.New("Feishu OpenAPI response exceeds size limit")
	}
	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		return errUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Feishu OpenAPI returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("Feishu OpenAPI response schema mismatch")
	}
	return nil
}

func (c *Client) invalidateToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == token {
		c.token = ""
		c.expiresAt = time.Time{}
	}
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
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}
