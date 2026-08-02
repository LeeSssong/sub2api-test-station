package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

type NewAPIAdapter struct{ *httpAdapter }

func NewNewAPIAdapter(baseURL, token string, client *http.Client) (*NewAPIAdapter, error) {
	a, err := newHTTPAdapter(baseURL, token, client)
	if err != nil {
		return nil, err
	}
	return &NewAPIAdapter{httpAdapter: a}, nil
}

type newAPILog struct {
	ID                any        `json:"id"`
	Type              int        `json:"type"`
	Quota             int64      `json:"quota"`
	RequestID         string     `json:"request_id"`
	UpstreamRequestID string     `json:"upstream_request_id"`
	Model             string     `json:"model_name"`
	PromptTokens      int64      `json:"prompt_tokens"`
	CompletionTokens  int64      `json:"completion_tokens"`
	CreatedAt         newAPITime `json:"created_at"`
}

type newAPITime struct{ time.Time }

func (t *newAPITime) UnmarshalJSON(value []byte) error {
	raw := strings.Trim(string(value), `"`)
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		t.Time = time.Unix(unix, 0).UTC()
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return fmt.Errorf("invalid New API created_at")
	}
	t.Time = parsed
	return nil
}

func (a *NewAPIAdapter) ListTransactions(ctx context.Context, q CostQuery) ([]CostTransaction, string, error) {
	values := url.Values{}
	if q.Limit > 0 {
		values.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Cursor != "" {
		values.Set("cursor", q.Cursor)
	}
	if q.Start != nil {
		values.Set("start_timestamp", strconv.FormatInt(q.Start.Unix(), 10))
	}
	if q.End != nil {
		values.Set("end_timestamp", strconv.FormatInt(q.End.Unix(), 10))
	}
	var response struct {
		Data       []newAPILog `json:"data"`
		Logs       []newAPILog `json:"logs"`
		NextCursor string      `json:"next_cursor"`
	}
	if err := a.doJSON(ctx, http.MethodGet, "/api/log/token", values, &response); err != nil {
		return nil, "", err
	}
	logs := response.Data
	if len(logs) == 0 && response.Logs != nil {
		logs = response.Logs
	}
	var status struct {
		Data struct {
			QuotaPerUnit int64 `json:"quota_per_unit"`
		} `json:"data"`
		QuotaPerUnit int64 `json:"quota_per_unit"`
	}
	if err := a.doJSON(ctx, http.MethodGet, "/api/status", nil, &status); err != nil {
		return nil, "", err
	}
	unit := status.Data.QuotaPerUnit
	if unit == 0 {
		unit = status.QuotaPerUnit
	}
	if unit <= 0 {
		return nil, "", fmt.Errorf("New API quota_per_unit is invalid")
	}
	out := make([]CostTransaction, 0, len(logs))
	for _, log := range logs {
		cost, err := newAPIQuotaCost(log.Quota, unit)
		if err != nil {
			return nil, "", err
		}
		if log.Type == 6 {
			cost = -cost
		}
		typ := "charge"
		if log.Type == 6 {
			typ = "refund"
		}
		out = append(out, CostTransaction{SourceID: fmt.Sprint(log.ID), RequestID: log.RequestID, UpstreamRequestID: log.UpstreamRequestID, Type: typ, Cost: cost, OccurredAt: log.CreatedAt.Time, Model: log.Model, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens})
	}
	return out, response.NextCursor, nil
}

func (a *NewAPIAdapter) ReadSnapshot(ctx context.Context) (CostSnapshot, error) {
	var total domain.MicroUSD
	cursor := ""
	seenCursors := make(map[string]struct{})
	for {
		rows, nextCursor, err := a.ListTransactions(ctx, CostQuery{Cursor: cursor, Limit: 1000})
		if err != nil {
			return CostSnapshot{}, err
		}
		for _, row := range rows {
			if (row.Cost > 0 && total > domain.MicroUSD(math.MaxInt64)-row.Cost) ||
				(row.Cost < 0 && total < domain.MicroUSD(math.MinInt64)-row.Cost) {
				return CostSnapshot{}, fmt.Errorf("New API cumulative cost overflows")
			}
			total += row.Cost
		}
		if nextCursor == "" {
			break
		}
		if nextCursor == cursor {
			return CostSnapshot{}, fmt.Errorf("New API billing cursor did not advance")
		}
		if _, repeated := seenCursors[nextCursor]; repeated {
			return CostSnapshot{}, fmt.Errorf("New API billing cursor repeated")
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	return CostSnapshot{ActualCost: total, ObservedAt: time.Now().UTC()}, nil
}

func newAPIQuotaCost(quota, quotaPerUnit int64) (domain.MicroUSD, error) {
	if quota < 0 || quotaPerUnit <= 0 || quota > (math.MaxInt64-quotaPerUnit/2)/1_000_000 {
		return 0, fmt.Errorf("New API quota is invalid")
	}
	return domain.MicroUSD((quota*1_000_000 + quotaPerUnit/2) / quotaPerUnit), nil
}

var _ json.Unmarshaler = (*newAPITime)(nil)
