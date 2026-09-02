package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type monitorV4Snapshotter interface {
	Snapshot(context.Context, int64, service.MonitorV4Window, time.Time) (*service.MonitorV4Snapshot, error)
}

type MonitorV4Handler struct{ service monitorV4Snapshotter }

func NewMonitorV4Handler(svc monitorV4Snapshotter) *MonitorV4Handler {
	return &MonitorV4Handler{service: svc}
}

type monitorV4GroupResponse struct {
	ID                        int64    `json:"id"`
	Name                      string   `json:"name"`
	Platform                  string   `json:"platform"`
	RateMultiplier            float64  `json:"rate_multiplier"`
	SuccessRate               *float64 `json:"success_rate"`
	RequestCount              int      `json:"request_count"`
	SuccessCount              int      `json:"success_count"`
	RealRequestCount          int      `json:"real_request_count"`
	RealSuccessCount          int      `json:"real_success_count"`
	ProbeFallbackBucketCount  int      `json:"probe_fallback_bucket_count"`
	ProbeFallbackRequestCount int      `json:"probe_fallback_request_count"`
	TTFTP95MS                 *float64 `json:"ttft_p95_ms"`
	TTFTSampleCount           int      `json:"ttft_sample_count"`
	LatencyP95MS              *float64 `json:"latency_p95_ms"`
	LatencySampleCount        int      `json:"latency_sample_count"`
	// Deprecated V2 fields are retained for rolling SPA compatibility.
	CacheReadTokensP95         *float64 `json:"cache_read_tokens_p95"`
	CacheReadTokensSampleCount int      `json:"cache_read_tokens_sample_count"`
	CacheHitRate               *float64 `json:"cache_hit_rate"`
	SourceUpdatedAt            *string  `json:"source_updated_at"`
	CurrentOperational         bool     `json:"current_operational"`
}

type monitorV4SnapshotResponse struct {
	ContractVersion        string                   `json:"contract_version"`
	Window                 service.MonitorV4Window  `json:"window"`
	RefreshIntervalSeconds int                      `json:"refresh_interval_seconds"`
	GeneratedAt            string                   `json:"generated_at"`
	Groups                 []monitorV4GroupResponse `json:"groups"`
}

func parseMonitorV4Window(raw string) (service.MonitorV4Window, bool) {
	switch service.MonitorV4Window(raw) {
	case "", service.MonitorV4Window7D:
		return service.MonitorV4Window7D, true
	case service.MonitorV4Window1H:
		return service.MonitorV4Window1H, true
	case service.MonitorV4Window24H:
		return service.MonitorV4Window24H, true
	default:
		return "", false
	}
}

func (h *MonitorV4Handler) Snapshot(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	window, ok := parseMonitorV4Window(c.Query("window"))
	if !ok {
		response.BadRequest(c, "unsupported monitor window")
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, "monitor v4 unavailable")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	snapshot, err := h.service.Snapshot(c.Request.Context(), subject.UserID, window, time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groups := make([]monitorV4GroupResponse, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		var updatedAt *string
		if group.SourceUpdatedAt != nil {
			value := group.SourceUpdatedAt.UTC().Format(time.RFC3339)
			updatedAt = &value
		}
		groups = append(groups, monitorV4GroupResponse{
			ID: group.ID, Name: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
			SuccessRate: group.SuccessRate, RequestCount: group.RequestCount, SuccessCount: group.SuccessCount,
			RealRequestCount: group.RealRequestCount, RealSuccessCount: group.RealSuccessCount,
			ProbeFallbackBucketCount: group.ProbeFallbackBucketCount, ProbeFallbackRequestCount: group.ProbeFallbackRequestCount,
			TTFTP95MS: group.TTFTP95MS, TTFTSampleCount: group.TTFTSampleCount,
			LatencyP95MS: group.LatencyP95MS, LatencySampleCount: group.LatencySampleCount,
			CacheReadTokensP95: nil, CacheReadTokensSampleCount: 0,
			CacheHitRate:    group.CacheHitRate,
			SourceUpdatedAt: updatedAt, CurrentOperational: group.CurrentOperational,
		})
	}
	response.Success(c, monitorV4SnapshotResponse{ContractVersion: snapshot.ContractVersion, Window: snapshot.Window, RefreshIntervalSeconds: snapshot.RefreshIntervalSeconds, GeneratedAt: snapshot.GeneratedAt.UTC().Format(time.RFC3339), Groups: groups})
}
