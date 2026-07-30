package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryPreservesLongTrendBuckets(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(16)
	cases := []struct {
		name          string
		bucketSeconds int
		duration      time.Duration
		wantBucket    string
		wantPoints    int
	}{
		{
			name:          "seven_days_uses_six_hour_buckets",
			bucketSeconds: 6 * 60 * 60,
			duration:      7 * 24 * time.Hour,
			wantBucket:    "6h",
			wantPoints:    28,
		},
		{
			name:          "thirty_days_uses_twenty_four_hour_buckets",
			bucketSeconds: 24 * 60 * 60,
			duration:      30 * 24 * time.Hour,
			wantBucket:    "24h",
			wantPoints:    30,
		},
	}

	for _, tc := range cases {
		t.Run("throughput/"+tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &opsRepository{db: db}
			filter := &service.OpsDashboardFilter{
				StartTime: start,
				EndTime:   start.Add(tc.duration),
				Platform:  service.PlatformOpenAI,
				GroupID:   &groupID,
			}
			bucketExpression := fmt.Sprintf(
				"to_timestamp(floor(extract(epoch from ul.created_at) / %d) * %d)",
				tc.bucketSeconds,
				tc.bucketSeconds,
			)
			mock.ExpectQuery(regexp.QuoteMeta(bucketExpression)).
				WillReturnRows(sqlmock.NewRows([]string{
					"bucket",
					"request_count",
					"token_consumed",
					"switch_count",
				}))

			got, err := repo.GetThroughputTrend(context.Background(), filter, tc.bucketSeconds)

			require.NoError(t, err)
			require.Equal(t, tc.wantBucket, got.Bucket)
			require.Len(t, got.Points, tc.wantPoints)
			require.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run("errors/"+tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &opsRepository{db: db}
			filter := &service.OpsDashboardFilter{
				StartTime: start,
				EndTime:   start.Add(tc.duration),
				Platform:  service.PlatformOpenAI,
				GroupID:   &groupID,
			}
			bucketExpression := fmt.Sprintf(
				"to_timestamp(floor(extract(epoch from created_at) / %d) * %d)",
				tc.bucketSeconds,
				tc.bucketSeconds,
			)
			mock.ExpectQuery(regexp.QuoteMeta(bucketExpression)).
				WillReturnRows(sqlmock.NewRows([]string{
					"bucket",
					"error_total",
					"business_limited",
					"error_sla",
					"upstream_excl",
					"upstream_429",
					"upstream_529",
				}))

			got, err := repo.GetErrorTrend(context.Background(), filter, tc.bucketSeconds)

			require.NoError(t, err)
			require.Equal(t, tc.wantBucket, got.Bucket)
			require.Len(t, got.Points, tc.wantPoints)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
