package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/pricing"
)

type Link struct{ Label, URL string }

type IncidentView struct {
	Title       string
	WhatWasDone []string
	Results     []string
	Change      string
	Focus       string
	Links       []Link
}

type FeishuMessage struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

func RenderFeishu(event IncidentView) FeishuMessage {
	var lines []string
	lines = append(lines, event.Title, "", "做了什么："+joinItems(event.WhatWasDone), "得到什么："+joinItems(event.Results))
	lines = append(lines, "发生什么变化："+defaultText(event.Change), "需要关注什么："+defaultText(event.Focus))
	linkText := "无"
	if len(event.Links) > 0 {
		items := make([]string, 0, len(event.Links))
		for _, link := range event.Links {
			items = append(items, link.Label+" "+link.URL)
		}
		linkText = strings.Join(items, "；")
	}
	lines = append(lines, "链接："+linkText)
	message := FeishuMessage{MsgType: "text"}
	message.Content.Text = strings.Join(lines, "\n")
	return message
}

func RenderSessionExpired(upstream, loginURL string) FeishuMessage {
	return RenderFeishu(IncidentView{
		Title:       "上游用量读取会话失效：" + upstream,
		WhatWasDone: []string{"读取上游用量页面并在 401 后重试 1 次"},
		Results:     []string{"质量和公开价格监控正常；真实消费核对暂停"},
		Change:      "登录会话已失效",
		Focus:       "打开登录链接并重新登录一次",
		Links:       []Link{{Label: "重新登录", URL: loginURL}},
	})
}

type Client struct {
	WebhookFile string
	HTTP        *http.Client
	Resolver    pricing.Resolver
}

func (c Client) Send(ctx context.Context, message FeishuMessage) error {
	webhookBytes, err := readNotifySecret(c.WebhookFile)
	if err != nil {
		return err
	}
	defer clearNotifySecret(webhookBytes)
	webhook := string(webhookBytes)
	resolver := c.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := pricing.ValidateRemoteURL(ctx, resolver, webhook); err != nil {
		return fmt.Errorf("Feishu webhook URL is unsafe")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode Feishu message")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build Feishu request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Feishu delivery failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Feishu delivery returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func readNotifySecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu webhook secret is unavailable")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) || info.Size() <= 0 || info.Size() > 8<<10 {
		return nil, fmt.Errorf("Feishu webhook secret is unsafe")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu webhook secret is unavailable")
	}
	value = bytes.TrimSpace(value)
	return value, nil
}

func clearNotifySecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
func joinItems(items []string) string {
	if len(items) == 0 {
		return "无"
	}
	return strings.Join(items, "；")
}
func defaultText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "无"
	}
	return value
}
