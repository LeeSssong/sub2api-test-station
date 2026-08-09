package sub2api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/controlplane"
)

const (
	maxResponseBytes = 2 << 20
	accountPageSize  = 100
	maxAccountPages  = 20
	maxAccounts      = accountPageSize * maxAccountPages
	usagePageSize    = 1000
	maxUsagePages    = 1000
)

var errResponseTooLarge = errors.New("Sub2API response exceeds size limit")
var errSchemaMismatch = errors.New("Sub2API response schema mismatch")
var forbiddenProjectionKey = regexp.MustCompile(`(?i)^(api[_-]?key|access[_-]?token|auth[_-]?token|cookie|authorization|password|secret|credential|raw[_-]?response|response[_-]?body|base[_-]?url|request[_-]?header)$`)

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

func (c *HTTPReader) ListAccounts(ctx context.Context) ([]Account, error) {
	type accountPage struct {
		Items    *[]Account `json:"items"`
		Total    *int       `json:"total"`
		Page     *int       `json:"page"`
		PageSize *int       `json:"page_size"`
	}

	accounts := make([]Account, 0, accountPageSize)
	seen := make(map[int64]struct{})
	expectedTotal := -1
	for page := 1; page <= maxAccountPages; page++ {
		var response accountPage
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(accountPageSize)}}
		if err := c.get(ctx, "/api/v1/admin/accounts", query, &response); err != nil {
			return nil, err
		}
		if response.Items == nil || response.Total == nil || response.Page == nil || response.PageSize == nil ||
			*response.Page != page || *response.PageSize != accountPageSize || *response.Total < 0 || *response.Total > maxAccounts {
			return nil, errSchemaMismatch
		}
		if expectedTotal == -1 {
			expectedTotal = *response.Total
		} else if *response.Total != expectedTotal {
			return nil, errSchemaMismatch
		}
		for _, account := range *response.Items {
			if account.ID <= 0 {
				return nil, errSchemaMismatch
			}
			if _, duplicate := seen[account.ID]; duplicate {
				return nil, errSchemaMismatch
			}
			seen[account.ID] = struct{}{}
			accounts = append(accounts, account)
		}
		if len(accounts) == expectedTotal {
			return accounts, nil
		}
		if len(*response.Items) == 0 || len(accounts) > expectedTotal {
			return nil, errSchemaMismatch
		}
	}
	return nil, errSchemaMismatch
}

func (c *HTTPReader) ListAccountMonitors(ctx context.Context) (AccountMonitorProjection, error) {
	var projection AccountMonitorProjection
	if err := c.getStrict(ctx, "/api/v1/admin/account-monitors", nil, &projection); err != nil {
		return AccountMonitorProjection{}, err
	}
	if err := validateAccountMonitorProjection(projection); err != nil {
		return AccountMonitorProjection{}, errSchemaMismatch
	}
	return projection, nil
}

