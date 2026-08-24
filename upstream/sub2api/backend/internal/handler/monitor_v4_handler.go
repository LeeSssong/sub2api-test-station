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
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	Platform            string  `json:"platform"`
	RateMultiplier      float64 `json:"rate_multiplier"`
	Availability        float64 `json:"availability"`
	AvailabilityBuckets int     `json:"availability_bucket_count"`
	TotalBuckets        int     `json:"total_bucket_count"`
	TTFTP95MS           float64 `json:"ttft_p95_ms"`
	LatencyP95MS        float64 `json:"latency_p95_ms"`
	SampleCount         int     `json:"sample_count"`
	SourceUpdatedAt     string  `json:"source_updated_at,omitempty"`
	CurrentOperational  bool    `json:"current_operational"`
	MetricFallback      bool    `json:"is_fallback_metric"`
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
	case service.MonitorV4Window24H:
		return service.MonitorV4Window24H, true
	case service.MonitorV4Window30D:
		return service.MonitorV4Window30D, true
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
		updatedAt := ""
		if group.SourceUpdatedAt != nil {
			updatedAt = group.SourceUpdatedAt.UTC().Format(time.RFC3339)
		}
		groups = append(groups, monitorV4GroupResponse{
			ID: group.ID, Name: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
			Availability: group.Availability, AvailabilityBuckets: group.AvailabilityBuckets, TotalBuckets: group.TotalBuckets,
			TTFTP95MS: group.TTFTP95MS, LatencyP95MS: group.LatencyP95MS, SampleCount: group.SampleCount,
			SourceUpdatedAt: updatedAt, CurrentOperational: group.CurrentOperational, MetricFallback: group.MetricFallback,
		})
	}
	response.Success(c, monitorV4SnapshotResponse{ContractVersion: snapshot.ContractVersion, Window: snapshot.Window, RefreshIntervalSeconds: snapshot.RefreshIntervalSeconds, GeneratedAt: snapshot.GeneratedAt.UTC().Format(time.RFC3339), Groups: groups})
}
