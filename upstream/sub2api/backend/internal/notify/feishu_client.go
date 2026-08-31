package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxFeishuResponseBytes = 1 << 20

type feishuAPIError struct{ code int }

func (e *feishuAPIError) Error() string {
	return fmt.Sprintf("Feishu OpenAPI request failed with code %d", e.code)
}

type FeishuSender struct {
	baseURL    string
	appID      string
	appSecret  string
	chatID     string
	httpClient *http.Client
	now        func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewFeishuSender(baseURL string, secrets UpstreamBalanceSecrets) *FeishuSender {
	return &FeishuSender{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		appID:   secrets.AppID, appSecret: secrets.AppSecret, chatID: secrets.ChatID,
		httpClient: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

func (s *FeishuSender) Send(ctx context.Context, input UpstreamBalanceCardInput) (string, error) {
	if s == nil || s.httpClient == nil || strings.TrimSpace(s.chatID) == "" {
		return "", errors.New("Feishu sender is unavailable")
	}
	parsed, err := url.Parse(s.baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("Feishu sender configuration is invalid")
	}
	card, err := RenderUpstreamBalanceCard(input)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		token, tokenErr := s.tenantToken(ctx)
		if tokenErr != nil {
			return "", tokenErr
		}
		messageID, sendErr := s.sendMessage(ctx, token, card)
		if sendErr == nil {
			return messageID, nil
		}
		lastErr = sendErr
		var apiErr *feishuAPIError
		if !errors.As(sendErr, &apiErr) {
			return "", sendErr
		}
		s.invalidateToken(token)
	}
	return "", lastErr
}

func (s *FeishuSender) tenantToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Before(s.expiresAt) {
		return s.token, nil
	}
	request := struct {
		AppID     string `json:"app_id"`
		AppSecret string `json:"app_secret"`
	}{AppID: s.appID, AppSecret: s.appSecret}
	var response struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := s.requestJSON(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", "", request, &response); err != nil {
		return "", err
	}
	if response.Code != 0 {
		return "", &feishuAPIError{code: response.Code}
	}
	if response.TenantAccessToken == "" || response.Expire <= 0 {
		return "", errors.New("Feishu token response is invalid")
	}
	seconds := response.Expire - 60
	if seconds < 1 {
		seconds = 1
	}
	s.token = response.TenantAccessToken
	s.expiresAt = s.now().Add(time.Duration(seconds) * time.Second)
	return s.token, nil
}

func (s *FeishuSender) sendMessage(ctx context.Context, token string, card []byte) (string, error) {
	request := struct {
		ReceiveID string `json:"receive_id"`
		MsgType   string `json:"msg_type"`
		Content   string `json:"content"`
	}{ReceiveID: s.chatID, MsgType: "interactive", Content: string(card)}
	var response struct {
		Code int `json:"code"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	path := "/open-apis/im/v1/messages?receive_id_type=chat_id"
	if err := s.requestJSON(ctx, http.MethodPost, path, token, request, &response); err != nil {
		return "", err
	}
	if response.Code != 0 {
		return "", &feishuAPIError{code: response.Code}
	}
	if response.Data.MessageID == "" {
		return "", errors.New("Feishu message response is invalid")
	}
	return response.Data.MessageID, nil
}

func (s *FeishuSender) requestJSON(ctx context.Context, method, path, token string, requestValue, responseValue any) error {
	body, err := json.Marshal(requestValue)
	if err != nil {
		return errors.New("encode Feishu request")
	}
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("build Feishu request")
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return errors.New("send Feishu request")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Feishu HTTP request failed with status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxFeishuResponseBytes+1))
	if err := decoder.Decode(responseValue); err != nil {
		return errors.New("decode Feishu response")
	}
	return nil
}

func (s *FeishuSender) invalidateToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == token {
		s.token = ""
		s.expiresAt = time.Time{}
	}
}
