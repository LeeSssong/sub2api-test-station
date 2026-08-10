package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const adapterMaxResponseBytes = 2 << 20
const providerRequestTimeout = 15 * time.Second

type httpAdapter struct {
	baseURL string
	token   string
	client  *http.Client
	timeout time.Duration
}

func newHTTPAdapter(baseURL, token string, client *http.Client) (*httpAdapter, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid billing adapter base URL")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("billing adapter token is empty")
	}
	if client == nil {
		client = &http.Client{Timeout: providerRequestTimeout}
	}
	return &httpAdapter{baseURL: baseURL, token: token, client: client, timeout: providerRequestTimeout}, nil
}

func (a *httpAdapter) doJSON(ctx context.Context, method, path string, query url.Values, out any) error {
	timeout := a.timeout
	if timeout <= 0 {
		timeout = providerRequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u, err := url.Parse(a.baseURL + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return fmt.Errorf("build billing request: %w", err)
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("build billing request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("billing request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, adapterMaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read billing response: %w", err)
	}
	if len(body) > adapterMaxResponseBytes {
		return fmt.Errorf("billing response exceeds size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream billing returned HTTP %d", resp.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode billing response: %w", err)
	}
	return nil
}
