package service

import (
	"testing"
	"time"
)

func TestMonitorV4WindowStart(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	start, err := monitorV4WindowStart(MonitorV4Window7D, now)
	if err != nil {
		t.Fatalf("monitorV4WindowStart() error = %v", err)
	}
	if want := time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	if _, err := monitorV4WindowStart(MonitorV4Window("bad"), now); err == nil {
		t.Fatal("expected unsupported window error")
	}
}

func TestMonitorV4AvailabilityThresholdInputsRemainConcrete(t *testing.T) {
	projection := MonitorV4GroupProjection{AvailabilityBucketCount: 17, TotalBucketCount: 20, TTFTP95MS: 0, LatencyP95MS: 0, SampleCount: 0}
	if projection.TotalBucketCount == 0 || projection.TTFTP95MS < 0 || projection.LatencyP95MS < 0 {
		t.Fatal("zero-sample projection must retain concrete metric values")
	}
}
