package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	subUpstreamCostWindow       = 10 * time.Minute
	subUpstreamCostPageLimit    = 1000
	subUpstreamCostMaxPages     = 10
	subUpstreamCostMaxBodyBytes = 2 << 20
	subUpstreamCostHTTPTimeout  = 10 * time.Second
)

// SubUpstreamCostDetail is the administrator-facing comparison between the
// site's recorded charge and the exact upstream Sub actual_cost.
type SubUpstreamCostDetail struct {
	UsageID            int64    `json:"usage_id"`
	LocalRequestID     string   `json:"local_request_id"`
	UpstreamRequestID  *string  `json:"upstream_request_id"`
	SiteActualCost     float64  `json:"site_actual_cost"`
	UpstreamActualCost *float64 `json:"upstream_actual_cost"`
	Profit             *float64 `json:"profit"`
	Status             string   `json:"status"`
	Reason             string   `json:"reason,omitempty"`
}

// SubUpstreamCostService retrieves one exact upstream usage row for an
// administrator request and computes site actual_cost minus upstream
// actual_cost. It intentionally has no billing credential or estimate path.
type SubUpstreamCostService struct {
	usageService *UsageService
	httpClient   *http.Client
}

func NewSubUpstreamCostService(usageService *UsageService) *SubUpstreamCostService {
	return &SubUpstreamCostService{
		usageService: usageService,
		httpClient:   &http.Client{Timeout: subUpstreamCostHTTPTimeout},
	}
}

// GetByUsageID returns an unavailable detail (rather than an estimated value)
// when credentials, the upstream endpoint, or an exact usage match is absent.
// A missing local usage row remains an error so callers can return HTTP 404.
func (s *SubUpstreamCostService) GetByUsageID(ctx context.Context, usageID int64) (*SubUpstreamCostDetail, error) {
	if s == nil || s.usageService == nil {
		return nil, errors.New("usage service unavailable")
	}
	usage, err := s.usageService.GetByID(ctx, usageID)
	if err != nil {
		return nil, err
	}
	detail := &SubUpstreamCostDetail{
		UsageID:           usage.ID,
		LocalRequestID:    usage.RequestID,
		UpstreamRequestID: cloneStringPtr(usage.UpstreamRequestID),
		SiteActualCost:    usage.ActualCost,
		Status:            "unavailable",
	}

	baseURL, apiKey, ok := subCredentials(usage.Account)
	if !ok {
		detail.Reason = "upstream credentials unavailable"
		return detail, nil
	}
	endpoint, err := subUsageRecordsURL(baseURL)
	if err != nil {
		detail.Reason = "upstream endpoint unavailable"
		return detail, nil
	}
	matched, reason := s.findUpstreamRecord(ctx, endpoint, apiKey, usage)
	if reason != "" {
		detail.Reason = reason
		return detail, nil
	}
	if matched == nil {
		detail.Reason = "upstream usage record not found"
		return detail, nil
	}
	upstreamCost := matched.ActualCost
	profit := usage.ActualCost - upstreamCost
	detail.UpstreamActualCost = &upstreamCost
	detail.Profit = &profit
	detail.Status = "confirmed"
	return detail, nil
}

type subUpstreamUsageRecord struct {
	RequestID         string  `json:"request_id"`
	UpstreamRequestID string  `json:"upstream_request_id"`
	ActualCost        float64 `json:"actual_cost"`
}

type subUpstreamUsageRecordsResponse struct {
	Data       []subUpstreamUsageRecord `json:"data"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
}

func (s *SubUpstreamCostService) findUpstreamRecord(ctx context.Context, endpoint, apiKey string, usage *UsageLog) (*subUpstreamUsageRecord, string) {
	start := usage.CreatedAt.Add(-subUpstreamCostWindow)
	end := usage.CreatedAt.Add(subUpstreamCostWindow)
	cursor := ""
	var best *subUpstreamUsageRecord
	bestRank := 4
	for page := 0; page < subUpstreamCostMaxPages; page++ {
		queryURL, err := url.Parse(endpoint)
		if err != nil {
			return nil, "upstream endpoint unavailable"
		}
		q := queryURL.Query()
		q.Set("start_time", start.Format(time.RFC3339Nano))
		q.Set("end_time", end.Format(time.RFC3339Nano))
		q.Set("limit", strconv.Itoa(subUpstreamCostPageLimit))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		queryURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL.String(), nil)
		if err != nil {
			return nil, "upstream request unavailable"
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, "upstream request unavailable"
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, subUpstreamCostMaxBodyBytes+1))
		_ = resp.Body.Close()
		if readErr != nil || len(body) > subUpstreamCostMaxBodyBytes {
			return nil, "upstream response unavailable"
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, "upstream response unavailable"
		}
		var payload subUpstreamUsageRecordsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, "upstream response unavailable"
		}
		for i := range payload.Data {
			rank := subUsageRecordMatchRank(&payload.Data[i], usage)
			if rank < bestRank {
				best = &payload.Data[i]
				bestRank = rank
				if bestRank == 1 {
					return best, ""
				}
			}
		}
		if !payload.HasMore || strings.TrimSpace(payload.NextCursor) == "" {
			return best, ""
		}
		cursor = strings.TrimSpace(payload.NextCursor)
	}
	if best != nil {
		return best, ""
	}
	return nil, "upstream usage pagination unavailable"
}

func subUsageRecordMatches(record *subUpstreamUsageRecord, usage *UsageLog) bool {
	return subUsageRecordMatchRank(record, usage) < 4
}

func subUsageRecordMatchRank(record *subUpstreamUsageRecord, usage *UsageLog) int {
	if record == nil || usage == nil {
		return 4
	}
	if id := strings.TrimSpace(usage.UpstreamRequestIDOrEmpty()); id != "" {
		if id == strings.TrimSpace(record.UpstreamRequestID) {
			return 1
		}
		if id == strings.TrimSpace(record.RequestID) {
			return 2
		}
	}
	if strings.TrimSpace(usage.RequestID) != "" && strings.TrimSpace(usage.RequestID) == strings.TrimSpace(record.RequestID) {
		return 3
	}
	return 4
}

func (u *UsageLog) UpstreamRequestIDOrEmpty() string {
	if u == nil || u.UpstreamRequestID == nil {
		return ""
	}
	return *u.UpstreamRequestID
}

func subCredentials(account *Account) (baseURL, apiKey string, ok bool) {
	if account == nil || account.Credentials == nil {
		return "", "", false
	}
	baseURL, _ = account.Credentials["base_url"].(string)
	apiKey, _ = account.Credentials["api_key"].(string)
	return strings.TrimSpace(baseURL), strings.TrimSpace(apiKey), strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != ""
}

func subUsageRecordsURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid upstream base url")
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path += "/usage/records"
	} else {
		path += "/v1/usage/records"
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
