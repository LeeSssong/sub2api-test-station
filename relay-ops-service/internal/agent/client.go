package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	APIKeyFile string
	Model      string
	HTTP       *http.Client
}

func (c Client) Analyze(ctx context.Context, contract IncidentContractV1) (Analysis, error) {
	if err := validateContract(contract); err != nil {
		return Analysis{}, err
	}
	base, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return Analysis{}, fmt.Errorf("agent base URL is invalid")
	}
	key, err := readAgentSecret(c.APIKeyFile)
	if err != nil {
		return Analysis{}, err
	}
	defer clearAgentSecret(key)
	contractJSON, _ := json.Marshal(contract)
	payload := map[string]any{
		"model": c.Model, "temperature": 0, "max_tokens": 800,
		"messages": []map[string]string{{"role": "system", "content": "Return only JSON matching the relay-ops analysis schema. Do not request tools or secrets."}, {"role": "user", "content": string(contractJSON)}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Analysis{}, fmt.Errorf("build agent request")
	}
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Analysis{}, fmt.Errorf("agent request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return Analysis{}, fmt.Errorf("agent response is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Analysis{}, fmt.Errorf("agent returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) != 1 {
		return Analysis{}, fmt.Errorf("agent response schema is invalid")
	}
	return ValidateAgentOutput([]byte(envelope.Choices[0].Message.Content))
}

func readAgentSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("agent API key is unavailable")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) || info.Size() <= 0 || info.Size() > 8<<10 {
		return nil, fmt.Errorf("agent API key is unsafe")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agent API key is unavailable")
	}
	return bytes.TrimSpace(value), nil
}
func clearAgentSecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