func (c *HTTPReader) ListAccountMonitorHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorHistoryEntry, error) {
	if accountID <= 0 || limit <= 0 {
		return nil, errSchemaMismatch
	}
	path := "/api/v1/admin/account-monitors/" + strconv.FormatInt(accountID, 10) + "/history"
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	var page accountMonitorHistoryPage
	if err := c.getStrict(ctx, path, query, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (c *HTTPReader) ListGroups(ctx context.Context) ([]Group, error) {
	var groups []Group
	if err := c.get(ctx, "/api/v1/admin/groups/all", nil, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (c *HTTPReader) GetGroup(ctx context.Context, id int64) (Group, error) {
	var group Group
	path := "/api/v1/admin/groups/" + strconv.FormatInt(id, 10)
	if err := c.get(ctx, path, nil, &group); err != nil {
		return Group{}, err
	}
	return group, nil
}

func (c *HTTPReader) GetAccount(ctx context.Context, id int64) (Account, error) {
	var account Account
	path := "/api/v1/admin/accounts/" + strconv.FormatInt(id, 10)
	if err := c.get(ctx, path, nil, &account); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (c *HTTPReader) GetAccountModels(ctx context.Context, id int64) ([]Model, error) {
	var models []Model
	path := "/api/v1/admin/accounts/" + strconv.FormatInt(id, 10) + "/models"
	if err := c.get(ctx, path, nil, &models); err != nil {
		return nil, err
	}
	return models, nil
}

func (c *HTTPReader) SyncUpstreamModels(ctx context.Context, id int64) ([]Model, error) {
	if id <= 0 {
		return nil, errSchemaMismatch
	}
	var response struct {
		Models *[]string `json:"models"`
	}
	path := "/api/v1/admin/accounts/" + strconv.FormatInt(id, 10) + "/models/sync-upstream"
	if err := c.emptyPost(ctx, path, &response); err != nil {
		return nil, err
	}
	if response.Models == nil || len(*response.Models) == 0 || len(*response.Models) > 4096 {
		return nil, errSchemaMismatch
	}
	seen := make(map[string]struct{}, len(*response.Models))
	models := make([]Model, 0, len(*response.Models))
	for _, modelID := range *response.Models {
		if !validModelID(modelID) {
			return nil, errSchemaMismatch
		}
		if _, duplicate := seen[modelID]; duplicate {
			return nil, errSchemaMismatch
		}
		seen[modelID] = struct{}{}
		models = append(models, Model{ID: modelID})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func (c *HTTPReader) SetAccountGroups(ctx context.Context, id int64, groupIDs []int64) (Account, error) {
	var account Account
	path := "/api/v1/admin/accounts/" + strconv.FormatInt(id, 10)
	body := struct {
		GroupIDs []int64 `json:"group_ids"`
	}{GroupIDs: groupIDs}
	if err := c.jsonRequest(ctx, http.MethodPut, path, body, &account); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (c *HTTPReader) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (Account, error) {
	var account Account
	path := "/api/v1/admin/accounts/" + strconv.FormatInt(id, 10) + "/schedulable"
	body := struct {
		Schedulable bool `json:"schedulable"`
	}{Schedulable: schedulable}
	if err := c.jsonRequest(ctx, http.MethodPost, path, body, &account); err != nil {
		return Account{}, err
	}
	return account, nil
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
	if query.AccountID > 0 {
		values.Set("account_id", strconv.FormatInt(query.AccountID, 10))
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
	if query.AccountID > 0 {
		values.Set("account_id", strconv.FormatInt(query.AccountID, 10))
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

// ListUsageLogs reads the complete, bounded result set for an exact time
// window. Reconciliation uses the durable usage ledger as the source of local
// request attempts, so a failed import can safely be retried.
func (c *HTTPReader) ListUsageLogs(ctx context.Context, query UsageLogQuery) ([]UsageLog, error) {
	if query.AccountID <= 0 || query.Start.IsZero() || query.End.IsZero() || !query.Start.Before(query.End) {
		return nil, errSchemaMismatch
	}
	type usagePage struct {
		Items    *[]UsageLog `json:"items"`
		Total    *int        `json:"total"`
		Page     *int        `json:"page"`
		PageSize *int        `json:"page_size"`
	}
	logs := make([]UsageLog, 0)
	seen := make(map[int64]struct{})
	expectedTotal := -1
	for page := 1; page <= maxUsagePages; page++ {
		values := url.Values{
			"account_id":  {strconv.FormatInt(query.AccountID, 10)},
			"exact_total": {"true"},
			"page":        {strconv.Itoa(page)},
			"page_size":   {strconv.Itoa(usagePageSize)},
			"sort_by":     {"created_at"},
			"sort_order":  {"asc"},
			"start_time":  {query.Start.UTC().Format(time.RFC3339Nano)},
			"end_time":    {query.End.UTC().Format(time.RFC3339Nano)},
		}
		var response usagePage
		if err := c.get(ctx, "/api/v1/admin/usage", values, &response); err != nil {
			return nil, err
		}
		if response.Items == nil || response.Total == nil || response.Page == nil || response.PageSize == nil ||
			*response.Page != page || *response.PageSize != usagePageSize || *response.Total < 0 {
			return nil, errSchemaMismatch
		}
		if expectedTotal == -1 {
			expectedTotal = *response.Total
		} else if expectedTotal != *response.Total {
			return nil, errSchemaMismatch
		}
		for _, log := range *response.Items {
			if log.ID <= 0 || log.AccountID != query.AccountID || log.CreatedAt.IsZero() ||
				log.CreatedAt.Before(query.Start) || !log.CreatedAt.Before(query.End) ||
				math.IsNaN(log.TotalCost) || math.IsInf(log.TotalCost, 0) || log.TotalCost < 0 ||
				log.InputTokens < 0 || log.OutputTokens < 0 {
				return nil, errSchemaMismatch
			}
			if _, duplicate := seen[log.ID]; duplicate {
				return nil, errSchemaMismatch
			}
			seen[log.ID] = struct{}{}
			logs = append(logs, log)
		}
		if len(logs) == expectedTotal {
			return logs, nil
		}
		if len(*response.Items) == 0 || len(logs) > expectedTotal {
			return nil, errSchemaMismatch
		}
	}
	return nil, errSchemaMismatch
}

func (c *HTTPReader) ListOpsAlertEvents(ctx context.Context, cursor OpsAlertEventCursor) ([]OpsAlertEvent, error) {
	limit := cursor.Limit
	if limit == 0 {
		limit = 500
	}
	if limit < 1 || limit > 500 || (cursor.BeforeFiredAt == nil) != (cursor.BeforeID == nil) {
		return nil, errSchemaMismatch
	}
	values := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor.BeforeFiredAt != nil && cursor.BeforeID != nil {
		if cursor.BeforeFiredAt.IsZero() || *cursor.BeforeID <= 0 {
			return nil, errSchemaMismatch
		}
		values.Set("before_fired_at", cursor.BeforeFiredAt.UTC().Format(time.RFC3339Nano))
		values.Set("before_id", strconv.FormatInt(*cursor.BeforeID, 10))
	}
	var payloads []opsAlertEventPayload
	if err := c.getStrict(ctx, "/api/v1/admin/ops/alert-events", values, &payloads); err != nil {
		return nil, err
	}
	events := make([]OpsAlertEvent, 0, len(payloads))
	for _, payload := range payloads {
		event := payload.event()
		if err := validateOpsAlertEvent(event); err != nil {
			return nil, errSchemaMismatch
		}
		events = append(events, event)
	}
	return events, nil
}

func (c *HTTPReader) GetOpsAlertEvent(ctx context.Context, id int64) (OpsAlertEvent, error) {
	if id <= 0 {
		return OpsAlertEvent{}, errSchemaMismatch
	}
	var payload opsAlertEventPayload
	path := "/api/v1/admin/ops/alert-events/" + strconv.FormatInt(id, 10)
	if err := c.getStrict(ctx, path, nil, &payload); err != nil {
		return OpsAlertEvent{}, err
	}
	event := payload.event()
	if event.ID != id || validateOpsAlertEvent(event) != nil {
		return OpsAlertEvent{}, errSchemaMismatch
	}
	return event, nil
}

type opsAlertEventPayload struct {
	ID             int64          `json:"id"`
	RuleID         int64          `json:"rule_id"`
	Severity       string         `json:"severity"`
	Status         string         `json:"status"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	MetricValue    *float64       `json:"metric_value"`
	ThresholdValue *float64       `json:"threshold_value"`
	Dimensions     map[string]any `json:"dimensions"`
	FiredAt        time.Time      `json:"fired_at"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
	EmailSent      bool           `json:"email_sent"`
	CreatedAt      time.Time      `json:"created_at"`
}

func (p opsAlertEventPayload) event() OpsAlertEvent {
	return OpsAlertEvent{
		ID:             p.ID,
		RuleID:         p.RuleID,
		Severity:       p.Severity,
		Status:         p.Status,
		Title:          p.Title,
		Description:    p.Description,
		MetricValue:    p.MetricValue,
		ThresholdValue: p.ThresholdValue,
		Dimensions:     p.Dimensions,
		FiredAt:        p.FiredAt,
		ResolvedAt:     p.ResolvedAt,
	}
}

func (c *HTTPReader) VerifyAdminSession(ctx context.Context, session adminauth.Session) (adminauth.Identity, error) {
	var identity struct {
		UserID int64  `json:"id"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if strings.TrimSpace(session.Bearer) == "" {
		return adminauth.Identity{}, fmt.Errorf("Sub2API session token is empty")
	}
	if err := c.getWithBearer(ctx, "/api/v1/auth/me", session, &identity); err != nil {
		return adminauth.Identity{}, err
	}
	if identity.UserID <= 0 || identity.Role == "" || identity.Status == "" {
		return adminauth.Identity{}, errSchemaMismatch
	}
	return adminauth.Identity{UserID: identity.UserID, Role: identity.Role, Status: identity.Status}, nil
}

func (c *HTTPReader) Me(ctx context.Context, bearer, clientIP, origin string) (controlplane.AdminIdentity, error) {
	identity, err := c.VerifyAdminSession(ctx, adminauth.Session{Bearer: bearer, ClientIP: clientIP, Origin: origin})
	if err != nil {
		return controlplane.AdminIdentity{}, err
	}
	return controlplane.AdminIdentity{UserID: identity.UserID, Role: identity.Role, Status: identity.Status}, nil
}

func (c *HTTPReader) RefreshAccount(ctx context.Context, id int64, idempotencyKey string) error {
	if id <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return errSchemaMismatch
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/admin/accounts/"+strconv.FormatInt(id, 10)+"/refresh", nil)
	if err != nil {
		return fmt.Errorf("build Sub2API refresh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.adminKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	return c.do(req, &struct{}{})
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
	return c.do(req, out)
}

func (c *HTTPReader) getStrict(ctx context.Context, path string, query url.Values, out any) error {
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
	return c.doStrict(req, out)
}

func (c *HTTPReader) jsonRequest(ctx context.Context, method, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode Sub2API request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build Sub2API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.adminKey)
	return c.do(req, out)
}

func (c *HTTPReader) emptyPost(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build Sub2API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.adminKey)
	return c.do(req, out)
}

func (c *HTTPReader) getWithBearer(ctx context.Context, path string, session adminauth.Session, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build Sub2API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+session.Bearer)
	req.Header["User-Agent"] = []string{session.UserAgent}
	if session.ClientIP != "" {
		req.Header.Set("X-Client-IP", session.ClientIP)
		req.Header.Set("X-Forwarded-For", session.ClientIP)
		req.Header.Set("X-Real-IP", session.ClientIP)
	} else {
		if session.ForwardedFor != "" {
			req.Header.Set("X-Forwarded-For", session.ForwardedFor)
		}
		if session.RealIP != "" {
			req.Header.Set("X-Real-IP", session.RealIP)
		}
	}
	if session.Origin != "" {
		req.Header.Set("Origin", session.Origin)
	}
	return c.do(req, out)
}

func (c *HTTPReader) do(req *http.Request, out any) error {
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

func (c *HTTPReader) doStrict(req *http.Request, out any) error {
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
	if containsForbiddenProjectionKey(data) {
		return errSchemaMismatch
	}
	if err := decodeStrictResponse(data, out); err != nil {
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

func decodeStrictResponse(data []byte, out any) error {
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
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func containsForbiddenProjectionKey(data []byte) bool {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&value) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(item any) bool {
		switch typed := item.(type) {
		case map[string]any:
			for key, child := range typed {
				if forbiddenProjectionKey.MatchString(strings.TrimSpace(key)) {
					return true
				}
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func validateAccountMonitorProjection(projection AccountMonitorProjection) error {
	if projection.SchemaVersion != 2 || projection.ObservedAt.IsZero() || projection.Settings.IntervalSeconds <= 0 || projection.Accounts == nil {
		return errSchemaMismatch
	}
	seen := make(map[int64]struct{}, len(projection.Accounts))
	for _, account := range projection.Accounts {
		if account.AccountID <= 0 || strings.TrimSpace(account.Name) == "" || strings.TrimSpace(account.Platform) == "" ||
			strings.TrimSpace(account.AccountType) == "" ||
			account.SampleCount < 0 || account.SuccessRate < 0 || account.SuccessRate > 1 ||
			account.RequestCount < 0 || account.ErrorCount < 0 ||
			!finiteNumber(account.SuccessRate) || !validAccountMonitorMultiplier(account.Multiplier, projection.ObservedAt) {
			return errSchemaMismatch
		}
		if _, ok := seen[account.AccountID]; ok {
			return errSchemaMismatch
		}
		seen[account.AccountID] = struct{}{}
		if account.TTFTP50MS != nil && (!finiteNumber(*account.TTFTP50MS) || *account.TTFTP50MS < 0) {
			return errSchemaMismatch
		}
		if account.TTFTP95MS != nil && (!finiteNumber(*account.TTFTP95MS) || *account.TTFTP95MS < 0) {
			return errSchemaMismatch
		}
		if account.LatencyP95MS != nil && (!finiteNumber(*account.LatencyP95MS) || *account.LatencyP95MS < 0) {
			return errSchemaMismatch
		}
		if account.CheckedAt != nil && account.CheckedAt.IsZero() {
			return errSchemaMismatch
		}
		if account.Latest != nil {
			if strings.TrimSpace(account.Latest.Status) == "" || account.Latest.CheckedAt.IsZero() ||
				(account.Latest.TTFTMS != nil && (!finiteNumber(*account.Latest.TTFTMS) || *account.Latest.TTFTMS < 0)) ||
				(account.Latest.LatencyMS != nil && (!finiteNumber(*account.Latest.LatencyMS) || *account.Latest.LatencyMS < 0)) {
				return errSchemaMismatch
			}
		}
		if account.TodayStats != nil && (account.TodayStats.Requests < 0 || account.TodayStats.Tokens < 0 ||
			!finiteNumber(account.TodayStats.Cost) || !finiteNumber(account.TodayStats.StandardCost) ||
			!finiteNumber(account.TodayStats.UserCost)) {
			return errSchemaMismatch
		}
		for _, window := range account.UsageWindows {
			if strings.TrimSpace(window.Name) == "" || window.Utilization < 0 || !finiteNumber(window.Utilization) ||
				window.Requests < 0 || window.Tokens < 0 {
				return errSchemaMismatch
			}
		}
	}
	return nil
}

func validAccountMonitorMultiplier(multiplier AccountMonitorMultiplier, projectionObservedAt time.Time) bool {
	switch multiplier.Status {
	case "ok":
		if multiplier.Source != "declared" && multiplier.Source != "measured" {
			return false
		}
		if multiplier.Value == nil || *multiplier.Value < 0 || !finiteNumber(*multiplier.Value) ||
			multiplier.ObservedAt == nil || multiplier.ObservedAt.IsZero() ||
			multiplier.ObservedAt.After(projectionObservedAt) {
			return false
		}
		return true
	case "stale", "unsupported", "failed", "unavailable":
		return multiplier.Value == nil
	default:
		return false
	}
}

func validateOpsAlertEvent(event OpsAlertEvent) error {
	if event.ID <= 0 || event.RuleID <= 0 || event.FiredAt.IsZero() {
		return errSchemaMismatch
	}
	switch event.Severity {
	case "P0", "P1", "P2", "P3":
	default:
		return errSchemaMismatch
	}
	switch event.Status {
	case "firing", "resolved", "manual_resolved":
	default:
		return errSchemaMismatch
	}
	if event.ResolvedAt != nil && event.ResolvedAt.IsZero() {
		return errSchemaMismatch
	}
	for _, value := range []*float64{event.MetricValue, event.ThresholdValue} {
		if value != nil && !finiteNumber(*value) {
			return errSchemaMismatch
		}
	}
	if event.Dimensions != nil {
		data, err := json.Marshal(event.Dimensions)
		if err != nil || len(data) > maxResponseBytes || containsForbiddenProjectionKey(data) {
			return errSchemaMismatch
		}
	}
	return nil
}

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func setIf(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}

func validModelID(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}
