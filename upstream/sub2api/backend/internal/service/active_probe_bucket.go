package service

import (
	"context"
	"time"
)

var activeProbeLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// ActiveProbeUsageWindowReader reports whether real user traffic exists in a
// bounded half-open time window for an account or group.
type ActiveProbeUsageWindowReader interface {
	HasAccountUsageInWindow(ctx context.Context, accountID int64, from, until time.Time) (bool, error)
	HasGroupUsageInWindow(ctx context.Context, groupID int64, from, until time.Time) (bool, error)
}

func currentActiveProbeBucket(now time.Time) (time.Time, time.Time) {
	local := now.In(activeProbeLocation)
	start := local.Truncate(5 * time.Minute)
	return start, start.Add(5 * time.Minute)
}

func activeProbeBucketKey(now time.Time) string {
	start, _ := currentActiveProbeBucket(now)
	return start.Format(time.RFC3339)
}
