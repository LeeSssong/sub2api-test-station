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
	"sort"
	"strconv"
	"strings"
	"time"

	"example.invalid/internal-test-service/internal/domain"
)

type HTTPClient struct {
	baseURL  string
	adminKey string
	http     *http.Client
}

func NewHTTPClient(baseURL, adminKeyFile string) (*HTTPClient, error) {
	key, err := os.ReadFile(adminKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read admin API key: %w", err)
	}
	key = bytes.TrimSpace(key)
	if len(key) == 0 {
		return nil, errors.New("admin API key is empty")
	}
	return &HTTPClient{baseURL: strings.TrimRight(baseURL, "/"), adminKey: string(key), http: &http.Client{Timeout: 20 * time.Second}}, nil
}

func NewHTTPClientWithKey(baseURL, key string) *HTTPClient {
	return &HTTPClient{baseURL: strings.TrimRight(baseURL, "/"), adminKey: key, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *HTTPClient) GetCurrentUser(ctx context.Context, bearer string) (User, error) {
	var wrapper CurrentUserResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, bearer, "", &wrapper); err != nil {
		return User{}, err
	}
	if wrapper.User.ID != 0 {
		return wrapper.User, nil
	}
	if wrapper.Data.ID != 0 {
		return wrapper.Data, nil
	}
	if wrapper.ID != 0 {
		return User{ID: wrapper.ID}, nil
	}
	var user User
	if err := decodeFallback(wrapper, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *HTTPClient) GenerateInvitation(ctx context.Context, count int, expiresAt *time.Time, idem string) ([]InvitationCode, error) {
	body := map[string]any{"count": count, "type": "invitation"}
	if expiresAt != nil {
		body["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	var response InvitationListResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/admin/redeem-codes/generate", body, "", idem, &response); err != nil {
		return nil, err
	}
	return responseItems(response), nil
}
func responseItems(r InvitationListResponse) []InvitationCode {
	if len(r.Data) > 0 {
		return r.Data
	}
	return r.Items
}

func (c *HTTPClient) ListInvitationCodes(ctx context.Context) ([]InvitationCode, error) {
	var all []InvitationCode
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {"1000"}, "sort_by": {"id"}, "sort_order": {"asc"}}
		var r InvitationListResponse
		if err := c.do(ctx, http.MethodGet, "/api/v1/admin/redeem-codes?"+q.Encode(), nil, "", "", &r); err != nil {
			return nil, err
		}
		items := responseItems(r)
		all = append(all, items...)
		if r.Pages <= page || len(items) == 0 {
			break
		}
	}
	return all, nil
}
func (c *HTTPClient) ExpireInvitation(ctx context.Context, codeID int64, idem string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/admin/redeem-codes/"+strconv.FormatInt(codeID, 10)+"/expire", map[string]any{}, "", idem, &struct{}{})
}

func (c *HTTPClient) AddBalance(ctx context.Context, userID int64, amount domain.MicroUSD, idem, note string) error {
	body := map[string]any{"balance": float64(amount) / 1_000_000, "operation": "add", "notes": fmt.Sprintf("%s [idempotency_key=%s]", note, idem)}
	return c.do(ctx, http.MethodPost, "/api/v1/admin/users/"+strconv.FormatInt(userID, 10)+"/balance", body, "", idem, &struct{}{})
}

func (c *HTTPClient) GetBalance(ctx context.Context, userID int64) (Balance, error) {
	var userResponse struct {
		Data User `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/admin/users/"+strconv.FormatInt(userID, 10), nil, "", "", &userResponse); err != nil {
		return Balance{}, err
	}
	money, err := parseJSONMoney(userResponse.Data.Balance)
	if err != nil {
		return Balance{}, err
	}
	var history []BalanceEntry
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {"1000"}}
		var historyResponse BalanceHistoryResponse
		if err := c.do(ctx, http.MethodGet, "/api/v1/admin/users/"+strconv.FormatInt(userID, 10)+"/balance-history?"+q.Encode(), nil, "", "", &historyResponse); err != nil {
			return Balance{}, err
		}
		items := historyResponse.Data
		if len(items) == 0 {
			items = historyResponse.Items
		}
		history = append(history, items...)
		pages := historyResponse.Pages
		if pages == 0 {
			pages = 1
		}
		if page >= pages || len(items) == 0 {
			break
		}
	}
	return Balance{UserID: userID, Balance: money, History: history}, nil
}

func (c *HTTPClient) ListUsage(ctx context.Context, userID int64, afterID int64) ([]Usage, error) {
	var all []Usage
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("user_id", strconv.FormatInt(userID, 10))
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", "1000")
		q.Set("exact_total", "true")
		q.Set("sort_by", "id")
		q.Set("sort_order", "desc")
		var r UsageListResponse
		if err := c.do(ctx, http.MethodGet, "/api/v1/admin/usage?"+q.Encode(), nil, "", "", &r); err != nil {
			return nil, err
		}
		items := r.Data
		if len(items) == 0 {
			items = r.Items
		}
		for _, item := range items {
			item = item.Normalize()
			if item.ID > afterID {
				all = append(all, item)
			}
		}
		if r.Pages <= page || len(items) == 0 {
			break
		}
		oldest := items[len(items)-1].ID
		if oldest <= afterID {
			break
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

func (c *HTTPClient) GetUser(ctx context.Context, userID int64) (User, error) {
	var response struct {
		Data User `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/admin/users/"+strconv.FormatInt(userID, 10), nil, "", "", &response); err != nil {
		return User{}, err
	}
	if response.Data.ID != userID {
		return User{}, fmt.Errorf("user not found")
	}
	return response.Data, nil
}

func parseJSONMoney(value any) (domain.MicroUSD, error) {
	switch v := value.(type) {
	case string:
		return domain.ParseMicroUSD(v)
	case float64:
		return domain.ParseMicroUSD(strconv.FormatFloat(v, 'f', 6, 64))
	case json.Number:
		return domain.ParseMicroUSD(v.String())
	case nil:
		return 0, fmt.Errorf("missing balance")
	default:
		return 0, fmt.Errorf("invalid balance")
	}
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, bearer, idem string, out any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if c.adminKey != "" && strings.HasPrefix(path, "/api/v1/admin/") {
		req.Header.Set("x-api-key", c.adminKey)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sub2api request failed: %w", err)
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return fmt.Errorf("sub2api response read failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sub2api returned HTTP %d", resp.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("sub2api response malformed")
	}
	return nil
}

func decodeFallback(wrapper CurrentUserResponse, out *User) error {
	data, _ := json.Marshal(wrapper)
	return json.Unmarshal(data, out)
}
