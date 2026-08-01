package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

type Sub2APIAdapter struct{ *httpAdapter }

func NewSub2APIAdapter(baseURL, apiKey string, client *http.Client) (*Sub2APIAdapter, error) {
	a, err := newHTTPAdapter(baseURL, apiKey, client)
	if err != nil {
		return nil, err
	}
	return &Sub2APIAdapter{httpAdapter: a}, nil
}

type sub2APIUsage struct {
	ID                json.Number `json:"id"`
	RequestID         string      `json:"request_id"`
	UpstreamRequestID string      `json:"upstream_request_id"`
	ActualCost        json.Number `json:"actual_cost"`
	Model             string      `json:"model"`
	InputTokens       int64       `json:"input_tokens"`
	OutputTokens      int64       `json:"output_tokens"`
	CreatedAt         time.Time   `json:"created_at"`
}

func (a *Sub2APIAdapter) ListTransactions(ctx context.Context, q CostQuery) ([]CostTransaction, string, error) {
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
		Data       []sub2APIUsage `json:"data"`
		Items      []sub2APIUsage `json:"items"`
		NextCursor string         `json:"next_cursor"`
	}
	if err := a.doJSON(ctx, http.MethodGet, "/v1/usage/records", values, &response); err != nil {
		return nil, "", err
	}
	rows := response.Data
	if len(rows) == 0 && response.Items != nil {
		rows = response.Items
	}
	out := make([]CostTransaction, 0, len(rows))
	for _, row := range rows {
		cost, err := domain.ParseMicroUSD(row.ActualCost.String())
		if err != nil {
			return nil, "", fmt.Errorf("invalid Sub2API actual_cost for usage %s", row.ID.String())
		}
		out = append(out, CostTransaction{SourceID: row.ID.String(), RequestID: row.RequestID, UpstreamRequestID: row.UpstreamRequestID, Type: "charge", Cost: cost, OccurredAt: row.CreatedAt, Model: row.Model, PromptTokens: row.InputTokens, CompletionTokens: row.OutputTokens})
	}
	return out, response.NextCursor, nil
}

func (a *Sub2APIAdapter) ReadSnapshot(ctx context.Context) (CostSnapshot, error) {
	var response struct {
		Usage struct {
			Total struct {
				ActualCost json.Number `json:"actual_cost"`
			} `json:"total"`
			ActualCost json.Number `json:"actual_cost"`
		} `json:"usage"`
		ActualCost json.Number `json:"actual_cost"`
	}
	if err := a.doJSON(ctx, http.MethodGet, "/v1/usage", nil, &response); err != nil {
		return CostSnapshot{}, err
	}
	raw := response.Usage.Total.ActualCost
	if raw.String() == "" {
		raw = response.Usage.ActualCost
	}
	if raw.String() == "" {
		raw = response.ActualCost
	}
	cost, err := domain.ParseMicroUSD(raw.String())
	if err != nil {
		return CostSnapshot{}, fmt.Errorf("invalid Sub2API cumulative actual_cost")
	}
	return CostSnapshot{ActualCost: cost, ObservedAt: time.Now().UTC()}, nil
}
