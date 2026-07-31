package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type monitorV2Snapshotter interface {
	Snapshot(
		context.Context,
		service.MonitorV2Window,
		time.Time,
		...service.MonitorV2Scope,
	) (*service.MonitorV2Snapshot, error)
}

type MonitorV2Handler struct {
	service monitorV2Snapshotter
}

func NewMonitorV2Handler(service monitorV2Snapshotter) *MonitorV2Handler {
	return &MonitorV2Handler{service: service}
}

type monitorV2MetricResponse struct {
	State       string   `json:"state"`
	Value       *float64 `json:"value"`
	SampleCount int64    `json:"sample_count"`
}

type monitorV2AvailabilityResponse struct {
	monitorV2MetricResponse
	SuccessCount  int64 `json:"success_count"`
	EligibleCount int64 `json:"eligible_count"`
}

type monitorV2TimelinePointResponse struct {
	BucketStart   string   `json:"bucket_start"`
	State         string   `json:"state"`
	Value         *float64 `json:"value"`
	SuccessCount  int64    `json:"success_count"`
	EligibleCount int64    `json:"eligible_count"`
}

type monitorV2GroupResponse struct {
	ID                 int64                            `json:"id"`
	Name               string                           `json:"name"`
	Platform           string                           `json:"platform"`
	RateMultiplier     float64                          `json:"rate_multiplier"`
	PeakRateEnabled    bool                             `json:"peak_rate_enabled"`
	PeakStart          string                           `json:"peak_start,omitempty"`
	PeakEnd            string                           `json:"peak_end,omitempty"`
	PeakRateMultiplier float64                          `json:"peak_rate_multiplier,omitempty"`
	Status             string                           `json:"status"`
	Availability       monitorV2AvailabilityResponse    `json:"availability"`
	TTFT               monitorV2MetricResponse          `json:"ttft"`
	TTFTP95            monitorV2MetricResponse          `json:"ttft_p95"`
	TPS                monitorV2MetricResponse          `json:"tps"`
	Latency            monitorV2MetricResponse          `json:"latency"`
	LatencyP95         monitorV2MetricResponse          `json:"latency_p95"`
	CacheHit           monitorV2MetricResponse          `json:"cache_hit"`
	Timeline           []monitorV2TimelinePointResponse `json:"timeline"`
}

type monitorV2SnapshotResponse struct {
	ContractVersion        string                   `json:"contract_version"`
	Window                 service.MonitorV2Window  `json:"window"`
	RefreshIntervalSeconds int                      `json:"refresh_interval_seconds"`
	GeneratedAt            string                   `json:"generated_at"`
	Groups                 []monitorV2GroupResponse `json:"groups"`
}

func (h *MonitorV2Handler) Snapshot(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	window, ok := parseMonitorV2Window(c.Query("window"))
	if !ok {
		response.BadRequest(c, "unsupported monitor window")
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, "monitor v2 unavailable")
		return
	}
	scope := service.MonitorV2ScopePublic
	if role, ok := middleware.GetUserRoleFromContext(c); ok && role == service.RoleAdmin {
		scope = service.MonitorV2ScopeAdmin
	}
	snapshot, err := h.service.Snapshot(c.Request.Context(), window, time.Now().UTC(), scope)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, monitorV2SnapshotFromService(snapshot))
}

func parseMonitorV2Window(raw string) (service.MonitorV2Window, bool) {
	switch strings.TrimSpace(raw) {
	case "", string(service.MonitorV2Window7D):
		return service.MonitorV2Window7D, true
	case string(service.MonitorV2Window24H):
		return service.MonitorV2Window24H, true
	case string(service.MonitorV2Window30D):
		return service.MonitorV2Window30D, true
	default:
		return "", false
	}
}

func monitorV2MetricFromService(metric service.MonitorV2Metric) monitorV2MetricResponse {
	return monitorV2MetricResponse{
		State:       metric.State,
		Value:       metric.Value,
		SampleCount: metric.SampleCount,
	}
}

func monitorV2SnapshotFromService(snapshot *service.MonitorV2Snapshot) monitorV2SnapshotResponse {
	if snapshot == nil {
		return monitorV2SnapshotResponse{
			ContractVersion:        service.MonitorV2ContractVersion,
			Window:                 service.MonitorV2Window7D,
			RefreshIntervalSeconds: service.MonitorPageRefreshIntervalSecondsDefault,
			Groups:                 []monitorV2GroupResponse{},
		}
	}
	groups := make([]monitorV2GroupResponse, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		timeline := make([]monitorV2TimelinePointResponse, 0, len(group.Timeline))
		for _, point := range group.Timeline {
			timeline = append(timeline, monitorV2TimelinePointResponse{
				BucketStart:   point.BucketStart.UTC().Format(time.RFC3339),
				State:         point.State,
				Value:         point.Value,
				SuccessCount:  point.SuccessCount,
				EligibleCount: point.EligibleCount,
			})
		}
		groups = append(groups, monitorV2GroupResponse{
			ID:                 group.ID,
			Name:               group.Name,
			Platform:           group.Platform,
			RateMultiplier:     group.RateMultiplier,
			PeakRateEnabled:    group.PeakRateEnabled,
			PeakStart:          group.PeakStart,
			PeakEnd:            group.PeakEnd,
			PeakRateMultiplier: group.PeakRateMultiplier,
			Status:             group.Status,
			Availability: monitorV2AvailabilityResponse{
				monitorV2MetricResponse: monitorV2MetricFromService(group.Availability.MonitorV2Metric),
				SuccessCount:            group.Availability.SuccessCount,
				EligibleCount:           group.Availability.EligibleCount,
			},
			TTFT:       monitorV2MetricFromService(group.TTFT),
			TTFTP95:    monitorV2MetricFromService(group.TTFTP95),
			TPS:        monitorV2MetricFromService(group.TPS),
			Latency:    monitorV2MetricFromService(group.Latency),
			LatencyP95: monitorV2MetricFromService(group.LatencyP95),
			CacheHit:   monitorV2MetricFromService(group.CacheHit),
			Timeline:   timeline,
		})
	}
	return monitorV2SnapshotResponse{
		ContractVersion:        snapshot.ContractVersion,
		Window:                 snapshot.Window,
		RefreshIntervalSeconds: snapshot.RefreshIntervalSeconds,
		GeneratedAt:            snapshot.GeneratedAt.UTC().Format(time.RFC3339),
		Groups:                 groups,
	}
}
