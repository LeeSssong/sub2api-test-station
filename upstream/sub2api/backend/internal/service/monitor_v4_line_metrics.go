package service

import (
	"context"
	"fmt"
	"math"
	"time"
)

// MonitorV4LineMetric keeps selection metrics separate from the monitor's
// legacy experience/probe projection. SLA counts follow the admin SLA source.
type MonitorV4LineMetric struct {
	ID              int64    `json:"id"`
	SLARequestCount int      `json:"sla_request_count"`
	SLASuccessCount int      `json:"sla_success_count"`
	TTFTP50MS       *float64 `json:"ttft_p50_ms"`
	TTFTSampleCount int      `json:"ttft_sample_count"`
}

type MonitorV4LineSelection struct {
	ContractVersion string                `json:"contract_version"`
	Window          MonitorV4Window       `json:"window"`
	WindowStart     time.Time             `json:"window_start"`
	WindowEnd       time.Time             `json:"window_end"`
	GeneratedAt     time.Time             `json:"generated_at"`
	Groups          []MonitorV4LineMetric `json:"groups"`
}

type MonitorV4LineMetricsReader interface {
	ReadMonitorV4LineMetrics(context.Context, []int64, time.Time, time.Time) ([]MonitorV4LineMetric, error)
}

func ValidateMonitorV4LineMetric(row MonitorV4LineMetric) error {
	if row.ID <= 0 || row.SLARequestCount < 0 || row.SLASuccessCount < 0 || row.SLASuccessCount > row.SLARequestCount || row.TTFTSampleCount < 0 || row.TTFTSampleCount > row.SLASuccessCount {
		return fmt.Errorf("invalid line metric counts")
	}
	if (row.TTFTP50MS == nil) != (row.TTFTSampleCount == 0) {
		return fmt.Errorf("inconsistent line metric samples")
	}
	if row.TTFTP50MS != nil && (*row.TTFTP50MS < 0 || math.IsNaN(*row.TTFTP50MS) || math.IsInf(*row.TTFTP50MS, 0)) {
		return fmt.Errorf("invalid line metric P50")
	}
	return nil
}

func (s *AccountMonitorService) ReadMonitorV4LineMetrics(ctx context.Context, ids []int64, start, end time.Time) ([]MonitorV4LineMetric, error) {
	if s == nil {
		return nil, fmt.Errorf("line metrics repository unavailable")
	}
	reader, ok := s.repo.(MonitorV4LineMetricsReader)
	if !ok {
		return nil, fmt.Errorf("line metrics repository unavailable")
	}
	return reader.ReadMonitorV4LineMetrics(ctx, ids, start, end)
}

func (s *MonitorV4Service) LineSelection(ctx context.Context, userID int64, now time.Time) (*MonitorV4LineSelection, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Reuse the existing user visibility checks and persisted statistical bounds.
	snapshot, err := s.Snapshot(ctx, userID, MonitorV4Window24H, now)
	if err != nil {
		return nil, err
	}
	if snapshot.WindowStart.IsZero() || snapshot.WindowEnd.Sub(snapshot.WindowStart) != 24*time.Hour {
		return nil, fmt.Errorf("invalid line selection snapshot bounds")
	}
	reader, ok := s.native.(MonitorV4LineMetricsReader)
	if !ok {
		return nil, fmt.Errorf("line metrics reader unavailable")
	}
	ids := make([]int64, 0, len(snapshot.Groups))
	visible := make(map[int64]bool, len(snapshot.Groups))
	for _, g := range snapshot.Groups {
		if g.ID <= 0 || visible[g.ID] {
			return nil, fmt.Errorf("invalid line selection group")
		}
		ids = append(ids, g.ID)
		visible[g.ID] = true
	}
	rows, err := reader.ReadMonitorV4LineMetrics(ctx, ids, snapshot.WindowStart, snapshot.WindowEnd)
	if err != nil {
		return nil, fmt.Errorf("read line metrics: %w", err)
	}
	byID := make(map[int64]MonitorV4LineMetric, len(rows))
	for _, row := range rows {
		if _, duplicate := byID[row.ID]; duplicate || !visible[row.ID] {
			return nil, fmt.Errorf("invalid line metrics group")
		}
		if err := ValidateMonitorV4LineMetric(row); err != nil {
			return nil, err
		}
		byID[row.ID] = row
	}
	if len(byID) != len(ids) {
		return nil, fmt.Errorf("incomplete line metrics groups")
	}
	groups := make([]MonitorV4LineMetric, 0, len(ids))
	for _, id := range ids {
		groups = append(groups, byID[id])
	}
	return &MonitorV4LineSelection{ContractVersion: "1", Window: MonitorV4Window24H, WindowStart: snapshot.WindowStart, WindowEnd: snapshot.WindowEnd, GeneratedAt: snapshot.GeneratedAt, Groups: groups}, nil
}
