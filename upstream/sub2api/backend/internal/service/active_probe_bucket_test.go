package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCurrentActiveProbeBucketUsesShanghaiFiveMinuteBoundary(t *testing.T) {
	start, end := currentActiveProbeBucket(time.Date(2026, 8, 27, 2, 7, 59, 0, time.UTC))
	require.Equal(t, "2026-08-27T10:05:00+08:00", start.Format(time.RFC3339))
	require.Equal(t, "2026-08-27T10:10:00+08:00", end.Format(time.RFC3339))
}
