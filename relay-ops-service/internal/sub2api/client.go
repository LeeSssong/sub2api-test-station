package sub2api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

var errResponseTooLarge = errors.New("Sub2API response exceeds size limit")
var errSchemaMismatch = errors.New("Sub2API response schema mismatch")

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Sub2API returned HTTP %d", e.StatusCode)
}

type HTTPReader struct {
	baseURL  string
	adminKey string
	http     *http.Client
}

func NewHTTPReader(baseURL, adminKeyFile string) (*HTTPReader, error) {
	key, err := os.ReadFile(adminKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read Sub2API admin key: %w", err)
	}
	key = bytes.TrimSpace(key)
	if len(key) == 0 {
		return nil, fmt.Errorf("Sub2API admin key is empty")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Sub2API base URL")
	}
	return &HTTPReader{baseURL: baseURL, adminKey: string(key), http: &http.Client{Timeout: 20 * time.Second}}, nil
}

func IsResponseTooLarge(err error) bool {
	return errors.Is(err, errResponseTooLarge)
}

func IsSchemaMismatch(err error) bool {
	return errors.Is(err, errSchemaMismatch)
}

func HTTPStatus(err error) (int, bool) {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return 0, false
	}
	return httpErr.StatusCode, true
}

func (c *HTTPReader) ListChannels(ctx context.Context) ([]Channel, error) {
	var response struct {
		Items *[]Channel `json:"items"`
	}
	query := url.Values{"page": {"1"}, "page_size": {"200"}}
	if err := c.get(ctx, "/api/v1/admin/channels", query, &response); err != nil {
		return nil, err
	}
	if response.Items == nil {
		return nil, errSchemaMismatch
	}
	return *response.Items, nil
}

func (c *HTTPReader) ListGroups(ctx context.Context) ([]Group, error) {
	var groups []Group
	if err := c.get(ctx, "/api/v1/admin/groups/all", nil, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (c *HTTPReader) ListChannelMonitors(ctx context.Context) ([]ChannelMonitor, error) {
	var response struct {
		Items *[]ChannelMonitor `json:"items"`
	}
	query := url.Values{"page": {"1"}, "page_size": {"200"}}
	if err := c.get(ctx, "/api/v1/admin/channel-monitors", query, &response); err != nil {
		return nil, err
	}
	if response.Items == nil {
		return nil, errSchemaMismatch
	}
	return *response.Items, nil
}

func (c *HTTPReader) GetChannelMonitorHistory(ctx context.Context, monitorID int64, model string, limit int) ([]MonitorHistory, error) {
	var response struct {
		Items *[]MonitorHistory `json:"items"`
	}
	query := url.Values{"model": {model}, "limit": {strconv.Itoa(limit)}}
	path := "/api/v1/admin/channel-monitors/" + strconv.FormatInt(monitorID, 10) + "/history"
	if err := c.get(ctx, path, query, &response); err != nil {
		return nil, err
	}
	if response.Items == nil {
		return nil, errSchemaMismatch
	}
	return *response.Items, nil
}

func (c *HTTPReader) GetOpsSnapshot(ctx context.Context, query OpsQuery) (OpsSnapshot, error) {
	values := url.Values{}
	setIf(values, "time_range", query.TimeRange)
	setIf(values, "platform", query.Platform)
	setIf(values, "mode", query.Mode)
	if query.GroupID > 0 {
		values.Set("group_id", strconv.FormatInt(query.GroupID, 10))
	}
	var snapshot OpsSnapshot
	if err := c.get(ctx, "/api/v1/admin/ops/dashboard/snapshot-v2", values, &snapshot); err != nil {
		return OpsSnapshot{}, err
	}
	return snapshot, nil
}

func (c *HTTPReader) GetUsageStats(ctx context.Context, query UsageQuery) (UsageStats, error) {
	values := url.Values{}
	if query.GroupID > 0 {
		values.Set("group_id", strconv.FormatInt(query.GroupID, 10))
	}
	setIf(values, "model", query.Model)
	setIf(values, "period", query.Period)
	setIf(values, "start_date", query.StartDate)
	setIf(values, "end_date", query.EndDate)
	setIf(values, "timezone", query.Timezone)
	var stats UsageStats
	if err := c.get(ctx, "/api/v1/admin/usage/stats", values, &stats); err != nil {
		return UsageStats{}, err
	}
	return stats, nil
}

func (c *HTTPReader) get(ctx context.Context, path string, query url.Values, out any) error {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("build Sub2API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.adminKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Sub2API request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Sub2API response")
	}
	if len(data) > maxResponseBytes {
		return errResponseTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode}
	}
	if err := decodeResponse(data, out); err != nil {
		return errSchemaMismatch
	}
	return nil
}

func decodeResponse(data []byte, out any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	payload := data
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		payload = envelope.Data
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func setIf(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}
